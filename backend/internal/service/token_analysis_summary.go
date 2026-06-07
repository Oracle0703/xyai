package service

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var tokenAnalysisRedactions = []*regexp.Regexp{
	regexp.MustCompile(`(?i)bearer\s+[a-z0-9._\-]{12,}`),
	regexp.MustCompile(`(?i)sk-[a-z0-9_\-]{8,}`),
	regexp.MustCompile(`(?i)(api[_-]?key|authorization|cookie)\s*[:=]\s*[^,\s]{8,}`),
	regexp.MustCompile(`[A-Za-z0-9+/]{120,}={0,2}`),
	regexp.MustCompile(`\S{240,}`),
}

func SummarizeTokenAnalysisRequest(endpoint string, body []byte, maxPreviewChars int) (TokenAnalysisBodySummary, error) {
	if maxPreviewChars <= 0 {
		maxPreviewChars = 300
	}
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return TokenAnalysisBodySummary{}, fmt.Errorf("parse request body: %w", err)
	}

	summary := TokenAnalysisBodySummary{
		Model:       tokenAnalysisString(root["model"]),
		SummaryJSON: map[string]any{},
	}
	switch {
	case tokenAnalysisArray(root["contents"]) != nil:
		summaryGeminiRequest(root, &summary)
	case root["prompt"] != nil && root["messages"] == nil && root["input"] == nil:
		summaryImageRequest(root, &summary)
	case root["input"] != nil:
		summaryResponsesRequest(root, &summary)
	case tokenAnalysisArray(root["messages"]) != nil:
		summaryMessagesRequest(root, &summary)
	default:
		summaryFallbackRequest(root, &summary)
	}

	if summary.Model == "" {
		summary.Model = tokenAnalysisModelFromEndpoint(endpoint)
	}
	summary.ToolsCount += tokenAnalysisToolCount(root)
	// sanitize 之前 LastUserPreview 即原始全文, 留一份给净输入留存。
	summary.LastUserText = summary.LastUserPreview
	summary.LastUserPreview = SanitizeTokenAnalysisPreview(summary.LastUserPreview, maxPreviewChars)
	summary.SummaryJSON["shape"] = tokenAnalysisShape(root)
	summary.SummaryJSON["endpoint"] = strings.TrimSpace(endpoint)
	summary.SummaryJSON["message_count"] = summary.MessageCount
	summary.SummaryJSON["system_chars"] = summary.SystemChars
	summary.SummaryJSON["user_chars"] = summary.UserChars
	summary.SummaryJSON["tools_count"] = summary.ToolsCount
	summary.SummaryJSON["image_count"] = summary.ImageCount
	summary.SummaryJSON["tool_message_count"] = summary.ToolMessageCount
	summary.SummaryJSON["tool_output_bytes"] = summary.ToolOutputBytes
	summary.SummaryJSON["max_tool_output_bytes"] = summary.MaxToolOutputBytes
	return summary, nil
}

func SanitizeTokenAnalysisPreview(input string, maxChars int) string {
	out := strings.TrimSpace(input)
	for _, re := range tokenAnalysisRedactions {
		out = re.ReplaceAllString(out, "[redacted]")
	}
	out = strings.Join(strings.Fields(out), " ")
	if maxChars <= 0 {
		return out
	}
	runes := []rune(out)
	if len(runes) <= maxChars {
		return out
	}
	return string(runes[:maxChars])
}

// SanitizeTokenAnalysisInputText 为净输入留存做脱敏与截断:
// 与预览不同, 保留换行/缩进等原始排版(后续按标准评估时格式本身就是信号)。
func SanitizeTokenAnalysisInputText(input string, maxChars int) (string, bool) {
	out := strings.TrimSpace(input)
	for _, re := range tokenAnalysisRedactions {
		out = re.ReplaceAllString(out, "[redacted]")
	}
	if maxChars <= 0 {
		return out, false
	}
	runes := []rune(out)
	if len(runes) <= maxChars {
		return out, false
	}
	return string(runes[:maxChars]), true
}

func summaryMessagesRequest(root map[string]any, summary *TokenAnalysisBodySummary) {
	summary.SummaryJSON["shape"] = "messages"
	summary.SystemChars += len([]rune(tokenAnalysisExtractText(root["system"])))
	for _, raw := range tokenAnalysisArray(root["messages"]) {
		msg, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		summary.MessageCount++
		content := msg["content"]
		text := tokenAnalysisExtractText(content)
		summary.ImageCount += tokenAnalysisImageCount(content)
		switch strings.ToLower(strings.TrimSpace(tokenAnalysisString(msg["role"]))) {
		case "system", "developer":
			summary.SystemChars += len([]rune(text))
		case "user":
			summary.UserChars += len([]rune(text))
			if strings.TrimSpace(text) != "" {
				summary.LastUserPreview = text
			}
		case "tool":
			bytesLen := int64(0)
			if raw, err := json.Marshal(content); err == nil {
				bytesLen = int64(len(raw))
			}
			summary.ToolMessageCount++
			summary.ToolOutputBytes += bytesLen
			if bytesLen > summary.MaxToolOutputBytes {
				summary.MaxToolOutputBytes = bytesLen
			}
		}
	}
}

func summaryResponsesRequest(root map[string]any, summary *TokenAnalysisBodySummary) {
	summary.SummaryJSON["shape"] = "responses"
	if instructions := tokenAnalysisExtractText(root["instructions"]); instructions != "" {
		summary.SystemChars += len([]rune(instructions))
	}
	input := root["input"]
	if text, ok := input.(string); ok {
		summary.MessageCount = 1
		summary.UserChars = len([]rune(text))
		summary.LastUserPreview = text
		return
	}
	for _, raw := range tokenAnalysisArray(input) {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(tokenAnalysisString(item["role"])))
		content := item["content"]
		if content == nil {
			content = item
		}
		text := tokenAnalysisExtractText(content)
		summary.ImageCount += tokenAnalysisImageCount(content)
		if role != "" || strings.EqualFold(tokenAnalysisString(item["type"]), "message") {
			summary.MessageCount++
		}
		switch role {
		case "system", "developer":
			summary.SystemChars += len([]rune(text))
		default:
			if text != "" {
				summary.UserChars += len([]rune(text))
				summary.LastUserPreview = text
			}
		}
	}
}

func summaryGeminiRequest(root map[string]any, summary *TokenAnalysisBodySummary) {
	summary.SummaryJSON["shape"] = "gemini"
	summary.SystemChars += len([]rune(tokenAnalysisExtractText(root["system_instruction"])))
	for _, raw := range tokenAnalysisArray(root["contents"]) {
		content, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		summary.MessageCount++
		parts := content["parts"]
		text := tokenAnalysisExtractText(parts)
		summary.ImageCount += tokenAnalysisImageCount(parts)
		role := strings.ToLower(strings.TrimSpace(tokenAnalysisString(content["role"])))
		if role == "model" {
			continue
		}
		summary.UserChars += len([]rune(text))
		if strings.TrimSpace(text) != "" {
			summary.LastUserPreview = text
		}
	}
}

func summaryImageRequest(root map[string]any, summary *TokenAnalysisBodySummary) {
	summary.SummaryJSON["shape"] = "image"
	prompt := tokenAnalysisExtractText(root["prompt"])
	summary.MessageCount = 1
	summary.UserChars = len([]rune(prompt))
	summary.LastUserPreview = prompt
	summary.ImageCount = tokenAnalysisNumberAsInt(root["n"])
	if summary.ImageCount <= 0 {
		summary.ImageCount = tokenAnalysisImageCount(root["image"])
	}
}

func summaryFallbackRequest(root map[string]any, summary *TokenAnalysisBodySummary) {
	summary.SummaryJSON["shape"] = "unknown"
	text := tokenAnalysisExtractText(root)
	summary.UserChars = len([]rune(text))
	summary.LastUserPreview = text
}

func tokenAnalysisExtractText(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if text := tokenAnalysisExtractText(item); strings.TrimSpace(text) != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, " ")
	case map[string]any:
		for _, key := range []string{"text", "input_text", "content", "prompt"} {
			if text, ok := v[key].(string); ok {
				return text
			}
		}
		if parts := tokenAnalysisArray(v["parts"]); parts != nil {
			return tokenAnalysisExtractText(parts)
		}
		if content := tokenAnalysisArray(v["content"]); content != nil {
			return tokenAnalysisExtractText(content)
		}
	}
	return ""
}

func tokenAnalysisImageCount(value any) int {
	switch v := value.(type) {
	case nil:
		return 0
	case string:
		if strings.TrimSpace(v) == "" {
			return 0
		}
		return 1
	case []any:
		total := 0
		for _, item := range v {
			total += tokenAnalysisImageCount(item)
		}
		return total
	case map[string]any:
		total := 0
		for _, key := range []string{"image", "image_url", "input_image", "inline_data", "inlineData", "file_data"} {
			if child, ok := v[key]; ok && child != nil {
				if nested := tokenAnalysisImageCount(child); nested > 0 {
					total += nested
				} else {
					total++
				}
			}
		}
		if content := tokenAnalysisArray(v["content"]); content != nil {
			total += tokenAnalysisImageCount(content)
		}
		if parts := tokenAnalysisArray(v["parts"]); parts != nil {
			total += tokenAnalysisImageCount(parts)
		}
		return total
	}
	return 0
}

func tokenAnalysisToolCount(root map[string]any) int {
	count := 0
	for _, raw := range tokenAnalysisArray(root["tools"]) {
		tool, ok := raw.(map[string]any)
		if !ok {
			count++
			continue
		}
		if declarations := tokenAnalysisArray(tool["function_declarations"]); len(declarations) > 0 {
			count += len(declarations)
			continue
		}
		count++
	}
	count += len(tokenAnalysisArray(root["functions"]))
	return count
}

func tokenAnalysisArray(value any) []any {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	return items
}

func tokenAnalysisString(value any) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func tokenAnalysisNumberAsInt(value any) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	case json.Number:
		i, _ := v.Int64()
		return int(i)
	default:
		return 0
	}
}

func tokenAnalysisShape(root map[string]any) string {
	if shape, ok := root["shape"].(string); ok {
		return shape
	}
	if tokenAnalysisArray(root["contents"]) != nil {
		return "gemini"
	}
	if root["prompt"] != nil && root["messages"] == nil && root["input"] == nil {
		return "image"
	}
	if root["input"] != nil {
		return "responses"
	}
	if tokenAnalysisArray(root["messages"]) != nil {
		return "messages"
	}
	return "unknown"
}

func tokenAnalysisModelFromEndpoint(endpoint string) string {
	const marker = "/models/"
	idx := strings.Index(endpoint, marker)
	if idx < 0 {
		return ""
	}
	rest := endpoint[idx+len(marker):]
	if colon := strings.Index(rest, ":"); colon >= 0 {
		rest = rest[:colon]
	}
	if slash := strings.Index(rest, "/"); slash >= 0 {
		rest = rest[:slash]
	}
	return strings.TrimSpace(rest)
}
