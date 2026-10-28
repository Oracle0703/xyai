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

// 计费关联方式(usage_match_method)与对外关联状态(usage_link_status)。
// 存量行 method 为空: 已关联的是旧"开始时间 ±窗口"结果, 由关联轮次复核。
// released 是内部状态: 弱关联被精确证据夺走账单, 等待重新关联并重算风险。
const (
	TokenAnalysisUsageMatchRequestID    = "request_id"
	TokenAnalysisUsageMatchTiming       = "timing"
	TokenAnalysisUsageMatchLegacyWindow = "legacy_window"
	TokenAnalysisUsageMatchReleased     = "released"

	TokenAnalysisUsageLinked      = "linked"
	TokenAnalysisUsageUnlinked    = "unlinked"
	TokenAnalysisUsageFailed      = "failed"
	TokenAnalysisUsageNotBillable = "not_billable"
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
	ClientRequestID      string                    `json:"client_request_id,omitempty"`
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
	ClientWorkdir     string `json:"client_workdir"`
	ClientProject     string `json:"client_project"`
	ClientBranch      string `json:"client_branch"`
	AttributionSource string `json:"attribution_source"`
	// UserAgent/ClientFamily 取自归档事件, 未关联计费的行也有; ClientFamilyVersion
	// 是映射规则版本, 0 表示未识别(该列上线前索引的历史行)。
	UserAgent           string    `json:"user_agent"`
	ClientFamily        string    `json:"client_family"`
	ClientFamilyVersion int32     `json:"client_family_version"`
	IndexedAt           time.Time `json:"indexed_at"`
	SourceFile          string    `json:"source_file"`
	SourceOffset        *int64    `json:"source_offset,omitempty"`
	// SourceLineStart/SourceLineLength 是该行在归档文件中的起点与字节长度(含换行),
	// 供重新匹配时重读原文; SourceOffset 是行尾之后的位置(续读水位口径)。
	SourceLineStart  *int64 `json:"-"`
	SourceLineLength *int64 `json:"-"`
	// Fingerprint 是前缀指纹; 不经接口返回。
	Fingerprint TokenAnalysisFingerprint `json:"-"`
}

// TokenAnalysisSummaryUpsert 是写入摘要后的候选快照状态: CandidatesDue 表示需要
// 计算候选边, 写回时以 CandInputVersion 作条件。
type TokenAnalysisSummaryUpsert struct {
	CandInputVersion int64
	CandidatesDue    bool
}

// 候选边类型, 见风险分类设计 5.4。
const (
	TokenAnalysisEdgeFullPrefix      = "full_prefix"
	TokenAnalysisEdgePrefixMinusTail = "prefix_minus_tail"
	TokenAnalysisEdgeItemsLink       = "items_link"

	TokenAnalysisCandidateStateOK              = "ok"
	TokenAnalysisCandidateStateTieOverflow     = "unknown:tie_overflow"
	TokenAnalysisCandidateStateBodyUnavailable = "unknown:body_unavailable"
	TokenAnalysisCandidateStateNoAPIKey        = "unsupported:no_api_key"
)

// TokenAnalysisPrefixCandidateQuery 查找子请求的候选父请求: 同 Key、同指纹版本、
// [From, To) 内, 前缀链末端/去尾哈希或关联链末端哈希出现在子请求链中。
type TokenAnalysisPrefixCandidateQuery struct {
	ArchiveID    string
	APIKeyID     int64
	FPVersion    int32
	From         time.Time
	To           time.Time
	PrefixHashes [][]byte
	ItemsHashes  [][]byte
}

type TokenAnalysisPrefixCandidate struct {
	ArchiveID       string
	EventTime       time.Time
	PrefixItems     int
	PrefixEndHash   []byte
	PrefixTail1Hash []byte
	ItemsEndHash    []byte
}

type TokenAnalysisPrefixEdge struct {
	ParentArchiveID string
	Kind            string
	MatchedItems    int
}

// TokenAnalysisPrefixEdgeWrite 以 ExpectedCandVersion 为条件整体替换子请求的候选边。
type TokenAnalysisPrefixEdgeWrite struct {
	ArchiveID           string
	ExpectedCandVersion int64
	Edges               []TokenAnalysisPrefixEdge
	State               string
	Overflow            bool
	// Query 非 nil 时, 仓储先取该 API Key 的候选锁, 再在同一事务内查找候选并交给 Select
	// 选边(结果覆盖 Edges/Overflow, overflow 时 State 记为 tie_overflow)。摘要写入持有同一
	// 把锁, 因此并发提交的新父请求要么被这次查找看到, 要么在提交时把本行标为待重新匹配。
	Query  *TokenAnalysisPrefixCandidateQuery
	Select func([]TokenAnalysisPrefixCandidate) ([]TokenAnalysisPrefixEdge, bool)
}

// TokenAnalysisCandidateRematchRow 是需要重新匹配的行(候选快照版本不一致)。
type TokenAnalysisCandidateRematchRow struct {
	ArchiveID        string
	CandInputVersion int64
	SourceFile       string
	SourceOffset     *int64
	SourceLineStart  *int64
	SourceLineLength *int64
	BodySHA256       string
	// 存下的指纹: 重读原文算出的指纹必须与之相同, 才能为这份快照确认候选。
	FPVersion       int32
	PrefixEndHash   []byte
	PrefixTail1Hash []byte
	ItemsEndHash    []byte
}

// TokenAnalysisClientFamilyRow 是客户端族重新映射的一行。UserAgent/Version 是读出时
// 的快照, 写回时作为条件; Family 是按当前规则算出的新族。
type TokenAnalysisClientFamilyRow struct {
	ID        int64
	UserAgent string
	Version   int32
	Family    string
}

type TokenAnalysisBodySummary struct {
	Model           string `json:"model"`
	MessageCount    int    `json:"message_count"`
	SystemChars     int    `json:"system_chars"`
	UserChars       int    `json:"user_chars"`
	LastUserPreview string `json:"last_user_preview"`
	// LastUserText 是最后一条用户输入的原始全文(未脱敏未截断),
	// 仅供索引器经 SanitizeTokenAnalysisInputText 处理后留存, 不直接外发。
	LastUserText string `json:"-"`
	// InputComplete 表示 body 被完整解析(非降级摘要); 只有此时 LastUserText 为空
	// 才能确认请求没有用户正文。
	InputComplete      bool           `json:"-"`
	ToolsCount         int            `json:"tools_count"`
	ImageCount         int            `json:"image_count"`
	ToolMessageCount   int            `json:"tool_message_count"`
	ToolOutputBytes    int64          `json:"tool_output_bytes"`
	MaxToolOutputBytes int64          `json:"max_tool_output_bytes"`
	SummaryJSON        map[string]any `json:"summary_json"`
}

// TokenAnalysisUserInput 是单次请求提取出的用户净输入全文留存行;
// 质量字段为占位, 评价标准确定后由评估任务回填, 重建索引不覆盖。
type TokenAnalysisUserInput struct {
	ID              int64          `json:"id"`
	ArchiveID       string         `json:"archive_id"`
	EventTime       time.Time      `json:"event_time"`
	UserID          *int64         `json:"user_id,omitempty"`
	Content         string         `json:"content"`
	ContentSHA256   string         `json:"content_sha256"`
	Chars           int            `json:"chars"`
	Truncated       bool           `json:"truncated"`
	QualityScore    *int16         `json:"quality_score,omitempty"`
	QualityFindings map[string]any `json:"quality_findings,omitempty"`
	QualityVersion  string         `json:"quality_version"`
	EvaluatedAt     *time.Time     `json:"evaluated_at,omitempty"`
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
	// Origin 按本次调用来源过滤: human/ai 匹配 summary_json.request_origin.kind;
	// unknown 含 kind=unknown 与尚未写入来源的旧行(与页面显示 "—" 一致)。
	Origin string `json:"origin,omitempty"`
	// Project 按客户端项目过滤; 特殊值 "unattributed" 表示未归因(client_project 为空)。
	Project string `json:"project,omitempty"`
	// UserEmail 按成员邮箱模糊搜索, 作用于概览(含同期计费请求数)、用户/项目排行与请求明细(JOIN users)。
	UserEmail string `json:"user_email,omitempty"`
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
	// HumanInputCount 估算人工输入次数: 人工标签行去掉短时重发的同一提示词
	// (仓储 tokenAnalysisHumanInputCountExpr), 不证明真实操作次数。
	HumanInputCount int64 `json:"human_input_count"`
	// BilledRequests 为同期 usage_logs 计费请求数(只应用时间/用户/密钥/账号/
	// 分组/模型过滤, endpoint/风险分/项目归因是归档侧维度不参与);
	// ArchiveCoverage = MatchedRequests / BilledRequests, 用于呈现归档样本
	// 相对真实计费请求的覆盖程度。
	BilledRequests  int64   `json:"billed_requests"`
	ArchiveCoverage float64 `json:"archive_coverage"`
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
	HumanInputCount     int64      `json:"human_input_count"`
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
	HumanInputCount     int64      `json:"human_input_count"`
	TotalTokens         int64      `json:"total_tokens"`
	InputTokens         int64      `json:"input_tokens"`
	OutputTokens        int64      `json:"output_tokens"`
	CacheReadTokens     int64      `json:"cache_read_tokens"`
	CacheCreationTokens int64      `json:"cache_creation_tokens"`
	ActualCost          float64    `json:"actual_cost"`
	LastEventTime       *time.Time `json:"last_event_time,omitempty"`
}

type TokenAnalysisRequestItem struct {
	ID              int64  `json:"id"`
	ArchiveID       string `json:"archive_id"`
	UsageLogID      *int64 `json:"usage_log_id,omitempty"`
	MatchConfidence int16  `json:"match_confidence"`
	// UsageLinkStatus 区分已关联/未关联/请求失败/不计费; 非 linked 时 Token 与
	// 费用字段为 0 只是占位, 不代表真实零消耗。UsageMatchMethod 仅 linked 时有值。
	UsageLinkStatus      string                    `json:"usage_link_status"`
	UsageMatchMethod     string                    `json:"usage_match_method,omitempty"`
	CompletedAt          *time.Time                `json:"completed_at,omitempty"`
	ResponseStatus       *int                      `json:"response_status,omitempty"`
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
	// HasInput/InputTruncated/QualityScore 来自 token_analysis_user_inputs LEFT JOIN;
	// 全文不进列表 payload, 经 GetUserInput 懒加载。
	HasInput       bool   `json:"has_input"`
	InputTruncated bool   `json:"input_truncated"`
	QualityScore   *int16 `json:"quality_score,omitempty"`
	// DuplicateCount 为当前筛选范围内净输入(content_sha256)相同的请求数,
	// >1 表示同一人类输入在 agent 轮次/重试里重发; 无留存输入时恒为 1。
	DuplicateCount int64 `json:"duplicate_count"`
}

type TokenAnalysisUsageMatch struct {
	UsageLogID          int64
	MatchConfidence     int16
	Method              string
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

// TokenAnalysisResponseFacts 是归档 response 行的完成事实, 用于计费关联。
type TokenAnalysisResponseFacts struct {
	ArchiveID    string
	CompletedAt  time.Time
	Status       int
	OutputTokens *int64
	// ResponseID 是响应体里的上游 response.id; WS 上游计费以它为 request_id。
	ResponseID string
	// Forward 是转发侧记录; 非原生路径或旧归档行为零值。
	Forward TokenAnalysisForwardFacts
}

// TokenAnalysisUsageLinkQuery 描述一次计费关联查找。精确关联键为
// "client:<ClientRequestID>" 与 ResponseID(同 api_key_id 下唯一)。时序兜底只在
// ExactOnly=false 时进行, 见仓储 findUsageByTiming 的约束。
type TokenAnalysisUsageLinkQuery struct {
	ArchiveID       string
	ClientRequestID string
	ResponseID      string
	StartedAt       time.Time
	CompletedAt     *time.Time
	OutputTokens    *int64
	UserID          *int64
	APIKeyID        *int64
	Model           string
	ExactOnly       bool
	Window          time.Duration
}

// TokenAnalysisUsageLinkState 是查找前读到的关联状态与复核版本。写入和确认都按它
// 做条件更新: 这一行已被其他索引实例改过、或其后又出现新的复核事件(完成事实变化、
// 账单被夺回)时放弃, 陈旧结果既不能覆盖较新的关联, 也不能确认掉新的复核任务。
// KeepPending: 写入后不确认该版本, 复核任务继续保留(仍在等待晚到账单)。
type TokenAnalysisUsageLinkState struct {
	UsageLogID  *int64
	Method      string
	Version     int64
	KeepPending bool
}

// TokenAnalysisUsageLinkWrite 是一次关联写入的结果: Applied=false 表示账单已被
// 其他请求认领或本行状态已变(并发索引); Displaced 表示精确关联从弱关联行夺回了账单。
type TokenAnalysisUsageLinkWrite struct {
	Applied   bool
	Displaced bool
}

// TokenAnalysisUsageLinkCandidate 是待关联或待复核旧关联的已索引请求, 附带
// 重算风险分所需的 body 侧信号(关联变化会改变依赖 usage 的风险原因)。
type TokenAnalysisUsageLinkCandidate struct {
	ID              int64
	ArchiveID       string
	ClientRequestID string
	ResponseID      string
	EventTime       time.Time
	CompletedAt     *time.Time
	ResponseStatus  *int
	OutputTokens    *int64
	UserID          *int64
	APIKeyID        *int64
	Model           string
	UsageLogID      *int64
	Method          string
	// LinkVersion 为读到的复核版本; LinkPending 表示该版本尚未确认(完成事实变化
	// 或账单被夺回), 需要不受日期范围限制地重新关联。RetryUntil 为等待晚到账单的截止时间。
	LinkVersion int64
	LinkPending bool
	RetryUntil  *time.Time
	BodySHA256  string
	Body        TokenAnalysisBodySummary
	// SameBodyRecentCount 为索引时计入风险的重复数; 旧行未记录时为 nil。
	SameBodyRecentCount *int
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
	// UpsertRequestSummary 写入摘要; 指纹新增或变化时在同一事务内给自身加候选版本,
	// 并把同 Key 之后 L 窗口内已有指纹的行标为待重新匹配(晚到的父请求)。
	// 有 API Key 的行在事务开头取该 Key 的候选锁(见 TokenAnalysisPrefixEdgeWrite.Query)。
	UpsertRequestSummary(ctx context.Context, summary *TokenAnalysisRequestSummary) (TokenAnalysisSummaryUpsert, error)
	// ReplacePrefixEdges 在候选版本仍等于期望值时整体替换候选边并确认版本; 版本已变时不写, 返回 false。
	// 带 Query 时在候选锁内查找候选再选边。
	ReplacePrefixEdges(ctx context.Context, write TokenAnalysisPrefixEdgeWrite) (bool, error)
	ListCandidateRematch(ctx context.Context, limit int) ([]TokenAnalysisCandidateRematchRow, error)
	// RecordResponse 写入归档 response 行的完成事实; 请求行尚未入库(跨日文件
	// 逆序索引)时暂存, 由 MergePendingResponses 补合并。
	RecordResponse(ctx context.Context, facts TokenAnalysisResponseFacts) error
	// MergePendingResponses 把已有请求行的暂存完成事实写回, 并清理过期暂存。
	MergePendingResponses(ctx context.Context) error
	// ListUsageLinkCandidates 按 id 续读 [start,end) 内非 count_tokens 的待关联行:
	// 未关联、旧窗口关联、被夺回账单的行, 以及可被精确证据纠正的时序关联行。
	ListUsageLinkCandidates(ctx context.Context, start, end time.Time, afterID int64, limit int) ([]TokenAnalysisUsageLinkCandidate, error)
	FindUsageLink(ctx context.Context, query TokenAnalysisUsageLinkQuery) (*TokenAnalysisUsageMatch, error)
	// SetUsageLink 写入关联结果与重算后的风险; match 为 nil 表示解除关联。按 expected
	// 条件更新, 认领在数据库内按账单加锁原子完成; 除非 KeepPending, 同时确认读到的版本。
	SetUsageLink(ctx context.Context, archiveID string, expected TokenAnalysisUsageLinkState, match *TokenAnalysisUsageMatch, riskScore int, riskReasons []TokenAnalysisRiskReason) (TokenAnalysisUsageLinkWrite, error)
	// ClearUsageLinkPending 在复核后无需改动时确认读到的复核版本; 版本已变化时不生效。
	ClearUsageLinkPending(ctx context.Context, archiveID string, version int64) error
	CountSameBodyRecent(ctx context.Context, bodySHA256 string, userID, apiKeyID *int64, eventTime time.Time, window time.Duration) (int, error)
	GetSummary(ctx context.Context, filters TokenAnalysisFilters) (*TokenAnalysisSummary, error)
	// CountBilledRequests 统计同期 usage_logs 计费请求数(覆盖率分母)。
	CountBilledRequests(ctx context.Context, filters TokenAnalysisFilters) (int64, error)
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
	// UpsertUserInput 留存用户净输入全文; quality_* 仅在 content_sha256 不变时保留。
	UpsertUserInput(ctx context.Context, input *TokenAnalysisUserInput) error
	// DeleteUserInput 删除单条净输入(重新索引确认请求没有用户正文时)。
	DeleteUserInput(ctx context.Context, archiveID string) error
	// GetUserInput 按 archive_id 取单条净输入(无记录返回 nil, nil)。
	GetUserInput(ctx context.Context, archiveID string) (*TokenAnalysisUserInput, error)
	// ListStaleClientFamilies 按 id 续读 afterID 之后已识别(版本非 0)但规则版本不是
	// version 的行, 带回读出时的 UA 与版本快照。
	ListStaleClientFamilies(ctx context.Context, version int32, afterID int64, limit int) ([]TokenAnalysisClientFamilyRow, error)
	// UpdateClientFamilies 写回重新映射的客户端族; 只更新 UA 与版本仍等于读出快照的行,
	// 期间被重新索引或被其他实例改过的行跳过, 由下一轮重新读取。
	UpdateClientFamilies(ctx context.Context, version int32, rows []TokenAnalysisClientFamilyRow) error
	// ListClassificationDue 按 id 续读 afterID 之后分类版本未对齐的行(带当前关联的账单)。
	ListClassificationDue(ctx context.Context, afterID int64, limit int) ([]TokenAnalysisClassificationRow, error)
	// LoadClassificationRows 按 archive_id 读取祖先的分类事实; 不存在的行不返回。
	LoadClassificationRows(ctx context.Context, archiveIDs []string) ([]TokenAnalysisClassificationRow, error)
	// LoadPrefixEdges 读取这些子请求的全部候选边(含关联边)。
	LoadPrefixEdges(ctx context.Context, childArchiveIDs []string) ([]TokenAnalysisStoredEdge, error)
	// WriteClassifications 按设计 10.3 条件写回: 子请求的 cls_input_version 与
	// cand_checked_version 未变, 有父请求时父请求的 cls_input_version 也未变。返回写入行数。
	WriteClassifications(ctx context.Context, results []TokenAnalysisClassificationResult) (int, error)
	// MaxSummaryID 返回摘要表当前最大 id(维护扫描的终点)。
	MaxSummaryID(ctx context.Context) (int64, error)
	// MarkClassificationStale 在 (fromID, toID] 内给已对齐但规则版本不是 ruleVersion 的行加分类版本。
	MarkClassificationStale(ctx context.Context, ruleVersion int32, fromID, toID int64) (int64, error)
	// DrainClassificationInvalidations 处理一批失效登记(账单被删除、外键置空时由触发器写入):
	// 给登记行的子孙加分类版本后删除登记。返回处理条数。
	DrainClassificationInvalidations(ctx context.Context, limit int) (int, error)
}

type tokenAnalysisArchiveEvent struct {
	ArchiveID       string `json:"archive_id"`
	ClientRequestID string `json:"client_request_id"`
	Event           string `json:"event"`
	Timestamp       string `json:"timestamp"`
	// Status/Usage/ResponseID 只出现在 response 行。
	Status     int            `json:"status"`
	Usage      map[string]any `json:"usage"`
	ResponseID string         `json:"response_id"`
	Method     string         `json:"method"`
	Path       string         `json:"path"`
	Endpoint   string         `json:"endpoint"`
	UserID     *int64         `json:"user_id"`
	APIKeyID   *int64         `json:"api_key_id"`
	GroupID    *int64         `json:"group_id"`
	AccountID  *int64         `json:"account_id"`
	Model      string         `json:"model"`
	UserAgent  string         `json:"user_agent"`
	// Headers 只取归档 header 白名单中与解析相关的键: content_type 用于
	// 识别 multipart 归档体(/v1/images/edits 图改图等)。
	Headers struct {
		ContentType string `json:"content_type"`
	} `json:"headers"`
	Body          string `json:"body"`
	BodySize      int64  `json:"body_size"`
	BodySHA256    string `json:"body_sha256"`
	BodyTruncated bool   `json:"body_truncated"`
	// TokenAnalysis 只出现在 response 行: 最后一次转发尝试的缓存相关事实。
	TokenAnalysis *TokenAnalysisForwardFacts `json:"token_analysis"`
}
