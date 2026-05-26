package promptmetrics

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
)

var (
	roleUserContentFragmentPattern = regexp.MustCompile(`(?is)"role"\s*:\s*"user"[^{}\[\]]*"(?:content|text)"\s*:\s*"((?:\\.|[^"\\])*)`)
	roleUserTextBlockPattern       = regexp.MustCompile(`(?is)"role"\s*:\s*"user"[^{}\[\]]*"content"\s*:\s*\[\s*\{[^{}\[\]]*"type"\s*:\s*"(?:text|input_text)"[^{}\[\]]*"text"\s*:\s*"((?:\\.|[^"\\])*)`)
	inputStringFragmentPattern     = regexp.MustCompile(`(?is)"input"\s*:\s*"((?:\\.|[^"\\])*)`)
	promptStringFragmentPattern    = regexp.MustCompile(`(?is)"prompt"\s*:\s*"((?:\\.|[^"\\])*)`)
	geminiTextFragmentPattern      = regexp.MustCompile(`(?is)"role"\s*:\s*"user"[^{}\[\]]*"parts"\s*:\s*\[\s*\{[^{}\[\]]*"text"\s*:\s*"((?:\\.|[^"\\])*)`)
)

// Extractor 从网关 JSON 请求体中提取用户手工输入.
// 只读取明确属于用户角色或顶层 prompt 的字段, 避免采集 system, assistant, tool 等内部内容.
type Extractor struct{}

// NewExtractor 构造无状态提取器, 便于 Wire 注入和单元测试复用.
func NewExtractor() *Extractor {
	return &Extractor{}
}

// Extract 按 endpoint 优先识别协议, 再用请求体字段兜底抽取用户输入.
// 非法 JSON 或缺少用户内容时返回空 Text, 调用方应跳过采集.
func (e *Extractor) Extract(endpoint string, body []byte) ExtractedPrompt {
	if !gjson.ValidBytes(body) {
		return ExtractedPrompt{}
	}
	path := strings.ToLower(strings.TrimSpace(endpoint))
	json := string(body)
	var result ExtractedPrompt
	result.RequestedModel = strings.TrimSpace(gjson.Get(json, "model").String())

	switch {
	case strings.Contains(path, "/images/"):
		result.SourceProtocol = "openai_images"
		result.Text = strings.TrimSpace(gjson.Get(json, "prompt").String())
		result.Segments = countSegments(result.Text)
	case strings.Contains(path, "/responses") || strings.Contains(path, "/backend-api/codex/"):
		result = e.extractResponses(json, result)
	case strings.Contains(path, "/chat/completions"):
		result = e.extractOpenAIChat(json, result)
	case strings.Contains(path, "/messages"):
		result = e.extractAnthropicMessages(json, result)
	case strings.Contains(path, "/v1beta/models/") || strings.Contains(path, "/v1/models/"):
		result = e.extractGemini(json, result)
	default:
		result = e.extractFallback(json, result)
	}

	result.Text = normalizePromptText(result.Text)
	result.Segments = countSegments(result.Text)
	return result
}

// ExtractTruncated 在请求体被 MaxPromptBytes 截断后做保守兜底提取.
// 该路径只处理已经进入预览窗口的字符串片段, 避免为了采集指标而完整读取大 body.
func (e *Extractor) ExtractTruncated(endpoint string, body []byte) ExtractedPrompt {
	path := strings.ToLower(strings.TrimSpace(endpoint))
	raw := string(body)
	var result ExtractedPrompt
	result.PromptTruncated = true
	switch {
	case strings.Contains(path, "/images/"):
		result.SourceProtocol = "openai_images"
		result.Text = firstJSONFragment(promptStringFragmentPattern, raw)
	case strings.Contains(path, "/responses") || strings.Contains(path, "/backend-api/codex/"):
		result.SourceProtocol = "openai_responses"
		if text := firstJSONFragment(inputStringFragmentPattern, raw); text != "" {
			result.Text = text
			break
		}
		result.Text = strings.Join(roleUserJSONFragments(raw), "\n\n")
	case strings.Contains(path, "/chat/completions"):
		result.SourceProtocol = "openai_chat"
		result.Text = strings.Join(roleUserJSONFragments(raw), "\n\n")
	case strings.Contains(path, "/messages"):
		result.SourceProtocol = "anthropic_messages"
		result.Text = strings.Join(roleUserJSONFragments(raw), "\n\n")
	case strings.Contains(path, "/v1beta/models/") || strings.Contains(path, "/v1/models/"):
		result.SourceProtocol = "gemini"
		result.Text = strings.Join(jsonFragments(geminiTextFragmentPattern, raw), "\n\n")
	default:
		result.Text = firstJSONFragment(promptStringFragmentPattern, raw)
		if result.Text != "" {
			result.SourceProtocol = "prompt"
			break
		}
		result.Text = firstJSONFragment(inputStringFragmentPattern, raw)
		if result.Text != "" {
			result.SourceProtocol = "input"
			break
		}
		result.Text = strings.Join(roleUserJSONFragments(raw), "\n\n")
	}
	result.Text = normalizePromptText(result.Text)
	result.Segments = countSegments(result.Text)
	return result
}

// extractOpenAIChat 提取 OpenAI Chat messages[].role=user 的文本内容.
// content 支持 string 和数组, 数组中只读取 text/input_text 类型片段.
func (e *Extractor) extractOpenAIChat(json string, result ExtractedPrompt) ExtractedPrompt {
	result.SourceProtocol = "openai_chat"
	segments := make([]string, 0)
	gjson.Get(json, "messages").ForEach(func(_, message gjson.Result) bool {
		if message.Get("role").String() != "user" {
			return true
		}
		segments = append(segments, extractContentText(message.Get("content"))...)
		return true
	})
	result.Text = strings.Join(segments, "\n\n")
	return result
}

// extractAnthropicMessages 提取 Anthropic messages[].role=user 的 text 片段.
// 不读取 system 字段和非文本 block, 满足只采集用户手工输入的边界.
func (e *Extractor) extractAnthropicMessages(json string, result ExtractedPrompt) ExtractedPrompt {
	result.SourceProtocol = "anthropic_messages"
	segments := make([]string, 0)
	gjson.Get(json, "messages").ForEach(func(_, message gjson.Result) bool {
		if message.Get("role").String() != "user" {
			return true
		}
		segments = append(segments, extractContentText(message.Get("content"))...)
		return true
	})
	result.Text = strings.Join(segments, "\n\n")
	return result
}

// extractResponses 提取 OpenAI Responses 和 Codex 风格 input 中的用户文本.
// input 为字符串时视为用户输入, input 为数组时只读取 role=user 的文本项.
func (e *Extractor) extractResponses(json string, result ExtractedPrompt) ExtractedPrompt {
	result.SourceProtocol = "openai_responses"
	input := gjson.Get(json, "input")
	if input.Type == gjson.String {
		result.Text = input.String()
		return result
	}
	segments := make([]string, 0)
	if input.IsArray() {
		input.ForEach(func(_, item gjson.Result) bool {
			role := item.Get("role").String()
			itemType := strings.TrimSpace(item.Get("type").String())
			if role != "user" || (itemType != "" && itemType != "message") {
				return true
			}
			if text := strings.TrimSpace(item.Get("text").String()); text != "" {
				segments = append(segments, text)
			}
			segments = append(segments, extractContentText(item.Get("content"))...)
			return true
		})
	}
	result.Text = strings.Join(segments, "\n\n")
	return result
}

// extractGemini 提取 Gemini contents[].role=user 的 parts.text.
func (e *Extractor) extractGemini(json string, result ExtractedPrompt) ExtractedPrompt {
	result.SourceProtocol = "gemini"
	segments := make([]string, 0)
	gjson.Get(json, "contents").ForEach(func(_, content gjson.Result) bool {
		role := content.Get("role").String()
		if role != "" && role != "user" {
			return true
		}
		content.Get("parts").ForEach(func(_, part gjson.Result) bool {
			if text := strings.TrimSpace(part.Get("text").String()); text != "" {
				segments = append(segments, text)
			}
			return true
		})
		return true
	})
	result.Text = strings.Join(segments, "\n\n")
	return result
}

// extractFallback 在 endpoint 无法识别时按常见字段做保守兜底.
// 兜底仍然只读取用户角色消息或顶层 prompt/input 字符串, 不读取 system/instructions.
func (e *Extractor) extractFallback(json string, result ExtractedPrompt) ExtractedPrompt {
	if text := strings.TrimSpace(gjson.Get(json, "prompt").String()); text != "" {
		result.SourceProtocol = "prompt"
		result.Text = text
		return result
	}
	if input := gjson.Get(json, "input"); input.Type == gjson.String {
		result.SourceProtocol = "input"
		result.Text = input.String()
		return result
	}
	if messages := gjson.Get(json, "messages"); messages.IsArray() {
		return e.extractOpenAIChat(json, result)
	}
	if contents := gjson.Get(json, "contents"); contents.IsArray() {
		return e.extractGemini(json, result)
	}
	return result
}

// extractContentText 从 OpenAI/Anthropic content 字段中提取文本片段.
// 数组内容仅接受 text/input_text 类型或含 text 字段的对象, 跳过 tool 和图片等非手工文本.
func extractContentText(content gjson.Result) []string {
	if !content.Exists() {
		return nil
	}
	if content.Type == gjson.String {
		if text := strings.TrimSpace(content.String()); text != "" {
			return []string{text}
		}
		return nil
	}
	if !content.IsArray() {
		return nil
	}
	segments := make([]string, 0)
	content.ForEach(func(_, item gjson.Result) bool {
		itemType := strings.TrimSpace(item.Get("type").String())
		if itemType != "" && itemType != "text" && itemType != "input_text" {
			return true
		}
		if text := strings.TrimSpace(item.Get("text").String()); text != "" {
			segments = append(segments, text)
		}
		return true
	})
	return segments
}

func roleUserJSONFragments(raw string) []string {
	segments := jsonFragments(roleUserContentFragmentPattern, raw)
	segments = append(segments, jsonFragments(roleUserTextBlockPattern, raw)...)
	return segments
}

// jsonFragments 从截断 JSON 片段中读取已出现的字符串内容.
// 只处理预编译白名单模式的第一个捕获组, 不尝试恢复完整 JSON 结构.
func jsonFragments(pattern *regexp.Regexp, raw string) []string {
	matches := pattern.FindAllStringSubmatch(raw, -1)
	segments := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		if text := strings.TrimSpace(unquoteJSONFragment(match[1])); text != "" {
			segments = append(segments, text)
		}
	}
	return segments
}

// firstJSONFragment 返回第一个匹配的截断字符串片段.
// 用于顶层 prompt/input 这类最多只需要一个文本值的协议字段.
func firstJSONFragment(pattern *regexp.Regexp, raw string) string {
	segments := jsonFragments(pattern, raw)
	if len(segments) == 0 {
		return ""
	}
	return segments[0]
}

// unquoteJSONFragment 尽量还原 JSON 字符串转义.
// 截断点可能落在反斜杠后, 这里会去掉尾部残缺转义以避免 Unquote 失败.
func unquoteJSONFragment(fragment string) string {
	text, err := strconv.Unquote(`"` + strings.TrimRight(fragment, `\`) + `"`)
	if err != nil {
		return fragment
	}
	return text
}

func normalizePromptText(text string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func countSegments(text string) int {
	if strings.TrimSpace(text) == "" {
		return 0
	}
	parts := strings.Split(text, "\n\n")
	count := 0
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			count++
		}
	}
	if count == 0 {
		return 1
	}
	return count
}
