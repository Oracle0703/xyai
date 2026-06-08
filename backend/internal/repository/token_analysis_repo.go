package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type tokenAnalysisRepository struct {
	db *sql.DB
}

func NewTokenAnalysisRepository(db *sql.DB) service.TokenAnalysisRepository {
	return &tokenAnalysisRepository{db: db}
}

func (r *tokenAnalysisRepository) UpsertRequestSummary(ctx context.Context, summary *service.TokenAnalysisRequestSummary) error {
	if summary == nil {
		return nil
	}
	summaryJSON, err := json.Marshal(nonNilMap(summary.SummaryJSON))
	if err != nil {
		return fmt.Errorf("marshal token analysis summary json: %w", err)
	}
	riskReasons, err := json.Marshal(summary.RiskReasons)
	if err != nil {
		return fmt.Errorf("marshal token analysis risk reasons: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
INSERT INTO token_analysis_request_summaries (
    archive_id, usage_log_id, match_confidence, event_time,
    user_id, api_key_id, account_id, group_id,
    model, endpoint, method,
    request_body_size, request_body_truncated, body_sha256,
    message_count, system_chars, user_chars, last_user_preview, tools_count, image_count,
    summary_json, risk_score, risk_reasons, source_file, source_offset,
    client_workdir, client_project, client_branch, attribution_source
) VALUES (
    $1, $2, $3, $4,
    $5, $6, $7, $8,
    $9, $10, $11,
    $12, $13, $14,
    $15, $16, $17, $18, $19, $20,
    $21::jsonb, $22, $23::jsonb, $24, $25,
    $26, $27, $28, $29
)
ON CONFLICT (archive_id) DO UPDATE SET
    usage_log_id = EXCLUDED.usage_log_id,
    match_confidence = EXCLUDED.match_confidence,
    event_time = EXCLUDED.event_time,
    user_id = EXCLUDED.user_id,
    api_key_id = EXCLUDED.api_key_id,
    account_id = EXCLUDED.account_id,
    group_id = EXCLUDED.group_id,
    model = EXCLUDED.model,
    endpoint = EXCLUDED.endpoint,
    method = EXCLUDED.method,
    request_body_size = EXCLUDED.request_body_size,
    request_body_truncated = EXCLUDED.request_body_truncated,
    body_sha256 = EXCLUDED.body_sha256,
    message_count = EXCLUDED.message_count,
    system_chars = EXCLUDED.system_chars,
    user_chars = EXCLUDED.user_chars,
    last_user_preview = EXCLUDED.last_user_preview,
    tools_count = EXCLUDED.tools_count,
    image_count = EXCLUDED.image_count,
    summary_json = EXCLUDED.summary_json,
    risk_score = EXCLUDED.risk_score,
    risk_reasons = EXCLUDED.risk_reasons,
    indexed_at = NOW(),
    source_file = EXCLUDED.source_file,
    source_offset = EXCLUDED.source_offset,
    client_workdir = EXCLUDED.client_workdir,
    client_project = EXCLUDED.client_project,
    client_branch = EXCLUDED.client_branch,
    attribution_source = EXCLUDED.attribution_source`,
		summary.ArchiveID,
		nullableInt64(summary.UsageLogID),
		summary.MatchConfidence,
		summary.EventTime,
		nullableInt64(summary.UserID),
		nullableInt64(summary.APIKeyID),
		nullableInt64(summary.AccountID),
		nullableInt64(summary.GroupID),
		summary.Model,
		summary.Endpoint,
		summary.Method,
		summary.RequestBodySize,
		summary.RequestBodyTruncated,
		summary.BodySHA256,
		summary.MessageCount,
		summary.SystemChars,
		summary.UserChars,
		summary.LastUserPreview,
		summary.ToolsCount,
		summary.ImageCount,
		string(summaryJSON),
		summary.RiskScore,
		string(riskReasons),
		summary.SourceFile,
		nullableInt64(summary.SourceOffset),
		summary.ClientWorkdir,
		summary.ClientProject,
		summary.ClientBranch,
		summary.AttributionSource,
	)
	if err != nil {
		return fmt.Errorf("upsert token analysis request summary: %w", err)
	}
	return nil
}

func (r *tokenAnalysisRepository) FindNearestUsageLog(ctx context.Context, eventTime time.Time, userID, apiKeyID *int64, model string, window time.Duration) (*service.TokenAnalysisUsageMatch, error) {
	start := eventTime.Add(-window)
	end := eventTime.Add(window)
	args := []any{start, end, eventTime}
	where := []string{"created_at >= $1", "created_at <= $2"}
	confidenceParts := make([]string, 0, 3)
	if userID != nil {
		args = append(args, *userID)
		placeholder := fmt.Sprintf("$%d", len(args))
		where = append(where, "user_id = "+placeholder)
		confidenceParts = append(confidenceParts, "1")
	}
	if apiKeyID != nil {
		args = append(args, *apiKeyID)
		placeholder := fmt.Sprintf("$%d", len(args))
		where = append(where, "api_key_id = "+placeholder)
		confidenceParts = append(confidenceParts, "1")
	}
	if strings.TrimSpace(model) != "" {
		args = append(args, strings.TrimSpace(model))
		placeholder := fmt.Sprintf("$%d", len(args))
		where = append(where, "(model = "+placeholder+" OR requested_model = "+placeholder+" OR upstream_model = "+placeholder+")")
		confidenceParts = append(confidenceParts, "1")
	}
	confidenceExpr := "1"
	if len(confidenceParts) > 0 {
		confidenceExpr = strings.Join(confidenceParts, " + ")
	}

	query := `
SELECT
    id,
    input_tokens,
    output_tokens,
    cache_read_tokens,
    cache_creation_tokens,
    total_cost,
    actual_cost,
    (` + confidenceExpr + `)::smallint AS confidence
FROM usage_logs
WHERE ` + strings.Join(where, " AND ") + `
ORDER BY ABS(EXTRACT(EPOCH FROM (created_at - $3::timestamptz))) ASC, id DESC
LIMIT 1`

	var match service.TokenAnalysisUsageMatch
	err := r.db.QueryRowContext(ctx, query, args...).Scan(
		&match.UsageLogID,
		&match.InputTokens,
		&match.OutputTokens,
		&match.CacheReadTokens,
		&match.CacheCreationTokens,
		&match.TotalCost,
		&match.ActualCost,
		&match.MatchConfidence,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find nearest usage log: %w", err)
	}
	return &match, nil
}

func (r *tokenAnalysisRepository) CountSameBodyRecent(ctx context.Context, bodySHA256 string, userID, apiKeyID *int64, eventTime time.Time, window time.Duration) (int, error) {
	if strings.TrimSpace(bodySHA256) == "" {
		return 0, nil
	}
	args := []any{bodySHA256, eventTime.Add(-window), eventTime.Add(window)}
	where := []string{"body_sha256 = $1", "event_time >= $2", "event_time <= $3"}
	if userID != nil {
		args = append(args, *userID)
		where = append(where, fmt.Sprintf("user_id = $%d", len(args)))
	}
	if apiKeyID != nil {
		args = append(args, *apiKeyID)
		where = append(where, fmt.Sprintf("api_key_id = $%d", len(args)))
	}
	var count int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM token_analysis_request_summaries WHERE "+strings.Join(where, " AND "), args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count same body recent: %w", err)
	}
	return count, nil
}

func (r *tokenAnalysisRepository) GetSummary(ctx context.Context, filters service.TokenAnalysisFilters) (*service.TokenAnalysisSummary, error) {
	where, args := buildTokenAnalysisWhere(filters, "s")
	query := `
SELECT
    COUNT(*) AS total_requests,
    COUNT(s.usage_log_id) AS matched_requests,
    COALESCE(SUM(ul.input_tokens), 0) AS total_input_tokens,
    COALESCE(SUM(ul.output_tokens), 0) AS total_output_tokens,
    COALESCE(SUM(ul.cache_read_tokens), 0) AS cache_read_tokens,
    COALESCE(SUM(ul.cache_creation_tokens), 0) AS cache_creation_tokens,
    COALESCE(SUM(ul.input_tokens + ul.output_tokens + ul.cache_read_tokens + ul.cache_creation_tokens), 0) AS total_tokens,
    COALESCE(SUM(ul.total_cost), 0) AS total_cost,
    COALESCE(SUM(ul.actual_cost), 0) AS total_actual_cost,
    COUNT(*) FILTER (WHERE s.risk_score > 0) AS risky_requests,
    COALESCE(SUM(ul.actual_cost) FILTER (WHERE s.risk_score > 0), 0) AS risky_cost,
    COALESCE((
        SELECT jsonb_agg(jsonb_build_object('code', code, 'count', count) ORDER BY count DESC, code ASC)
        FROM (
            SELECT reason->>'code' AS code, COUNT(*) AS count
            FROM token_analysis_request_summaries reason_source
            LEFT JOIN usage_logs reason_ul ON reason_ul.id = reason_source.usage_log_id
            CROSS JOIN LATERAL jsonb_array_elements(reason_source.risk_reasons) AS reason
            WHERE ` + strings.Join(rewriteTokenAnalysisAlias(where, "s", "reason_source"), " AND ") + `
              AND reason->>'code' <> ''
            GROUP BY reason->>'code'
            ORDER BY count DESC, code ASC
            LIMIT 8
        ) ranked_reasons
    ), '[]'::jsonb) AS risk_reasons
FROM token_analysis_request_summaries s
LEFT JOIN usage_logs ul ON ul.id = s.usage_log_id
WHERE ` + strings.Join(where, " AND ")

	var summary service.TokenAnalysisSummary
	var riskReasonsRaw []byte
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(
		&summary.TotalRequests,
		&summary.MatchedRequests,
		&summary.TotalInputTokens,
		&summary.TotalOutputTokens,
		&summary.CacheReadTokens,
		&summary.CacheCreationTokens,
		&summary.TotalTokens,
		&summary.TotalCost,
		&summary.TotalActualCost,
		&summary.RiskyRequests,
		&summary.RiskyCost,
		&riskReasonsRaw,
	); err != nil {
		return nil, fmt.Errorf("get token analysis summary: %w", err)
	}
	summary.UnmatchedRequests = summary.TotalRequests - summary.MatchedRequests
	if summary.TotalRequests > 0 {
		summary.UnmatchedRate = float64(summary.UnmatchedRequests) / float64(summary.TotalRequests)
		summary.RiskRequestRate = float64(summary.RiskyRequests) / float64(summary.TotalRequests)
	}
	cacheable := summary.TotalInputTokens + summary.CacheReadTokens + summary.CacheCreationTokens
	if cacheable > 0 {
		summary.CacheHitRate = float64(summary.CacheReadTokens) / float64(cacheable)
	}
	summary.RiskReasons = []service.TokenAnalysisRiskReasonSummary{}
	_ = json.Unmarshal(riskReasonsRaw, &summary.RiskReasons)
	return &summary, nil
}

func (r *tokenAnalysisRepository) ListUserUsage(ctx context.Context, filters service.TokenAnalysisFilters, params pagination.PaginationParams) ([]service.TokenAnalysisUserUsage, *pagination.PaginationResult, error) {
	where, args := buildTokenAnalysisWhere(filters, "s")
	whereSQL := "WHERE " + strings.Join(where, " AND ")
	countQuery := `
SELECT COUNT(*) FROM (
    SELECT s.user_id, s.api_key_id
    FROM token_analysis_request_summaries s
    LEFT JOIN usage_logs ul ON ul.id = s.usage_log_id
    ` + whereSQL + `
    GROUP BY s.user_id, s.api_key_id
) grouped`
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, nil, fmt.Errorf("count token analysis user usage: %w", err)
	}

	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, params.Limit(), params.Offset())
	query := `
SELECT
    s.user_id,
    COALESCE(u.email, '') AS user_email,
    s.api_key_id,
    COALESCE(k.name, '') AS api_key_name,
    COUNT(*) AS request_count,
    COUNT(*) FILTER (WHERE s.risk_score > 0) AS risky_request_count,
    COALESCE(SUM(ul.input_tokens + ul.output_tokens + ul.cache_read_tokens + ul.cache_creation_tokens), 0) AS total_tokens,
    COALESCE(SUM(ul.input_tokens), 0) AS input_tokens,
    COALESCE(SUM(ul.output_tokens), 0) AS output_tokens,
    COALESCE(SUM(ul.cache_read_tokens), 0) AS cache_read_tokens,
    COALESCE(SUM(ul.cache_creation_tokens), 0) AS cache_creation_tokens,
    COALESCE(SUM(ul.actual_cost), 0) AS actual_cost,
    COALESCE(SUM(ul.actual_cost) FILTER (WHERE s.risk_score > 0), 0) AS risky_cost,
    MAX(s.event_time) AS last_event_time
FROM token_analysis_request_summaries s
LEFT JOIN usage_logs ul ON ul.id = s.usage_log_id
LEFT JOIN users u ON u.id = s.user_id
LEFT JOIN api_keys k ON k.id = s.api_key_id
` + whereSQL + `
GROUP BY s.user_id, u.email, s.api_key_id, k.name
ORDER BY actual_cost DESC, total_tokens DESC, request_count DESC
LIMIT $` + fmt.Sprint(len(queryArgs)-1) + ` OFFSET $` + fmt.Sprint(len(queryArgs))

	rows, err := r.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, nil, fmt.Errorf("list token analysis user usage: %w", err)
	}
	defer rows.Close()

	items := make([]service.TokenAnalysisUserUsage, 0)
	for rows.Next() {
		var item service.TokenAnalysisUserUsage
		var userID, apiKeyID sql.NullInt64
		var lastEvent sql.NullTime
		if err := rows.Scan(
			&userID,
			&item.UserEmail,
			&apiKeyID,
			&item.APIKeyName,
			&item.RequestCount,
			&item.RiskyRequestCount,
			&item.TotalTokens,
			&item.InputTokens,
			&item.OutputTokens,
			&item.CacheReadTokens,
			&item.CacheCreationTokens,
			&item.ActualCost,
			&item.RiskyCost,
			&lastEvent,
		); err != nil {
			return nil, nil, fmt.Errorf("scan token analysis user usage: %w", err)
		}
		if userID.Valid {
			item.UserID = &userID.Int64
		}
		if apiKeyID.Valid {
			item.APIKeyID = &apiKeyID.Int64
		}
		if lastEvent.Valid {
			item.LastEventTime = &lastEvent.Time
		}
		cacheable := item.InputTokens + item.CacheReadTokens + item.CacheCreationTokens
		if cacheable > 0 {
			item.CacheHitRate = float64(item.CacheReadTokens) / float64(cacheable)
		}
		if item.RequestCount > 0 {
			item.RiskRatio = float64(item.RiskyRequestCount) / float64(item.RequestCount)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate token analysis user usage: %w", err)
	}
	return items, paginationResultFromTotal(total, params), nil
}

// ListProjectUsage 按 "项目 × 成员" 聚合 token 消耗。client_project 为空的行
// 归入 unattributed 桶, 在结果中显式呈现而不是悄悄丢弃。
func (r *tokenAnalysisRepository) ListProjectUsage(ctx context.Context, filters service.TokenAnalysisFilters, params pagination.PaginationParams) ([]service.TokenAnalysisProjectUsage, *pagination.PaginationResult, error) {
	where, args := buildTokenAnalysisWhere(filters, "s")
	whereSQL := "WHERE " + strings.Join(where, " AND ")
	countQuery := `
SELECT COUNT(*) FROM (
    SELECT s.client_project, s.user_id
    FROM token_analysis_request_summaries s
    LEFT JOIN usage_logs ul ON ul.id = s.usage_log_id
    ` + whereSQL + `
    GROUP BY s.client_project, s.user_id
) grouped`
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, nil, fmt.Errorf("count token analysis project usage: %w", err)
	}

	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, params.Limit(), params.Offset())
	query := `
SELECT
    CASE WHEN s.client_project = '' THEN 'unattributed' ELSE s.client_project END AS project,
    s.user_id,
    COALESCE(u.email, '') AS user_email,
    COUNT(*) AS request_count,
    COUNT(s.usage_log_id) AS matched_request_count,
    COALESCE(SUM(ul.input_tokens + ul.output_tokens + ul.cache_read_tokens + ul.cache_creation_tokens), 0) AS total_tokens,
    COALESCE(SUM(ul.input_tokens), 0) AS input_tokens,
    COALESCE(SUM(ul.output_tokens), 0) AS output_tokens,
    COALESCE(SUM(ul.cache_read_tokens), 0) AS cache_read_tokens,
    COALESCE(SUM(ul.cache_creation_tokens), 0) AS cache_creation_tokens,
    COALESCE(SUM(ul.actual_cost), 0) AS actual_cost,
    MAX(s.event_time) AS last_event_time
FROM token_analysis_request_summaries s
LEFT JOIN usage_logs ul ON ul.id = s.usage_log_id
LEFT JOIN users u ON u.id = s.user_id
` + whereSQL + `
GROUP BY s.client_project, s.user_id, u.email
ORDER BY total_tokens DESC, actual_cost DESC, request_count DESC
LIMIT $` + fmt.Sprint(len(queryArgs)-1) + ` OFFSET $` + fmt.Sprint(len(queryArgs))

	rows, err := r.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, nil, fmt.Errorf("list token analysis project usage: %w", err)
	}
	defer rows.Close()

	items := make([]service.TokenAnalysisProjectUsage, 0)
	for rows.Next() {
		var item service.TokenAnalysisProjectUsage
		var userID sql.NullInt64
		var lastEvent sql.NullTime
		if err := rows.Scan(
			&item.Project,
			&userID,
			&item.UserEmail,
			&item.RequestCount,
			&item.MatchedRequestCount,
			&item.TotalTokens,
			&item.InputTokens,
			&item.OutputTokens,
			&item.CacheReadTokens,
			&item.CacheCreationTokens,
			&item.ActualCost,
			&lastEvent,
		); err != nil {
			return nil, nil, fmt.Errorf("scan token analysis project usage: %w", err)
		}
		if userID.Valid {
			item.UserID = &userID.Int64
		}
		if lastEvent.Valid {
			item.LastEventTime = &lastEvent.Time
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate token analysis project usage: %w", err)
	}
	return items, paginationResultFromTotal(total, params), nil
}

func (r *tokenAnalysisRepository) ListRequests(ctx context.Context, filters service.TokenAnalysisFilters, params pagination.PaginationParams) ([]service.TokenAnalysisRequestItem, *pagination.PaginationResult, error) {
	where, args := buildTokenAnalysisWhere(filters, "s")
	whereSQL := "WHERE " + strings.Join(where, " AND ")

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM token_analysis_request_summaries s LEFT JOIN usage_logs ul ON ul.id = s.usage_log_id "+whereSQL, args...).Scan(&total); err != nil {
		return nil, nil, fmt.Errorf("count token analysis requests: %w", err)
	}

	orderBy := tokenAnalysisRequestOrderBy(params)
	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, params.Limit(), params.Offset())
	query := `
SELECT
    s.id, s.archive_id, s.usage_log_id, s.match_confidence, s.event_time,
    s.user_id, COALESCE(u.email, '') AS user_email,
    s.api_key_id, COALESCE(k.name, '') AS api_key_name,
    s.account_id, s.group_id, s.model, s.endpoint, s.method,
    s.request_body_size, s.request_body_truncated,
    s.message_count, s.system_chars, s.user_chars, s.last_user_preview, s.tools_count, s.image_count,
    s.summary_json, s.risk_score, s.risk_reasons,
    s.client_workdir, s.client_project, s.client_branch, s.attribution_source,
    COALESCE(ul.input_tokens, 0), COALESCE(ul.output_tokens, 0), COALESCE(ul.cache_read_tokens, 0), COALESCE(ul.cache_creation_tokens, 0),
    COALESCE(ul.input_tokens + ul.output_tokens + ul.cache_read_tokens + ul.cache_creation_tokens, 0), COALESCE(ul.actual_cost, 0),
    ui.archive_id IS NOT NULL AS has_input, COALESCE(ui.truncated, FALSE), ui.quality_score,
    COUNT(*) OVER (PARTITION BY COALESCE(ui.content_sha256, s.archive_id)) AS duplicate_count
FROM token_analysis_request_summaries s
LEFT JOIN usage_logs ul ON ul.id = s.usage_log_id
LEFT JOIN users u ON u.id = s.user_id
LEFT JOIN api_keys k ON k.id = s.api_key_id
LEFT JOIN token_analysis_user_inputs ui ON ui.archive_id = s.archive_id
` + whereSQL + `
ORDER BY ` + orderBy + `
LIMIT $` + fmt.Sprint(len(queryArgs)-1) + ` OFFSET $` + fmt.Sprint(len(queryArgs))

	rows, err := r.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, nil, fmt.Errorf("list token analysis requests: %w", err)
	}
	defer rows.Close()

	items := make([]service.TokenAnalysisRequestItem, 0)
	for rows.Next() {
		item, err := scanTokenAnalysisRequestItem(rows)
		if err != nil {
			return nil, nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate token analysis requests: %w", err)
	}
	return items, paginationResultFromTotal(total, params), nil
}

func (r *tokenAnalysisRepository) GetIndexStatus(ctx context.Context) (*service.TokenAnalysisIndexStatus, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT source_file, last_offset, last_archive_id, processed_rows, failed_rows, last_error, started_at, finished_at, updated_at
FROM token_analysis_index_state
ORDER BY updated_at DESC
LIMIT 50`)
	if err != nil {
		return nil, fmt.Errorf("get token analysis index status: %w", err)
	}
	defer rows.Close()

	status := &service.TokenAnalysisIndexStatus{Files: []service.TokenAnalysisIndexState{}}
	for rows.Next() {
		var state service.TokenAnalysisIndexState
		if err := rows.Scan(
			&state.SourceFile,
			&state.LastOffset,
			&state.LastArchiveID,
			&state.ProcessedRows,
			&state.FailedRows,
			&state.LastError,
			&state.StartedAt,
			&state.FinishedAt,
			&state.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan token analysis index state: %w", err)
		}
		status.ProcessedRows += state.ProcessedRows
		status.FailedRows += state.FailedRows
		if state.LastError != "" && status.LastError == "" {
			status.LastError = state.LastError
		}
		if status.UpdatedAt == nil || state.UpdatedAt.After(*status.UpdatedAt) {
			v := state.UpdatedAt
			status.UpdatedAt = &v
		}
		if state.StartedAt != nil && state.FinishedAt == nil {
			status.Running = true
		}
		status.Files = append(status.Files, state)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate token analysis index status: %w", err)
	}
	return status, nil
}

func (r *tokenAnalysisRepository) GetIndexState(ctx context.Context, sourceFile string) (*service.TokenAnalysisIndexState, error) {
	var state service.TokenAnalysisIndexState
	err := r.db.QueryRowContext(ctx, `
SELECT source_file, last_offset, last_archive_id, processed_rows, failed_rows, last_error, started_at, finished_at, updated_at
FROM token_analysis_index_state
WHERE source_file = $1`, sourceFile).Scan(
		&state.SourceFile,
		&state.LastOffset,
		&state.LastArchiveID,
		&state.ProcessedRows,
		&state.FailedRows,
		&state.LastError,
		&state.StartedAt,
		&state.FinishedAt,
		&state.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get token analysis index state: %w", err)
	}
	return &state, nil
}

func (r *tokenAnalysisRepository) UpdateIndexState(ctx context.Context, state service.TokenAnalysisIndexState) error {
	_, err := r.db.ExecContext(ctx, `
INSERT INTO token_analysis_index_state (
    source_file, last_offset, last_archive_id, processed_rows, failed_rows, last_error, started_at, finished_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, NOW()
)
ON CONFLICT (source_file) DO UPDATE SET
    last_offset = EXCLUDED.last_offset,
    last_archive_id = EXCLUDED.last_archive_id,
    processed_rows = token_analysis_index_state.processed_rows + EXCLUDED.processed_rows,
    failed_rows = token_analysis_index_state.failed_rows + EXCLUDED.failed_rows,
    last_error = EXCLUDED.last_error,
    started_at = COALESCE(EXCLUDED.started_at, token_analysis_index_state.started_at),
    finished_at = EXCLUDED.finished_at,
    updated_at = NOW()`,
		state.SourceFile,
		state.LastOffset,
		state.LastArchiveID,
		state.ProcessedRows,
		state.FailedRows,
		state.LastError,
		state.StartedAt,
		state.FinishedAt,
	)
	if err != nil {
		return fmt.Errorf("update token analysis index state: %w", err)
	}
	return nil
}

func (r *tokenAnalysisRepository) LoadProjectRoots(ctx context.Context) (map[string]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT root, project FROM token_analysis_project_roots`)
	if err != nil {
		return nil, fmt.Errorf("load token analysis project roots: %w", err)
	}
	defer rows.Close()
	roots := make(map[string]string)
	for rows.Next() {
		var root, project string
		if err := rows.Scan(&root, &project); err != nil {
			return nil, fmt.Errorf("scan token analysis project root: %w", err)
		}
		roots[root] = project
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate token analysis project roots: %w", err)
	}
	return roots, nil
}

func (r *tokenAnalysisRepository) UpsertProjectRoots(ctx context.Context, roots map[string]string) error {
	if len(roots) == 0 {
		return nil
	}
	for root, project := range roots {
		if strings.TrimSpace(root) == "" {
			continue
		}
		if _, err := r.db.ExecContext(ctx, `
INSERT INTO token_analysis_project_roots (root, project)
VALUES ($1, $2)
ON CONFLICT (root) DO UPDATE SET
    project = EXCLUDED.project,
    last_seen = NOW()`, root, project); err != nil {
			return fmt.Errorf("upsert token analysis project root: %w", err)
		}
	}
	return nil
}

func (r *tokenAnalysisRepository) UpsertUserInput(ctx context.Context, input *service.TokenAnalysisUserInput) error {
	if input == nil {
		return nil
	}
	// 冲突时只刷新内容字段, quality_* 由评估任务回填, 重建索引不得覆盖。
	_, err := r.db.ExecContext(ctx, `
INSERT INTO token_analysis_user_inputs (
    archive_id, event_time, user_id, content, content_sha256, chars, truncated
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
)
ON CONFLICT (archive_id) DO UPDATE SET
    event_time = EXCLUDED.event_time,
    user_id = EXCLUDED.user_id,
    content = EXCLUDED.content,
    content_sha256 = EXCLUDED.content_sha256,
    chars = EXCLUDED.chars,
    truncated = EXCLUDED.truncated,
    updated_at = NOW()`,
		input.ArchiveID,
		input.EventTime,
		nullableInt64(input.UserID),
		input.Content,
		input.ContentSHA256,
		input.Chars,
		input.Truncated,
	)
	if err != nil {
		return fmt.Errorf("upsert token analysis user input: %w", err)
	}
	return nil
}

func (r *tokenAnalysisRepository) GetUserInput(ctx context.Context, archiveID string) (*service.TokenAnalysisUserInput, error) {
	var input service.TokenAnalysisUserInput
	var userID sql.NullInt64
	var qualityScore sql.NullInt16
	var findingsRaw []byte
	err := r.db.QueryRowContext(ctx, `
SELECT id, archive_id, event_time, user_id, content, content_sha256, chars, truncated,
       quality_score, quality_findings, quality_version, evaluated_at
FROM token_analysis_user_inputs
WHERE archive_id = $1`, archiveID).Scan(
		&input.ID,
		&input.ArchiveID,
		&input.EventTime,
		&userID,
		&input.Content,
		&input.ContentSHA256,
		&input.Chars,
		&input.Truncated,
		&qualityScore,
		&findingsRaw,
		&input.QualityVersion,
		&input.EvaluatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get token analysis user input: %w", err)
	}
	if userID.Valid {
		input.UserID = &userID.Int64
	}
	if qualityScore.Valid {
		v := qualityScore.Int16
		input.QualityScore = &v
	}
	if len(findingsRaw) > 0 {
		input.QualityFindings = map[string]any{}
		_ = json.Unmarshal(findingsRaw, &input.QualityFindings)
	}
	return &input, nil
}

type tokenAnalysisRowsScanner interface {
	Scan(dest ...any) error
}

func scanTokenAnalysisRequestItem(rows tokenAnalysisRowsScanner) (service.TokenAnalysisRequestItem, error) {
	var item service.TokenAnalysisRequestItem
	var usageLogID, userID, apiKeyID, accountID, groupID sql.NullInt64
	var qualityScore sql.NullInt16
	var summaryRaw, reasonsRaw []byte
	if err := rows.Scan(
		&item.ID,
		&item.ArchiveID,
		&usageLogID,
		&item.MatchConfidence,
		&item.EventTime,
		&userID,
		&item.UserEmail,
		&apiKeyID,
		&item.APIKeyName,
		&accountID,
		&groupID,
		&item.Model,
		&item.Endpoint,
		&item.Method,
		&item.RequestBodySize,
		&item.RequestBodyTruncated,
		&item.MessageCount,
		&item.SystemChars,
		&item.UserChars,
		&item.LastUserPreview,
		&item.ToolsCount,
		&item.ImageCount,
		&summaryRaw,
		&item.RiskScore,
		&reasonsRaw,
		&item.ClientWorkdir,
		&item.ClientProject,
		&item.ClientBranch,
		&item.AttributionSource,
		&item.InputTokens,
		&item.OutputTokens,
		&item.CacheReadTokens,
		&item.CacheCreationTokens,
		&item.TotalTokens,
		&item.ActualCost,
		&item.HasInput,
		&item.InputTruncated,
		&qualityScore,
		&item.DuplicateCount,
	); err != nil {
		return item, fmt.Errorf("scan token analysis request: %w", err)
	}
	if qualityScore.Valid {
		v := qualityScore.Int16
		item.QualityScore = &v
	}
	if usageLogID.Valid {
		item.UsageLogID = &usageLogID.Int64
	}
	if userID.Valid {
		item.UserID = &userID.Int64
	}
	if apiKeyID.Valid {
		item.APIKeyID = &apiKeyID.Int64
	}
	if accountID.Valid {
		item.AccountID = &accountID.Int64
	}
	if groupID.Valid {
		item.GroupID = &groupID.Int64
	}
	item.SummaryJSON = map[string]any{}
	_ = json.Unmarshal(summaryRaw, &item.SummaryJSON)
	item.RiskReasons = []service.TokenAnalysisRiskReason{}
	_ = json.Unmarshal(reasonsRaw, &item.RiskReasons)
	return item, nil
}

func buildTokenAnalysisWhere(filters service.TokenAnalysisFilters, alias string) ([]string, []any) {
	prefix := strings.TrimSpace(alias)
	if prefix != "" {
		prefix += "."
	}
	where := []string{"1=1"}
	args := make([]any, 0)
	add := func(expr string, value any) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(expr, len(args)))
	}
	if filters.StartTime != nil {
		add(prefix+"event_time >= $%d", *filters.StartTime)
	}
	if filters.EndTime != nil {
		add(prefix+"event_time < $%d", *filters.EndTime)
	}
	if filters.UserID != nil {
		add(prefix+"user_id = $%d", *filters.UserID)
	}
	if filters.APIKeyID != nil {
		add(prefix+"api_key_id = $%d", *filters.APIKeyID)
	}
	if filters.AccountID != nil {
		add(prefix+"account_id = $%d", *filters.AccountID)
	}
	if filters.GroupID != nil {
		add(prefix+"group_id = $%d", *filters.GroupID)
	}
	if model := strings.TrimSpace(filters.Model); model != "" {
		add(prefix+"model = $%d", model)
	}
	if endpoint := strings.TrimSpace(filters.Endpoint); endpoint != "" {
		add(prefix+"endpoint = $%d", endpoint)
	}
	if filters.RiskMin > 0 {
		add(prefix+"risk_score >= $%d", filters.RiskMin)
	}
	if reason := strings.TrimSpace(filters.RiskReason); reason != "" {
		add(prefix+"risk_reasons @> $%d::jsonb", `[{"code":"`+escapeJSONString(reason)+`"}]`)
	}
	if project := strings.TrimSpace(filters.Project); project != "" {
		if project == "unattributed" {
			where = append(where, prefix+"client_project = ''")
		} else {
			add(prefix+"client_project = $%d", project)
		}
	}
	if !filters.IncludeUnmatched {
		where = append(where, prefix+"usage_log_id IS NOT NULL")
	}
	return where, args
}

func rewriteTokenAnalysisAlias(where []string, fromAlias, toAlias string) []string {
	from := strings.TrimSpace(fromAlias)
	to := strings.TrimSpace(toAlias)
	if from == "" || to == "" || from == to {
		return where
	}
	out := make([]string, 0, len(where))
	for _, expr := range where {
		out = append(out, strings.ReplaceAll(expr, from+".", to+"."))
	}
	return out
}

func tokenAnalysisRequestOrderBy(params pagination.PaginationParams) string {
	order := params.NormalizedSortOrder(pagination.SortOrderDesc)
	switch strings.TrimSpace(params.SortBy) {
	case "event_time":
		return "s.event_time " + order + ", s.id DESC"
	case "total_tokens":
		return "total_tokens " + order + ", s.event_time DESC"
	case "actual_cost":
		return "actual_cost " + order + ", s.event_time DESC"
	default:
		return "s.risk_score " + order + ", s.event_time DESC, s.id DESC"
	}
}

func nullableInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func nonNilMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func escapeJSONString(value string) string {
	raw, _ := json.Marshal(value)
	return strings.Trim(string(raw), `"`)
}
