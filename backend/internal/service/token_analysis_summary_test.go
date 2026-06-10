package service

import (
	"context"
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
