package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

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
