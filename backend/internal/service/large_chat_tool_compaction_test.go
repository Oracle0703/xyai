package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
)

func TestCompactLargeChatToolOutputsFixture(t *testing.T) {
	body := readLargeRequestFixture(t)
	var req apicompat.ChatCompletionsRequest
	require.NoError(t, json.Unmarshal(body, &req))

	result, err := CompactLargeChatToolOutputs(req, body, defaultLargeRequestTestConfig(), LargeRequestScope{Enabled: true})
	require.NoError(t, err)
	require.True(t, result.LargeRequestDetected)
	require.True(t, result.Compacted)
	require.Equal(t, 1, result.CompressedToolMessages)
	require.Equal(t, []int{450}, result.CompressedMessageIndices)
	require.Equal(t, int64(1922708), result.RequestBodySizeBefore)
	require.InDelta(t, int64(1348121), result.RequestBodySizeAfter, 512)
	require.Less(t, result.RequestBodySizeAfter, result.RequestBodySizeBefore)
	require.Contains(t, string(result.Request.Messages[450].Content), "[Sub2API compressed historical tool output]")
	require.Contains(t, string(result.Request.Messages[450].Content), "original_bytes: 591817")
}

func TestCompactLargeChatToolOutputsWarnModeDoesNotMutate(t *testing.T) {
	body := readLargeRequestFixture(t)
	var req apicompat.ChatCompletionsRequest
	require.NoError(t, json.Unmarshal(body, &req))
	cfg := defaultLargeRequestTestConfig()
	cfg.Mode = "warn"

	result, err := CompactLargeChatToolOutputs(req, body, cfg, LargeRequestScope{Enabled: true})
	require.NoError(t, err)
	require.True(t, result.LargeRequestDetected)
	require.False(t, result.Compacted)
	require.Equal(t, int64(len(body)), result.RequestBodySizeAfter)
	require.JSONEq(t, string(body), string(result.Body))
}

func TestCompactLargeChatToolOutputsRespectsAbsoluteRecentToolKeep(t *testing.T) {
	req := apicompat.ChatCompletionsRequest{Model: "gpt-5.5"}
	big := makeLargeToolText("absolute", 600*1024)
	for i := 0; i < 8; i++ {
		req.Messages = append(req.Messages, apicompat.ChatMessage{
			Role:       "tool",
			Content:    mustJSONRawString(t, big),
			ToolCallID: "call",
		})
	}
	body, err := json.Marshal(req)
	require.NoError(t, err)
	cfg := defaultLargeRequestTestConfig()
	cfg.AbsoluteRecentToolKeep = 6
	cfg.RecentToolKeep = 20

	result, err := CompactLargeChatToolOutputs(req, body, cfg, LargeRequestScope{Enabled: true})
	require.NoError(t, err)
	require.True(t, result.Compacted)
	require.Equal(t, []int{0, 1}, result.CompressedMessageIndices)
	for _, idx := range []int{2, 3, 4, 5, 6, 7} {
		require.NotContains(t, string(result.Request.Messages[idx].Content), "Sub2API compressed")
	}
}

func TestCompactLargeChatToolOutputsKeepsMessagesAfterLastUser(t *testing.T) {
	req := apicompat.ChatCompletionsRequest{Model: "gpt-5.5"}
	req.Messages = append(req.Messages, apicompat.ChatMessage{Role: "user", Content: mustJSONRawString(t, "before")})
	for i := 0; i < 7; i++ {
		req.Messages = append(req.Messages, apicompat.ChatMessage{
			Role:       "tool",
			Content:    mustJSONRawString(t, makeLargeToolText("old", 600*1024)),
			ToolCallID: "old",
		})
	}
	req.Messages = append(req.Messages,
		apicompat.ChatMessage{Role: "user", Content: mustJSONRawString(t, "current")},
		apicompat.ChatMessage{Role: "tool", Content: mustJSONRawString(t, makeLargeToolText("new", 600*1024)), ToolCallID: "new"},
	)
	body, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := CompactLargeChatToolOutputs(req, body, defaultLargeRequestTestConfig(), LargeRequestScope{Enabled: true})
	require.NoError(t, err)
	require.True(t, result.Compacted)
	require.NotEmpty(t, result.CompressedMessageIndices)
	for _, idx := range result.CompressedMessageIndices {
		require.Less(t, idx, len(result.Request.Messages)-2)
	}
	require.NotContains(t, string(result.Request.Messages[len(result.Request.Messages)-1].Content), "Sub2API compressed")
}

func TestLargeRequestScopeFromConfigMatchesAPIKey(t *testing.T) {
	cfg := defaultLargeRequestTestConfig()
	cfg.EnabledAPIKeyIDs = []int64{30}

	scope := LargeRequestScopeFromConfig(cfg, LargeRequestIdentity{UserID: 21, APIKeyID: 30, GroupID: 11})

	require.True(t, scope.Enabled)
}

func TestLargeRequestScopeFromConfigNoMatch(t *testing.T) {
	cfg := defaultLargeRequestTestConfig()
	cfg.EnabledUserIDs = []int64{99}

	scope := LargeRequestScopeFromConfig(cfg, LargeRequestIdentity{UserID: 21, APIKeyID: 30, GroupID: 11})

	require.False(t, scope.Enabled)
}

func TestMaybeInjectLargeRequestPromptCacheKeyDoesNotOverrideClientValue(t *testing.T) {
	req := apicompat.ChatCompletionsRequest{
		Model:          "gpt-5.5",
		PromptCacheKey: "client-key",
		Messages: []apicompat.ChatMessage{
			{Role: "user", Content: mustJSONRawString(t, "hello")},
		},
	}
	body, err := json.Marshal(req)
	require.NoError(t, err)
	result, err := CompactLargeChatToolOutputs(req, body, defaultLargeRequestTestConfig(), LargeRequestScope{Enabled: true})
	require.NoError(t, err)

	MaybeInjectLargeRequestPromptCacheKey(&result, LargeRequestIdentity{UserID: 21, APIKeyID: 30, GroupID: 11})

	require.False(t, result.PromptCacheKeyInjected)
	require.Equal(t, "client-key", result.Request.PromptCacheKey)
}

func TestMaybeInjectLargeRequestPromptCacheKeyUsesStablePrefix(t *testing.T) {
	body := readLargeRequestFixture(t)
	var req apicompat.ChatCompletionsRequest
	require.NoError(t, json.Unmarshal(body, &req))
	result, err := CompactLargeChatToolOutputs(req, body, defaultLargeRequestTestConfig(), LargeRequestScope{Enabled: true})
	require.NoError(t, err)

	MaybeInjectLargeRequestPromptCacheKey(&result, LargeRequestIdentity{UserID: 21, APIKeyID: 30, GroupID: 11})

	require.True(t, result.PromptCacheKeyInjected)
	require.NotEmpty(t, result.PromptCacheKey)
	require.Equal(t, result.PromptCacheKey, result.Request.PromptCacheKey)
	require.Contains(t, string(result.Body), `"prompt_cache_key":"`+result.PromptCacheKey+`"`)
}

func readLargeRequestFixture(t *testing.T) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "..", "fixtures", "chat-completions-large-20260525-113534-archive-2b9ad158-request.json")
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	return body
}

func defaultLargeRequestTestConfig() config.GatewayLargeRequestConfig {
	return config.GatewayLargeRequestConfig{
		Enabled:                  true,
		Mode:                     "tool_output_compact",
		BodyThresholdBytes:       1048576,
		ToolTotalThresholdBytes:  524288,
		NormalToolThresholdBytes: 131072,
		GiantToolThresholdBytes:  524288,
		RecentToolKeep:           20,
		AbsoluteRecentToolKeep:   6,
		TargetBodyBytes:          921600,
		HeadChars:                8000,
		TailChars:                8000,
		AutoPromptCacheKey:       true,
	}
}

func mustJSONRawString(t *testing.T, value string) json.RawMessage {
	t.Helper()
	body, err := json.Marshal(value)
	require.NoError(t, err)
	return body
}

func makeString(ch byte, n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = ch
	}
	return string(b)
}

func makeLargeToolText(prefix string, n int) string {
	seed := prefix + ": tool output line with spaces, punctuation, and json-like text {\"ok\":true}\n"
	out := make([]byte, 0, n+len(seed))
	for len(out) < n {
		out = append(out, seed...)
	}
	return string(out[:n])
}
