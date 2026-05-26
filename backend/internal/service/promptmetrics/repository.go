package promptmetrics

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Repository 使用 raw SQL 隔离 prompt metrics 表结构, 避免引入 ent schema 生成变更.
type Repository struct {
	db *sql.DB
}

// NewRepository 创建 prompt metrics 仓储.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// Insert 写入一条提示词事件. request_id/api_key_id 非空时由唯一索引做幂等保护.
func (r *Repository) Insert(ctx context.Context, event Event) error {
	if r == nil || r.db == nil {
		return nil
	}
	if strings.TrimSpace(event.PromptHash) == "" || strings.TrimSpace(event.PromptExcerpt) == "" {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO user_prompt_events (
    request_id, user_id, api_key_id, group_id,
    model, requested_model, endpoint, source_protocol,
    prompt_text, prompt_excerpt, prompt_hash,
    prompt_chars, prompt_segments, prompt_tokens_estimated,
    project_name, git_branch, client_name, client_version,
    user_agent, ip_address, truncated, analysis_status, created_at
) VALUES (
    $1, $2, $3, $4,
    $5, $6, $7, $8,
    NULLIF($9, ''), $10, $11,
    $12, $13, $14,
    NULLIF($15, ''), NULLIF($16, ''), NULLIF($17, ''), NULLIF($18, ''),
    NULLIF($19, ''), NULLIF($20, ''), $21, $22, $23
)
ON CONFLICT (request_id, api_key_id) WHERE request_id IS NOT NULL AND api_key_id IS NOT NULL DO NOTHING`,
		nullStringValue(event.RequestID),
		nullInt64Ptr(event.UserID),
		nullInt64Ptr(event.APIKeyID),
		nullInt64Ptr(event.GroupID),
		event.Model,
		event.RequestedModel,
		event.Endpoint,
		event.SourceProtocol,
		event.PromptText,
		event.PromptExcerpt,
		event.PromptHash,
		event.PromptChars,
		event.PromptSegments,
		event.PromptTokensEstimated,
		event.ProjectName,
		event.GitBranch,
		event.ClientName,
		event.ClientVersion,
		event.UserAgent,
		event.IPAddress,
		event.Truncated,
		defaultAnalysisStatus(event.AnalysisStatus),
		defaultTime(event.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("insert prompt metrics event: %w", err)
	}
	return nil
}

// Overview 返回总览指标, token 和 cost 通过 usage_logs 弱关联补齐.
func (r *Repository) Overview(ctx context.Context, filters Filters) (*Overview, error) {
	where, args := buildWhere(filters, "e")
	query := `
SELECT
    COUNT(*) AS total_events,
    COUNT(DISTINCT e.user_id) FILTER (WHERE e.user_id IS NOT NULL) AS active_users,
    COUNT(*) FILTER (WHERE a.quality_score < 60) AS low_quality,
    COUNT(*) FILTER (WHERE e.truncated) AS truncated,
    COUNT(*) FILTER (WHERE e.analysis_status = 'pending') AS pending_analysis,
    COALESCE(SUM(COALESCE(ul.input_tokens, 0) + COALESCE(ul.output_tokens, 0) + COALESCE(ul.cache_creation_tokens, 0) + COALESCE(ul.cache_read_tokens, 0)), 0) AS total_tokens,
    COALESCE(SUM(COALESCE(ul.actual_cost, 0)), 0) AS total_cost,
    COALESCE(AVG(a.quality_score), 0) AS average_quality
FROM user_prompt_events e
LEFT JOIN user_prompt_analysis a ON a.prompt_event_id = e.id
LEFT JOIN usage_logs ul ON ul.request_id = e.request_id AND ul.api_key_id = e.api_key_id
` + where
	var overview Overview
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(
		&overview.TotalEvents,
		&overview.ActiveUsers,
		&overview.LowQuality,
		&overview.Truncated,
		&overview.PendingAnalysis,
		&overview.TotalTokens,
		&overview.TotalCost,
		&overview.AverageQuality,
	); err != nil {
		return nil, fmt.Errorf("query prompt metrics overview: %w", err)
	}
	return &overview, nil
}

// Trend 按 day 或 hour 聚合趋势. bucket 只接受白名单, 防止 SQL 注入.
func (r *Repository) Trend(ctx context.Context, filters Filters, bucket string) ([]TrendPoint, error) {
	dateExpr := "YYYY-MM-DD"
	if bucket == "hour" {
		dateExpr = "YYYY-MM-DD HH24:00"
	}
	where, args := buildWhere(filters, "e")
	query := fmt.Sprintf(`
SELECT
    TO_CHAR(date_trunc('%s', e.created_at), '%s') AS bucket,
    COUNT(*) AS events,
    COUNT(DISTINCT e.user_id) FILTER (WHERE e.user_id IS NOT NULL) AS users,
    COALESCE(SUM(COALESCE(ul.input_tokens, 0) + COALESCE(ul.output_tokens, 0) + COALESCE(ul.cache_creation_tokens, 0) + COALESCE(ul.cache_read_tokens, 0)), 0) AS tokens,
    COALESCE(SUM(COALESCE(ul.actual_cost, 0)), 0) AS cost,
    COALESCE(AVG(a.quality_score), 0) AS avg_quality,
    COUNT(*) FILTER (WHERE a.quality_score < 60) AS low_quality,
    COUNT(*) FILTER (WHERE e.analysis_status = 'pending') AS pending_count
FROM user_prompt_events e
LEFT JOIN user_prompt_analysis a ON a.prompt_event_id = e.id
LEFT JOIN usage_logs ul ON ul.request_id = e.request_id AND ul.api_key_id = e.api_key_id
%s
GROUP BY date_trunc('%s', e.created_at)
ORDER BY date_trunc('%s', e.created_at) ASC`, bucketGranularity(bucket), dateExpr, where, bucketGranularity(bucket), bucketGranularity(bucket))
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query prompt metrics trend: %w", err)
	}
	defer rows.Close()
	points := make([]TrendPoint, 0)
	for rows.Next() {
		var point TrendPoint
		if err := rows.Scan(&point.Bucket, &point.Events, &point.Users, &point.Tokens, &point.Cost, &point.AvgQuality, &point.LowQuality, &point.PendingCount); err != nil {
			return nil, err
		}
		points = append(points, point)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return points, nil
}

// Rank 返回指定维度的排行. dimension 会映射到固定 SQL 表达式, 不拼接用户原始输入.
func (r *Repository) Rank(ctx context.Context, filters Filters, dimension string, limit int) ([]RankItem, error) {
	expr, labelExpr, ok := rankDimensionExpr(dimension)
	if !ok {
		return nil, fmt.Errorf("invalid rank dimension")
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	where, args := buildWhere(filters, "e")
	args = append(args, limit)
	query := fmt.Sprintf(`
SELECT
    COALESCE(%s, '') AS key,
    COALESCE(%s, '') AS label,
    COUNT(*) AS events,
    COUNT(DISTINCT e.user_id) FILTER (WHERE e.user_id IS NOT NULL) AS users,
    COALESCE(SUM(COALESCE(ul.input_tokens, 0) + COALESCE(ul.output_tokens, 0) + COALESCE(ul.cache_creation_tokens, 0) + COALESCE(ul.cache_read_tokens, 0)), 0) AS tokens,
    COALESCE(SUM(COALESCE(ul.actual_cost, 0)), 0) AS cost,
    COALESCE(AVG(a.quality_score), 0) AS avg_quality
FROM user_prompt_events e
LEFT JOIN user_prompt_analysis a ON a.prompt_event_id = e.id
LEFT JOIN usage_logs ul ON ul.request_id = e.request_id AND ul.api_key_id = e.api_key_id
LEFT JOIN users u ON u.id = e.user_id
%s
GROUP BY %s, %s
ORDER BY events DESC, cost DESC
LIMIT $%d`, expr, labelExpr, where, expr, labelExpr, len(args))
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query prompt metrics rank: %w", err)
	}
	defer rows.Close()
	items := make([]RankItem, 0)
	for rows.Next() {
		var item RankItem
		if err := rows.Scan(&item.Key, &item.Label, &item.Events, &item.Users, &item.Tokens, &item.Cost, &item.AvgQuality); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

// ListEvents 返回分页事件列表, 默认按创建时间倒序.
func (r *Repository) ListEvents(ctx context.Context, filters Filters, page, pageSize int) ([]Event, Pagination, error) {
	page, pageSize = normalizePage(page, pageSize)
	where, args := buildWhere(filters, "e")
	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM user_prompt_events e LEFT JOIN user_prompt_analysis a ON a.prompt_event_id = e.id "+where, args...).Scan(&total); err != nil {
		return nil, Pagination{}, fmt.Errorf("count prompt metrics events: %w", err)
	}
	args = append(args, pageSize, (page-1)*pageSize)
	query := eventSelectSQL() + where + fmt.Sprintf(" ORDER BY e.created_at DESC, e.id DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, Pagination{}, fmt.Errorf("list prompt metrics events: %w", err)
	}
	defer rows.Close()
	events, err := scanEvents(rows)
	if err != nil {
		return nil, Pagination{}, err
	}
	return events, Pagination{Page: page, PageSize: pageSize, Total: total}, nil
}

// EventByID 查询单条事件详情, 包含全文和分析结果.
func (r *Repository) EventByID(ctx context.Context, id int64) (*Event, error) {
	rows, err := r.db.QueryContext(ctx, eventSelectSQL()+" WHERE e.id = $1", id)
	if err != nil {
		return nil, fmt.Errorf("query prompt metrics event: %w", err)
	}
	defer rows.Close()
	events, err := scanEvents(rows)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, sql.ErrNoRows
	}
	return &events[0], nil
}

// UpsertAnalysis 写入本地规则分析结果, 并同步事件分析状态.
func (r *Repository) UpsertAnalysis(ctx context.Context, analysis Analysis) error {
	categories, err := json.Marshal(analysis.Categories)
	if err != nil {
		return fmt.Errorf("marshal prompt analysis categories: %w", err)
	}
	suggestions, err := json.Marshal(analysis.ImprovementSuggestions)
	if err != nil {
		return fmt.Errorf("marshal prompt analysis suggestions: %w", err)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	_, err = tx.ExecContext(ctx, `
INSERT INTO user_prompt_analysis (
    prompt_event_id, summary, quality_score, clarity_score, context_score,
    actionability_score, constraint_score, risk_score, categories,
    improvement_suggestions, analyzer_model, analyzed_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10::jsonb, $11, $12)
ON CONFLICT (prompt_event_id) DO UPDATE SET
    summary = EXCLUDED.summary,
    quality_score = EXCLUDED.quality_score,
    clarity_score = EXCLUDED.clarity_score,
    context_score = EXCLUDED.context_score,
    actionability_score = EXCLUDED.actionability_score,
    constraint_score = EXCLUDED.constraint_score,
    risk_score = EXCLUDED.risk_score,
    categories = EXCLUDED.categories,
    improvement_suggestions = EXCLUDED.improvement_suggestions,
    analyzer_model = EXCLUDED.analyzer_model,
    analyzed_at = EXCLUDED.analyzed_at,
    updated_at = NOW()`,
		analysis.PromptEventID,
		analysis.Summary,
		analysis.QualityScore,
		analysis.ClarityScore,
		analysis.ContextScore,
		analysis.ActionabilityScore,
		analysis.ConstraintScore,
		analysis.RiskScore,
		string(categories),
		string(suggestions),
		analysis.AnalyzerModel,
		defaultTime(analysis.AnalyzedAt),
	)
	if err != nil {
		return fmt.Errorf("upsert prompt analysis: %w", err)
	}
	if _, err = tx.ExecContext(ctx, "UPDATE user_prompt_events SET analysis_status = $1, updated_at = NOW() WHERE id = $2", AnalysisStatusDone, analysis.PromptEventID); err != nil {
		return fmt.Errorf("update prompt analysis status: %w", err)
	}
	return tx.Commit()
}

// MarkAnalysisStatus 更新分析状态, 用于 reanalyze 开始和失败路径.
func (r *Repository) MarkAnalysisStatus(ctx context.Context, id int64, status string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE user_prompt_events SET analysis_status = $1, updated_at = NOW() WHERE id = $2", defaultAnalysisStatus(status), id)
	if err != nil {
		return fmt.Errorf("mark prompt analysis status: %w", err)
	}
	return nil
}

func eventSelectSQL() string {
	return `
SELECT
    e.id, e.request_id, e.user_id, e.api_key_id, e.group_id,
    e.model, e.requested_model, e.endpoint, e.source_protocol,
    e.prompt_text, e.prompt_excerpt, e.prompt_hash,
    e.prompt_chars, e.prompt_segments, e.prompt_tokens_estimated,
    e.project_name, e.git_branch, e.client_name, e.client_version,
    e.user_agent, e.ip_address, e.truncated, e.analysis_status, e.created_at,
    COALESCE(ul.input_tokens, 0), COALESCE(ul.output_tokens, 0),
    COALESCE(ul.cache_creation_tokens, 0), COALESCE(ul.cache_read_tokens, 0),
    COALESCE(ul.actual_cost, 0),
    COALESCE(u.email, ''), COALESCE(k.name, ''), COALESCE(g.name, ''),
    a.summary, a.quality_score, a.clarity_score, a.context_score,
    a.actionability_score, a.constraint_score, a.risk_score,
    a.categories, a.improvement_suggestions, a.analyzer_model, a.analyzed_at
FROM user_prompt_events e
LEFT JOIN usage_logs ul ON ul.request_id = e.request_id AND ul.api_key_id = e.api_key_id
LEFT JOIN users u ON u.id = e.user_id
LEFT JOIN api_keys k ON k.id = e.api_key_id
LEFT JOIN groups g ON g.id = e.group_id
LEFT JOIN user_prompt_analysis a ON a.prompt_event_id = e.id
`
}

func scanEvents(rows *sql.Rows) ([]Event, error) {
	events := make([]Event, 0)
	for rows.Next() {
		var event Event
		var requestID, model, requestedModel, endpoint, protocol, promptText, project, branch, client, clientVersion, userAgent, ipAddress sql.NullString
		var userID, apiKeyID, groupID sql.NullInt64
		var summary, categories, suggestions, analyzer sql.NullString
		var quality, clarity, contextScore, actionability, constraint, risk sql.NullInt64
		var analyzedAt sql.NullTime
		if err := rows.Scan(
			&event.ID, &requestID, &userID, &apiKeyID, &groupID,
			&model, &requestedModel, &endpoint, &protocol,
			&promptText, &event.PromptExcerpt, &event.PromptHash,
			&event.PromptChars, &event.PromptSegments, &event.PromptTokensEstimated,
			&project, &branch, &client, &clientVersion,
			&userAgent, &ipAddress, &event.Truncated, &event.AnalysisStatus, &event.CreatedAt,
			&event.InputTokens, &event.OutputTokens, &event.CacheCreationTokens, &event.CacheReadTokens,
			&event.ActualCost,
			&event.UserEmail, &event.APIKeyName, &event.GroupName,
			&summary, &quality, &clarity, &contextScore,
			&actionability, &constraint, &risk,
			&categories, &suggestions, &analyzer, &analyzedAt,
		); err != nil {
			return nil, err
		}
		event.TotalTokens = event.InputTokens + event.OutputTokens + event.CacheCreationTokens + event.CacheReadTokens
		event.RequestID = nullStringToString(requestID)
		event.Model = nullStringToString(model)
		event.RequestedModel = nullStringToString(requestedModel)
		event.Endpoint = nullStringToString(endpoint)
		event.SourceProtocol = nullStringToString(protocol)
		event.PromptText = nullStringToString(promptText)
		event.ProjectName = nullStringToString(project)
		event.GitBranch = nullStringToString(branch)
		event.ClientName = nullStringToString(client)
		event.ClientVersion = nullStringToString(clientVersion)
		event.UserAgent = nullStringToString(userAgent)
		event.IPAddress = nullStringToString(ipAddress)
		event.UserID = nullInt64ToPtr(userID)
		event.APIKeyID = nullInt64ToPtr(apiKeyID)
		event.GroupID = nullInt64ToPtr(groupID)
		if summary.Valid {
			event.Analysis = &Analysis{
				PromptEventID:          event.ID,
				Summary:                summary.String,
				QualityScore:           int(quality.Int64),
				ClarityScore:           int(clarity.Int64),
				ContextScore:           int(contextScore.Int64),
				ActionabilityScore:     int(actionability.Int64),
				ConstraintScore:        int(constraint.Int64),
				RiskScore:              int(risk.Int64),
				Categories:             jsonStringSlice(categories),
				ImprovementSuggestions: jsonStringSlice(suggestions),
				AnalyzerModel:          nullStringToString(analyzer),
			}
			if analyzedAt.Valid {
				event.Analysis.AnalyzedAt = analyzedAt.Time
			}
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func buildWhere(filters Filters, alias string) (string, []any) {
	conditions := make([]string, 0)
	args := make([]any, 0)
	add := func(condition string, value any) {
		args = append(args, value)
		conditions = append(conditions, fmt.Sprintf(condition, len(args)))
	}
	if filters.From != nil {
		add(alias+".created_at >= $%d", *filters.From)
	}
	if filters.To != nil {
		add(alias+".created_at <= $%d", *filters.To)
	}
	if filters.UserID != nil {
		add(alias+".user_id = $%d", *filters.UserID)
	}
	if filters.APIKeyID != nil {
		add(alias+".api_key_id = $%d", *filters.APIKeyID)
	}
	if filters.GroupID != nil {
		add(alias+".group_id = $%d", *filters.GroupID)
	}
	if filters.ProjectName != "" {
		add(alias+".project_name = $%d", filters.ProjectName)
	}
	if filters.GitBranch != "" {
		add(alias+".git_branch = $%d", filters.GitBranch)
	}
	if filters.ClientName != "" {
		add(alias+".client_name = $%d", filters.ClientName)
	}
	if filters.Model != "" {
		add(alias+".requested_model = $%d", filters.Model)
	}
	if filters.Endpoint != "" {
		add(alias+".endpoint = $%d", filters.Endpoint)
	}
	if filters.Hash != "" {
		add(alias+".prompt_hash = $%d", filters.Hash)
	}
	if filters.MinQuality != nil {
		add("a.quality_score >= $%d", *filters.MinQuality)
	}
	if filters.MaxQuality != nil {
		add("a.quality_score <= $%d", *filters.MaxQuality)
	}
	if filters.OnlyLowQuality {
		conditions = append(conditions, "a.quality_score < 60")
	}
	if len(conditions) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}

func rankDimensionExpr(dimension string) (string, string, bool) {
	switch strings.ToLower(strings.TrimSpace(dimension)) {
	case "user":
		return "e.user_id::text", "COALESCE(u.email, e.user_id::text)", true
	case "project":
		return "e.project_name", "e.project_name", true
	case "branch":
		return "e.git_branch", "e.git_branch", true
	case "client":
		return "e.client_name", "e.client_name", true
	case "model":
		return "e.requested_model", "e.requested_model", true
	default:
		return "", "", false
	}
}

func bucketGranularity(bucket string) string {
	if bucket == "hour" {
		return "hour"
	}
	return "day"
}

func normalizePage(page, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 1000 {
		pageSize = 1000
	}
	return page, pageSize
}

func defaultAnalysisStatus(status string) string {
	switch status {
	case AnalysisStatusPending, AnalysisStatusDone, AnalysisStatusFailed, AnalysisStatusSkipped:
		return status
	default:
		return AnalysisStatusPending
	}
}

func defaultTime(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now()
	}
	return t
}

func nullStringValue(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return strings.TrimSpace(v)
}

func nullInt64Ptr(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullStringToString(v sql.NullString) string {
	if !v.Valid {
		return ""
	}
	return v.String
}

func nullInt64ToPtr(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	value := v.Int64
	return &value
}

func jsonStringSlice(v sql.NullString) []string {
	if !v.Valid || strings.TrimSpace(v.String) == "" {
		return nil
	}
	out := make([]string, 0)
	if err := json.Unmarshal([]byte(v.String), &out); err != nil {
		return nil
	}
	return out
}

func isNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}
