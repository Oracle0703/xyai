package service

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

const (
	TokenAnalysisRiskHugeInputTinyOutput   = "huge_input_tiny_output"
	TokenAnalysisRiskRepeatUncachedBody    = "repeat_uncached_body"
	TokenAnalysisRiskLowCacheHitLargeInput = "low_cache_hit_large_input"
	TokenAnalysisRiskRapidSimilarRequests  = "rapid_similar_requests"
	TokenAnalysisRiskOversizedSystemPrompt = "oversized_system_prompt"
	TokenAnalysisRiskToolHeavyShortOutput  = "tool_heavy_short_output"
	TokenAnalysisRiskLargeToolHistory      = "large_tool_history"
	TokenAnalysisRiskGiantToolOutput       = "giant_tool_output"
)

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
	// ClientWorkdir/ClientProject/ClientBranch/AttributionSource 为离线提取的
	// 项目归因字段, 见 docs/features/token-analysis-project-attribution-design-cn.md。
	ClientWorkdir     string    `json:"client_workdir"`
	ClientProject     string    `json:"client_project"`
	ClientBranch      string    `json:"client_branch"`
	AttributionSource string    `json:"attribution_source"`
	IndexedAt         time.Time `json:"indexed_at"`
	SourceFile        string    `json:"source_file"`
	SourceOffset      *int64    `json:"source_offset,omitempty"`
}

type TokenAnalysisBodySummary struct {
	Model              string         `json:"model"`
	MessageCount       int            `json:"message_count"`
	SystemChars        int            `json:"system_chars"`
	UserChars          int            `json:"user_chars"`
	LastUserPreview    string         `json:"last_user_preview"`
	ToolsCount         int            `json:"tools_count"`
	ImageCount         int            `json:"image_count"`
	ToolMessageCount   int            `json:"tool_message_count"`
	ToolOutputBytes    int64          `json:"tool_output_bytes"`
	MaxToolOutputBytes int64          `json:"max_tool_output_bytes"`
	SummaryJSON        map[string]any `json:"summary_json"`
}

type TokenAnalysisUsageSignals struct {
	InputTokens         int     `json:"input_tokens"`
	OutputTokens        int     `json:"output_tokens"`
	CacheReadTokens     int     `json:"cache_read_tokens"`
	CacheCreationTokens int     `json:"cache_creation_tokens"`
	TotalCost           float64 `json:"total_cost"`
	ActualCost          float64 `json:"actual_cost"`
}

type TokenAnalysisDuplicateSignals struct {
	SameBodyRecentCount int `json:"same_body_recent_count"`
	SimilarRecentCount  int `json:"similar_recent_count"`
}

type TokenAnalysisFilters struct {
	StartTime        *time.Time `json:"start_time,omitempty"`
	EndTime          *time.Time `json:"end_time,omitempty"`
	UserID           *int64     `json:"user_id,omitempty"`
	APIKeyID         *int64     `json:"api_key_id,omitempty"`
	AccountID        *int64     `json:"account_id,omitempty"`
	GroupID          *int64     `json:"group_id,omitempty"`
	Model            string     `json:"model,omitempty"`
	Endpoint         string     `json:"endpoint,omitempty"`
	RiskMin          int64      `json:"risk_min,omitempty"`
	RiskReason       string     `json:"risk_reason,omitempty"`
	IncludeUnmatched bool       `json:"include_unmatched,omitempty"`
	// Project 按客户端项目过滤; 特殊值 "unattributed" 表示未归因(client_project 为空)。
	Project string `json:"project,omitempty"`
}

type TokenAnalysisSummary struct {
	TotalRequests       int64                            `json:"total_requests"`
	MatchedRequests     int64                            `json:"matched_requests"`
	UnmatchedRequests   int64                            `json:"unmatched_requests"`
	TotalInputTokens    int64                            `json:"total_input_tokens"`
	TotalOutputTokens   int64                            `json:"total_output_tokens"`
	CacheReadTokens     int64                            `json:"cache_read_tokens"`
	CacheCreationTokens int64                            `json:"cache_creation_tokens"`
	TotalTokens         int64                            `json:"total_tokens"`
	TotalCost           float64                          `json:"total_cost"`
	TotalActualCost     float64                          `json:"total_actual_cost"`
	CacheHitRate        float64                          `json:"cache_hit_rate"`
	RiskyRequests       int64                            `json:"risky_requests"`
	RiskyCost           float64                          `json:"risky_cost"`
	UnmatchedRate       float64                          `json:"unmatched_rate"`
	RiskRequestRate     float64                          `json:"risk_request_rate"`
	RiskReasons         []TokenAnalysisRiskReasonSummary `json:"risk_reasons"`
}

type TokenAnalysisRiskReasonSummary struct {
	Code  string `json:"code"`
	Count int64  `json:"count"`
}

type TokenAnalysisUserUsage struct {
	UserID              *int64     `json:"user_id,omitempty"`
	UserEmail           string     `json:"user_email"`
	APIKeyID            *int64     `json:"api_key_id,omitempty"`
	APIKeyName          string     `json:"api_key_name"`
	RequestCount        int64      `json:"request_count"`
	RiskyRequestCount   int64      `json:"risky_request_count"`
	TotalTokens         int64      `json:"total_tokens"`
	InputTokens         int64      `json:"input_tokens"`
	OutputTokens        int64      `json:"output_tokens"`
	CacheReadTokens     int64      `json:"cache_read_tokens"`
	CacheCreationTokens int64      `json:"cache_creation_tokens"`
	ActualCost          float64    `json:"actual_cost"`
	RiskyCost           float64    `json:"risky_cost"`
	CacheHitRate        float64    `json:"cache_hit_rate"`
	RiskRatio           float64    `json:"risk_ratio"`
	LastEventTime       *time.Time `json:"last_event_time,omitempty"`
}

// TokenAnalysisProjectUsage 是"成员 × 项目"维度的 token 消耗聚合行。
type TokenAnalysisProjectUsage struct {
	Project             string     `json:"project"`
	UserID              *int64     `json:"user_id,omitempty"`
	UserEmail           string     `json:"user_email"`
	RequestCount        int64      `json:"request_count"`
	MatchedRequestCount int64      `json:"matched_request_count"`
	TotalTokens         int64      `json:"total_tokens"`
	InputTokens         int64      `json:"input_tokens"`
	OutputTokens        int64      `json:"output_tokens"`
	CacheReadTokens     int64      `json:"cache_read_tokens"`
	CacheCreationTokens int64      `json:"cache_creation_tokens"`
	ActualCost          float64    `json:"actual_cost"`
	LastEventTime       *time.Time `json:"last_event_time,omitempty"`
}

type TokenAnalysisRequestItem struct {
	ID                   int64                     `json:"id"`
	ArchiveID            string                    `json:"archive_id"`
	UsageLogID           *int64                    `json:"usage_log_id,omitempty"`
	MatchConfidence      int16                     `json:"match_confidence"`
	EventTime            time.Time                 `json:"event_time"`
	UserID               *int64                    `json:"user_id,omitempty"`
	UserEmail            string                    `json:"user_email"`
	APIKeyID             *int64                    `json:"api_key_id,omitempty"`
	APIKeyName           string                    `json:"api_key_name"`
	AccountID            *int64                    `json:"account_id,omitempty"`
	GroupID              *int64                    `json:"group_id,omitempty"`
	Model                string                    `json:"model"`
	Endpoint             string                    `json:"endpoint"`
	Method               string                    `json:"method"`
	RequestBodySize      int64                     `json:"request_body_size"`
	RequestBodyTruncated bool                      `json:"request_body_truncated"`
	MessageCount         int                       `json:"message_count"`
	SystemChars          int                       `json:"system_chars"`
	UserChars            int                       `json:"user_chars"`
	LastUserPreview      string                    `json:"last_user_preview"`
	ToolsCount           int                       `json:"tools_count"`
	ImageCount           int                       `json:"image_count"`
	SummaryJSON          map[string]any            `json:"summary_json"`
	ClientWorkdir        string                    `json:"client_workdir"`
	ClientProject        string                    `json:"client_project"`
	ClientBranch         string                    `json:"client_branch"`
	AttributionSource    string                    `json:"attribution_source"`
	InputTokens          int64                     `json:"input_tokens"`
	OutputTokens         int64                     `json:"output_tokens"`
	CacheReadTokens      int64                     `json:"cache_read_tokens"`
	CacheCreationTokens  int64                     `json:"cache_creation_tokens"`
	TotalTokens          int64                     `json:"total_tokens"`
	ActualCost           float64                   `json:"actual_cost"`
	RiskScore            int                       `json:"risk_score"`
	RiskReasons          []TokenAnalysisRiskReason `json:"risk_reasons"`
}

type TokenAnalysisUsageMatch struct {
	UsageLogID          int64
	MatchConfidence     int16
	InputTokens         int
	OutputTokens        int
	CacheReadTokens     int
	CacheCreationTokens int
	TotalCost           float64
	ActualCost          float64
}

func (m TokenAnalysisUsageMatch) UsageSignals() TokenAnalysisUsageSignals {
	return TokenAnalysisUsageSignals{
		InputTokens:         m.InputTokens,
		OutputTokens:        m.OutputTokens,
		CacheReadTokens:     m.CacheReadTokens,
		CacheCreationTokens: m.CacheCreationTokens,
		TotalCost:           m.TotalCost,
		ActualCost:          m.ActualCost,
	}
}

type TokenAnalysisIndexRequest struct {
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	Timezone  string `json:"timezone"`
}

type TokenAnalysisIndexResult struct {
	IndexedRows int64 `json:"indexed_rows"`
	SkippedRows int64 `json:"skipped_rows"`
	FailedRows  int64 `json:"failed_rows"`
	Files       int   `json:"files"`
}

type TokenAnalysisIndexStatus struct {
	Running       bool                      `json:"running"`
	ProcessedRows int64                     `json:"processed_rows"`
	FailedRows    int64                     `json:"failed_rows"`
	Files         []TokenAnalysisIndexState `json:"files"`
	LastError     string                    `json:"last_error"`
	UpdatedAt     *time.Time                `json:"updated_at,omitempty"`
}

type TokenAnalysisIndexState struct {
	SourceFile    string     `json:"source_file"`
	LastOffset    int64      `json:"last_offset"`
	LastArchiveID string     `json:"last_archive_id"`
	ProcessedRows int64      `json:"processed_rows"`
	FailedRows    int64      `json:"failed_rows"`
	LastError     string     `json:"last_error"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type TokenAnalysisRepository interface {
	UpsertRequestSummary(ctx context.Context, summary *TokenAnalysisRequestSummary) error
	FindNearestUsageLog(ctx context.Context, eventTime time.Time, userID, apiKeyID *int64, model string, window time.Duration) (*TokenAnalysisUsageMatch, error)
	CountSameBodyRecent(ctx context.Context, bodySHA256 string, userID, apiKeyID *int64, eventTime time.Time, window time.Duration) (int, error)
	GetSummary(ctx context.Context, filters TokenAnalysisFilters) (*TokenAnalysisSummary, error)
	ListUserUsage(ctx context.Context, filters TokenAnalysisFilters, params pagination.PaginationParams) ([]TokenAnalysisUserUsage, *pagination.PaginationResult, error)
	ListProjectUsage(ctx context.Context, filters TokenAnalysisFilters, params pagination.PaginationParams) ([]TokenAnalysisProjectUsage, *pagination.PaginationResult, error)
	ListRequests(ctx context.Context, filters TokenAnalysisFilters, params pagination.PaginationParams) ([]TokenAnalysisRequestItem, *pagination.PaginationResult, error)
	GetIndexStatus(ctx context.Context) (*TokenAnalysisIndexStatus, error)
	GetIndexState(ctx context.Context, sourceFile string) (*TokenAnalysisIndexState, error)
	UpdateIndexState(ctx context.Context, state TokenAnalysisIndexState) error
	// LoadProjectRoots 加载全部已知仓库根(root -> project)。
	LoadProjectRoots(ctx context.Context) (map[string]string, error)
	// UpsertProjectRoots 批量学习仓库根, 已存在时仅刷新 last_seen。
	UpsertProjectRoots(ctx context.Context, roots map[string]string) error
}

type tokenAnalysisArchiveEvent struct {
	ArchiveID     string `json:"archive_id"`
	Event         string `json:"event"`
	Timestamp     string `json:"timestamp"`
	Method        string `json:"method"`
	Path          string `json:"path"`
	Endpoint      string `json:"endpoint"`
	UserID        *int64 `json:"user_id"`
	APIKeyID      *int64 `json:"api_key_id"`
	GroupID       *int64 `json:"group_id"`
	AccountID     *int64 `json:"account_id"`
	Model         string `json:"model"`
	UserAgent     string `json:"user_agent"`
	Body          string `json:"body"`
	BodySize      int64  `json:"body_size"`
	BodySHA256    string `json:"body_sha256"`
	BodyTruncated bool   `json:"body_truncated"`
}
