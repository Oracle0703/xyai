package apicompat

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ResponsesToChatCompletionsRequest converts a Responses API request into a
// Chat Completions request. This is used for third-party OpenAI-compatible
// upstreams that only implement /v1/chat/completions.
func ResponsesToChatCompletionsRequest(req *ResponsesRequest) (*ChatCompletionsRequest, error) {
	return ResponsesToChatCompletionsRequestWithOptions(req, ResponsesToChatCompletionsOptions{})
}

// ResponsesToChatCompletionsOptions controls compatibility filtering applied
// while converting a Responses request into a Chat Completions request.
type ResponsesToChatCompletionsOptions struct {
	DropTemperature         bool
	DropMaxCompletionTokens bool
}

// ResponsesToChatCompletionsRequestWithOptions converts a Responses API
// request into a Chat Completions request with upstream-specific filtering.
func ResponsesToChatCompletionsRequestWithOptions(req *ResponsesRequest, opts ResponsesToChatCompletionsOptions) (*ChatCompletionsRequest, error) {
	if req == nil {
		return nil, fmt.Errorf("nil responses request")
	}

	messages, err := responsesInputToChatMessages(req.Instructions, req.Input)
	if err != nil {
		return nil, err
	}

	out := &ChatCompletionsRequest{
		Model:             req.Model,
		Messages:          messages,
		TopP:              req.TopP,
		Stream:            req.Stream,
		ParallelToolCalls: req.ParallelToolCalls,
		ServiceTier:       req.ServiceTier,
	}
	if !opts.DropTemperature {
		out.Temperature = req.Temperature
	}
	if req.Stream {
		out.StreamOptions = &ChatStreamOptions{IncludeUsage: true}
	}

	if req.MaxOutputTokens != nil && !opts.DropMaxCompletionTokens {
		v := *req.MaxOutputTokens
		out.MaxCompletionTokens = &v
	}
	if req.Reasoning != nil {
		out.ReasoningEffort = strings.TrimSpace(req.Reasoning.Effort)
	}
	effectiveTools, err := EffectiveResponsesTools(req)
	if err != nil {
		return nil, err
	}
	if len(effectiveTools) > 0 {
		tools, err := responsesToolsToChatTools(effectiveTools)
		if err != nil {
			return nil, err
		}
		out.Tools = tools
	}
	if len(out.Tools) > 0 && len(req.ToolChoice) > 0 {
		declared := make(map[string]bool, len(out.Tools))
		for _, tool := range out.Tools {
			if tool.Function != nil {
				declared[tool.Function.Name] = true
			}
			if strings.EqualFold(strings.TrimSpace(tool.Type), "x_search") {
				declared["x_search"] = true
			}
		}
		out.ToolChoice = responsesToolChoiceToChatToolChoice(req.ToolChoice, declared)
	}
	if req.Text != nil {
		out.ResponseFormat = responsesTextFormatToChatResponseFormat(req.Text.Format)
	}

	return out, nil
}

func convertResponsesToolsToChat(tools []ResponsesTool) []ChatTool {
	out := make([]ChatTool, 0, len(tools))
	for _, tool := range tools {
		if strings.TrimSpace(tool.Type) != "function" {
			continue
		}
		fn := &ChatFunction{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.Parameters,
			Strict:      tool.Strict,
		}
		out = append(out, ChatTool{
			Type:     "function",
			Function: fn,
		})
	}
	return out
}

func convertResponsesInputToChatMessages(raw json.RawMessage) ([]ChatMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		content, err := json.Marshal(text)
		if err != nil {
			return nil, err
		}
		return []ChatMessage{{
			Role:    "user",
			Content: content,
		}}, nil
	}

	var rawItems []json.RawMessage
	if err := json.Unmarshal(raw, &rawItems); err != nil {
		return nil, fmt.Errorf("parse responses input: %w", err)
	}

	messages := make([]ChatMessage, 0, len(rawItems))
	for _, rawItem := range rawItems {
		rawItem = bytesTrimSpace(rawItem)
		if len(rawItem) == 0 || string(rawItem) == "null" {
			continue
		}
		var item ResponsesInputItem
		if err := json.Unmarshal(rawItem, &item); err != nil {
			var text string
			if textErr := json.Unmarshal(rawItem, &text); textErr == nil {
				content, err := json.Marshal(text)
				if err != nil {
					return nil, err
				}
				messages = append(messages, ChatMessage{Role: "user", Content: content})
				continue
			}
			return nil, fmt.Errorf("parse responses input item: %w", err)
		}
		var itemMap map[string]json.RawMessage
		_ = json.Unmarshal(rawItem, &itemMap)
		switch strings.TrimSpace(item.Type) {
		case "function_call":
			messages = append(messages, ChatMessage{
				Role: "assistant",
				ToolCalls: []ChatToolCall{{
					ID:   item.CallID,
					Type: "function",
					Function: ChatFunctionCall{
						Name:      item.Name,
						Arguments: normalizeResponsesArguments(item.Arguments),
					},
				}},
			})
		case "function_call_output":
			output, err := json.Marshal(item.Output)
			if err != nil {
				return nil, err
			}
			messages = append(messages, ChatMessage{
				Role:       "tool",
				ToolCallID: item.CallID,
				Content:    output,
			})
		case "input_text", "text":
			text := rawString(itemMap["text"])
			if strings.TrimSpace(text) == "" {
				continue
			}
			content, err := json.Marshal(text)
			if err != nil {
				return nil, err
			}
			messages = append(messages, ChatMessage{
				Role:    "user",
				Content: content,
			})
		case "input_image", "image_url":
			imageURL := rawString(itemMap["image_url"])
			if imageURL == "" {
				imageURL = rawNestedString(itemMap["image_url"], "url")
			}
			if strings.TrimSpace(imageURL) == "" {
				continue
			}
			content, err := json.Marshal([]ChatContentPart{{
				Type: "image_url",
				ImageURL: &ChatImageURL{
					URL: imageURL,
				},
			}})
			if err != nil {
				return nil, err
			}
			messages = append(messages, ChatMessage{
				Role:    "user",
				Content: content,
			})
		default:
			role := normalizeResponsesInputRole(item.Role)
			if role == "" {
				continue
			}
			content, err := convertResponsesContentToChatMessageContent(role, item.Content)
			if err != nil {
				return nil, err
			}
			messages = append(messages, ChatMessage{
				Role:    role,
				Content: content,
			})
		}
	}

	return messages, nil
}

func normalizeResponsesInputRole(role string) string {
	switch strings.TrimSpace(role) {
	case "developer", "system":
		return "system"
	case "user":
		return "user"
	case "assistant":
		return "assistant"
	default:
		return ""
	}
}

func convertResponsesContentToChatMessageContent(role string, raw json.RawMessage) (json.RawMessage, error) {
	raw = bytesTrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return json.Marshal("")
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return json.Marshal(text)
	}

	var parts []ResponsesContentPart
	if err := json.Unmarshal(raw, &parts); err != nil {
		var part map[string]json.RawMessage
		if err := json.Unmarshal(raw, &part); err != nil {
			return nil, fmt.Errorf("parse responses content: %w", err)
		}
		return convertSingleResponsesContentPartToChat(role, part)
	}

	chatParts := make([]ChatContentPart, 0, len(parts))
	var textOnly strings.Builder
	hasImage := false
	for _, part := range parts {
		switch strings.TrimSpace(part.Type) {
		case "input_text", "output_text", "text", "":
			if part.Text == "" {
				continue
			}
			if textOnly.Len() > 0 {
				textOnly.WriteString("\n\n")
			}
			textOnly.WriteString(part.Text)
			chatParts = append(chatParts, ChatContentPart{
				Type: "text",
				Text: part.Text,
			})
		case "input_image", "image_url":
			if part.ImageURL == "" {
				continue
			}
			hasImage = true
			chatParts = append(chatParts, ChatContentPart{
				Type: "image_url",
				ImageURL: &ChatImageURL{
					URL: part.ImageURL,
				},
			})
		}
	}

	if role != "user" || !hasImage {
		return json.Marshal(textOnly.String())
	}
	return json.Marshal(chatParts)
}

func convertSingleResponsesContentPartToChat(role string, part map[string]json.RawMessage) (json.RawMessage, error) {
	switch rawString(part["type"]) {
	case "input_image", "image_url":
		imageURL := rawString(part["image_url"])
		if imageURL == "" {
			imageURL = rawNestedString(part["image_url"], "url")
		}
		if role != "user" || imageURL == "" {
			return json.Marshal("")
		}
		return json.Marshal([]ChatContentPart{{
			Type:     "image_url",
			ImageURL: &ChatImageURL{URL: imageURL},
		}})
	default:
		return json.Marshal(rawString(part["text"]))
	}
}

func normalizeResponsesArguments(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "{}"
	}
	return raw
}
