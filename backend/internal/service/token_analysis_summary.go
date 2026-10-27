package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"regexp"
	"strconv"
	"strings"
)

var tokenAnalysisRedactions = []*regexp.Regexp{
	regexp.MustCompile(`(?i)bearer\s+[a-z0-9._\-]{12,}`),
	regexp.MustCompile(`(?i)sk-[a-z0-9_\-]{8,}`),
	regexp.MustCompile(`(?i)(api[_-]?key|authorization|cookie)\s*[:=]\s*[^,\s]{8,}`),
	regexp.MustCompile(`[A-Za-z0-9+/]{120,}={0,2}`),
	regexp.MustCompile(`\S{240,}`),
}

// sanitizeTokenAnalysisText 剥离 PostgreSQL text/jsonb 列拒收的 NUL 字节并
// 兜底替换非法 UTF-8。归档 JSON 字符串里合法的 \u0000 转义经解码后就是真实
// 0x00, 任何 body 衍生文本入库前都必须过这里, 否则 pq: invalid byte sequence。
func sanitizeTokenAnalysisText(s string) string {
	if strings.ContainsRune(s, 0) {
		s = strings.ReplaceAll(s, "\x00", "")
	}
	return strings.ToValidUTF8(s, "�")
}

func SummarizeTokenAnalysisRequest(endpoint string, body []byte, maxPreviewChars int) (TokenAnalysisBodySummary, error) {
	root, err := parseTokenAnalysisRequestBody(body)
	if err != nil {
		return TokenAnalysisBodySummary{}, fmt.Errorf("parse request body: %w", err)
	}
	return summarizeTokenAnalysisRoot(endpoint, root, maxPreviewChars), nil
}

// summarizeTokenAnalysisRoot 从已解码的请求体生成摘要; 索引器与指纹共用同一次解码。
func summarizeTokenAnalysisRoot(endpoint string, root map[string]any, maxPreviewChars int) TokenAnalysisBodySummary {
	if maxPreviewChars <= 0 {
		maxPreviewChars = 300
	}
	endpoint = sanitizeTokenAnalysisText(endpoint)
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
	summary.SummaryJSON["shape"] = tokenAnalysisShape(root)
	setTokenAnalysisRequestOrigin(&summary, classifyTokenAnalysisRequestOrigin(root))
	finalizeTokenAnalysisBodySummary(endpoint, &summary, maxPreviewChars)
	return summary
}

// finalizeTokenAnalysisBodySummary 统一摘要尾部(JSON 与 multipart 两条解析
// 路径共用): 预览脱敏与 SummaryJSON 计数字段, 保证下游展示字段齐全。
func finalizeTokenAnalysisBodySummary(endpoint string, summary *TokenAnalysisBodySummary, maxPreviewChars int) {
	// 走到这里即完整解析(降级摘要不经过 finalize), 净输入为空才可视为确认无正文。
	summary.InputComplete = true
	// sanitize 之前 LastUserPreview 即原始全文, 留一份给净输入留存。
	summary.LastUserText = summary.LastUserPreview
	summary.LastUserPreview = SanitizeTokenAnalysisPreview(summary.LastUserPreview, maxPreviewChars)
	summary.SummaryJSON["endpoint"] = strings.TrimSpace(endpoint)
	summary.SummaryJSON["message_count"] = summary.MessageCount
	summary.SummaryJSON["system_chars"] = summary.SystemChars
	summary.SummaryJSON["user_chars"] = summary.UserChars
	summary.SummaryJSON["tools_count"] = summary.ToolsCount
	summary.SummaryJSON["image_count"] = summary.ImageCount
	summary.SummaryJSON["tool_message_count"] = summary.ToolMessageCount
	summary.SummaryJSON["tool_output_bytes"] = summary.ToolOutputBytes
	summary.SummaryJSON["max_tool_output_bytes"] = summary.MaxToolOutputBytes
}

// SummarizeTokenAnalysisMultipartRequest 解析 multipart/form-data 归档体
// (/v1/images/edits 图改图等): 归档原样存了 multipart 原文, JSON 解析必报
// invalid character '-'。前端构造时文本域(model/prompt/size/n)在图片分片
// 之前, 因此即使 body 被归档上限截断, 前缀仍能解出完整文本域; 图片二进制
// 经 string()+JSON 转码已被 U+FFFD 损毁, 只计数不读取。
// 返回 ok=false 表示根本不是 multipart(调用方按原错误继续处理); 是
// multipart 但什么都解不出时返回降级摘要, 不应计失败行。
func SummarizeTokenAnalysisMultipartRequest(endpoint, contentType string, body []byte, maxPreviewChars int) (TokenAnalysisBodySummary, bool) {
	boundary, ok := tokenAnalysisMultipartBoundary(contentType, body)
	if !ok {
		return TokenAnalysisBodySummary{}, false
	}
	if maxPreviewChars <= 0 {
		maxPreviewChars = 300
	}
	endpoint = sanitizeTokenAnalysisText(endpoint)

	fields := map[string]string{}
	imageParts := 0
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		part, err := reader.NextPart()
		if err != nil {
			// 截断 body 的最后一个分片必然 unexpected EOF: 已读到的域照常使用。
			break
		}
		name := part.FormName()
		switch {
		case name == "":
		case name == "image" || strings.HasPrefix(name, "image["):
			// 与网关 parseOpenAIImagesMultipartRequest 的收集口径一致。
			imageParts++
		case part.FileName() != "":
			// mask 等其他文件域: 内容已损毁, 跳过(NextPart 自动丢弃剩余字节)。
		default:
			if _, exists := fields[name]; !exists {
				// 文本域远小于归档上限, 限读仅防御异常构造。
				data, _ := io.ReadAll(io.LimitReader(part, 64*1024))
				fields[name] = string(data)
			}
		}
	}
	if len(fields) == 0 && imageParts == 0 {
		summary := TokenAnalysisBodySummary{SummaryJSON: map[string]any{"degraded": "multipart_body"}}
		setTokenAnalysisRequestOrigin(&summary, tokenAnalysisOrigin("unsupported_shape"))
		return summary, true
	}

	summary := TokenAnalysisBodySummary{
		Model:       sanitizeTokenAnalysisText(strings.TrimSpace(fields["model"])),
		SummaryJSON: map[string]any{},
	}
	prompt := fields["prompt"]
	summary.MessageCount = 1
	summary.UserChars = len([]rune(prompt))
	summary.LastUserPreview = tokenAnalysisUserInputText(prompt)
	// 与 summaryImageRequest 同口径: ImageCount 取请求张数 n, 缺省回退输入图数。
	summary.ImageCount = tokenAnalysisFormInt(fields["n"])
	if summary.ImageCount <= 0 {
		summary.ImageCount = imageParts
	}
	summary.SummaryJSON["shape"] = "image"
	summary.SummaryJSON["multipart"] = true
	summary.SummaryJSON["source_image_count"] = imageParts
	if size := sanitizeTokenAnalysisText(strings.TrimSpace(fields["size"])); size != "" {
		summary.SummaryJSON["size"] = size
	}
	origin, _ := tokenAnalysisOriginFromUserContent(prompt, false)
	setTokenAnalysisRequestOrigin(&summary, origin)
	finalizeTokenAnalysisBodySummary(endpoint, &summary, maxPreviewChars)
	return summary, true
}

// tokenAnalysisMultipartBoundary 从归档的 content_type 头解析 multipart
// boundary; 缺 headers 的历史归档行兜底从 body 首行 "--boundary" 提取。
func tokenAnalysisMultipartBoundary(contentType string, body []byte) (string, bool) {
	if mediaType, params, err := mime.ParseMediaType(contentType); err == nil &&
		strings.HasPrefix(mediaType, "multipart/") {
		if boundary := params["boundary"]; boundary != "" {
			return boundary, true
		}
	}
	if !bytes.HasPrefix(body, []byte("--")) {
		return "", false
	}
	line := body[2:]
	if idx := bytes.IndexByte(line, '\n'); idx >= 0 {
		line = line[:idx]
	}
	boundary := strings.TrimRight(string(line), "\r")
	// RFC 2046 boundary 上限 70 字符, 留余量防把非 multipart 文本误判成 boundary。
	if boundary == "" || len(boundary) > 200 {
		return "", false
	}
	return boundary, true
}

func tokenAnalysisFormInt(value string) int {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return n
}

func SanitizeTokenAnalysisPreview(input string, maxChars int) string {
	out := sanitizeTokenAnalysisText(strings.TrimSpace(input))
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
	return truncateTokenAnalysisRunes(RedactTokenAnalysisInputText(input), maxChars)
}

// RedactTokenAnalysisInputText 只做脱敏不截断, 供留存哈希计算使用:
// 哈希必须对脱敏后文本计算, 否则 secret 会留下可离线比对的原文指纹(codex 审查重要1)。
func RedactTokenAnalysisInputText(input string) string {
	out := sanitizeTokenAnalysisText(strings.TrimSpace(input))
	for _, re := range tokenAnalysisRedactions {
		out = re.ReplaceAllString(out, "[redacted]")
	}
	return out
}

func truncateTokenAnalysisRunes(out string, maxChars int) (string, bool) {
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
		summary.ImageCount += tokenAnalysisImageCount(content)
		switch strings.ToLower(strings.TrimSpace(tokenAnalysisString(msg["role"]))) {
		case "system", "developer":
			summary.SystemChars += len([]rune(tokenAnalysisExtractText(content)))
		case "user":
			text := tokenAnalysisSplitToolResults(content, false, summary)
			summary.UserChars += len([]rune(text))
			if input := tokenAnalysisUserInputText(content); strings.TrimSpace(input) != "" {
				summary.LastUserPreview = input
			}
		case "tool", "function":
			recordTokenAnalysisToolOutput(summary, content)
		}
	}
}

// recordTokenAnalysisToolOutput 计入一条工具输出; 字节数取输出值的 JSON 编码长度,
// 各协议同一口径。
func recordTokenAnalysisToolOutput(summary *TokenAnalysisBodySummary, output any) {
	bytesLen := int64(0)
	if output != nil {
		if raw, err := json.Marshal(output); err == nil {
			bytesLen = int64(len(raw))
		}
	}
	summary.ToolMessageCount++
	summary.ToolOutputBytes += bytesLen
	if bytesLen > summary.MaxToolOutputBytes {
		summary.MaxToolOutputBytes = bytesLen
	}
}

// tokenAnalysisResponsesToolOutputPayload 取 Responses 工具输出条目的结果字段:
// tool_search_output 的发现结果在 tools, 其余 *_call_output 在 output。
func tokenAnalysisResponsesToolOutputPayload(itemType string, item map[string]any) any {
	if itemType == "tool_search_output" {
		return item["tools"]
	}
	return item["output"]
}

// tokenAnalysisSplitToolResults 把用户角色内容中的工具结果块(Anthropic tool_result、
// Gemini functionResponse)计入工具输出, 返回其余块的文本, 用于 UserChars。
func tokenAnalysisSplitToolResults(content any, gemini bool, summary *TokenAnalysisBodySummary) string {
	blocks := tokenAnalysisArray(content)
	if blocks == nil {
		if block, ok := content.(map[string]any); ok {
			blocks = []any{block}
		} else {
			return tokenAnalysisExtractText(content)
		}
	}
	rest := make([]any, 0, len(blocks))
	for _, raw := range blocks {
		block, ok := raw.(map[string]any)
		if ok && tokenAnalysisIsToolResultBlock(block, gemini) {
			switch {
			case !gemini:
				recordTokenAnalysisToolOutput(summary, block["content"])
			case block["functionResponse"] != nil:
				recordTokenAnalysisToolOutput(summary, block["functionResponse"])
			default:
				recordTokenAnalysisToolOutput(summary, block["function_response"])
			}
			continue
		}
		rest = append(rest, raw)
	}
	return tokenAnalysisExtractText(rest)
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
		summary.LastUserPreview = tokenAnalysisUserInputText(text)
		return
	}
	for _, raw := range tokenAnalysisArray(input) {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(tokenAnalysisString(item["role"])))
		itemType := tokenAnalysisString(item["type"])
		if role == "" && tokenAnalysisIsToolOutputItemType(itemType) {
			// 工具输出不是用户文本; 结果所在字段随类型而定。输出里也可能带图片
			// (如 function_call_output 的 input_image 块), 同样计入图片数。
			payload := tokenAnalysisResponsesToolOutputPayload(itemType, item)
			recordTokenAnalysisToolOutput(summary, payload)
			summary.ImageCount += tokenAnalysisImageCount(payload)
			continue
		}
		content := item["content"]
		if content == nil {
			content = item
		}
		summary.ImageCount += tokenAnalysisImageCount(content)
		if role != "" || strings.EqualFold(itemType, "message") {
			summary.MessageCount++
		}
		switch role {
		case "system", "developer":
			summary.SystemChars += len([]rune(tokenAnalysisExtractText(content)))
		case "user":
			// assistant 文本、函数调用、reasoning 等不是用户输入, 不计入 UserChars。
			summary.UserChars += len([]rune(tokenAnalysisExtractText(content)))
			if input := tokenAnalysisUserInputText(item["content"]); strings.TrimSpace(input) != "" {
				summary.LastUserPreview = input
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
		summary.ImageCount += tokenAnalysisImageCount(parts)
		role := strings.ToLower(strings.TrimSpace(tokenAnalysisString(content["role"])))
		if role == "model" {
			continue
		}
		text := tokenAnalysisSplitToolResults(parts, true, summary)
		summary.UserChars += len([]rune(text))
		if input := tokenAnalysisUserInputText(parts); strings.TrimSpace(input) != "" {
			summary.LastUserPreview = input
		}
	}
}

func summaryImageRequest(root map[string]any, summary *TokenAnalysisBodySummary) {
	summary.SummaryJSON["shape"] = "image"
	prompt := tokenAnalysisExtractText(root["prompt"])
	summary.MessageCount = 1
	summary.UserChars = len([]rune(prompt))
	summary.LastUserPreview = tokenAnalysisUserInputText(prompt)
	summary.ImageCount = tokenAnalysisNumberAsInt(root["n"])
	if summary.ImageCount <= 0 {
		summary.ImageCount = tokenAnalysisImageValueCount(root["image"])
	}
}

func summaryFallbackRequest(root map[string]any, summary *TokenAnalysisBodySummary) {
	summary.SummaryJSON["shape"] = "unknown"
	text := tokenAnalysisExtractText(root)
	summary.UserChars = len([]rune(text))
	summary.LastUserPreview = text
}

// tokenAnalysisUserInputText 取一条用户消息中由用户本人写的文本, 作为预览与
// 净输入留存的来源: 跳过工具结果(tool_result/functionResponse 等非文本块)和
// 文本开头的完整客户端注入包装(见 tokenAnalysisInjectedContextTags)。没有用户
// 文本时返回空串, 调用方保留更早的用户输入, 因此同一 agent 轮次内的请求共享
// 同一净输入。UserChars 另按"用户角色的非工具文本"累计(含注入包装), 与预览口径不同。
func tokenAnalysisUserInputText(content any) string {
	switch v := content.(type) {
	case string:
		return tokenAnalysisUserAuthoredText(v)
	case map[string]any:
		return tokenAnalysisUserInputText([]any{v})
	case []any:
		parts := make([]string, 0, len(v))
		for _, raw := range v {
			text, ok := raw.(string)
			if !ok {
				block, isBlock := raw.(map[string]any)
				if !isBlock || !tokenAnalysisIsTextBlock(block) {
					continue
				}
				text, _ = block["text"].(string)
			}
			if text = tokenAnalysisUserAuthoredText(text); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, " ")
	}
	return ""
}

// tokenAnalysisUserAuthoredText drops the injected context leading one text
// part; text without such a prefix is returned unchanged.
func tokenAnalysisUserAuthoredText(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	if rest, stripped := stripTokenAnalysisInjectedPrefix(text); stripped {
		return rest
	}
	return text
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

// tokenAnalysisImageFields 是承载图片的字段(OpenAI image_url/input_image, Gemini inline/file data)。
var tokenAnalysisImageFields = []string{"image", "image_url", "input_image", "inline_data", "inlineData", "file_data", "fileData"}

// tokenAnalysisImageBlockTypes 是图片内容块类型; Anthropic 的图片块只有 type 与 source。
var tokenAnalysisImageBlockTypes = map[string]bool{"image": true, "input_image": true, "image_url": true}

// tokenAnalysisImageCount 统计消息内容中的图片: 只按图片字段与图片块类型计数。内容中的
// 字符串是文本(Responses/Chat/Messages 都允许 content 为纯文本字符串), 不是图片。
func tokenAnalysisImageCount(value any) int {
	switch v := value.(type) {
	case []any:
		total := 0
		for _, item := range v {
			total += tokenAnalysisImageCount(item)
		}
		return total
	case map[string]any:
		total := 0
		for _, key := range tokenAnalysisImageFields {
			if child, ok := v[key]; ok && child != nil {
				total += max(tokenAnalysisImageValueCount(child), 1)
			}
		}
		if total == 0 && tokenAnalysisImageBlockTypes[strings.ToLower(tokenAnalysisString(v["type"]))] {
			total = 1
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

// tokenAnalysisImageValueCount 统计图片字段的值: 非空字符串(URL/base64)是一张图, 数组逐项计数。
func tokenAnalysisImageValueCount(value any) int {
	switch v := value.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return 0
		}
		return 1
	case []any:
		total := 0
		for _, item := range v {
			total += tokenAnalysisImageValueCount(item)
		}
		return total
	case map[string]any:
		return 1
	}
	return 0
}

func tokenAnalysisToolCount(root map[string]any) int {
	count := tokenAnalysisToolEntries(root["tools"])
	count += len(tokenAnalysisArray(root["functions"]))
	// Codex(Responses Lite)把工具定义放在 input 的 additional_tools 条目里, 不在顶层 tools。
	for _, raw := range tokenAnalysisArray(root["input"]) {
		if item, ok := raw.(map[string]any); ok && tokenAnalysisString(item["type"]) == "additional_tools" {
			count += tokenAnalysisToolEntries(item["tools"])
		}
	}
	return count
}

// tokenAnalysisToolEntries 统计可调用的工具定义数: Gemini function_declarations 逐个计,
// namespace 等带嵌套 tools 的条目递归计其叶子, 其余条目各计 1。
func tokenAnalysisToolEntries(value any) int {
	count := 0
	for _, raw := range tokenAnalysisArray(value) {
		tool, ok := raw.(map[string]any)
		if !ok {
			count++
			continue
		}
		if declarations := tokenAnalysisArray(tool["function_declarations"]); len(declarations) > 0 {
			count += len(declarations)
			continue
		}
		if nested, isNested := tokenAnalysisNestedTools(tool); isNested {
			count += tokenAnalysisToolEntries(nested)
			continue
		}
		count++
	}
	return count
}

// tokenAnalysisNestedTools 取容器条目(namespace、additional_tools)的子工具: 非空 tools 优先;
// namespace 回退 children 别名, 与 apicompat 的 namespaceChildren 口径一致。只有 namespace
// 才认 children, 不对任意对象的 children 字段递归。
func tokenAnalysisNestedTools(tool map[string]any) ([]any, bool) {
	tools, hasTools := tool["tools"].([]any)
	if len(tools) > 0 {
		return tools, true
	}
	if strings.EqualFold(tokenAnalysisString(tool["type"]), "namespace") {
		if children, ok := tool["children"].([]any); ok {
			return children, true
		}
	}
	return tools, hasTools
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
	return sanitizeTokenAnalysisText(strings.TrimSpace(text))
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
		// shape 来自 body 原文, 同样要过 NUL 清洗才能进 jsonb。
		return sanitizeTokenAnalysisText(shape)
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
