# Large Chat Tool Output Compaction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add configurable large Chat Completions request protection that observes large `role=tool` histories by default and can compact oversized historical tool outputs for selected users/API keys/groups before forwarding to OpenAI Responses.

**Architecture:** Keep compaction as a pure service-layer utility so it can be tested with fixtures without network calls. Add config defaults under `gateway.large_request`, extend Chat Completions request structs for `prompt_cache_key`, then call the utility from `OpenAIGatewayService.ForwardAsChatCompletions` before Chat Completions to Responses conversion. Token analysis gets additional summary/risk fields for large historical tool output so the admin page can explain why a request is risky even when compaction is not enabled.

**Tech Stack:** Go 1.21+, Gin, Viper config, `encoding/json`, existing `apicompat` request structs, existing token analysis service/repository, PowerShell test commands on Windows.

---

## File Map

| File | Responsibility |
| --- | --- |
| `backend/internal/config/config.go` | Add `GatewayLargeRequestConfig`, defaults, and validation for `gateway.large_request` |
| `backend/internal/config/config_test.go` | Verify large request defaults and invalid config validation |
| `backend/internal/pkg/apicompat/types.go` | Add Chat Completions passthrough fields `prompt_cache_key` and `previous_response_id` |
| `backend/internal/pkg/apicompat/chatcompletions_to_responses.go` | Preserve `prompt_cache_key` and `previous_response_id` when converting Chat Completions to Responses |
| `backend/internal/pkg/apicompat/chatcompletions_responses_test.go` | Test prompt cache key propagation |
| `backend/internal/service/large_chat_tool_compaction.go` | Pure compaction analyzer/applicator and prompt cache key derivation helper |
| `backend/internal/service/large_chat_tool_compaction_test.go` | Unit tests for fixture regression, protection rules, warn vs compact, skip reasons |
| `backend/internal/service/openai_gateway_chat_completions.go` | Invoke analyzer/compactor before conversion and log structured metrics |
| `backend/internal/service/openai_gateway_chat_completions_test.go` | Gateway-level tests verifying upstream receives compacted body only when configured |
| `backend/internal/service/token_analysis_types.go` | Add risk reason constants for large tool history and giant tool output |
| `backend/internal/service/token_analysis_summary.go` | Include `tool_message_count`, `tool_output_bytes`, and max tool output bytes in summary JSON |
| `backend/internal/service/token_analysis_summary_test.go` | Test summary extraction for large tool outputs without leaking content |
| `backend/internal/service/token_analysis_risk.go` | Score large tool histories and giant tool outputs as explicit risk reasons |
| `backend/internal/service/token_analysis_risk_test.go` | Test the new risk reasons |

## Task 1: Add Config Surface

**Files:**
- Modify: `backend/internal/config/config.go`
- Modify: `backend/internal/config/config_test.go`

- [ ] **Step 1: Write the failing defaults test**

Add this test near existing config default tests:

```go
func TestLoadDefaultGatewayLargeRequestConfig(t *testing.T) {
	cfg, err := LoadConfigForTest(t, "")
	require.NoError(t, err)

	lr := cfg.Gateway.LargeRequest
	require.True(t, lr.Enabled)
	require.Equal(t, "warn", lr.Mode)
	require.Equal(t, int64(1048576), lr.BodyThresholdBytes)
	require.Equal(t, int64(524288), lr.ToolTotalThresholdBytes)
	require.Equal(t, int64(131072), lr.NormalToolThresholdBytes)
	require.Equal(t, int64(524288), lr.GiantToolThresholdBytes)
	require.Equal(t, 20, lr.RecentToolKeep)
	require.Equal(t, 6, lr.AbsoluteRecentToolKeep)
	require.Equal(t, int64(921600), lr.TargetBodyBytes)
	require.Equal(t, 8000, lr.HeadChars)
	require.Equal(t, 8000, lr.TailChars)
	require.Empty(t, lr.EnabledUserIDs)
	require.Empty(t, lr.EnabledAPIKeyIDs)
	require.Empty(t, lr.EnabledGroupIDs)
	require.True(t, lr.AutoPromptCacheKey)
}
```

If this repo uses a different helper than `LoadConfigForTest`, use the existing helper in `config_test.go` and keep the assertions unchanged.

- [ ] **Step 2: Run the failing test**

```powershell
go test -count=1 ./internal/config -run TestLoadDefaultGatewayLargeRequestConfig
```

Expected: FAIL because `cfg.Gateway.LargeRequest` does not exist.

- [ ] **Step 3: Add config type, field, defaults, and validation**

Add this type near other gateway config structs:

```go
type GatewayLargeRequestConfig struct {
	Enabled                  bool    `mapstructure:"enabled"`
	Mode                     string  `mapstructure:"mode"`
	BodyThresholdBytes       int64   `mapstructure:"body_threshold_bytes"`
	ToolTotalThresholdBytes  int64   `mapstructure:"tool_total_threshold_bytes"`
	NormalToolThresholdBytes int64   `mapstructure:"normal_tool_threshold_bytes"`
	GiantToolThresholdBytes  int64   `mapstructure:"giant_tool_threshold_bytes"`
	RecentToolKeep           int     `mapstructure:"recent_tool_keep"`
	AbsoluteRecentToolKeep   int     `mapstructure:"absolute_recent_tool_keep"`
	TargetBodyBytes          int64   `mapstructure:"target_body_bytes"`
	HeadChars                int     `mapstructure:"head_chars"`
	TailChars                int     `mapstructure:"tail_chars"`
	EnabledUserIDs           []int64 `mapstructure:"enabled_user_ids"`
	EnabledAPIKeyIDs         []int64 `mapstructure:"enabled_api_key_ids"`
	EnabledGroupIDs          []int64 `mapstructure:"enabled_group_ids"`
	AutoPromptCacheKey       bool    `mapstructure:"auto_prompt_cache_key"`
}
```

Add this to `GatewayConfig`:

```go
LargeRequest GatewayLargeRequestConfig `mapstructure:"large_request"`
```

Add defaults:

```go
viper.SetDefault("gateway.large_request.enabled", true)
viper.SetDefault("gateway.large_request.mode", "warn")
viper.SetDefault("gateway.large_request.body_threshold_bytes", int64(1048576))
viper.SetDefault("gateway.large_request.tool_total_threshold_bytes", int64(524288))
viper.SetDefault("gateway.large_request.normal_tool_threshold_bytes", int64(131072))
viper.SetDefault("gateway.large_request.giant_tool_threshold_bytes", int64(524288))
viper.SetDefault("gateway.large_request.recent_tool_keep", 20)
viper.SetDefault("gateway.large_request.absolute_recent_tool_keep", 6)
viper.SetDefault("gateway.large_request.target_body_bytes", int64(921600))
viper.SetDefault("gateway.large_request.head_chars", 8000)
viper.SetDefault("gateway.large_request.tail_chars", 8000)
viper.SetDefault("gateway.large_request.enabled_user_ids", []int64{})
viper.SetDefault("gateway.large_request.enabled_api_key_ids", []int64{})
viper.SetDefault("gateway.large_request.enabled_group_ids", []int64{})
viper.SetDefault("gateway.large_request.auto_prompt_cache_key", true)
```

Add validation in the existing config validation function:

```go
mode := strings.TrimSpace(c.Gateway.LargeRequest.Mode)
if mode == "" {
	mode = "warn"
}
switch mode {
case "off", "warn", "tool_output_compact":
	c.Gateway.LargeRequest.Mode = mode
default:
	return fmt.Errorf("gateway.large_request.mode must be one of off, warn, tool_output_compact")
}
if c.Gateway.LargeRequest.BodyThresholdBytes <= 0 {
	return fmt.Errorf("gateway.large_request.body_threshold_bytes must be positive")
}
if c.Gateway.LargeRequest.ToolTotalThresholdBytes <= 0 {
	return fmt.Errorf("gateway.large_request.tool_total_threshold_bytes must be positive")
}
if c.Gateway.LargeRequest.NormalToolThresholdBytes <= 0 {
	return fmt.Errorf("gateway.large_request.normal_tool_threshold_bytes must be positive")
}
if c.Gateway.LargeRequest.GiantToolThresholdBytes < c.Gateway.LargeRequest.NormalToolThresholdBytes {
	return fmt.Errorf("gateway.large_request.giant_tool_threshold_bytes must be >= normal_tool_threshold_bytes")
}
if c.Gateway.LargeRequest.AbsoluteRecentToolKeep < 0 || c.Gateway.LargeRequest.RecentToolKeep < 0 {
	return fmt.Errorf("gateway.large_request recent keep values must be non-negative")
}
if c.Gateway.LargeRequest.AbsoluteRecentToolKeep > c.Gateway.LargeRequest.RecentToolKeep {
	return fmt.Errorf("gateway.large_request.absolute_recent_tool_keep must be <= recent_tool_keep")
}
if c.Gateway.LargeRequest.HeadChars <= 0 || c.Gateway.LargeRequest.TailChars <= 0 {
	return fmt.Errorf("gateway.large_request head_chars and tail_chars must be positive")
}
```

Use existing imports; add `strings` only if needed.

- [ ] **Step 4: Verify config tests**

```powershell
go test -count=1 ./internal/config -run LargeRequest
```

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add backend/internal/config/config.go backend/internal/config/config_test.go
git commit -m "feat: add large request compaction config"
```

## Task 2: Preserve Prompt Cache Key in API Compatibility Types

**Files:**
- Modify: `backend/internal/pkg/apicompat/types.go`
- Modify: `backend/internal/pkg/apicompat/chatcompletions_to_responses.go`
- Modify: `backend/internal/pkg/apicompat/chatcompletions_responses_test.go`

- [ ] **Step 1: Write the failing propagation test**

Add near existing `ChatCompletionsToResponses` tests:

```go
func TestChatCompletionsToResponses_PromptCacheKey(t *testing.T) {
	req := &ChatCompletionsRequest{
		Model:          "gpt-5.5",
		Messages:       []ChatMessage{{Role: "user", Content: json.RawMessage(`"hello"`)}},
		PromptCacheKey: "pcache_test_key",
	}

	resp, err := ChatCompletionsToResponses(req)
	require.NoError(t, err)
	require.Equal(t, "pcache_test_key", resp.PromptCacheKey)
}
```

- [ ] **Step 2: Run the failing test**

```powershell
go test -count=1 ./internal/pkg/apicompat -run TestChatCompletionsToResponses_PromptCacheKey
```

Expected: FAIL because `PromptCacheKey` is undefined or not propagated.

- [ ] **Step 3: Add fields and propagation**

In `ChatCompletionsRequest`, add:

```go
PromptCacheKey     string `json:"prompt_cache_key,omitempty"`
PreviousResponseID string `json:"previous_response_id,omitempty"`
```

In `ChatCompletionsToResponses`, after creating `out`, add:

```go
out.PromptCacheKey = strings.TrimSpace(req.PromptCacheKey)
out.PreviousResponseID = strings.TrimSpace(req.PreviousResponseID)
```

- [ ] **Step 4: Verify apicompat tests**

```powershell
go test -count=1 ./internal/pkg/apicompat
```

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add backend/internal/pkg/apicompat/types.go backend/internal/pkg/apicompat/chatcompletions_to_responses.go backend/internal/pkg/apicompat/chatcompletions_responses_test.go
git commit -m "feat: preserve chat prompt cache key"
```

## Task 3: Build Pure Large Tool Output Compactor

**Files:**
- Create: `backend/internal/service/large_chat_tool_compaction.go`
- Create: `backend/internal/service/large_chat_tool_compaction_test.go`

- [ ] **Step 1: Write fixture and protection-rule tests**

Create `large_chat_tool_compaction_test.go` with these cases:

```go
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
	require.Equal(t, int64(1348121), result.RequestBodySizeAfter)
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
	big := makeString('x', 600*1024)
	for i := 0; i < 8; i++ {
		req.Messages = append(req.Messages, apicompat.ChatMessage{Role: "tool", Content: mustJSONRawString(t, big), ToolCallID: "call"})
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
}

func TestCompactLargeChatToolOutputsKeepsMessagesAfterLastUser(t *testing.T) {
	req := apicompat.ChatCompletionsRequest{Model: "gpt-5.5", Messages: []apicompat.ChatMessage{
		{Role: "user", Content: mustJSONRawString(t, "before")},
		{Role: "tool", Content: mustJSONRawString(t, makeString('a', 600*1024)), ToolCallID: "old"},
		{Role: "user", Content: mustJSONRawString(t, "current")},
		{Role: "tool", Content: mustJSONRawString(t, makeString('b', 600*1024)), ToolCallID: "new"},
	}}
	body, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := CompactLargeChatToolOutputs(req, body, defaultLargeRequestTestConfig(), LargeRequestScope{Enabled: true})
	require.NoError(t, err)
	require.True(t, result.Compacted)
	require.Equal(t, []int{1}, result.CompressedMessageIndices)
	require.NotContains(t, string(result.Request.Messages[3].Content), "Sub2API compressed")
}
```

Add helpers:

```go
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
```

- [ ] **Step 2: Run tests and verify failure**

```powershell
$tmp = Join-Path $env:TEMP ("xyai-gotmp-" + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Force $tmp | Out-Null
$env:GOTMPDIR=$tmp
go test -count=1 ./internal/service -run CompactLargeChatToolOutputs
Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
```

Expected: FAIL because compactor types/functions are undefined.

- [ ] **Step 3: Implement the pure compactor**

Create `large_chat_tool_compaction.go` with:

```go
type LargeRequestScope struct {
	Enabled bool
}

type LargeChatToolCompactionResult struct {
	Request                  apicompat.ChatCompletionsRequest
	Body                     []byte
	LargeRequestDetected     bool
	Compacted                bool
	Mode                     string
	ToolOutputBytes          int64
	ToolMessageCount         int
	CompressedToolMessages   int
	CompressedMessageIndices []int
	CompressedOriginalBytes  int64
	CompressedFinalBytes     int64
	RequestBodySizeBefore    int64
	RequestBodySizeAfter     int64
	PromptCacheKeyInjected   bool
	PromptCacheKey           string
	SkipReasons              map[string]int
}
```

Implement `CompactLargeChatToolOutputs(req, originalBody, cfg, scope)` with the design rules:

| Rule | Implementation detail |
| --- | --- |
| detect large request | `len(originalBody) > BodyThresholdBytes` and total tool content bytes > `ToolTotalThresholdBytes` |
| warn mode | set `LargeRequestDetected`, return original request/body |
| absolute protection | last `AbsoluteRecentToolKeep` tool messages and messages after last user are never compacted |
| normal protection | last `RecentToolKeep` tool messages are skipped unless single message exceeds `GiantToolThresholdBytes` |
| normal candidate | tool content string over `NormalToolThresholdBytes` |
| replacement | deterministic head/tail text with sha256, original bytes, omitted chars |
| stop condition | stop once body size is at or below `TargetBodyBytes` |

Use `json.Marshal(req)` after each replacement so tests compare actual forwarded JSON size.

- [ ] **Step 4: Verify compactor tests**

```powershell
$tmp = Join-Path $env:TEMP ("xyai-gotmp-" + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Force $tmp | Out-Null
$env:GOTMPDIR=$tmp
go test -count=1 ./internal/service -run CompactLargeChatToolOutputs
Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
```

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add backend/internal/service/large_chat_tool_compaction.go backend/internal/service/large_chat_tool_compaction_test.go
git commit -m "feat: compact large chat tool outputs"
```

## Task 4: Add Scope Gating and Prompt Cache Key Injection

**Files:**
- Modify: `backend/internal/service/large_chat_tool_compaction.go`
- Modify: `backend/internal/service/large_chat_tool_compaction_test.go`

- [ ] **Step 1: Write failing scope and prompt-cache tests**

Add:

```go
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
		Messages:       []apicompat.ChatMessage{{Role: "user", Content: mustJSONRawString(t, "hello")}},
	}
	body, err := json.Marshal(req)
	require.NoError(t, err)
	result, err := CompactLargeChatToolOutputs(req, body, defaultLargeRequestTestConfig(), LargeRequestScope{Enabled: true})
	require.NoError(t, err)
	MaybeInjectLargeRequestPromptCacheKey(&result, LargeRequestIdentity{UserID: 21, APIKeyID: 30, GroupID: 11})
	require.False(t, result.PromptCacheKeyInjected)
	require.Equal(t, "client-key", result.Request.PromptCacheKey)
}
```

- [ ] **Step 2: Run tests and verify failure**

```powershell
go test -count=1 ./internal/service -run "TestLargeRequestScopeFromConfig|TestMaybeInjectLargeRequestPromptCacheKey"
```

Expected: FAIL because helpers are undefined.

- [ ] **Step 3: Implement scope and prompt cache helpers**

Add:

```go
type LargeRequestIdentity struct {
	UserID   int64
	APIKeyID int64
	GroupID  int64
}

func LargeRequestScopeFromConfig(cfg config.GatewayLargeRequestConfig, identity LargeRequestIdentity) LargeRequestScope {
	if !cfg.Enabled || normalizeLargeRequestMode(cfg.Mode) != "tool_output_compact" {
		return LargeRequestScope{Enabled: false}
	}
	if len(cfg.EnabledUserIDs) == 0 && len(cfg.EnabledAPIKeyIDs) == 0 && len(cfg.EnabledGroupIDs) == 0 {
		return LargeRequestScope{Enabled: false}
	}
	return LargeRequestScope{
		Enabled: containsInt64(cfg.EnabledUserIDs, identity.UserID) ||
			containsInt64(cfg.EnabledAPIKeyIDs, identity.APIKeyID) ||
			containsInt64(cfg.EnabledGroupIDs, identity.GroupID),
	}
}
```

Implement `MaybeInjectLargeRequestPromptCacheKey(result, identity)`:

| Behavior | Detail |
| --- | --- |
| no override | if `result.Request.PromptCacheKey` is non-empty, leave it unchanged |
| only large request | inject only when `result.LargeRequestDetected` is true |
| key seed | model + user id + api key id + group id + system/developer content + tools JSON |
| key format | `sub2api-large-` + first 32 hex chars of sha256 |
| body update | marshal updated request back into `result.Body` and update `RequestBodySizeAfter` |

- [ ] **Step 4: Verify helper tests**

```powershell
go test -count=1 ./internal/service -run "TestLargeRequestScopeFromConfig|TestMaybeInjectLargeRequestPromptCacheKey|CompactLargeChatToolOutputs"
```

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add backend/internal/service/large_chat_tool_compaction.go backend/internal/service/large_chat_tool_compaction_test.go
git commit -m "feat: gate large request compaction"
```

## Task 5: Wire Compaction Into OpenAI Chat Completions Gateway

**Files:**
- Modify: `backend/internal/service/openai_gateway_chat_completions.go`
- Modify: `backend/internal/service/openai_gateway_chat_completions_test.go`

- [ ] **Step 1: Write failing gateway test**

Use existing service test helpers where possible. The test must configure `gateway.large_request.mode=tool_output_compact` and `enabled_api_key_ids=[30]`, forward the fixture body, and assert the upstream body contains the compaction marker and is smaller than the original:

```go
func TestForwardAsChatCompletionsCompactsLargeToolOutputForEnabledAPIKey(t *testing.T) {
	body := readLargeRequestFixture(t)
	service, upstream := newOpenAIChatCompletionsTestService(t, func(cfg *config.Config) {
		cfg.Gateway.LargeRequest.Enabled = true
		cfg.Gateway.LargeRequest.Mode = "tool_output_compact"
		cfg.Gateway.LargeRequest.EnabledAPIKeyIDs = []int64{30}
	})
	account := openAIChatCompletionsOAuthTestAccount()
	account.ID = 10

	c := newGinTestContextWithAPIKeyID(t, 30)
	_, err := service.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
	require.NoError(t, err)
	require.Contains(t, string(upstream.LastBody()), "Sub2API compressed historical tool output")
	require.Less(t, len(upstream.LastBody()), len(body))
}
```

Use the real helper names present in the file; do not duplicate a second harness if one already exists.

- [ ] **Step 2: Run test and verify failure**

```powershell
go test -count=1 ./internal/service -run TestForwardAsChatCompletionsCompactsLargeToolOutputForEnabledAPIKey
```

Expected: FAIL because gateway is not wired.

- [ ] **Step 3: Invoke compaction before conversion**

In `ForwardAsChatCompletions`, after parsing `chatReq` and after model resolution, add flow equivalent to:

```go
largeRequestResult := LargeChatToolCompactionResult{
	Request:               chatReq,
	Body:                  body,
	RequestBodySizeBefore: int64(len(body)),
	RequestBodySizeAfter:  int64(len(body)),
}
if s.cfg != nil {
	identity := LargeRequestIdentity{
		UserID:   ctxInt64Value(c, "user_id"),
		APIKeyID: ctxInt64Value(c, "api_key_id"),
		GroupID:  ctxInt64Value(c, "group_id"),
	}
	scope := LargeRequestScopeFromConfig(s.cfg.Gateway.LargeRequest, identity)
	largeRequestResult, err = CompactLargeChatToolOutputs(chatReq, body, s.cfg.Gateway.LargeRequest, scope)
	if err != nil {
		logger.L().Warn("openai chat_completions: large request compaction failed", zap.Error(err))
	} else {
		if s.cfg.Gateway.LargeRequest.AutoPromptCacheKey {
			MaybeInjectLargeRequestPromptCacheKey(&largeRequestResult, identity)
			if largeRequestResult.PromptCacheKeyInjected && strings.TrimSpace(promptCacheKey) == "" {
				promptCacheKey = largeRequestResult.PromptCacheKey
			}
		}
		chatReq = largeRequestResult.Request
		body = largeRequestResult.Body
		logLargeChatToolCompactionResult(account, largeRequestResult)
	}
}
```

Add a local `ctxInt64Value` helper if no equivalent exists. Add `logLargeChatToolCompactionResult` that logs:

```go
zap.String("mode", result.Mode)
zap.Int64("request_body_size_before", result.RequestBodySizeBefore)
zap.Int64("request_body_size_after", result.RequestBodySizeAfter)
zap.Int64("tool_output_bytes", result.ToolOutputBytes)
zap.Int("tool_message_count", result.ToolMessageCount)
zap.Bool("compacted", result.Compacted)
zap.Int("compressed_tool_messages", result.CompressedToolMessages)
zap.Int64("compressed_original_bytes", result.CompressedOriginalBytes)
zap.Int64("compressed_final_bytes", result.CompressedFinalBytes)
zap.Bool("prompt_cache_key_injected", result.PromptCacheKeyInjected)
```

Make sure `handleChatStreamingResponse(..., len(body))` receives the compacted body length.

- [ ] **Step 4: Verify gateway tests**

```powershell
$tmp = Join-Path $env:TEMP ("xyai-gotmp-" + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Force $tmp | Out-Null
$env:GOTMPDIR=$tmp
go test -count=1 ./internal/service -run "TestForwardAsChatCompletionsCompactsLargeToolOutputForEnabledAPIKey|CompactLargeChatToolOutputs|SilentRefusal"
Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
```

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add backend/internal/service/openai_gateway_chat_completions.go backend/internal/service/openai_gateway_chat_completions_test.go
git commit -m "feat: apply large tool compaction in chat gateway"
```

## Task 6: Add Token Analysis Observability

**Files:**
- Modify: `backend/internal/service/token_analysis_types.go`
- Modify: `backend/internal/service/token_analysis_summary.go`
- Modify: `backend/internal/service/token_analysis_summary_test.go`
- Modify: `backend/internal/service/token_analysis_risk.go`
- Modify: `backend/internal/service/token_analysis_risk_test.go`

- [ ] **Step 1: Write failing summary test**

Add:

```go
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
```

- [ ] **Step 2: Run summary test and verify failure**

```powershell
go test -count=1 ./internal/service -run TestTokenAnalysisSummarizeChatCompletionsToolOutputMetrics
```

Expected: FAIL because new fields do not exist.

- [ ] **Step 3: Add summary fields and extraction**

In `TokenAnalysisBodySummary`, add:

```go
ToolMessageCount   int   `json:"tool_message_count"`
ToolOutputBytes    int64 `json:"tool_output_bytes"`
MaxToolOutputBytes int64 `json:"max_tool_output_bytes"`
```

In `summaryMessagesRequest`, for role `tool`, add safe byte accounting:

```go
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
```

In `SummarizeTokenAnalysisRequest`, add:

```go
summary.SummaryJSON["tool_message_count"] = summary.ToolMessageCount
summary.SummaryJSON["tool_output_bytes"] = summary.ToolOutputBytes
summary.SummaryJSON["max_tool_output_bytes"] = summary.MaxToolOutputBytes
```

- [ ] **Step 4: Add risk tests and implementation**

Add constants:

```go
TokenAnalysisRiskLargeToolHistory = "large_tool_history"
TokenAnalysisRiskGiantToolOutput  = "giant_tool_output"
```

Add test:

```go
func TestTokenAnalysisRiskLargeToolHistory(t *testing.T) {
	summary := TokenAnalysisBodySummary{ToolMessageCount: 40, ToolOutputBytes: 800000, MaxToolOutputBytes: 600000}
	score, reasons := ScoreTokenAnalysisRisk(summary, TokenAnalysisUsageSignals{}, TokenAnalysisDuplicateSignals{})
	require.Greater(t, score, 0)
	requireRiskReason(t, reasons, TokenAnalysisRiskLargeToolHistory)
	requireRiskReason(t, reasons, TokenAnalysisRiskGiantToolOutput)
}
```

In `ScoreTokenAnalysisRisk`, add:

```go
if summary.ToolOutputBytes >= 524288 && summary.ToolMessageCount >= 10 {
	add(TokenAnalysisRiskLargeToolHistory, "历史 tool 输出体积过大", 25, map[string]any{
		"tool_message_count": summary.ToolMessageCount,
		"tool_output_bytes":  summary.ToolOutputBytes,
	})
}
if summary.MaxToolOutputBytes >= 524288 {
	add(TokenAnalysisRiskGiantToolOutput, "存在单条巨型 tool 输出", 25, map[string]any{
		"max_tool_output_bytes": summary.MaxToolOutputBytes,
	})
}
```

- [ ] **Step 5: Verify token analysis tests**

```powershell
go test -count=1 ./internal/service -run "TokenAnalysis|TestTokenAnalysisRiskLargeToolHistory"
```

Expected: PASS.

- [ ] **Step 6: Commit**

```powershell
git add backend/internal/service/token_analysis_types.go backend/internal/service/token_analysis_summary.go backend/internal/service/token_analysis_summary_test.go backend/internal/service/token_analysis_risk.go backend/internal/service/token_analysis_risk_test.go
git commit -m "feat: surface large tool output risks"
```

## Task 7: Full Verification

**Files:**
- No source edits expected unless verification fails.

- [ ] **Step 1: Run focused backend tests**

```powershell
$tmp = Join-Path $env:TEMP ("xyai-gotmp-" + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Force $tmp | Out-Null
$env:GOTMPDIR=$tmp
go test -count=1 ./internal/config ./internal/pkg/apicompat ./internal/service -run "LargeRequest|CompactLargeChatToolOutputs|PromptCacheKey|TokenAnalysis|SilentRefusal"
Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
```

Expected: PASS.

- [ ] **Step 2: Run broader backend tests**

```powershell
$tmp = Join-Path $env:TEMP ("xyai-gotmp-" + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Force $tmp | Out-Null
$env:GOTMPDIR=$tmp
go test -count=1 ./cmd/server ./internal/pkg/apicompat ./internal/service ./internal/handler ./internal/handler/admin
Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
```

Expected: PASS.

- [ ] **Step 3: Run diff hygiene**

```powershell
git diff --check
```

Expected: no output.

- [ ] **Step 4: Review git status**

```powershell
git status --short
```

Expected: only intended tracked source/test changes are present. Existing untracked `fixtures/`, `report/`, and runtime report files remain untracked unless explicitly added later.
