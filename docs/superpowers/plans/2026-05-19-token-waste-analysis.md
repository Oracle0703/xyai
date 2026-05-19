# Token Waste Analysis Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build an admin Token Analysis page that shows department usage, user/API key rankings, suspicious token waste requests, and sanitized request parameter summaries without exposing full response bodies.

**Architecture:** Add a database-backed request summary index built from `data/request-archive/*.jsonl`, then join that indexed summary with `usage_logs` for token/cache/cost analysis. Keep the indexing path asynchronous and admin-triggerable so gateway request handling is not changed. Add dedicated admin APIs and a compact Vue admin page under `/admin/token-analysis`.

**Tech Stack:** Go 1.26, Gin, PostgreSQL SQL migrations, repository/service/handler layers, Vue 3, TypeScript, Vitest, existing Tailwind-style admin components.

---

## File Structure

| Area | File | Responsibility |
|---|---|---|
| Migration | `backend/migrations/136_token_analysis_request_summaries.sql` | Create request summary and index status tables, indexes, uniqueness |
| Config | `backend/internal/config/config.go` | Add `token_analysis` config defaults for indexing batch size and preview length |
| Service types | `backend/internal/service/token_analysis_types.go` | Public DTOs, filters, repository interface, risk reason constants |
| Summary extraction | `backend/internal/service/token_analysis_summary.go` | Parse archive request bodies, extract sanitized summaries across OpenAI/Claude/Gemini/image formats |
| Risk rules | `backend/internal/service/token_analysis_risk.go` | Deterministic risk scoring and reason generation |
| Indexer | `backend/internal/service/token_analysis_indexer.go` | Read JSONL archive files incrementally, upsert summaries, match usage logs |
| Main service | `backend/internal/service/token_analysis_service.go` | Query summary/users/requests/status and trigger indexing |
| Repository | `backend/internal/repository/token_analysis_repo.go` | Raw SQL persistence and aggregations |
| Handler | `backend/internal/handler/admin/token_analysis_handler.go` | Admin HTTP endpoints and parameter parsing |
| Routing | `backend/internal/server/routes/admin.go` | Register `/api/v1/admin/token-analysis/*` |
| DI | `backend/internal/handler/handler.go`, `backend/internal/handler/wire.go`, `backend/internal/repository/wire.go`, `backend/internal/service/wire.go`, `backend/cmd/server/wire_gen.go` | Wire service/repository/handler into the app |
| Frontend API | `frontend/src/api/admin/tokenAnalysis.ts`, `frontend/src/api/admin/index.ts` | Typed admin API client |
| Frontend view | `frontend/src/views/admin/TokenAnalysisView.vue` | Admin page layout, filters, summary cards, rankings, request table, detail drawer |
| Frontend route/nav | `frontend/src/router/index.ts`, `frontend/src/components/layout/AppSidebar.vue` | Add route and sidebar entry |
| i18n | `frontend/src/i18n/locales/zh.ts`, `frontend/src/i18n/locales/en.ts` | Add labels and messages |
| Tests | Files listed in each task | TDD coverage for extractor, scorer, repo, handler, API, and view |

---

## Data Contracts

### Database Tables

`token_analysis_request_summaries` stores one row per request archive `archive_id`.

```sql
CREATE TABLE IF NOT EXISTS token_analysis_request_summaries (
  id BIGSERIAL PRIMARY KEY,
  archive_id TEXT NOT NULL UNIQUE,
  usage_log_id BIGINT NULL REFERENCES usage_logs(id) ON DELETE SET NULL,
  match_confidence SMALLINT NOT NULL DEFAULT 0,
  event_time TIMESTAMPTZ NOT NULL,
  user_id BIGINT NULL,
  api_key_id BIGINT NULL,
  account_id BIGINT NULL,
  group_id BIGINT NULL,
  model TEXT NOT NULL DEFAULT '',
  endpoint TEXT NOT NULL DEFAULT '',
  method TEXT NOT NULL DEFAULT '',
  request_body_size BIGINT NOT NULL DEFAULT 0,
  request_body_truncated BOOLEAN NOT NULL DEFAULT FALSE,
  body_sha256 TEXT NOT NULL DEFAULT '',
  message_count INTEGER NOT NULL DEFAULT 0,
  system_chars INTEGER NOT NULL DEFAULT 0,
  user_chars INTEGER NOT NULL DEFAULT 0,
  last_user_preview TEXT NOT NULL DEFAULT '',
  tools_count INTEGER NOT NULL DEFAULT 0,
  image_count INTEGER NOT NULL DEFAULT 0,
  summary_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  risk_score INTEGER NOT NULL DEFAULT 0,
  risk_reasons JSONB NOT NULL DEFAULT '[]'::jsonb,
  indexed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  source_file TEXT NOT NULL DEFAULT '',
  source_offset BIGINT NULL
);

CREATE INDEX IF NOT EXISTS idx_token_analysis_event_time
  ON token_analysis_request_summaries (event_time DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_token_analysis_user_time
  ON token_analysis_request_summaries (user_id, event_time DESC);
CREATE INDEX IF NOT EXISTS idx_token_analysis_api_key_time
  ON token_analysis_request_summaries (api_key_id, event_time DESC);
CREATE INDEX IF NOT EXISTS idx_token_analysis_group_time
  ON token_analysis_request_summaries (group_id, event_time DESC);
CREATE INDEX IF NOT EXISTS idx_token_analysis_risk_time
  ON token_analysis_request_summaries (risk_score DESC, event_time DESC);
CREATE INDEX IF NOT EXISTS idx_token_analysis_body_hash_time
  ON token_analysis_request_summaries (body_sha256, event_time DESC);
CREATE INDEX IF NOT EXISTS idx_token_analysis_usage_log_id
  ON token_analysis_request_summaries (usage_log_id);
```

`token_analysis_index_state` stores per-file incremental progress and visible status.

```sql
CREATE TABLE IF NOT EXISTS token_analysis_index_state (
  source_file TEXT PRIMARY KEY,
  last_offset BIGINT NOT NULL DEFAULT 0,
  last_archive_id TEXT NOT NULL DEFAULT '',
  processed_rows BIGINT NOT NULL DEFAULT 0,
  failed_rows BIGINT NOT NULL DEFAULT 0,
  last_error TEXT NOT NULL DEFAULT '',
  started_at TIMESTAMPTZ NULL,
  finished_at TIMESTAMPTZ NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_token_analysis_index_state_updated
  ON token_analysis_index_state (updated_at DESC);
```

### Backend Types

Create these core types in `backend/internal/service/token_analysis_types.go`.

```go
type TokenAnalysisRiskReason struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Score   int            `json:"score"`
	Metrics map[string]any `json:"metrics,omitempty"`
}

type TokenAnalysisRequestSummary struct {
	ID                   int64                     `json:"id"`
	ArchiveID            string                    `json:"archive_id"`
	UsageLogID           *int64                    `json:"usage_log_id,omitempty"`
	MatchConfidence      int16                     `json:"match_confidence"`
	EventTime            time.Time                 `json:"event_time"`
	UserID               *int64                    `json:"user_id,omitempty"`
	APIKeyID             *int64                    `json:"api_key_id,omitempty"`
	AccountID            *int64                    `json:"account_id,omitempty"`
	GroupID              *int64                    `json:"group_id,omitempty"`
	Model                string                    `json:"model"`
	Endpoint             string                    `json:"endpoint"`
	Method               string                    `json:"method"`
	RequestBodySize      int64                     `json:"request_body_size"`
	RequestBodyTruncated bool                      `json:"request_body_truncated"`
	BodySHA256           string                    `json:"body_sha256"`
	MessageCount         int                       `json:"message_count"`
	SystemChars          int                       `json:"system_chars"`
	UserChars            int                       `json:"user_chars"`
	LastUserPreview      string                    `json:"last_user_preview"`
	ToolsCount           int                       `json:"tools_count"`
	ImageCount           int                       `json:"image_count"`
	SummaryJSON          map[string]any            `json:"summary_json"`
	RiskScore            int                       `json:"risk_score"`
	RiskReasons          []TokenAnalysisRiskReason `json:"risk_reasons"`
	IndexedAt            time.Time                 `json:"indexed_at"`
	SourceFile           string                    `json:"source_file"`
	SourceOffset         *int64                    `json:"source_offset,omitempty"`
}
```

Keep all user-visible request content in `LastUserPreview` and `SummaryJSON` capped and sanitized.

---

## Task 1: Migration And Config

**Files:**
- Create: `backend/migrations/136_token_analysis_request_summaries.sql`
- Modify: `backend/internal/config/config.go`
- Test: `backend/internal/config/config_test.go`

- [ ] **Step 1: Write the failing config test**

Add this test to `backend/internal/config/config_test.go`.

```go
func TestLoadDefaultTokenAnalysisConfig(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)
	require.True(t, cfg.TokenAnalysis.IndexEnabled)
	require.Equal(t, 1000, cfg.TokenAnalysis.IndexBatchSize)
	require.Equal(t, 300, cfg.TokenAnalysis.MaxPreviewChars)
	require.Equal(t, 300, cfg.TokenAnalysis.AutoIndexIntervalSeconds)
	require.Equal(t, 10, cfg.TokenAnalysis.UsageMatchWindowSeconds)
}
```

- [ ] **Step 2: Run the config test and verify RED**

Run:

```powershell
cd backend
go test ./internal/config -run TestLoadDefaultTokenAnalysisConfig
```

Expected: FAIL because `cfg.TokenAnalysis` does not exist.

- [ ] **Step 3: Add config type and defaults**

Add to `backend/internal/config/config.go`:

```go
type TokenAnalysisConfig struct {
	IndexEnabled              bool `mapstructure:"index_enabled"`
	IndexBatchSize            int  `mapstructure:"index_batch_size"`
	MaxPreviewChars           int  `mapstructure:"max_preview_chars"`
	AutoIndexIntervalSeconds  int  `mapstructure:"auto_index_interval_seconds"`
	UsageMatchWindowSeconds   int  `mapstructure:"usage_match_window_seconds"`
}
```

Add field to `Config`:

```go
TokenAnalysis TokenAnalysisConfig `mapstructure:"token_analysis"`
```

Add defaults near existing `viper.SetDefault` calls:

```go
viper.SetDefault("token_analysis.index_enabled", true)
viper.SetDefault("token_analysis.index_batch_size", 1000)
viper.SetDefault("token_analysis.max_preview_chars", 300)
viper.SetDefault("token_analysis.auto_index_interval_seconds", 300)
viper.SetDefault("token_analysis.usage_match_window_seconds", 10)
```

Add validation in `Validate()`:

```go
if c.TokenAnalysis.IndexBatchSize <= 0 {
	return fmt.Errorf("token_analysis.index_batch_size must be positive")
}
if c.TokenAnalysis.MaxPreviewChars <= 0 || c.TokenAnalysis.MaxPreviewChars > 2000 {
	return fmt.Errorf("token_analysis.max_preview_chars must be between 1 and 2000")
}
if c.TokenAnalysis.AutoIndexIntervalSeconds < 0 {
	return fmt.Errorf("token_analysis.auto_index_interval_seconds must be non-negative")
}
if c.TokenAnalysis.UsageMatchWindowSeconds <= 0 || c.TokenAnalysis.UsageMatchWindowSeconds > 120 {
	return fmt.Errorf("token_analysis.usage_match_window_seconds must be between 1 and 120")
}
```

- [ ] **Step 4: Add the SQL migration**

Create `backend/migrations/136_token_analysis_request_summaries.sql` using the SQL from the "Database Tables" section. Keep it idempotent with `IF NOT EXISTS`.

- [ ] **Step 5: Run config test and migration schema test**

Run:

```powershell
cd backend
go test ./internal/config -run TokenAnalysis
go test ./internal/repository -run MigrationsSchema
```

Expected: PASS.

- [ ] **Step 6: Commit**

```powershell
git add backend/internal/config/config.go backend/internal/config/config_test.go backend/migrations/136_token_analysis_request_summaries.sql
git commit -m "feat: add token analysis schema config"
```

---

## Task 2: Request Summary Extraction And Sanitization

**Files:**
- Create: `backend/internal/service/token_analysis_types.go`
- Create: `backend/internal/service/token_analysis_summary.go`
- Test: `backend/internal/service/token_analysis_summary_test.go`

- [ ] **Step 1: Write failing summary tests**

Create `backend/internal/service/token_analysis_summary_test.go`.

```go
package service

import (
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
```

- [ ] **Step 2: Run summary tests and verify RED**

Run:

```powershell
cd backend
go test ./internal/service -run TokenAnalysisSummarize
```

Expected: FAIL because `SummarizeTokenAnalysisRequest` does not exist.

- [ ] **Step 3: Implement summary types**

Create `backend/internal/service/token_analysis_types.go` with the shared structs from "Backend Types" and these additional structs:

```go
type TokenAnalysisBodySummary struct {
	Model           string         `json:"model"`
	MessageCount    int            `json:"message_count"`
	SystemChars     int            `json:"system_chars"`
	UserChars       int            `json:"user_chars"`
	LastUserPreview string         `json:"last_user_preview"`
	ToolsCount      int            `json:"tools_count"`
	ImageCount      int            `json:"image_count"`
	SummaryJSON     map[string]any `json:"summary_json"`
}

type tokenAnalysisArchiveEvent struct {
	ArchiveID     string  `json:"archive_id"`
	Event         string  `json:"event"`
	Timestamp     string  `json:"timestamp"`
	Method        string  `json:"method"`
	Path          string  `json:"path"`
	Endpoint      string  `json:"endpoint"`
	UserID        *int64  `json:"user_id"`
	APIKeyID      *int64  `json:"api_key_id"`
	GroupID       *int64  `json:"group_id"`
	AccountID     *int64  `json:"account_id"`
	Model         string  `json:"model"`
	Body          string  `json:"body"`
	BodySize      int64   `json:"body_size"`
	BodySHA256    string  `json:"body_sha256"`
	BodyTruncated bool    `json:"body_truncated"`
}
```

- [ ] **Step 4: Implement extraction and sanitization**

Create `backend/internal/service/token_analysis_summary.go` with these public functions:

```go
func SummarizeTokenAnalysisRequest(endpoint string, body []byte, maxPreviewChars int) (TokenAnalysisBodySummary, error)
func SanitizeTokenAnalysisPreview(input string, maxChars int) string
```

Extraction rules:

| Shape | Detection | Counts |
|---|---|---|
| Chat Completions | top-level `messages` array | system/developer chars from role, user chars from role, tools from `tools` or `functions` |
| Responses | top-level `input` | count text input items and extract last user text |
| Claude | top-level `messages`, optional `system` | system string or array text, user text from message content |
| Gemini | top-level `contents` | system text from `system_instruction`, images from `inline_data`, tools from `tools` |
| Image | `prompt` | preview from prompt, image count from `image`/`n` fields |

Sanitization rules:

```go
var tokenAnalysisRedactions = []*regexp.Regexp{
	regexp.MustCompile(`(?i)bearer\s+[a-z0-9._\-]{12,}`),
	regexp.MustCompile(`(?i)sk-[a-z0-9_\-]{8,}`),
	regexp.MustCompile(`(?i)(api[_-]?key|authorization|cookie)\s*[:=]\s*[^,\s]{8,}`),
	regexp.MustCompile(`[A-Za-z0-9+/]{120,}={0,2}`),
	regexp.MustCompile(`\S{240,}`),
}
```

Replace each match with `[redacted]`, collapse whitespace, and cut by rune count so multibyte text is not split.

- [ ] **Step 5: Run summary tests and verify GREEN**

Run:

```powershell
cd backend
go test ./internal/service -run TokenAnalysisSummarize
```

Expected: PASS.

- [ ] **Step 6: Commit**

```powershell
git add backend/internal/service/token_analysis_types.go backend/internal/service/token_analysis_summary.go backend/internal/service/token_analysis_summary_test.go
git commit -m "feat: summarize token analysis requests"
```

---

## Task 3: Risk Scoring Rules

**Files:**
- Modify: `backend/internal/service/token_analysis_types.go`
- Create: `backend/internal/service/token_analysis_risk.go`
- Test: `backend/internal/service/token_analysis_risk_test.go`

- [ ] **Step 1: Write failing risk tests**

Create `backend/internal/service/token_analysis_risk_test.go`.

```go
package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTokenAnalysisRiskHugeInputTinyOutput(t *testing.T) {
	usage := TokenAnalysisUsageSignals{
		InputTokens: 220000,
		OutputTokens: 32,
		CacheReadTokens: 0,
		CacheCreationTokens: 0,
		TotalCost: 1.5,
	}
	summary := TokenAnalysisBodySummary{MessageCount: 3, UserChars: 20000}

	score, reasons := ScoreTokenAnalysisRisk(summary, usage, TokenAnalysisDuplicateSignals{})

	require.GreaterOrEqual(t, score, 40)
	requireRiskReason(t, reasons, TokenAnalysisRiskHugeInputTinyOutput)
}

func TestTokenAnalysisRiskRepeatUncachedBody(t *testing.T) {
	usage := TokenAnalysisUsageSignals{
		InputTokens: 90000,
		OutputTokens: 900,
		CacheReadTokens: 0,
		CacheCreationTokens: 100,
	}
	summary := TokenAnalysisBodySummary{MessageCount: 2, UserChars: 10000}
	dupe := TokenAnalysisDuplicateSignals{SameBodyRecentCount: 4}

	score, reasons := ScoreTokenAnalysisRisk(summary, usage, dupe)

	require.GreaterOrEqual(t, score, 30)
	requireRiskReason(t, reasons, TokenAnalysisRiskRepeatUncachedBody)
}

func TestTokenAnalysisRiskCapsAt100(t *testing.T) {
	usage := TokenAnalysisUsageSignals{InputTokens: 500000, OutputTokens: 1, CacheReadTokens: 0}
	summary := TokenAnalysisBodySummary{SystemChars: 60000, UserChars: 100000, ToolsCount: 25}
	dupe := TokenAnalysisDuplicateSignals{SameBodyRecentCount: 20}

	score, _ := ScoreTokenAnalysisRisk(summary, usage, dupe)

	require.Equal(t, 100, score)
}

func requireRiskReason(t *testing.T, reasons []TokenAnalysisRiskReason, code string) {
	t.Helper()
	for _, r := range reasons {
		if r.Code == code {
			return
		}
	}
	t.Fatalf("missing risk reason %s in %#v", code, reasons)
}
```

- [ ] **Step 2: Run risk tests and verify RED**

Run:

```powershell
cd backend
go test ./internal/service -run TokenAnalysisRisk
```

Expected: FAIL because risk types/functions do not exist.

- [ ] **Step 3: Add risk types and constants**

Add to `backend/internal/service/token_analysis_types.go`:

```go
const (
	TokenAnalysisRiskHugeInputTinyOutput = "huge_input_tiny_output"
	TokenAnalysisRiskRepeatUncachedBody  = "repeat_uncached_body"
	TokenAnalysisRiskLowCacheHitLargeInput = "low_cache_hit_large_input"
	TokenAnalysisRiskRapidSimilarRequests = "rapid_similar_requests"
	TokenAnalysisRiskOversizedSystemPrompt = "oversized_system_prompt"
	TokenAnalysisRiskToolHeavyShortOutput = "tool_heavy_short_output"
)

type TokenAnalysisUsageSignals struct {
	InputTokens         int
	OutputTokens        int
	CacheReadTokens     int
	CacheCreationTokens int
	TotalCost           float64
	ActualCost          float64
}

type TokenAnalysisDuplicateSignals struct {
	SameBodyRecentCount int
	SimilarRecentCount  int
}
```

- [ ] **Step 4: Implement scorer**

Create `backend/internal/service/token_analysis_risk.go`.

Rules:

| Code | Score | Condition |
|---|---:|---|
| `huge_input_tiny_output` | 45 | `input_tokens >= 100000 && output_tokens <= 200` |
| `repeat_uncached_body` | 35 | `same_body_recent_count >= 3 && input_tokens >= 20000 && cache_read_tokens < input_tokens/10` |
| `low_cache_hit_large_input` | 25 | `input_tokens >= 50000 && cache_read_tokens < input_tokens/20` |
| `rapid_similar_requests` | 20 | `similar_recent_count >= 5 && input_tokens >= 10000` |
| `oversized_system_prompt` | 20 | `system_chars >= 20000 || system_chars > user_chars` |
| `tool_heavy_short_output` | 15 | `tools_count >= 10 && input_tokens >= 20000 && output_tokens <= 500` |

Clamp score to `[0,100]`. Include metric values in every reason.

- [ ] **Step 5: Run risk tests and verify GREEN**

Run:

```powershell
cd backend
go test ./internal/service -run TokenAnalysisRisk
```

Expected: PASS.

- [ ] **Step 6: Commit**

```powershell
git add backend/internal/service/token_analysis_types.go backend/internal/service/token_analysis_risk.go backend/internal/service/token_analysis_risk_test.go
git commit -m "feat: score token waste risk"
```

---

## Task 4: Repository Persistence And Aggregations

**Files:**
- Create: `backend/internal/repository/token_analysis_repo.go`
- Modify: `backend/internal/repository/wire.go`
- Test: `backend/internal/repository/token_analysis_repo_test.go`
- Modify: `backend/internal/service/token_analysis_types.go`

- [ ] **Step 1: Define repository interface before implementation**

Add to `backend/internal/service/token_analysis_types.go`:

```go
type TokenAnalysisRepository interface {
	UpsertRequestSummary(ctx context.Context, summary *TokenAnalysisRequestSummary) error
	FindNearestUsageLog(ctx context.Context, eventTime time.Time, userID, apiKeyID *int64, model string, window time.Duration) (*TokenAnalysisUsageMatch, error)
	CountSameBodyRecent(ctx context.Context, bodySHA256 string, userID, apiKeyID *int64, eventTime time.Time, window time.Duration) (int, error)
	GetSummary(ctx context.Context, filters TokenAnalysisFilters) (*TokenAnalysisSummary, error)
	ListUserUsage(ctx context.Context, filters TokenAnalysisFilters, params pagination.PaginationParams) ([]TokenAnalysisUserUsage, *pagination.PaginationResult, error)
	ListRequests(ctx context.Context, filters TokenAnalysisFilters, params pagination.PaginationParams) ([]TokenAnalysisRequestItem, *pagination.PaginationResult, error)
	GetIndexStatus(ctx context.Context) (*TokenAnalysisIndexStatus, error)
	UpdateIndexState(ctx context.Context, state TokenAnalysisIndexState) error
}
```

Define `TokenAnalysisFilters`, `TokenAnalysisSummary`, `TokenAnalysisUserUsage`, `TokenAnalysisRequestItem`, `TokenAnalysisUsageMatch`, `TokenAnalysisIndexStatus`, and `TokenAnalysisIndexState` in the same file. Each type must use JSON field names matching the API contract in the spec.

- [ ] **Step 2: Write failing repository tests**

Create `backend/internal/repository/token_analysis_repo_test.go` with SQL mock tests:

```go
package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestTokenAnalysisRepositoryUpsertRequestSummary(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := NewTokenAnalysisRepository(nil, db)
	usageID := int64(11)
	userID := int64(22)
	now := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)

	mock.ExpectExec("INSERT INTO token_analysis_request_summaries").
		WithArgs(
			"arch-1", usageID, int16(3), now,
			userID, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			"gpt-4.1", "/v1/chat/completions", "POST",
			int64(1200), false, "abc",
			2, 10, 20, "hello", 1, 0,
			sqlmock.AnyArg(), 50, sqlmock.AnyArg(), "2026-05-19.jsonl", sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = repo.UpsertRequestSummary(context.Background(), &service.TokenAnalysisRequestSummary{
		ArchiveID: "arch-1", UsageLogID: &usageID, MatchConfidence: 3, EventTime: now,
		UserID: &userID, Model: "gpt-4.1", Endpoint: "/v1/chat/completions", Method: "POST",
		RequestBodySize: 1200, BodySHA256: "abc", MessageCount: 2, SystemChars: 10,
		UserChars: 20, LastUserPreview: "hello", ToolsCount: 1, RiskScore: 50,
		RiskReasons: []service.TokenAnalysisRiskReason{{Code: service.TokenAnalysisRiskHugeInputTinyOutput, Score: 50}},
		SummaryJSON: map[string]any{"shape": "chat"}, SourceFile: "2026-05-19.jsonl",
	})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTokenAnalysisRepositoryFindNearestUsageLog(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := NewTokenAnalysisRepository(nil, db)
	now := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)
	userID := int64(7)
	apiKeyID := int64(8)

	rows := sqlmock.NewRows([]string{
		"id", "input_tokens", "output_tokens", "cache_read_tokens", "cache_creation_tokens",
		"total_cost", "actual_cost", "created_at",
	}).AddRow(int64(99), 1000, 200, 300, 400, 0.03, 0.04, now)

	mock.ExpectQuery("SELECT id, input_tokens, output_tokens").
		WithArgs(now.Add(-10*time.Second), now.Add(10*time.Second), userID, apiKeyID, "gpt-4.1").
		WillReturnRows(rows)

	got, err := repo.FindNearestUsageLog(context.Background(), now, &userID, &apiKeyID, "gpt-4.1", 10*time.Second)

	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, int64(99), got.UsageLogID)
	require.Equal(t, int16(3), got.MatchConfidence)
	require.NoError(t, mock.ExpectationsWereMet())
}
```

- [ ] **Step 3: Run repository tests and verify RED**

Run:

```powershell
cd backend
go test ./internal/repository -run TokenAnalysisRepository
```

Expected: FAIL because repository types/functions do not exist.

- [ ] **Step 4: Implement repository**

Create `backend/internal/repository/token_analysis_repo.go`.

Implementation rules:

| Method | SQL behavior |
|---|---|
| `UpsertRequestSummary` | `INSERT ... ON CONFLICT (archive_id) DO UPDATE` all mutable summary/risk/source fields |
| `FindNearestUsageLog` | Query `usage_logs` in `[eventTime-window, eventTime+window]`, filter by user/API key/model when provided, order by `ABS(EXTRACT(EPOCH FROM (created_at - $eventTime)))`, return confidence 3 when user+api_key+model match |
| `CountSameBodyRecent` | Count same `body_sha256` in `[eventTime-30m, eventTime+30m]` for same user/API key when available |
| `GetSummary` | Aggregate joined `usage_logs` for token/cache/cost plus count of `risk_score > 0` |
| `ListUserUsage` | Group by `COALESCE(t.user_id, usage_logs.user_id)` and API key when requested, join users/API keys for email/name |
| `ListRequests` | Return rows joined to `usage_logs`, users, and API keys; support `risk_min`, `risk_reason`, `include_unmatched` |
| `GetIndexStatus` | Return last updated states and aggregate processed/failed rows |
| `UpdateIndexState` | Upsert one file state |

Use `encoding/json` for `summary_json` and `risk_reasons`, and reuse `pagination.PaginationParams` for pagination.

- [ ] **Step 5: Wire repository**

Modify `backend/internal/repository/wire.go`:

```go
NewTokenAnalysisRepository,
```

Add it near `NewUsageLogRepository` because it depends on `*sql.DB` and is usage-adjacent.

- [ ] **Step 6: Run repository tests and verify GREEN**

Run:

```powershell
cd backend
go test ./internal/repository -run TokenAnalysisRepository
```

Expected: PASS.

- [ ] **Step 7: Commit**

```powershell
git add backend/internal/service/token_analysis_types.go backend/internal/repository/token_analysis_repo.go backend/internal/repository/token_analysis_repo_test.go backend/internal/repository/wire.go
git commit -m "feat: persist token analysis summaries"
```

---

## Task 5: Indexing Service

**Files:**
- Create: `backend/internal/service/token_analysis_indexer.go`
- Create: `backend/internal/service/token_analysis_service.go`
- Modify: `backend/internal/service/wire.go`
- Test: `backend/internal/service/token_analysis_indexer_test.go`

- [ ] **Step 1: Write failing indexer tests**

Create `backend/internal/service/token_analysis_indexer_test.go`.

```go
package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestTokenAnalysisIndexerIndexesOnlyRequestEvents(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "2026-05-19.jsonl")
	err := os.WriteFile(file, []byte(
		`{"archive_id":"a1","event":"request","timestamp":"2026-05-19T01:02:03Z","method":"POST","endpoint":"/v1/chat/completions","user_id":7,"api_key_id":9,"model":"gpt-4.1","body":"{\"model\":\"gpt-4.1\",\"messages\":[{\"role\":\"user\",\"content\":\"hello\"}]}","body_size":64,"body_sha256":"hash1"}`+"\n"+
			`{"archive_id":"a1","event":"response","timestamp":"2026-05-19T01:02:04Z","status":200,"body":"{\"ok\":true}"}`+"\n"),
		0o600,
	)
	require.NoError(t, err)

	repo := &tokenAnalysisRepoStub{}
	svc := NewTokenAnalysisService(repo, &config.Config{
		Gateway: config.GatewayConfig{RequestArchive: config.GatewayRequestArchiveConfig{Dir: dir}},
		TokenAnalysis: config.TokenAnalysisConfig{
			IndexEnabled: true, IndexBatchSize: 1000, MaxPreviewChars: 300, UsageMatchWindowSeconds: 10,
		},
	})

	result, err := svc.IndexRange(context.Background(), TokenAnalysisIndexRequest{
		StartDate: "2026-05-19",
		EndDate: "2026-05-19",
	})

	require.NoError(t, err)
	require.Equal(t, int64(1), result.IndexedRows)
	require.Len(t, repo.upserts, 1)
	require.Equal(t, "a1", repo.upserts[0].ArchiveID)
	require.Equal(t, "hello", repo.upserts[0].LastUserPreview)
	require.Equal(t, int16(3), repo.upserts[0].MatchConfidence)
	require.Equal(t, int64(123), *repo.upserts[0].UsageLogID)
}
```

Implement the stub in the same test file with methods needed by `TokenAnalysisRepository`. `FindNearestUsageLog` should return `UsageLogID: 123`, `MatchConfidence: 3`, and usage signals with nonzero tokens. `CountSameBodyRecent` should return `1`.

- [ ] **Step 2: Run indexer test and verify RED**

Run:

```powershell
cd backend
go test ./internal/service -run TokenAnalysisIndexer
```

Expected: FAIL because `NewTokenAnalysisService` and `IndexRange` do not exist.

- [ ] **Step 3: Implement service constructor and query methods**

Create `backend/internal/service/token_analysis_service.go` with:

```go
type TokenAnalysisService struct {
	repo TokenAnalysisRepository
	cfg  *config.Config
	mu   sync.Mutex
}

func NewTokenAnalysisService(repo TokenAnalysisRepository, cfg *config.Config) *TokenAnalysisService
func (s *TokenAnalysisService) GetSummary(ctx context.Context, filters TokenAnalysisFilters) (*TokenAnalysisSummary, error)
func (s *TokenAnalysisService) ListUserUsage(ctx context.Context, filters TokenAnalysisFilters, params pagination.PaginationParams) ([]TokenAnalysisUserUsage, *pagination.PaginationResult, error)
func (s *TokenAnalysisService) ListRequests(ctx context.Context, filters TokenAnalysisFilters, params pagination.PaginationParams) ([]TokenAnalysisRequestItem, *pagination.PaginationResult, error)
func (s *TokenAnalysisService) GetIndexStatus(ctx context.Context) (*TokenAnalysisIndexStatus, error)
func (s *TokenAnalysisService) IndexRange(ctx context.Context, req TokenAnalysisIndexRequest) (*TokenAnalysisIndexResult, error)
```

Use `mu` to prevent concurrent manual index runs in one process. Return a conflict-style service error if an index run is already active.

- [ ] **Step 4: Implement JSONL indexing**

Create `backend/internal/service/token_analysis_indexer.go`.

Indexing rules:

| Step | Behavior |
|---|---|
| File selection | Use `Gateway.RequestArchive.Dir`, pick files from `start_date` to `end_date` inclusive |
| Read format | Read line by line with `bufio.Reader`; track byte offsets after each line |
| Event filter | Skip lines where `event != "request"` |
| Parse | Unmarshal archive event; parse RFC3339 timestamp |
| Summarize | Call `SummarizeTokenAnalysisRequest(event.Endpoint, []byte(event.Body), cfg.TokenAnalysis.MaxPreviewChars)` |
| Match usage | Call `FindNearestUsageLog` with configured match window |
| Duplicate signal | Call `CountSameBodyRecent` with 30 minute window |
| Risk | Call `ScoreTokenAnalysisRisk` |
| Upsert | Save `TokenAnalysisRequestSummary` |
| State | Call `UpdateIndexState` after each batch and at file end |

Bad JSON lines increment `FailedRows` and do not stop the file. Missing files inside the requested range count as zero rows and do not return an error.

- [ ] **Step 5: Wire service**

Modify `backend/internal/service/wire.go`:

```go
NewTokenAnalysisService,
```

Add it near `NewUsageService`.

- [ ] **Step 6: Run indexer tests and verify GREEN**

Run:

```powershell
cd backend
go test ./internal/service -run 'TokenAnalysis(Indexer|Summarize|Risk)'
```

Expected: PASS.

- [ ] **Step 7: Commit**

```powershell
git add backend/internal/service/token_analysis_indexer.go backend/internal/service/token_analysis_service.go backend/internal/service/token_analysis_indexer_test.go backend/internal/service/wire.go
git commit -m "feat: index token analysis archives"
```

---

## Task 6: Admin Handler, Routes, And Wire

**Files:**
- Create: `backend/internal/handler/admin/token_analysis_handler.go`
- Modify: `backend/internal/handler/handler.go`
- Modify: `backend/internal/handler/wire.go`
- Modify: `backend/internal/server/routes/admin.go`
- Modify: `backend/cmd/server/wire_gen.go`
- Test: `backend/internal/handler/admin/token_analysis_handler_test.go`

- [ ] **Step 1: Write failing handler tests**

Create `backend/internal/handler/admin/token_analysis_handler_test.go`.

```go
package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestTokenAnalysisHandlerSummaryParsesDateRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &tokenAnalysisServiceStub{
		summary: &service.TokenAnalysisSummary{
			TotalRequests: 3,
			TotalTokens: 1000,
			RiskyRequests: 1,
		},
	}
	h := NewTokenAnalysisHandler(svc)

	r := gin.New()
	r.GET("/summary", h.Summary)
	req := httptest.NewRequest(http.MethodGet, "/summary?start_date=2026-05-19&end_date=2026-05-19&timezone=Asia/Shanghai&risk_min=30", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, int64(30), svc.lastFilters.RiskMin)
	require.NotNil(t, svc.lastFilters.StartTime)
	require.NotNil(t, svc.lastFilters.EndTime)
	require.Contains(t, w.Body.String(), `"total_tokens":1000`)
}
```

The stub must implement the service methods called by the handler.

- [ ] **Step 2: Run handler tests and verify RED**

Run:

```powershell
cd backend
go test ./internal/handler/admin -run TokenAnalysisHandler
```

Expected: FAIL because handler does not exist.

- [ ] **Step 3: Implement handler**

Create `backend/internal/handler/admin/token_analysis_handler.go`.

Handler methods:

```go
type TokenAnalysisHandler struct {
	service tokenAnalysisService
}

func NewTokenAnalysisHandler(service *service.TokenAnalysisService) *TokenAnalysisHandler
func (h *TokenAnalysisHandler) Summary(c *gin.Context)
func (h *TokenAnalysisHandler) Users(c *gin.Context)
func (h *TokenAnalysisHandler) Requests(c *gin.Context)
func (h *TokenAnalysisHandler) TriggerIndex(c *gin.Context)
func (h *TokenAnalysisHandler) IndexStatus(c *gin.Context)
```

Parsing behavior:

| Parameter | Parse behavior |
|---|---|
| `start_date/end_date/timezone` | Use existing `timezone.ParseInUserLocation`; end date is exclusive next-day |
| numeric IDs | Reject invalid values with `response.BadRequest` |
| `risk_min` | Default 0, reject negative or above 100 |
| `include_unmatched` | Boolean, default false |
| sorting | Allow `event_time`, `risk_score`, `total_tokens`, `actual_cost`; default request sort `risk_score desc` |

Use existing `response.Success` and `response.Paginated` helpers.

- [ ] **Step 4: Register handler and routes**

Modify `backend/internal/handler/handler.go`:

```go
TokenAnalysis *admin.TokenAnalysisHandler
```

Modify `backend/internal/handler/wire.go`:

```go
tokenAnalysisHandler *admin.TokenAnalysisHandler,
```

Return field:

```go
TokenAnalysis: tokenAnalysisHandler,
```

Provider set:

```go
admin.NewTokenAnalysisHandler,
```

Modify `backend/internal/server/routes/admin.go`:

```go
registerTokenAnalysisRoutes(admin, h)
```

Add:

```go
func registerTokenAnalysisRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	tokenAnalysis := admin.Group("/token-analysis")
	{
		tokenAnalysis.GET("/summary", h.Admin.TokenAnalysis.Summary)
		tokenAnalysis.GET("/users", h.Admin.TokenAnalysis.Users)
		tokenAnalysis.GET("/requests", h.Admin.TokenAnalysis.Requests)
		tokenAnalysis.POST("/index", h.Admin.TokenAnalysis.TriggerIndex)
		tokenAnalysis.GET("/index/status", h.Admin.TokenAnalysis.IndexStatus)
	}
}
```

- [ ] **Step 5: Regenerate wire**

Run:

```powershell
cd backend
go run github.com/google/wire/cmd/wire ./cmd/server
```

Expected: `backend/cmd/server/wire_gen.go` updates successfully.

- [ ] **Step 6: Run handler tests and route compile**

Run:

```powershell
cd backend
go test ./internal/handler/admin -run TokenAnalysisHandler
go test ./internal/server/routes -run Admin
```

Expected: PASS.

- [ ] **Step 7: Commit**

```powershell
git add backend/internal/handler/admin/token_analysis_handler.go backend/internal/handler/admin/token_analysis_handler_test.go backend/internal/handler/handler.go backend/internal/handler/wire.go backend/internal/server/routes/admin.go backend/cmd/server/wire_gen.go
git commit -m "feat: expose token analysis admin api"
```

---

## Task 7: Frontend API Client

**Files:**
- Create: `frontend/src/api/admin/tokenAnalysis.ts`
- Modify: `frontend/src/api/admin/index.ts`
- Test: `frontend/src/api/__tests__/admin.tokenAnalysis.spec.ts`

- [ ] **Step 1: Write failing API tests**

Create `frontend/src/api/__tests__/admin.tokenAnalysis.spec.ts`.

```ts
import { describe, expect, it, vi, beforeEach } from 'vitest'
import tokenAnalysisAPI from '../admin/tokenAnalysis'

const get = vi.fn()
const post = vi.fn()

vi.mock('../client', () => ({
  apiClient: { get, post }
}))

describe('admin tokenAnalysis API', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
  })

  it('loads request list with filters', async () => {
    get.mockResolvedValue({ data: { items: [], total: 0, page: 1, page_size: 20 } })

    await tokenAnalysisAPI.listRequests({
      start_date: '2026-05-19',
      end_date: '2026-05-19',
      risk_min: 30,
      include_unmatched: true
    })

    expect(get).toHaveBeenCalledWith('/admin/token-analysis/requests', {
      params: {
        start_date: '2026-05-19',
        end_date: '2026-05-19',
        risk_min: 30,
        include_unmatched: true
      },
      signal: undefined
    })
  })

  it('triggers index with date range', async () => {
    post.mockResolvedValue({ data: { indexed_rows: 1, failed_rows: 0 } })

    const result = await tokenAnalysisAPI.triggerIndex({ start_date: '2026-05-19', end_date: '2026-05-19' })

    expect(post).toHaveBeenCalledWith('/admin/token-analysis/index', {
      start_date: '2026-05-19',
      end_date: '2026-05-19'
    })
    expect(result.indexed_rows).toBe(1)
  })
})
```

- [ ] **Step 2: Run API test and verify RED**

Run:

```powershell
cd frontend
npm run test:run -- src/api/__tests__/admin.tokenAnalysis.spec.ts
```

Expected: FAIL because `tokenAnalysis.ts` does not exist.

- [ ] **Step 3: Implement API module**

Create `frontend/src/api/admin/tokenAnalysis.ts` with exported interfaces:

```ts
export interface TokenAnalysisQueryParams {
  start_date?: string
  end_date?: string
  timezone?: string
  user_id?: number
  api_key_id?: number
  account_id?: number
  group_id?: number
  model?: string
  endpoint?: string
  risk_min?: number
  risk_reason?: string
  include_unmatched?: boolean
  page?: number
  page_size?: number
  sort_by?: string
  sort_order?: 'asc' | 'desc'
}
```

Implement:

```ts
async function getSummary(params: TokenAnalysisQueryParams): Promise<TokenAnalysisSummary>
async function listUsers(params: TokenAnalysisQueryParams, options?: { signal?: AbortSignal }): Promise<PaginatedResponse<TokenAnalysisUserUsage>>
async function listRequests(params: TokenAnalysisQueryParams, options?: { signal?: AbortSignal }): Promise<PaginatedResponse<TokenAnalysisRequestItem>>
async function triggerIndex(payload: TokenAnalysisIndexRequest): Promise<TokenAnalysisIndexResult>
async function getIndexStatus(): Promise<TokenAnalysisIndexStatus>
```

Modify `frontend/src/api/admin/index.ts` to import/export `tokenAnalysisAPI` and add `tokenAnalysis` to `adminAPI`.

- [ ] **Step 4: Run API test and verify GREEN**

Run:

```powershell
cd frontend
npm run test:run -- src/api/__tests__/admin.tokenAnalysis.spec.ts
```

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add frontend/src/api/admin/tokenAnalysis.ts frontend/src/api/admin/index.ts frontend/src/api/__tests__/admin.tokenAnalysis.spec.ts
git commit -m "feat: add token analysis admin client"
```

---

## Task 8: Frontend Page, Route, Sidebar, And I18n

**Files:**
- Create: `frontend/src/views/admin/TokenAnalysisView.vue`
- Modify: `frontend/src/router/index.ts`
- Modify: `frontend/src/components/layout/AppSidebar.vue`
- Modify: `frontend/src/i18n/locales/zh.ts`
- Modify: `frontend/src/i18n/locales/en.ts`
- Test: `frontend/src/views/admin/__tests__/TokenAnalysisView.spec.ts`

- [ ] **Step 1: Write failing view test**

Create `frontend/src/views/admin/__tests__/TokenAnalysisView.spec.ts`.

```ts
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import TokenAnalysisView from '../TokenAnalysisView.vue'

const api = vi.hoisted(() => ({
  getSummary: vi.fn(),
  listUsers: vi.fn(),
  listRequests: vi.fn(),
  getIndexStatus: vi.fn(),
  triggerIndex: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    tokenAnalysis: api
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const AppLayoutStub = { template: '<div><slot /></div>' }

describe('TokenAnalysisView', () => {
  beforeEach(() => {
    api.getSummary.mockReset()
    api.listUsers.mockReset()
    api.listRequests.mockReset()
    api.getIndexStatus.mockReset()
    api.triggerIndex.mockReset()
    api.getSummary.mockResolvedValue({
      total_requests: 12,
      total_tokens: 9000,
      total_actual_cost: 1.23,
      cache_read_tokens: 4000,
      cache_creation_tokens: 1000,
      cache_hit_rate: 0.4,
      risky_requests: 2,
      risky_cost: 0.6
    })
    api.listUsers.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20 })
    api.listRequests.mockResolvedValue({
      items: [{
        id: 1,
        archive_id: 'arch-1',
        event_time: '2026-05-19T01:00:00Z',
        user_email: 'user@example.com',
        api_key_name: 'dev',
        model: 'gpt-4.1',
        input_tokens: 220000,
        output_tokens: 32,
        cache_read_tokens: 0,
        cache_creation_tokens: 0,
        actual_cost: 1.23,
        risk_score: 45,
        risk_reasons: [{ code: 'huge_input_tiny_output', message: 'large input tiny output', score: 45 }],
        last_user_preview: 'hello',
        match_confidence: 3
      }],
      total: 1,
      page: 1,
      page_size: 20
    })
    api.getIndexStatus.mockResolvedValue({ running: false, processed_rows: 10, failed_rows: 0, files: [] })
  })

  it('renders summary and suspicious request rows', async () => {
    const wrapper = mount(TokenAnalysisView, {
      global: { stubs: { AppLayout: AppLayoutStub } }
    })

    await flushPromises()

    expect(wrapper.text()).toContain('9000')
    expect(wrapper.text()).toContain('user@example.com')
    expect(wrapper.text()).toContain('gpt-4.1')
    expect(wrapper.text()).toContain('hello')
  })
})
```

- [ ] **Step 2: Run view test and verify RED**

Run:

```powershell
cd frontend
npm run test:run -- src/views/admin/__tests__/TokenAnalysisView.spec.ts
```

Expected: FAIL because `TokenAnalysisView.vue` does not exist.

- [ ] **Step 3: Implement page**

Create `frontend/src/views/admin/TokenAnalysisView.vue` with:

| Section | Implementation |
|---|---|
| Top filters | Date inputs, user ID, API key ID, model, risk minimum select, include unmatched checkbox |
| Summary cards | Six compact cards: tokens, cost, cache read, cache hit rate, risky requests, risky cost |
| Ranking table | Dense table for `listUsers` results with user email, request count, total tokens, cost, cache hit rate, risk ratio |
| Request table | Dense table for suspicious requests with risk badge, usage metrics, reason chips, request preview |
| Detail drawer | Right-side fixed panel with endpoint, model, match confidence, message/system/tool/image counts, last user preview |
| Index status | Small toolbar showing processed/failed rows and a button to trigger indexing for current date range |

Keep page styling close to existing admin pages: `AppLayout`, `card`, `btn`, `input`, `table`, restrained colors, no hero layout.

- [ ] **Step 4: Add route**

Modify `frontend/src/router/index.ts` near `/admin/usage`:

```ts
{
  path: '/admin/token-analysis',
  name: 'AdminTokenAnalysis',
  component: () => import('@/views/admin/TokenAnalysisView.vue'),
  meta: {
    requiresAuth: true,
    requiresAdmin: true,
    title: 'Token Analysis',
    titleKey: 'admin.tokenAnalysis.title',
    descriptionKey: 'admin.tokenAnalysis.description'
  }
}
```

- [ ] **Step 5: Add sidebar entry**

Modify `frontend/src/components/layout/AppSidebar.vue` in `adminNavItems`, near `/admin/usage`:

```ts
{ path: '/admin/token-analysis', label: t('nav.tokenAnalysis'), icon: ChartIcon, hideInSimpleMode: true },
```

- [ ] **Step 6: Add i18n keys**

Add to `nav` in `zh.ts`:

```ts
tokenAnalysis: 'Token 分析',
```

Add to `admin` in `zh.ts`:

```ts
tokenAnalysis: {
  title: 'Token 分析',
  description: '查看部门用量、缓存命中和疑似浪费请求',
  indexNow: '索引当前范围',
  riskMin: '风险分',
  includeUnmatched: '包含未匹配归档',
  summary: {
    totalTokens: '总 Token',
    totalCost: '总费用',
    cacheRead: '缓存命中',
    cacheHitRate: '缓存命中率',
    riskyRequests: '可疑请求',
    riskyCost: '可疑费用'
  }
}
```

Add equivalent English keys in `en.ts`.

- [ ] **Step 7: Run view test and typecheck**

Run:

```powershell
cd frontend
npm run test:run -- src/views/admin/__tests__/TokenAnalysisView.spec.ts
npm run typecheck
```

Expected: PASS.

- [ ] **Step 8: Commit**

```powershell
git add frontend/src/views/admin/TokenAnalysisView.vue frontend/src/views/admin/__tests__/TokenAnalysisView.spec.ts frontend/src/router/index.ts frontend/src/components/layout/AppSidebar.vue frontend/src/i18n/locales/zh.ts frontend/src/i18n/locales/en.ts
git commit -m "feat: add token analysis admin page"
```

---

## Task 9: Full Verification And Polish

**Files:**
- Review all files changed by Tasks 1-8
- Update tests only when assertions need to match final user-facing labels or response shapes

- [ ] **Step 1: Run backend focused tests**

Run:

```powershell
cd backend
go test ./internal/config ./internal/service ./internal/repository ./internal/handler/admin ./internal/server/routes -run 'TokenAnalysis|RequestArchive|Usage'
```

Expected: PASS.

- [ ] **Step 2: Run frontend focused tests**

Run:

```powershell
cd frontend
npm run test:run -- src/api/__tests__/admin.tokenAnalysis.spec.ts src/views/admin/__tests__/TokenAnalysisView.spec.ts
npm run typecheck
```

Expected: PASS.

- [ ] **Step 3: Run broader regression checks**

Run:

```powershell
cd backend
go test ./internal/server/middleware ./internal/config ./internal/service -run 'RequestArchive|RequestIntercept|Usage|Cache|TokenAnalysis'
cd ..
python tools/assemble_request_archive_test.py
```

Expected: PASS.

- [ ] **Step 4: Manual browser verification**

Start frontend and backend in the normal local development way used by this repo. Open `/admin/token-analysis` as an admin and verify:

| Check | Expected |
|---|---|
| Page loads | No console errors |
| Summary cards | Show numeric values or zero state |
| Index button | Calls `/admin/token-analysis/index` and shows success/error toast |
| Requests table | Shows risk reasons and request preview |
| Detail drawer | Shows summary fields and does not show response body |
| Filters | Change API request params and reload tables |

- [ ] **Step 5: Inspect final diff**

Run:

```powershell
git diff --stat
git diff --check
git status --short
```

Expected: only planned files changed; `git diff --check` has no output.

- [ ] **Step 6: Commit final fixes if any**

If Step 5 reveals small label, formatting, or typing fixes, commit them:

```powershell
git add <changed-files>
git commit -m "chore: polish token analysis"
```

---

## Acceptance Checklist

| Requirement | Covered By |
|---|---|
| Department usage overview | Task 4 repository aggregations, Task 6 summary API, Task 8 cards |
| User/API Key ranking | Task 4 `ListUserUsage`, Task 6 users API, Task 8 ranking table |
| Suspicious request list | Task 3 risk rules, Task 4 `ListRequests`, Task 8 request table |
| Request parameter summary | Task 2 summary extraction, Task 5 indexer, Task 8 detail drawer |
| No full response body exposure | Task 2 only reads request events, Task 8 drawer shows summaries only |
| Async indexing instead of gateway hot-path writes | Task 5 manual/background indexer, no gateway changes |
| Cache hit visibility | Task 4 joins `usage_logs.cache_read_tokens/cache_creation_tokens`, Task 8 columns/cards |
| Admin-only access | Task 6 routes under existing admin router, Task 8 route meta requires admin |

## Final Verification Command Set

Run these before merging:

```powershell
cd backend
go test ./internal/config ./internal/service ./internal/repository ./internal/handler/admin ./internal/server/routes -run 'TokenAnalysis|RequestArchive|Usage'
cd ..
python tools/assemble_request_archive_test.py
cd frontend
npm run test:run -- src/api/__tests__/admin.tokenAnalysis.spec.ts src/views/admin/__tests__/TokenAnalysisView.spec.ts
npm run typecheck
```
