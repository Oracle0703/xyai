package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestTokenAnalysisGetSummaryForcesUnmatchedAndComputesCoverage(t *testing.T) {
	repo := &tokenAnalysisRepoStub{
		summary:        &TokenAnalysisSummary{TotalRequests: 12, MatchedRequests: 9},
		billedRequests: 18,
	}
	svc := NewTokenAnalysisService(repo, &config.Config{}, nil)

	got, err := svc.GetSummary(context.Background(), TokenAnalysisFilters{IncludeUnmatched: false})

	require.NoError(t, err)
	// 概览口径固定含未匹配行, 不随页面 include_unmatched 勾选变化。
	require.NotNil(t, repo.summaryFilters)
	require.True(t, repo.summaryFilters.IncludeUnmatched)
	require.Equal(t, int64(18), got.BilledRequests)
	require.InDelta(t, 0.5, got.ArchiveCoverage, 1e-9)
}

func TestTokenAnalysisGetSummaryZeroBilledRequestsKeepsCoverageZero(t *testing.T) {
	repo := &tokenAnalysisRepoStub{summary: &TokenAnalysisSummary{MatchedRequests: 3}}
	svc := NewTokenAnalysisService(repo, &config.Config{}, nil)

	got, err := svc.GetSummary(context.Background(), TokenAnalysisFilters{})

	require.NoError(t, err)
	require.Zero(t, got.BilledRequests)
	require.Zero(t, got.ArchiveCoverage)
}

func TestTokenAnalysisSanitizersStripNUL(t *testing.T) {
	// Postgres text 列拒收 0x00; 预览与留存脱敏入口都必须剥离。
	require.Equal(t, "ab", SanitizeTokenAnalysisPreview("a\x00b", 10))
	require.Equal(t, "ab", RedactTokenAnalysisInputText("a\x00b"))
}

func TestTokenAnalysisSummarizeChatCompletionsSanitizesPreview(t *testing.T) {
	body := []byte(`{
		"model":"gpt-4.1",
		"messages":[
			{"role":"system","content":"You are helpful."},
			{"role":"user","content":"Bearer sk-secret-1234567890 should not be shown. Please explain caching."}
		],
		"tools":[{"type":"function","function":{"name":"lookup"}}]
	}`)

	got, err := SummarizeTokenAnalysisRequest("/v1/chat/completions", body, 300)

	require.NoError(t, err)
	require.Equal(t, "gpt-4.1", got.Model)
	require.Equal(t, 2, got.MessageCount)
	require.Equal(t, len("You are helpful."), got.SystemChars)
	require.Greater(t, got.UserChars, 20)
	require.Equal(t, 1, got.ToolsCount)
	require.NotContains(t, got.LastUserPreview, "sk-secret")
	require.Contains(t, got.LastUserPreview, "[redacted]")
}

func TestTokenAnalysisSummarizeChatCompletionsToolOutputMetrics(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","messages":[{"role":"user","content":"run"},{"role":"tool","content":"` + strings.Repeat("x", 2000) + `","tool_call_id":"call_1"}],"tools":[{"type":"function","function":{"name":"read","parameters":{}}}]}`)

	got, err := SummarizeTokenAnalysisRequest("/v1/chat/completions", body, 300)

	require.NoError(t, err)
	require.Equal(t, 2, got.MessageCount)
	require.Equal(t, 1, got.ToolMessageCount)
	require.GreaterOrEqual(t, got.ToolOutputBytes, int64(2000))
	require.GreaterOrEqual(t, got.MaxToolOutputBytes, int64(2000))
	require.Equal(t, 1, got.ToolsCount)
	require.EqualValues(t, 1, got.SummaryJSON["tool_message_count"])
}

func TestTokenAnalysisSummarizeClaudeMessages(t *testing.T) {
	body := []byte(`{
		"model":"claude-sonnet-4-5",
		"system":"Follow the project instructions.",
		"messages":[{"role":"user","content":[{"type":"text","text":"Review this large patch"}]}],
		"tools":[{"name":"read_file"}]
	}`)

	got, err := SummarizeTokenAnalysisRequest("/v1/messages", body, 120)

	require.NoError(t, err)
	require.Equal(t, "claude-sonnet-4-5", got.Model)
	require.Equal(t, 1, got.MessageCount)
	require.Equal(t, len("Follow the project instructions."), got.SystemChars)
	require.Equal(t, "Review this large patch", got.LastUserPreview)
	require.Equal(t, 1, got.ToolsCount)
}

// Codex 把工具定义放在 input 的 additional_tools 条目(namespace 内嵌叶子工具)里,
// 顶层 tools 可能缺失; tools_count 统计全部叶子工具定义。结构取自生产样本。
func TestTokenAnalysisSummarizeCountsCodexAdditionalTools(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-sol","input":[
		{"type":"additional_tools","id":"at_1","role":"developer","tools":[{"type":"namespace","name":"functions","description":"d","tools":[{"type":"custom","name":"exec","description":"d"},{"type":"function","name":"apply_patch","description":"d"}]}]},
		{"role":"user","content":[{"type":"input_text","text":"fix it"}]}
	],"tools":[{"type":"tool_search"},{"type":"namespace","name":"empty","tools":[]}]}`)
	got, err := SummarizeTokenAnalysisRequest("/v1/responses", body, 300)
	require.NoError(t, err)
	require.Equal(t, 3, got.ToolsCount, "2 leaf tools in additional_tools + top-level tool_search; empty namespace counts 0")
	require.Equal(t, 2, got.MessageCount, "additional_tools carries role=developer and counts as a message, as before")
	require.Equal(t, len("fix it"), got.UserChars)
}

func TestTokenAnalysisSummarizeGeminiContents(t *testing.T) {
	body := []byte(`{
		"contents":[
			{"role":"user","parts":[{"text":"Generate a concise answer"},{"inline_data":{"mime_type":"image/png","data":"AAAA"}}]}
		],
		"system_instruction":{"parts":[{"text":"Be concise"}]},
		"tools":[{"function_declarations":[{"name":"search"}]}]
	}`)

	got, err := SummarizeTokenAnalysisRequest("/v1beta/models/gemini-2.5-pro:generateContent", body, 120)

	require.NoError(t, err)
	require.Equal(t, 1, got.MessageCount)
	require.Equal(t, len("Be concise"), got.SystemChars)
	require.Equal(t, "Generate a concise answer", got.LastUserPreview)
	require.Equal(t, 1, got.ImageCount)
	require.Equal(t, 1, got.ToolsCount)
}

func TestTokenAnalysisSummarizeKeepsRawLastUserText(t *testing.T) {
	body := []byte(`{"model":"gpt-4.1","messages":[{"role":"user","content":"line one\nline two with sk-secret-1234567890"}]}`)

	got, err := SummarizeTokenAnalysisRequest("/v1/chat/completions", body, 20)

	require.NoError(t, err)
	// LastUserText 保留原文(未脱敏未截断), 供净输入留存自行脱敏。
	require.Equal(t, "line one\nline two with sk-secret-1234567890", got.LastUserText)
	require.NotContains(t, got.LastUserPreview, "sk-secret")
	require.LessOrEqual(t, len([]rune(got.LastUserPreview)), 20)
}

// buildTokenAnalysisMultipartImageEditBody 构造与前端 editImages 同序的
// /v1/images/edits multipart 体: 文本域在前, image[] 图片分片在后。
func buildTokenAnalysisMultipartImageEditBody(t *testing.T, imageSizes ...int) (string, []byte) {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	require.NoError(t, writer.WriteField("model", "gpt-image-2"))
	require.NoError(t, writer.WriteField("prompt", "把这张图改成赛博朋克风格"))
	require.NoError(t, writer.WriteField("size", "1024x1024"))
	require.NoError(t, writer.WriteField("n", "2"))
	require.NoError(t, writer.WriteField("response_format", "b64_json"))
	for i, size := range imageSizes {
		part, err := writer.CreateFormFile("image[]", fmt.Sprintf("src-%d.png", i))
		require.NoError(t, err)
		// 非法 UTF-8 字节模拟图片二进制(归档转码后会被 U+FFFD 替换)。
		_, err = part.Write(bytes.Repeat([]byte{0xff, 0xd8, 0xab}, size/3+1)[:size])
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	return writer.FormDataContentType(), buf.Bytes()
}

func TestTokenAnalysisSummarizeMultipartImageEdit(t *testing.T) {
	contentType, body := buildTokenAnalysisMultipartImageEditBody(t, 64, 64)

	got, ok := SummarizeTokenAnalysisMultipartRequest("/v1/images/edits", contentType, body, 300)

	require.True(t, ok)
	require.Equal(t, "gpt-image-2", got.Model)
	require.Equal(t, 1, got.MessageCount)
	// ImageCount 与 JSON image 形态同口径: 取请求张数 n。
	require.Equal(t, 2, got.ImageCount)
	require.Equal(t, "把这张图改成赛博朋克风格", got.LastUserPreview)
	require.Equal(t, "image", got.SummaryJSON["shape"])
	require.Equal(t, true, got.SummaryJSON["multipart"])
	require.Equal(t, 2, got.SummaryJSON["source_image_count"])
	require.Equal(t, "1024x1024", got.SummaryJSON["size"])
	require.Equal(t, "/v1/images/edits", got.SummaryJSON["endpoint"])
}

func TestTokenAnalysisSummarizeMultipartTruncatedStillExtractsTextFields(t *testing.T) {
	contentType, body := buildTokenAnalysisMultipartImageEditBody(t, 4096)
	// 模拟归档端按 max_request_body_bytes 截断: 切点落在图片分片中间,
	// 文本域在前仍完整可解(前端构造顺序保证)。
	truncated := body[:len(body)-2048]

	got, ok := SummarizeTokenAnalysisMultipartRequest("/v1/images/edits", contentType, truncated, 300)

	require.True(t, ok)
	require.Equal(t, "gpt-image-2", got.Model)
	require.Contains(t, got.LastUserPreview, "赛博朋克")
	require.Equal(t, 2, got.ImageCount)
	require.Equal(t, 1, got.SummaryJSON["source_image_count"])
}

func TestTokenAnalysisSummarizeMultipartBoundaryFallbackFromBody(t *testing.T) {
	_, body := buildTokenAnalysisMultipartImageEditBody(t, 64)

	// 缺 content_type 的历史归档行: 从 body 首行 "--boundary" 兜底提取。
	got, ok := SummarizeTokenAnalysisMultipartRequest("/v1/images/edits", "", body, 300)

	require.True(t, ok)
	require.Equal(t, "gpt-image-2", got.Model)
	require.Equal(t, 1, got.SummaryJSON["source_image_count"])
}

func TestTokenAnalysisSummarizeMultipartRejectsNonMultipart(t *testing.T) {
	_, ok := SummarizeTokenAnalysisMultipartRequest("/v1/responses", "application/json", []byte(`{"input":"hi"}`), 300)
	require.False(t, ok)

	_, ok = SummarizeTokenAnalysisMultipartRequest("/v1/responses", "", []byte("not-json"), 300)
	require.False(t, ok)
}

func TestTokenAnalysisSummarizeMultipartGarbageDegrades(t *testing.T) {
	// boundary 能识别但分片解不出: 降级摘要而非失败, 失败行只留给真坏数据。
	got, ok := SummarizeTokenAnalysisMultipartRequest("/v1/images/edits", "multipart/form-data; boundary=xyz", []byte("--xyz\r\ngarbage-without-headers"), 300)

	require.True(t, ok)
	require.Equal(t, "multipart_body", got.SummaryJSON["degraded"])
}

func TestSanitizeTokenAnalysisInputTextPreservesFormatting(t *testing.T) {
	text := "第一行\n  indented code\n\nBearer sk-secret-1234567890"

	out, truncated := SanitizeTokenAnalysisInputText(text, 8000)

	require.False(t, truncated)
	// 与预览不同, 保留换行与缩进。
	require.Contains(t, out, "第一行\n  indented code\n\n")
	require.NotContains(t, out, "sk-secret")
	require.Contains(t, out, "[redacted]")

	short, shortTruncated := SanitizeTokenAnalysisInputText("abcdef", 3)
	require.True(t, shortTruncated)
	require.Equal(t, "abc", short)
}

// 预览/净输入只取用户本人写的文本: 工具结果、注入包装、assistant 文本都不覆盖它,
// 同一 agent 轮次内的工具续执行请求共享发起该轮的用户输入。
func TestTokenAnalysisSummarizePreviewUsesUserAuthoredText(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		body     string
	}{
		{"responses trailing assistant", "/v1/responses", `{"input":[{"role":"user","content":[{"type":"input_text","text":"fix the login bug"}]},{"role":"assistant","content":[{"type":"output_text","text":"I will read the file"}]}]}`},
		{"responses codex tool loop", "/v1/responses", `{"input":[{"role":"user","content":[{"type":"input_text","text":"<environment_context><cwd>/w</cwd></environment_context>"}]},{"role":"user","content":[{"type":"input_text","text":"fix the login bug"}]},{"type":"function_call","name":"shell","arguments":"{}"},{"type":"function_call_output","output":"tool output"},{"type":"agent_message","content":"child said hi"}]}`},
		{"claude tool loop", "/v1/messages", `{"messages":[{"role":"user","content":[{"type":"text","text":"<system-reminder>ctx</system-reminder>"},{"type":"text","text":"fix the login bug"}]},{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Read","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"tool output"},{"type":"text","text":"<system-reminder>file changed</system-reminder>"}]}]}`},
		{"claude reminder-only message after assistant", "/v1/messages", `{"messages":[{"role":"user","content":[{"type":"text","text":"<system-reminder>ctx</system-reminder>"},{"type":"text","text":"fix the login bug"}]},{"role":"assistant","content":[{"type":"text","text":"done"}]},{"role":"user","content":[{"type":"text","text":"<system-reminder>background notice</system-reminder>"}]}]}`},
		{"chat reminder string after prompt", "/v1/chat/completions", `{"messages":[{"role":"user","content":"fix the login bug"},{"role":"assistant","content":"ok"},{"role":"user","content":"<system-reminder>nudge</system-reminder>"}]}`},
		{"gemini function response", "/v1beta/models/gemini-2.5-pro:generateContent", `{"contents":[{"role":"user","parts":[{"text":"fix the login bug"}]},{"role":"model","parts":[{"functionCall":{"name":"read"}}]},{"role":"user","parts":[{"functionResponse":{"name":"read","response":{"text":"tool output"}}}]}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SummarizeTokenAnalysisRequest(tt.endpoint, []byte(tt.body), 300)
			require.NoError(t, err)
			require.Equal(t, "fix the login bug", got.LastUserPreview)
			require.Equal(t, "fix the login bug", got.LastUserText)
		})
	}
}

// 只剥离文本开头完整闭合的客户端注入包装(及其后 Codex 的 "## My request:"
// 标题); 用户引用的标签、未闭合包装、正文之后的包装都保留原文。
func TestTokenAnalysisSummarizeStripsLeadingClientContext(t *testing.T) {
	const ambient = "\n<in-app-browser-context source=\"ambient-ui-state\">\n- Current URL: http://example/admin\n</in-app-browser-context>\n\n## My request:\n"
	tests := []struct {
		name string
		text string
		want string
	}{
		{"ambient context then request", ambient + "为什么看不到?\n", "为什么看不到?"},
		{"stacked wrappers", "<environment_context><cwd>/w</cwd></environment_context>\n" + ambient + "fix it", "fix it"},
		{"environment then instruction", "<environment_context>cwd</environment_context> now edit the file", "now edit the file"},
		{"heading without wrapper is user text", "## My request:\nfix it", "## My request:\nfix it"},
		{"quoted tag mid text", "为什么预览里有 <in-app-browser-context>x</in-app-browser-context>?", "为什么预览里有 <in-app-browser-context>x</in-app-browser-context>?"},
		{"unclosed wrapper", "<in-app-browser-context source=\"ambient-ui-state\">\nfix it", "<in-app-browser-context source=\"ambient-ui-state\">\nfix it"},
		{"different tag sharing the prefix", "<in-app-browser-contextual>x</in-app-browser-contextual> fix it", "<in-app-browser-contextual>x</in-app-browser-contextual> fix it"},
		{"wrapper after user text", "fix it\n<system-reminder>x</system-reminder>", "fix it\n<system-reminder>x</system-reminder>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := json.Marshal(tt.text)
			require.NoError(t, err)
			body := `{"input":[{"role":"user","content":[{"type":"input_text","text":` + string(raw) + `}]}]}`
			got, err := SummarizeTokenAnalysisRequest("/v1/responses", []byte(body), 300)
			require.NoError(t, err)
			require.Equal(t, tt.want, got.LastUserText)
			require.Equal(t, "user_input", got.SummaryJSON["request_origin"].(tokenAnalysisRequestOrigin).Reason)
		})
	}

	// 整块注入的用户消息不算新输入, 预览保留更早的用户正文。
	got, err := SummarizeTokenAnalysisRequest("/v1/responses", []byte(`{"input":[{"role":"user","content":"fix it"},{"role":"assistant","content":"ok"},{"role":"user","content":"<in-app-browser-context>url</in-app-browser-context>"}]}`), 300)
	require.NoError(t, err)
	require.Equal(t, "fix it", got.LastUserPreview)
}

// 各协议的工具输出计入工具字段(字节为输出值的 JSON 编码长度), UserChars 只计
// 用户角色的非工具文本; assistant 文本、函数调用、reasoning 不计入。
func TestTokenAnalysisSummarizeToolOutputsAcrossProtocols(t *testing.T) {
	tests := []struct {
		name          string
		endpoint      string
		body          string
		userChars     int
		toolMessages  int
		toolBytes     int64
		maxToolBytes  int64
		messageCount  int
		systemChars   int
		lastUserInput string
	}{
		{
			name:      "anthropic tool_result is not user text",
			endpoint:  "/v1/messages",
			body:      `{"messages":[{"role":"user","content":"hi"},{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Read","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"0123456789"},{"type":"text","text":"more"}]}]}`,
			userChars: len("hi") + len("more"), toolMessages: 1, toolBytes: int64(len(`"0123456789"`)), maxToolBytes: int64(len(`"0123456789"`)),
			messageCount: 3, lastUserInput: "more",
		},
		{
			name:      "responses tool outputs and non-user items",
			endpoint:  "/v1/responses",
			body:      `{"instructions":"sys","input":[{"role":"user","content":[{"type":"input_text","text":"fix it"}]},{"type":"reasoning","summary":[{"type":"summary_text","text":"thinking"}],"content":[{"type":"reasoning_text","text":"hidden"}]},{"role":"assistant","content":[{"type":"output_text","text":"I will read"}]},{"type":"function_call","name":"shell","arguments":"{\"cmd\":\"ls\"}"},{"type":"function_call_output","call_id":"c1","output":"abcdef"},{"type":"custom_tool_call_output","call_id":"c2","output":"` + strings.Repeat("y", 100) + `"},{"type":"agent_message","content":"child said hi"}]}`,
			userChars: len("fix it"), toolMessages: 2, toolBytes: int64(len(`"abcdef"`) + 102), maxToolBytes: 102,
			messageCount: 2, systemChars: len("sys"), lastUserInput: "fix it",
		},
		{
			// 真实协议结构: tool_search_output 的发现结果在 tools, 没有 output。
			name:      "responses tool_search_output carries tools",
			endpoint:  "/v1/responses",
			body:      `{"input":[{"role":"user","content":"find tools"},{"type":"tool_search_call","call_id":"s1","arguments":{"query":"agent"},"execution":"client","status":"completed"},{"type":"tool_search_output","call_id":"s1","execution":"client","status":"completed","tools":[{"type":"namespace","name":"github","description":"` + strings.Repeat("d", 2000) + `"}]}]}`,
			userChars: len("find tools"), toolMessages: 1,
			toolBytes:    int64(len(`[{"description":"` + strings.Repeat("d", 2000) + `","name":"github","type":"namespace"}]`)),
			maxToolBytes: int64(len(`[{"description":"` + strings.Repeat("d", 2000) + `","name":"github","type":"namespace"}]`)),
			messageCount: 1, lastUserInput: "find tools",
		},
		{
			name:      "gemini functionResponse",
			endpoint:  "/v1beta/models/gemini-2.5-pro:generateContent",
			body:      `{"contents":[{"role":"user","parts":[{"text":"fix it"}]},{"role":"model","parts":[{"functionCall":{"name":"read"}}]},{"role":"user","parts":[{"functionResponse":{"name":"read","response":{"text":"tool output"}}}]}]}`,
			userChars: len("fix it"), toolMessages: 1, toolBytes: int64(len(`{"name":"read","response":{"text":"tool output"}}`)), maxToolBytes: int64(len(`{"name":"read","response":{"text":"tool output"}}`)),
			messageCount: 3, lastUserInput: "fix it",
		},
		{
			name:      "chat legacy function role",
			endpoint:  "/v1/chat/completions",
			body:      `{"messages":[{"role":"user","content":"fix it"},{"role":"function","name":"read","content":"abc"}]}`,
			userChars: len("fix it"), toolMessages: 1, toolBytes: int64(len(`"abc"`)), maxToolBytes: int64(len(`"abc"`)),
			messageCount: 2, lastUserInput: "fix it",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SummarizeTokenAnalysisRequest(tt.endpoint, []byte(tt.body), 300)
			require.NoError(t, err)
			require.Equal(t, tt.userChars, got.UserChars)
			require.Equal(t, tt.toolMessages, got.ToolMessageCount)
			require.Equal(t, tt.toolBytes, got.ToolOutputBytes)
			require.Equal(t, tt.maxToolBytes, got.MaxToolOutputBytes)
			require.Equal(t, tt.messageCount, got.MessageCount)
			require.Equal(t, tt.systemChars, got.SystemChars)
			require.Equal(t, tt.lastUserInput, got.LastUserText)
			require.EqualValues(t, tt.toolMessages, got.SummaryJSON["tool_message_count"])
		})
	}
}

// namespace 的子工具与 apicompat 适配器口径一致: 非空 tools 优先, 否则回退 children(2026-10-27 第四轮复核 F2)。
func TestTokenAnalysisToolCountNamespaceChildren(t *testing.T) {
	functions := func(n int) string {
		parts := make([]string, n)
		for i := range parts {
			parts[i] = fmt.Sprintf(`{"type":"function","name":"f%d","parameters":{"type":"object"}}`, i)
		}
		return "[" + strings.Join(parts, ",") + "]"
	}
	for name, tc := range map[string]struct {
		namespace string
		want      int
	}{
		"children only":                     {`{"type":"namespace","name":"ns","children":` + functions(10) + `}`, 10},
		"empty tools falls back":            {`{"type":"namespace","name":"ns","tools":[],"children":` + functions(10) + `}`, 10},
		"tools win over children":           {`{"type":"namespace","name":"ns","tools":` + functions(3) + `,"children":` + functions(10) + `}`, 3},
		"empty namespace":                   {`{"type":"namespace","name":"ns","tools":[]}`, 0},
		"children on non-namespace ignored": {`{"type":"function","name":"g","children":` + functions(10) + `}`, 1},
	} {
		t.Run(name, func(t *testing.T) {
			body := []byte(`{"model":"gpt-5.5","tools":[` + tc.namespace + `],"input":[{"role":"user","content":"hi"}]}`)
			got, err := SummarizeTokenAnalysisRequest("/v1/responses", body, 100)
			require.NoError(t, err)
			require.Equal(t, tc.want, got.ToolsCount)
			additional := []byte(`{"model":"gpt-5.5","input":[{"type":"additional_tools","role":"developer","tools":[` + tc.namespace + `]},{"role":"user","content":"hi"}]}`)
			got, err = SummarizeTokenAnalysisRequest("/v1/responses", additional, 100)
			require.NoError(t, err)
			require.Equal(t, tc.want, got.ToolsCount, "additional_tools uses the same leaf counting")
		})
	}
}
