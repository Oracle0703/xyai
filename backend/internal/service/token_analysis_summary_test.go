package service

import (
	"bytes"
	"context"
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
