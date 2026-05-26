package promptmetrics

import "time"

const (
	AnalysisStatusPending = "pending"
	AnalysisStatusDone    = "done"
	AnalysisStatusFailed  = "failed"
	AnalysisStatusSkipped = "skipped"
)

// Event 保存一次用户手工提示词输入的不可变快照.
// 该结构只承载中间件同步阶段已经读取到的数据, 异步写入时不再访问 gin.Context.
type Event struct {
	ID                    int64     `json:"id"`
	RequestID             string    `json:"request_id"`
	UserID                *int64    `json:"user_id,omitempty"`
	APIKeyID              *int64    `json:"api_key_id,omitempty"`
	GroupID               *int64    `json:"group_id,omitempty"`
	Model                 string    `json:"model"`
	RequestedModel        string    `json:"requested_model"`
	Endpoint              string    `json:"endpoint"`
	SourceProtocol        string    `json:"source_protocol"`
	PromptText            string    `json:"prompt_text,omitempty"`
	PromptExcerpt         string    `json:"prompt_excerpt"`
	PromptHash            string    `json:"prompt_hash"`
	PromptChars           int       `json:"prompt_chars"`
	PromptSegments        int       `json:"prompt_segments"`
	PromptTokensEstimated int       `json:"prompt_tokens_estimated"`
	ProjectName           string    `json:"project_name"`
	GitBranch             string    `json:"git_branch"`
	ClientName            string    `json:"client_name"`
	ClientVersion         string    `json:"client_version"`
	UserAgent             string    `json:"user_agent"`
	IPAddress             string    `json:"ip_address"`
	Truncated             bool      `json:"truncated"`
	AnalysisStatus        string    `json:"analysis_status"`
	CreatedAt             time.Time `json:"created_at"`
	Analysis              *Analysis `json:"analysis,omitempty"`
	InputTokens           int64     `json:"input_tokens,omitempty"`
	OutputTokens          int64     `json:"output_tokens,omitempty"`
	CacheCreationTokens   int64     `json:"cache_creation_tokens,omitempty"`
	CacheReadTokens       int64     `json:"cache_read_tokens,omitempty"`
	TotalTokens           int64     `json:"total_tokens,omitempty"`
	ActualCost            float64   `json:"actual_cost,omitempty"`
	UserEmail             string    `json:"user_email,omitempty"`
	APIKeyName            string    `json:"api_key_name,omitempty"`
	GroupName             string    `json:"group_name,omitempty"`
}

// ExtractedPrompt 表示从不同协议请求体中抽取出的用户输入片段.
// Text 为空时表示该请求不含可采集的用户手工输入.
type ExtractedPrompt struct {
	Text            string
	Segments        int
	SourceProtocol  string
	RequestedModel  string
	PromptTruncated bool
}

// ClientContext 保存从 header, UA 和 body 中推断出的客户端上下文.
type ClientContext struct {
	ProjectName   string
	GitBranch     string
	ClientName    string
	ClientVersion string
}

// Filters 描述管理端查询条件, 只允许仓储层使用参数化 SQL 消费.
type Filters struct {
	From           *time.Time
	To             *time.Time
	UserID         *int64
	APIKeyID       *int64
	GroupID        *int64
	ProjectName    string
	GitBranch      string
	ClientName     string
	Model          string
	Endpoint       string
	Hash           string
	MinQuality     *int
	MaxQuality     *int
	OnlyLowQuality bool
}

// Overview 汇总管理端总览指标.
type Overview struct {
	TotalEvents     int64   `json:"total_events"`
	ActiveUsers     int64   `json:"active_users"`
	LowQuality      int64   `json:"low_quality"`
	Truncated       int64   `json:"truncated"`
	PendingAnalysis int64   `json:"pending_analysis"`
	TotalTokens     int64   `json:"total_tokens"`
	TotalCost       float64 `json:"total_cost"`
	AverageQuality  float64 `json:"average_quality"`
}

// TrendPoint 表示按小时或天聚合的交互趋势.
type TrendPoint struct {
	Bucket       string  `json:"bucket"`
	Events       int64   `json:"events"`
	Users        int64   `json:"users"`
	Tokens       int64   `json:"tokens"`
	Cost         float64 `json:"cost"`
	AvgQuality   float64 `json:"avg_quality"`
	LowQuality   int64   `json:"low_quality"`
	PendingCount int64   `json:"pending_count"`
}

// RankItem 表示项目, 分支, 客户端, 模型或用户维度排行.
type RankItem struct {
	Key        string  `json:"key"`
	Label      string  `json:"label"`
	Events     int64   `json:"events"`
	Users      int64   `json:"users"`
	Tokens     int64   `json:"tokens"`
	Cost       float64 `json:"cost"`
	AvgQuality float64 `json:"avg_quality"`
}

// Analysis 保存提示词本地规则分析结果.
type Analysis struct {
	PromptEventID          int64     `json:"prompt_event_id"`
	Summary                string    `json:"summary"`
	QualityScore           int       `json:"quality_score"`
	ClarityScore           int       `json:"clarity_score"`
	ContextScore           int       `json:"context_score"`
	ActionabilityScore     int       `json:"actionability_score"`
	ConstraintScore        int       `json:"constraint_score"`
	RiskScore              int       `json:"risk_score"`
	Categories             []string  `json:"categories"`
	ImprovementSuggestions []string  `json:"improvement_suggestions"`
	AnalyzerModel          string    `json:"analyzer_model"`
	AnalyzedAt             time.Time `json:"analyzed_at"`
}

// Pagination 保存列表分页信息.
type Pagination struct {
	Page     int
	PageSize int
	Total    int64
}
