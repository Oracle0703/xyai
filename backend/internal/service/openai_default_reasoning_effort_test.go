//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSupportsOpenAIReasoningEffort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		model string
		want  bool
	}{
		// 推理模型
		{"gpt-5", true},
		{"gpt-5.5", true},
		{"gpt-5-codex", true},
		{"gpt-5-high", true},
		{"GPT-5", true},
		{"openai/gpt-5.5", true},
		{"o1", true},
		{"o1-mini", true},
		{"o3-mini", true},
		{"o4-mini", true},
		// 非推理模型
		{"gpt-4o", false},
		{"gpt-4.1", false},
		{"gpt-3.5-turbo", false},
		{"claude-sonnet-4.5", false},
		{"deepseek-chat", false},
		{"", false},
		{"gpt-", false},
		{"o", false},
	}
	for _, tt := range tests {
		require.Equalf(t, tt.want, SupportsOpenAIReasoningEffort(tt.model), "model=%q", tt.model)
	}
}

func TestApplyDefaultOpenAIReasoningEffort(t *testing.T) {
	t.Parallel()

	// account=nil 时 resolveOpenAIForwardModel 直接返回 body 的 model。
	const ccBody = `{"model":"gpt-5.5","messages":[{"role":"user","content":"hi"}]}`

	t.Run("inject when absent on reasoning model", func(t *testing.T) {
		out := applyDefaultOpenAIReasoningEffort([]byte(ccBody), nil, "", "high")
		require.Equal(t, "high", gjson.GetBytes(out, "reasoning_effort").String())
	})

	t.Run("normalizes config value (max -> xhigh)", func(t *testing.T) {
		out := applyDefaultOpenAIReasoningEffort([]byte(ccBody), nil, "", "max")
		require.Equal(t, "xhigh", gjson.GetBytes(out, "reasoning_effort").String())
	})

	t.Run("disabled when config empty", func(t *testing.T) {
		out := applyDefaultOpenAIReasoningEffort([]byte(ccBody), nil, "", "")
		require.False(t, gjson.GetBytes(out, "reasoning_effort").Exists())
	})

	t.Run("config none normalizes to empty -> disabled", func(t *testing.T) {
		out := applyDefaultOpenAIReasoningEffort([]byte(ccBody), nil, "", "none")
		require.False(t, gjson.GetBytes(out, "reasoning_effort").Exists())
	})

	t.Run("explicit reasoning_effort preserved (no clobber of minimal)", func(t *testing.T) {
		body := `{"model":"gpt-5.5","messages":[{"role":"user","content":"hi"}],"reasoning_effort":"minimal"}`
		out := applyDefaultOpenAIReasoningEffort([]byte(body), nil, "", "high")
		require.Equal(t, "minimal", gjson.GetBytes(out, "reasoning_effort").String())
	})

	t.Run("explicit reasoning.effort skips injection", func(t *testing.T) {
		body := `{"model":"gpt-5.5","messages":[{"role":"user","content":"hi"}],"reasoning":{"effort":"low"}}`
		out := applyDefaultOpenAIReasoningEffort([]byte(body), nil, "", "high")
		require.False(t, gjson.GetBytes(out, "reasoning_effort").Exists())
		require.Equal(t, "low", gjson.GetBytes(out, "reasoning.effort").String())
	})

	t.Run("model name suffix counts as specified", func(t *testing.T) {
		body := `{"model":"gpt-5-high","messages":[{"role":"user","content":"hi"}]}`
		out := applyDefaultOpenAIReasoningEffort([]byte(body), nil, "", "medium")
		require.False(t, gjson.GetBytes(out, "reasoning_effort").Exists())
	})

	t.Run("non-reasoning model not injected", func(t *testing.T) {
		body := `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`
		out := applyDefaultOpenAIReasoningEffort([]byte(body), nil, "", "high")
		require.False(t, gjson.GetBytes(out, "reasoning_effort").Exists())
	})

	t.Run("responses-shape body (no messages) not injected", func(t *testing.T) {
		body := `{"model":"gpt-5.5","input":[{"type":"message","role":"user","content":"hi"}]}`
		out := applyDefaultOpenAIReasoningEffort([]byte(body), nil, "", "high")
		require.False(t, gjson.GetBytes(out, "reasoning_effort").Exists())
	})
}
