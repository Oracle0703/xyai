package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func (s *OpenAIGatewayService) forwardResponsesViaRawChatCompletions(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	reqBody map[string]any,
	originalModel string,
	billingModel string,
	upstreamModel string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	var responsesReq apicompat.ResponsesRequest
	if err := json.Unmarshal(body, &responsesReq); err != nil {
		return nil, fmt.Errorf("parse responses request: %w", err)
	}

	chatReq, err := apicompat.ResponsesToChatCompletionsRequestWithOptions(&responsesReq, openAICompatibleResponsesToChatOptions(account, upstreamModel))
	if err != nil {
		return nil, fmt.Errorf("convert responses to chat completions: %w", err)
	}
	chatReq.Model = upstreamModel

	chatBody, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("marshal chat completions request: %w", err)
	}

	updatedBody, policyErr := s.applyOpenAIFastPolicyToBody(ctx, account, upstreamModel, chatBody)
	if policyErr != nil {
		var blocked *OpenAIFastBlockedError
		if errors.As(policyErr, &blocked) {
			writeOpenAIFastPolicyBlockedResponse(c, blocked)
		}
		return nil, policyErr
	}
	chatBody = updatedBody

	setOpsUpstreamRequestBody(c, chatBody)

	apiKey := account.GetOpenAIApiKey()
	if apiKey == "" {
		return nil, fmt.Errorf("account %d missing api_key", account.ID)
	}
	baseURL := account.GetOpenAIBaseURL()
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	validatedURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base_url: %w", err)
	}

	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	upstreamReq, err := http.NewRequestWithContext(upstreamCtx, http.MethodPost, buildOpenAIChatCompletionsURL(validatedURL), bytes.NewReader(chatBody))
	releaseUpstreamCtx()
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Authorization", "Bearer "+apiKey)
	if chatReq.Stream {
		upstreamReq.Header.Set("Accept", "text/event-stream")
	} else {
		upstreamReq.Header.Set("Accept", "application/json")
	}
	for key, values := range c.Request.Header {
		lowerKey := strings.ToLower(key)
		if openaiCCRawAllowedHeaders[lowerKey] {
			for _, v := range values {
				upstreamReq.Header.Add(key, v)
			}
		}
	}
	if customUA := account.GetOpenAIUserAgent(); customUA != "" {
		upstreamReq.Header.Set("user-agent", customUA)
	}

	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	upstreamStart := time.Now()
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: 0,
			Kind:               "request_error",
			Message:            safeErr,
		})
		c.JSON(http.StatusBadGateway, gin.H{
			"error": gin.H{
				"type":    "upstream_error",
				"message": "Upstream request failed",
			},
		})
		return nil, fmt.Errorf("upstream request failed: %s", safeErr)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))

		upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
		upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
		if s.shouldFailoverOpenAIUpstreamResponse(resp.StatusCode, upstreamMsg, respBody) {
			upstreamDetail := ""
			if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
				maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
				if maxBytes <= 0 {
					maxBytes = 2048
				}
				upstreamDetail = truncateString(string(respBody), maxBytes)
			}
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				Kind:               "failover",
				Message:            upstreamMsg,
				Detail:             upstreamDetail,
			})
			if s.rateLimitService != nil {
				s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
			}
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				RetryableOnSameAccount: account.IsPoolMode() && (isPoolModeRetryableStatus(resp.StatusCode) || isOpenAITransientProcessingError(resp.StatusCode, upstreamMsg, respBody)),
			}
		}
		return s.handleCompatErrorResponse(resp, c, account, writeResponsesError)
	}

	reasoningEffort := extractOpenAIReasoningEffort(reqBody, originalModel)
	serviceTier := extractOpenAIServiceTier(reqBody)
	if chatReq.Stream {
		return s.streamResponsesViaRawChatCompletions(c, resp, originalModel, billingModel, upstreamModel, reasoningEffort, serviceTier, startTime)
	}
	return s.bufferResponsesViaRawChatCompletions(c, resp, originalModel, billingModel, upstreamModel, reasoningEffort, serviceTier, startTime)
}

func (s *OpenAIGatewayService) bufferResponsesViaRawChatCompletions(
	c *gin.Context,
	resp *http.Response,
	originalModel string,
	billingModel string,
	upstreamModel string,
	reasoningEffort *string,
	serviceTier *string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")

	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, fmt.Errorf("read upstream body: %w", err)
	}

	var chatResp apicompat.ChatCompletionsResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		writeResponsesError(c, http.StatusBadGateway, "api_error", "Failed to parse upstream response")
		return nil, fmt.Errorf("parse upstream chat completions response: %w", err)
	}
	if chatResp.Usage != nil {
		applyOpenAICompatibleChatUsageDetailsFromJSON(respBody, chatResp.Usage, "usage")
	}

	responsesResp := chatCompletionsToResponsesResponse(&chatResp, originalModel)

	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}
	c.JSON(http.StatusOK, responsesResp)

	usage := OpenAIUsage{}
	if chatResp.Usage != nil {
		usage.InputTokens = chatResp.Usage.PromptTokens
		usage.OutputTokens = chatResp.Usage.CompletionTokens
		applyOpenAICompatibleCacheUsageFromJSON(respBody, &usage, "usage")
	}

	return &OpenAIForwardResult{
		RequestID:       requestID,
		ResponseID:      responsesResp.ID,
		Usage:           usage,
		Model:           originalModel,
		BillingModel:    billingModel,
		UpstreamModel:   upstreamModel,
		ServiceTier:     serviceTier,
		ReasoningEffort: reasoningEffort,
		Stream:          false,
		Duration:        time.Since(startTime),
	}, nil
}

func (s *OpenAIGatewayService) streamResponsesViaRawChatCompletions(
	c *gin.Context,
	resp *http.Response,
	originalModel string,
	billingModel string,
	upstreamModel string,
	reasoningEffort *string,
	serviceTier *string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")

	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)

	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	state := newChatCompletionsToResponsesStreamState(originalModel)
	var usage OpenAIUsage
	var firstTokenMs *int
	clientDisconnected := false

	flushEvents := func(events []apicompat.ResponsesStreamEvent) {
		for _, evt := range events {
			sse, err := apicompat.ResponsesEventToSSE(evt)
			if err != nil {
				logger.L().Warn("openai responses compat stream: marshal event failed",
					zap.Error(err),
					zap.String("request_id", requestID),
				)
				continue
			}
			if clientDisconnected {
				continue
			}
			if _, err := fmt.Fprint(c.Writer, sse); err != nil {
				clientDisconnected = true
				logger.L().Info("openai responses compat stream: client disconnected, continuing to drain upstream",
					zap.String("request_id", requestID),
				)
			}
		}
		if len(events) > 0 && !clientDisconnected {
			c.Writer.Flush()
		}
	}

	for scanner.Scan() {
		line := scanner.Text()
		payload, ok := extractOpenAISSEDataLine(line)
		if !ok {
			if !clientDisconnected {
				if _, err := c.Writer.WriteString(line + "\n"); err == nil && line == "" {
					c.Writer.Flush()
				}
			}
			continue
		}

		trimmed := strings.TrimSpace(payload)
		if trimmed == "" {
			continue
		}
		if trimmed == "[DONE]" {
			finalEvents := state.Finalize()
			flushEvents(finalEvents)
			if !clientDisconnected {
				_, _ = fmt.Fprint(c.Writer, "data: [DONE]\n\n")
				c.Writer.Flush()
			}
			return &OpenAIForwardResult{
				RequestID:       requestID,
				ResponseID:      state.ResponseID,
				Usage:           usage,
				Model:           originalModel,
				BillingModel:    billingModel,
				UpstreamModel:   upstreamModel,
				ServiceTier:     serviceTier,
				ReasoningEffort: reasoningEffort,
				Stream:          true,
				Duration:        time.Since(startTime),
				FirstTokenMs:    firstTokenMs,
			}, nil
		}

		var chunk apicompat.ChatCompletionsChunk
		if err := json.Unmarshal([]byte(trimmed), &chunk); err != nil {
			logger.L().Warn("openai responses compat stream: parse chunk failed",
				zap.Error(err),
				zap.String("request_id", requestID),
			)
			continue
		}

		if chunk.ID != "" && state.ResponseID == "" {
			state.ResponseID = chunk.ID
		}
		if chunk.Model != "" && state.Model == "" {
			state.Model = chunk.Model
		}
		if firstTokenMs == nil && len(chunk.Choices) > 0 {
			hasContent := false
			for _, choice := range chunk.Choices {
				if choice.Delta.Content != nil && *choice.Delta.Content != "" {
					hasContent = true
					break
				}
				if len(choice.Delta.ToolCalls) > 0 {
					hasContent = true
					break
				}
			}
			if hasContent {
				ms := int(time.Since(startTime).Milliseconds())
				firstTokenMs = &ms
			}
		}

		if chunk.Usage != nil {
			applyOpenAICompatibleChatUsageDetailsFromJSON([]byte(payload), chunk.Usage, "usage")
			usage = OpenAIUsage{
				InputTokens:  chunk.Usage.PromptTokens,
				OutputTokens: chunk.Usage.CompletionTokens,
			}
			applyOpenAICompatibleCacheUsageFromJSON([]byte(payload), &usage, "usage")
		}

		events := state.ProcessChunk(&chunk, usage)
		flushEvents(events)
	}

	if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		logger.L().Warn("openai responses compat stream: read error",
			zap.Error(err),
			zap.String("request_id", requestID),
		)
	}

	finalEvents := state.Finalize()
	flushEvents(finalEvents)
	if !clientDisconnected {
		_, _ = fmt.Fprint(c.Writer, "data: [DONE]\n\n")
		c.Writer.Flush()
	}

	return &OpenAIForwardResult{
		RequestID:       requestID,
		ResponseID:      state.ResponseID,
		Usage:           usage,
		Model:           originalModel,
		BillingModel:    billingModel,
		UpstreamModel:   upstreamModel,
		ServiceTier:     serviceTier,
		ReasoningEffort: reasoningEffort,
		Stream:          true,
		Duration:        time.Since(startTime),
		FirstTokenMs:    firstTokenMs,
	}, nil
}

func openAICompatibleResponsesToChatOptions(account *Account, upstreamModel string) apicompat.ResponsesToChatCompletionsOptions {
	return apicompat.ResponsesToChatCompletionsOptions{
		DropTemperature:         shouldDropOpenAICompatibleResponsesTemperature(account, upstreamModel),
		DropMaxCompletionTokens: shouldDropOpenAICompatibleMaxCompletionTokens(account, upstreamModel),
	}
}

func shouldDropOpenAICompatibleResponsesTemperature(account *Account, upstreamModel string) bool {
	if account == nil || account.Platform != PlatformOpenAI || account.Type != AccountTypeAPIKey {
		return false
	}
	if account.Extra != nil {
		if v, ok := account.Extra["openai_compat_drop_temperature"].(bool); ok {
			return v
		}
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(upstreamModel)), "gpt-5.5")
}

func shouldDropOpenAICompatibleMaxCompletionTokens(account *Account, upstreamModel string) bool {
	if account == nil || account.Platform != PlatformOpenAI || account.Type != AccountTypeAPIKey {
		return false
	}
	if account.Extra != nil {
		if v, ok := account.Extra["openai_compat_drop_max_completion_tokens"].(bool); ok {
			return v
		}
	}
	return false
}

func chatCompletionsToResponsesResponse(chatResp *apicompat.ChatCompletionsResponse, model string) *apicompat.ResponsesResponse {
	if chatResp == nil {
		return &apicompat.ResponsesResponse{
			Object: "response",
			Model:  model,
			Status: "completed",
			Output: []apicompat.ResponsesOutput{},
		}
	}

	resp := &apicompat.ResponsesResponse{
		ID:     chatResp.ID,
		Object: "response",
		Model:  model,
		Status: "completed",
		Output: []apicompat.ResponsesOutput{},
	}

	if len(chatResp.Choices) > 0 {
		choice := chatResp.Choices[0]
		msg := choice.Message

		text := decodeJSONStringRaw(msg.Content)
		if text != "" {
			resp.Output = append(resp.Output, apicompat.ResponsesOutput{
				Type:   "message",
				Role:   "assistant",
				Status: "completed",
				Content: []apicompat.ResponsesContentPart{{
					Type: "output_text",
					Text: text,
				}},
			})
		}
		for _, toolCall := range msg.ToolCalls {
			resp.Output = append(resp.Output, apicompat.ResponsesOutput{
				Type:      "function_call",
				CallID:    toolCall.ID,
				Name:      toolCall.Function.Name,
				Arguments: normalizeCompatArguments(toolCall.Function.Arguments),
				Status:    "completed",
			})
		}
		if choice.FinishReason == "length" {
			resp.Status = "incomplete"
			resp.IncompleteDetails = &apicompat.ResponsesIncompleteDetails{
				Reason: "max_output_tokens",
			}
		}
	}

	if chatResp.Usage != nil {
		resp.Usage = &apicompat.ResponsesUsage{
			InputTokens:  chatResp.Usage.PromptTokens,
			OutputTokens: chatResp.Usage.CompletionTokens,
			TotalTokens:  chatResp.Usage.TotalTokens,
		}
		if chatResp.Usage.PromptTokensDetails != nil && chatResp.Usage.PromptTokensDetails.CachedTokens > 0 {
			resp.Usage.InputTokensDetails = &apicompat.ResponsesInputTokensDetails{
				CachedTokens: chatResp.Usage.PromptTokensDetails.CachedTokens,
			}
		}
	}

	return resp
}

func decodeJSONStringRaw(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var out string
	if err := json.Unmarshal(raw, &out); err != nil {
		return ""
	}
	return out
}

type chatCompletionsToResponsesStreamState struct {
	ResponseID      string
	Model           string
	SequenceNumber  int
	createdSent     bool
	completed       bool
	messageOpened   bool
	messageDone     bool
	messageItemID   string
	contentPartOpen bool
	messageText     strings.Builder
	functionIndexes map[int]int
	functionItemIDs map[int]string
}

func newChatCompletionsToResponsesStreamState(model string) *chatCompletionsToResponsesStreamState {
	return &chatCompletionsToResponsesStreamState{
		Model:           model,
		functionIndexes: make(map[int]int),
		functionItemIDs: make(map[int]string),
	}
}

func (s *chatCompletionsToResponsesStreamState) ProcessChunk(chunk *apicompat.ChatCompletionsChunk, usage OpenAIUsage) []apicompat.ResponsesStreamEvent {
	if chunk == nil {
		return nil
	}
	if chunk.ID != "" && s.ResponseID == "" {
		s.ResponseID = chunk.ID
	}
	if chunk.Model != "" && s.Model == "" {
		s.Model = chunk.Model
	}

	var events []apicompat.ResponsesStreamEvent
	if !s.createdSent {
		events = append(events, s.nextEvent("response.created", &apicompat.ResponsesStreamEvent{
			Response: &apicompat.ResponsesResponse{
				ID:     s.ResponseID,
				Object: "response",
				Model:  s.Model,
				Status: "in_progress",
				Output: []apicompat.ResponsesOutput{},
			},
		}))
		s.createdSent = true
	}

	for _, choice := range chunk.Choices {
		if len(choice.Delta.ToolCalls) > 0 {
			for _, toolCall := range choice.Delta.ToolCalls {
				idx := 0
				if toolCall.Index != nil {
					idx = *toolCall.Index
				}
				outputIndex, exists := s.functionIndexes[idx]
				if !exists {
					outputIndex = len(s.functionIndexes)
					if s.messageOpened {
						outputIndex++
					}
					s.functionIndexes[idx] = outputIndex
					itemID := generateCompatResponsesItemID("fc", outputIndex)
					s.functionItemIDs[idx] = itemID
					events = append(events, s.nextEvent("response.output_item.added", &apicompat.ResponsesStreamEvent{
						OutputIndex: outputIndex,
						Item: &apicompat.ResponsesOutput{
							Type:   "function_call",
							ID:     itemID,
							CallID: toolCall.ID,
							Name:   toolCall.Function.Name,
							Status: "in_progress",
						},
					}))
				}
				if args := toolCall.Function.Arguments; args != "" {
					events = append(events, s.nextEvent("response.function_call_arguments.delta", &apicompat.ResponsesStreamEvent{
						OutputIndex: outputIndex,
						ItemID:      s.functionItemIDs[idx],
						CallID:      toolCall.ID,
						Name:        toolCall.Function.Name,
						Delta:       args,
					}))
				}
			}
		}

		if choice.Delta.Content != nil && *choice.Delta.Content != "" {
			if !s.messageOpened {
				s.messageOpened = true
				s.messageItemID = generateCompatResponsesItemID("msg", 0)
				events = append(events, s.nextEvent("response.output_item.added", &apicompat.ResponsesStreamEvent{
					OutputIndex: 0,
					StreamItem: &apicompat.ResponsesStreamOutputItem{
						Type:    "message",
						ID:      s.messageItemID,
						Role:    "assistant",
						Content: []apicompat.ResponsesStreamContentPart{},
						Status:  "in_progress",
					},
				}))
			}
			if !s.contentPartOpen {
				s.contentPartOpen = true
				events = append(events, s.nextEvent("response.content_part.added", &apicompat.ResponsesStreamEvent{
					OutputIndex:  0,
					ContentIndex: 0,
					ItemID:       s.messageItemID,
					StreamPart: &apicompat.ResponsesStreamContentPart{
						Type: "output_text",
						Text: "",
					},
				}))
			}
			events = append(events, s.nextEvent("response.output_text.delta", &apicompat.ResponsesStreamEvent{
				OutputIndex:  0,
				ContentIndex: 0,
				Delta:        *choice.Delta.Content,
				ItemID:       s.messageItemID,
			}))
			s.messageText.WriteString(*choice.Delta.Content)
		}

		if choice.FinishReason != nil {
			events = append(events, s.closeOpenItems()...)
			events = append(events, s.finalResponseEvent(*choice.FinishReason, usage))
		}
	}

	return events
}

func (s *chatCompletionsToResponsesStreamState) Finalize() []apicompat.ResponsesStreamEvent {
	if !s.createdSent || s.completed {
		return nil
	}
	events := s.closeOpenItems()
	events = append(events, s.finalResponseEvent("stop", OpenAIUsage{}))
	return events
}

func (s *chatCompletionsToResponsesStreamState) closeOpenItems() []apicompat.ResponsesStreamEvent {
	var events []apicompat.ResponsesStreamEvent
	if s.messageOpened && !s.messageDone {
		events = append(events,
			s.nextEvent("response.output_text.done", &apicompat.ResponsesStreamEvent{
				OutputIndex:  0,
				ContentIndex: 0,
				ItemID:       s.messageItemID,
				Text:         s.messageText.String(),
			}),
			s.nextEvent("response.content_part.done", &apicompat.ResponsesStreamEvent{
				OutputIndex:  0,
				ContentIndex: 0,
				ItemID:       s.messageItemID,
				Part: &apicompat.ResponsesContentPart{
					Type: "output_text",
					Text: s.messageText.String(),
				},
			}),
			s.nextEvent("response.output_item.done", &apicompat.ResponsesStreamEvent{
				OutputIndex: 0,
				StreamItem: &apicompat.ResponsesStreamOutputItem{
					Type: "message",
					ID:   s.messageItemID,
					Role: "assistant",
					Content: []apicompat.ResponsesStreamContentPart{{
						Type: "output_text",
						Text: s.messageText.String(),
					}},
					Status: "completed",
				},
			}),
		)
		s.messageDone = true
	}
	for idx, outputIndex := range s.functionIndexes {
		itemID := s.functionItemIDs[idx]
		events = append(events,
			s.nextEvent("response.function_call_arguments.done", &apicompat.ResponsesStreamEvent{
				OutputIndex: outputIndex,
				ItemID:      itemID,
			}),
			s.nextEvent("response.output_item.done", &apicompat.ResponsesStreamEvent{
				OutputIndex: outputIndex,
				Item: &apicompat.ResponsesOutput{
					Type:   "function_call",
					ID:     itemID,
					Status: "completed",
				},
			}),
		)
		delete(s.functionIndexes, idx)
		delete(s.functionItemIDs, idx)
	}
	return events
}

func (s *chatCompletionsToResponsesStreamState) finalResponseEvent(finishReason string, usage OpenAIUsage) apicompat.ResponsesStreamEvent {
	s.completed = true
	status := "completed"
	var incomplete *apicompat.ResponsesIncompleteDetails
	if finishReason == "length" {
		status = "incomplete"
		incomplete = &apicompat.ResponsesIncompleteDetails{Reason: "max_output_tokens"}
	}

	responseUsage := &apicompat.ResponsesUsage{
		InputTokens:  usage.InputTokens,
		OutputTokens: usage.OutputTokens,
		TotalTokens:  usage.InputTokens + usage.OutputTokens,
	}
	if usage.CacheReadInputTokens > 0 {
		responseUsage.InputTokensDetails = &apicompat.ResponsesInputTokensDetails{CachedTokens: usage.CacheReadInputTokens}
	}

	return s.nextEvent("response.completed", &apicompat.ResponsesStreamEvent{
		Response: &apicompat.ResponsesResponse{
			ID:                s.ResponseID,
			Object:            "response",
			Model:             s.Model,
			Status:            status,
			Output:            s.completedOutputItems(),
			Usage:             responseUsage,
			IncompleteDetails: incomplete,
		},
	})
}

func (s *chatCompletionsToResponsesStreamState) completedOutputItems() []apicompat.ResponsesOutput {
	if !s.messageOpened {
		return []apicompat.ResponsesOutput{}
	}
	return []apicompat.ResponsesOutput{{
		Type: "message",
		ID:   s.messageItemID,
		Role: "assistant",
		Content: []apicompat.ResponsesContentPart{{
			Type: "output_text",
			Text: s.messageText.String(),
		}},
		Status: "completed",
	}}
}

func (s *chatCompletionsToResponsesStreamState) nextEvent(eventType string, evt *apicompat.ResponsesStreamEvent) apicompat.ResponsesStreamEvent {
	out := *evt
	out.Type = eventType
	out.SequenceNumber = s.SequenceNumber
	s.SequenceNumber++
	return out
}

func normalizeCompatArguments(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "{}"
	}
	return raw
}

func generateCompatResponsesItemID(prefix string, outputIndex int) string {
	return fmt.Sprintf("item_%s_%d", prefix, outputIndex)
}
