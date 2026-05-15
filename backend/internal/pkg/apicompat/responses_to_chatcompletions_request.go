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

	out := &ChatCompletionsRequest{
		Model:       req.Model,
		TopP:        req.TopP,
		Stream:      req.Stream,
		ToolChoice:  req.ToolChoice,
		ServiceTier: req.ServiceTier,
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
	if len(req.Tools) > 0 {
		out.Tools = convertResponsesToolsToChat(req.Tools)
	}

	messages, err := convertResponsesInputToChatMessages(req.Input)
	if err != nil {
		return nil, err
	}
	if instructions := strings.TrimSpace(req.Instructions); instructions != "" {
		systemContent, err := json.Marshal(instructions)
		if err != nil {
			return nil, err
		}
		messages = append([]ChatMessage{{
			Role:    "system",
			Content: systemContent,
		}}, messages...)
	}
	out.Messages = messages

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

	var items []ResponsesInputItem
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("parse responses input: %w", err)
	}

	messages := make([]ChatMessage, 0, len(items))
	for _, item := range items {
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
	if len(raw) == 0 {
		return json.Marshal("")
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return json.Marshal(text)
	}

	var parts []ResponsesContentPart
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, fmt.Errorf("parse responses content: %w", err)
	}

	chatParts := make([]ChatContentPart, 0, len(parts))
	var textOnly strings.Builder
	hasImage := false
	for _, part := range parts {
		switch part.Type {
		case "input_text", "output_text":
			if part.Text == "" {
				continue
			}
			textOnly.WriteString(part.Text)
			chatParts = append(chatParts, ChatContentPart{
				Type: "text",
				Text: part.Text,
			})
		case "input_image":
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

func normalizeResponsesArguments(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "{}"
	}
	return raw
}
