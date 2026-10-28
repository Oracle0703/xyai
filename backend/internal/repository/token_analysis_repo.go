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
	"github.com/lib/pq"
)

type tokenAnalysisRepository struct {
	db *sql.DB
}

func NewTokenAnalysisRepository(db *sql.DB) service.TokenAnalysisRepository {
	return &tokenAnalysisRepository{db: db}
}

func (r *tokenAnalysisRepository) UpsertRequestSummary(ctx context.Context, summary *service.TokenAnalysisRequestSummary) (service.TokenAnalysisSummaryUpsert, error) {
	if summary == nil {
		return service.TokenAnalysisSummaryUpsert{}, nil
	}
	summaryJSON, err := json.Marshal(nonNilMap(summary.SummaryJSON))
	if err != nil {
		return service.TokenAnalysisSummaryUpsert{}, fmt.Errorf("marshal token analysis summary json: %w", err)
	}
	riskReasons, err := json.Marshal(summary.RiskReasons)
	if err != nil {
		return service.TokenAnalysisSummaryUpsert{}, fmt.Errorf("marshal token analysis risk reasons: %w", err)
	}
	var upserted service.TokenAnalysisSummaryUpsert
	err = r.inTx(ctx, "token analysis summary upsert", func(tx *sql.Tx) (bool, error) {
		var err error
		upserted, err = upsertTokenAnalysisSummaryTx(ctx, tx, summary, string(summaryJSON), string(riskReasons))
		return err == nil, err
	})
	if err != nil {
		return service.TokenAnalysisSummaryUpsert{}, err
	}
	return upserted, nil
}

func upsertTokenAnalysisSummaryTx(ctx context.Context, tx *sql.Tx, summary *service.TokenAnalysisRequestSummary, summaryJSON, riskReasons string) (service.TokenAnalysisSummaryUpsert, error) {
	// 有 API Key 的行先取候选锁: 本行可能成为同 Key 后续请求的新父请求, 与它们的候选
	// 查找串行(设计 5.5); 锁在行锁之前取, 加锁顺序统一。
	if summary.APIKeyID != nil {
		if err := lockTokenAnalysisCandidates(ctx, tx, *summary.APIKeyID); err != nil {
			return service.TokenAnalysisSummaryUpsert{}, err
		}
	}
	prev, err := lockTokenAnalysisFingerprintState(ctx, tx, summary.ArchiveID)
	if err != nil {
		return service.TokenAnalysisSummaryUpsert{}, err
	}
	plan := planTokenAnalysisCandidateVersions(prev, summary.Fingerprint)
	fp := summary.Fingerprint
	_, err = tx.ExecContext(ctx, `
INSERT INTO token_analysis_request_summaries (
    archive_id, usage_log_id, match_confidence, event_time,
    user_id, api_key_id, account_id, group_id,
    model, endpoint, method,
    request_body_size, request_body_truncated, body_sha256,
    message_count, system_chars, user_chars, last_user_preview, tools_count, image_count,
    summary_json, risk_score, risk_reasons, source_file, source_offset,
    client_workdir, client_project, client_branch, attribution_source,
    client_request_id, user_agent, client_family, client_family_version,
    source_line_start, source_line_length,
    fp_version, fp_state, prefix_end_hash, prefix_tail1_hash, items_end_hash, client_cache_key_hash,
    prefix_items, has_continuation_ref,
    cand_input_version, cand_checked_version, cand_state, cand_overflow
) VALUES (
    $1, $2, $3, $4,
    $5, $6, $7, $8,
    $9, $10, $11,
    $12, $13, $14,
    $15, $16, $17, $18, $19, $20,
    $21::jsonb, $22, $23::jsonb, $24, $25,
    $26, $27, $28, $29,
    $30, $31, $32, $33,
    $34, $35,
    $36, $37, $38, $39, $40, $41,
    $42, $43,
    $44, $45, $46, FALSE
)
ON CONFLICT (archive_id) DO UPDATE SET
    usage_log_id = EXCLUDED.usage_log_id,
    match_confidence = EXCLUDED.match_confidence,
    usage_match_method = '',
    client_request_id = EXCLUDED.client_request_id,
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
    attribution_source = EXCLUDED.attribution_source,
    user_agent = EXCLUDED.user_agent,
    client_family = EXCLUDED.client_family,
    client_family_version = EXCLUDED.client_family_version,
    source_line_start = EXCLUDED.source_line_start,
    source_line_length = EXCLUDED.source_line_length,
    fp_version = EXCLUDED.fp_version,
    fp_state = EXCLUDED.fp_state,
    prefix_end_hash = EXCLUDED.prefix_end_hash,
    prefix_tail1_hash = EXCLUDED.prefix_tail1_hash,
    items_end_hash = EXCLUDED.items_end_hash,
    client_cache_key_hash = EXCLUDED.client_cache_key_hash,
    prefix_items = EXCLUDED.prefix_items,
    has_continuation_ref = EXCLUDED.has_continuation_ref,
    cand_input_version = EXCLUDED.cand_input_version,
    cand_checked_version = EXCLUDED.cand_checked_version,
    cand_state = CASE WHEN $47::boolean THEN token_analysis_request_summaries.cand_state ELSE EXCLUDED.cand_state END,
    cand_overflow = CASE WHEN $47::boolean THEN token_analysis_request_summaries.cand_overflow ELSE FALSE END`,
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
		summaryJSON,
		summary.RiskScore,
		riskReasons,
		summary.SourceFile,
		nullableInt64(summary.SourceOffset),
		summary.ClientWorkdir,
		summary.ClientProject,
		summary.ClientBranch,
		summary.AttributionSource,
		summary.ClientRequestID,
		summary.UserAgent,
		summary.ClientFamily,
		summary.ClientFamilyVersion,
		nullableInt64(summary.SourceLineStart),
		nullableInt64(summary.SourceLineLength),
		fp.Version,
		fp.State,
		nullableBytes(fp.PrefixEndHash),
		nullableBytes(fp.PrefixTail1Hash),
		nullableBytes(fp.ItemsEndHash),
		nullableBytes(fp.ClientCacheKeyHash),
		fp.PrefixItems,
		fp.HasContinuationRef,
		plan.inputVersion,
		plan.checkedVersion,
		plan.state,
		plan.keepState,
	)
	if err != nil {
		return service.TokenAnalysisSummaryUpsert{}, fmt.Errorf("upsert token analysis request summary: %w", err)
	}
	if plan.clearEdges {
		if _, err := tx.ExecContext(ctx, `DELETE FROM token_analysis_prefix_edges WHERE child_archive_id = $1`, summary.ArchiveID); err != nil {
			return service.TokenAnalysisSummaryUpsert{}, fmt.Errorf("clear token analysis prefix edges: %w", err)
		}
	}
	if plan.bumpChildren {
		if _, err := tx.ExecContext(ctx, `
UPDATE token_analysis_request_summaries
SET cand_input_version = cand_input_version + 1, cls_dep_version = cls_dep_version + 1
WHERE archive_id IN (SELECT child_archive_id FROM token_analysis_prefix_edges WHERE parent_archive_id = $1)`,
			summary.ArchiveID); err != nil {
			return service.TokenAnalysisSummaryUpsert{}, fmt.Errorf("mark token analysis candidate children: %w", err)
		}
	}
	if plan.fingerprintChanged && summary.APIKeyID != nil {
		// 晚到的父请求: 同 Key、之后 L 窗口内、相同指纹版本且已有指纹的行可能漏选了本行,
		// 标为待重新匹配。尚未补算指纹的行(首次回填)与旧版本指纹的行不受影响, 它们
		// 处理到时会自行查找本行。
		if _, err := tx.ExecContext(ctx, `
UPDATE token_analysis_request_summaries
SET cand_input_version = cand_input_version + 1, cls_dep_version = cls_dep_version + 1
WHERE api_key_id = $1 AND event_time > $2 AND event_time <= $3
    AND fp_version = $4 AND prefix_end_hash IS NOT NULL AND archive_id <> $5`,
			*summary.APIKeyID, summary.EventTime, summary.EventTime.Add(service.TokenAnalysisCandidateLookback), fp.Version, summary.ArchiveID); err != nil {
			return service.TokenAnalysisSummaryUpsert{}, fmt.Errorf("mark token analysis late parent candidates: %w", err)
		}
	}
	// 摘要重写可能改变自身的分类输入, 并影响以本行为父或祖先的子孙。
	if err := markTokenAnalysisClassificationDirty(ctx, tx, []string{summary.ArchiveID}); err != nil {
		return service.TokenAnalysisSummaryUpsert{}, err
	}
	return service.TokenAnalysisSummaryUpsert{
		CandInputVersion: plan.inputVersion,
		CandidatesDue:    fp.Comparable() && plan.inputVersion != plan.checkedVersion,
	}, nil
}

func (r *tokenAnalysisRepository) RecordResponse(ctx context.Context, facts service.TokenAnalysisResponseFacts) error {
	if strings.TrimSpace(facts.ArchiveID) == "" {
		return nil
	}
	var status any
	if facts.Status > 0 {
		status = facts.Status
	}
	args := []any{facts.ArchiveID, facts.CompletedAt, status, nullableInt64(facts.OutputTokens), facts.ResponseID,
		facts.Forward.ForwardPath, facts.Forward.CacheKeyHandling, facts.Forward.UpstreamCacheKeyHash, facts.Forward.UsageCacheSource,
		facts.Forward.UpstreamItemsHash, facts.Forward.UpstreamRootHash}
	// 完成事实变化时登记一次复核(不受请求日期范围限制): 跨日请求的 response 在
	// 后一日文件里, 弱关联也要按新的状态/完成时间复核; 精确关联与探测行不需要。
	// 完成事实与转发记录是分类输入: 同一事务内给本行及其子孙加分类版本。
	return r.inTx(ctx, "token analysis response", func(tx *sql.Tx) (bool, error) {
		result, err := tx.ExecContext(ctx, `
UPDATE token_analysis_request_summaries
SET completed_at = $2, response_status = $3, response_output_tokens = $4, response_id = $5,
    forward_path = $6, cache_key_handling = $7, upstream_cache_key_hash = $8, usage_cache_source = $9,
    upstream_items_hash = $10, upstream_root_hash = $11,
    `+tokenAnalysisUsageLinkRecheckWhen("", tokenAnalysisResponseFactsChanged("", "$2", "$3", "$5"))+`
WHERE archive_id = $1`, args...)
		if err != nil {
			return false, fmt.Errorf("record token analysis response: %w", err)
		}
		updated, err := result.RowsAffected()
		if err != nil {
			return false, fmt.Errorf("record token analysis response: %w", err)
		}
		if updated > 0 {
			return true, markTokenAnalysisClassificationDirty(ctx, tx, []string{facts.ArchiveID})
		}
		// 请求行在前一日文件里且尚未索引(跨日文件逆序回填): 暂存, 请求行入库后合并。
		if _, err := tx.ExecContext(ctx, `
INSERT INTO token_analysis_pending_responses (archive_id, completed_at, response_status, response_output_tokens, response_id,
    forward_path, cache_key_handling, upstream_cache_key_hash, usage_cache_source, upstream_items_hash, upstream_root_hash)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (archive_id) DO UPDATE SET
    completed_at = EXCLUDED.completed_at,
    response_status = EXCLUDED.response_status,
    response_output_tokens = EXCLUDED.response_output_tokens,
    response_id = EXCLUDED.response_id,
    forward_path = EXCLUDED.forward_path,
    cache_key_handling = EXCLUDED.cache_key_handling,
    upstream_cache_key_hash = EXCLUDED.upstream_cache_key_hash,
    usage_cache_source = EXCLUDED.usage_cache_source,
    upstream_items_hash = EXCLUDED.upstream_items_hash,
    upstream_root_hash = EXCLUDED.upstream_root_hash,
    recorded_at = NOW()`, args...); err != nil {
			return false, fmt.Errorf("stash token analysis response: %w", err)
		}
		return true, nil
	})
}

// tokenAnalysisResponseFactsChanged 判断写回的完成事实是否改变了未精确关联的行
// (SET 表达式读到的是更新前的列值)。
func tokenAnalysisResponseFactsChanged(alias, completedAt, status, responseID string) string {
	return `(` + alias + `completed_at IS DISTINCT FROM ` + completedAt + ` OR ` + alias + `response_status IS DISTINCT FROM ` + status +
		` OR ` + alias + `response_id <> ` + responseID + `)
        AND ` + alias + `usage_match_method <> 'request_id' AND ` + alias + `endpoint NOT LIKE '%/count_tokens'`
}

// 复核事件后, 带关联键的成功请求等待晚到账单的时长; 期满仍无证据即确认未关联。
const tokenAnalysisUsageLinkRetryWindow = 48 * time.Hour

// tokenAnalysisUsageLinkRecheckWhen 在 cond 成立时登记一次复核: 版本加一并重置
// 等待晚到账单的截止时间(SET 片段, 列不带别名前缀时 alias 传空)。
func tokenAnalysisUsageLinkRecheckWhen(alias, cond string) string {
	return fmt.Sprintf(`usage_link_version = %[1]susage_link_version + CASE WHEN %[2]s THEN 1 ELSE 0 END,
    usage_link_retry_until = CASE WHEN %[2]s THEN NOW() + INTERVAL '%[3]d seconds' ELSE %[1]susage_link_retry_until END`,
		alias, cond, int64(tokenAnalysisUsageLinkRetryWindow.Seconds()))
}

// 请求行一直没有入库(归档队列丢弃)的暂存完成事实保留期。
const tokenAnalysisPendingResponseTTL = 7 * 24 * time.Hour

func (r *tokenAnalysisRepository) MergePendingResponses(ctx context.Context) error {
	return r.inTx(ctx, "token analysis pending responses", func(tx *sql.Tx) (bool, error) {
		rows, err := tx.QueryContext(ctx, `
WITH merged AS (
    UPDATE token_analysis_request_summaries s
    SET completed_at = p.completed_at,
        response_status = p.response_status,
        response_output_tokens = p.response_output_tokens,
        response_id = p.response_id,
        forward_path = p.forward_path,
        cache_key_handling = p.cache_key_handling,
        upstream_cache_key_hash = p.upstream_cache_key_hash,
        usage_cache_source = p.usage_cache_source,
        upstream_items_hash = p.upstream_items_hash,
        upstream_root_hash = p.upstream_root_hash,
        `+tokenAnalysisUsageLinkRecheckWhen("s.", tokenAnalysisResponseFactsChanged("s.", "p.completed_at", "p.response_status", "p.response_id"))+`
    FROM token_analysis_pending_responses p
    WHERE s.archive_id = p.archive_id
    RETURNING s.archive_id
), deleted AS (
    DELETE FROM token_analysis_pending_responses p
    WHERE p.archive_id IN (SELECT archive_id FROM merged) OR p.recorded_at < $1
)
SELECT archive_id FROM merged`, time.Now().Add(-tokenAnalysisPendingResponseTTL))
		if err != nil {
			return false, fmt.Errorf("merge token analysis pending responses: %w", err)
		}
		merged, err := scanTokenAnalysisArchiveIDs(rows)
		if err != nil {
			return false, fmt.Errorf("merge token analysis pending responses: %w", err)
		}
		return true, markTokenAnalysisClassificationDirty(ctx, tx, merged)
	})
}

func scanTokenAnalysisArchiveIDs(rows *sql.Rows) ([]string, error) {
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (r *tokenAnalysisRepository) ListUsageLinkCandidates(ctx context.Context, start, end time.Time, afterID int64, limit int) ([]service.TokenAnalysisUsageLinkCandidate, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT
    s.id, s.archive_id, s.client_request_id, s.response_id, s.event_time, s.completed_at, s.response_status,
    s.response_output_tokens, s.user_id, s.api_key_id, s.model, s.usage_log_id, s.usage_match_method,
    s.usage_link_version, s.usage_link_version <> s.usage_link_checked_version, s.usage_link_retry_until, s.body_sha256,
    s.system_chars, s.user_chars, s.tools_count,
    COALESCE((s.summary_json->>'tool_message_count')::bigint, 0),
    COALESCE((s.summary_json->>'tool_output_bytes')::bigint, 0),
    COALESCE((s.summary_json->>'max_tool_output_bytes')::bigint, 0),
    (s.summary_json->>'same_body_recent_count')::int
FROM token_analysis_request_summaries s
WHERE s.id > $3
    AND (s.usage_link_version <> s.usage_link_checked_version OR (s.event_time >= $1 AND s.event_time < $2))
    AND s.endpoint NOT LIKE '%/count_tokens'
    AND (s.usage_link_version <> s.usage_link_checked_version OR s.usage_match_method IN ('', 'released')
        OR (s.usage_match_method = 'timing' AND (s.client_request_id <> '' OR s.response_id <> '')))
ORDER BY s.id
LIMIT $4`, start, end, afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("list token analysis usage link candidates: %w", err)
	}
	defer rows.Close()

	candidates := make([]service.TokenAnalysisUsageLinkCandidate, 0)
	for rows.Next() {
		var c service.TokenAnalysisUsageLinkCandidate
		var completedAt, retryUntil sql.NullTime
		var status, sameBody sql.NullInt32
		var outputTokens, userID, apiKeyID, usageLogID, toolMessages sql.NullInt64
		if err := rows.Scan(
			&c.ID, &c.ArchiveID, &c.ClientRequestID, &c.ResponseID, &c.EventTime, &completedAt, &status,
			&outputTokens, &userID, &apiKeyID, &c.Model, &usageLogID, &c.Method,
			&c.LinkVersion, &c.LinkPending, &retryUntil, &c.BodySHA256,
			&c.Body.SystemChars, &c.Body.UserChars, &c.Body.ToolsCount,
			&toolMessages, &c.Body.ToolOutputBytes, &c.Body.MaxToolOutputBytes,
			&sameBody,
		); err != nil {
			return nil, fmt.Errorf("scan token analysis usage link candidate: %w", err)
		}
		if completedAt.Valid {
			c.CompletedAt = &completedAt.Time
		}
		if retryUntil.Valid {
			c.RetryUntil = &retryUntil.Time
		}
		if status.Valid {
			n := int(status.Int32)
			c.ResponseStatus = &n
		}
		c.OutputTokens = nullableInt64Ptr(outputTokens)
		c.UserID = nullableInt64Ptr(userID)
		c.APIKeyID = nullableInt64Ptr(apiKeyID)
		c.UsageLogID = nullableInt64Ptr(usageLogID)
		c.Body.ToolMessageCount = int(toolMessages.Int64)
		if sameBody.Valid {
			n := int(sameBody.Int32)
			c.SameBodyRecentCount = &n
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate token analysis usage link candidates: %w", err)
	}
	return candidates, nil
}

const (
	// 归档与计费由同一进程按 time.Now 记录: usage 推算的转发开始时间
	// (created_at - duration_ms)不早于归档请求开始, 只留毫秒取整余量。计费任务
	// 排队只会让推算值变晚, 所以它只作下界, 不作确认归属的上界。
	tokenAnalysisUsageLinkStartSlack = 500 * time.Millisecond
	// 计费任务在转发结束后提交, 归档 response 行在处理器返回后写入: 账单落库
	// 不会明显早于完成时间。
	tokenAnalysisUsageLinkCompletionSlack = 2 * time.Second
	// 完成时间未知(旧行)时账单落库时间的扫描上限, 只为收窄索引范围扫描。
	tokenAnalysisUsageLinkMaxDuration = 2 * time.Hour
	tokenAnalysisUsageMatchColumns    = `ul.id, ul.input_tokens, ul.output_tokens, ul.cache_read_tokens,
    ul.cache_creation_tokens, ul.total_cost, ul.actual_cost`
	tokenAnalysisUsageForwardStart = "(ul.created_at - COALESCE(ul.duration_ms, 0) * INTERVAL '1 millisecond')"
	// 附加计费(联网搜索、视频/音频、Gemini 缓存)不是请求的主账单, 不参与时序兜底。
	tokenAnalysisUsageNotAddOnBill = `ul.request_id NOT LIKE ALL (ARRAY['web_search:%', 'grok-video:%', 'grok_audio:%', 'grok_realtime:%', 'gcache:%'])`
)

// FindUsageLink 先按关联键精确关联: "client:<client_request_id>"(HTTP 转发)与上游
// response.id(WS 上游), 两者在同一 api_key_id 下唯一; 精确证据可夺回只被弱关联
// 占用的账单。ExactOnly 为 false 时再做时序兜底。
func (r *tokenAnalysisRepository) FindUsageLink(ctx context.Context, q service.TokenAnalysisUsageLinkQuery) (*service.TokenAnalysisUsageMatch, error) {
	if q.APIKeyID != nil {
		keys := make([]any, 0, 2)
		if q.ClientRequestID != "" {
			keys = append(keys, "client:"+q.ClientRequestID)
		}
		if q.ResponseID != "" {
			keys = append(keys, q.ResponseID)
		}
		if len(keys) > 0 {
			match, err := r.findUsageByRequestID(ctx, q, keys)
			if err != nil || match != nil {
				return match, err
			}
		}
	}
	if q.ExactOnly {
		return nil, nil
	}
	return r.findUsageByTiming(ctx, q)
}

func (r *tokenAnalysisRepository) findUsageByRequestID(ctx context.Context, q service.TokenAnalysisUsageLinkQuery, keys []any) (*service.TokenAnalysisUsageMatch, error) {
	placeholders := make([]string, len(keys))
	for i := range keys {
		placeholders[i] = fmt.Sprintf("$%d", i+3)
	}
	query := `
SELECT ` + tokenAnalysisUsageMatchColumns + `
FROM usage_logs ul
WHERE ul.api_key_id = $1 AND ul.request_id IN (` + strings.Join(placeholders, ", ") + `)
    AND NOT EXISTS (
        SELECT 1 FROM token_analysis_request_summaries o
        WHERE o.usage_log_id = ul.id AND o.archive_id <> $2 AND o.usage_match_method = 'request_id')
ORDER BY ul.id
LIMIT 1`
	row := r.db.QueryRowContext(ctx, query, append([]any{*q.APIKeyID, q.ArchiveID}, keys...)...)
	return scanTokenAnalysisUsageMatch(row, service.TokenAnalysisUsageMatchRequestID, 3)
}

// findUsageByTiming 在没有精确证据时按时序推断, 只接受无歧义的结果:
//   - 账单合理: 同用户/密钥/模型; 推算转发开始不早于请求开始; 有完成时间时账单
//     耗时不超过请求耗时、在完成时间附近落库; 旧行(无完成时间)要求转发在开始后
//     window 内启动。
//   - 账单未被他行占用, 也不属于另一条带关联键请求的精确账单。
//   - 归属唯一: 没有其他尚未确认关联的同键请求同样能解释这笔账单。计费排队会让
//     顺序请求在时序上无法区分, 此时保留未关联, 不猜测。
//
// 有关联键的新请求只看上游原生 ID(不含 ':')的账单: client: 账单归精确关联。
func (r *tokenAnalysisRepository) findUsageByTiming(ctx context.Context, q service.TokenAnalysisUsageLinkQuery) (*service.TokenAnalysisUsageMatch, error) {
	fs := tokenAnalysisUsageForwardStart
	slackMs := tokenAnalysisUsageLinkStartSlack.Milliseconds()
	windowMs := q.Window.Milliseconds()
	completionSlackMs := tokenAnalysisUsageLinkCompletionSlack.Milliseconds()
	maxDurationMs := tokenAnalysisUsageLinkMaxDuration.Milliseconds()

	args := []any{q.StartedAt.Add(-tokenAnalysisUsageLinkStartSlack), q.StartedAt}
	where := []string{"ul.created_at >= $1", fs + " >= $1"}
	var distance string
	if q.CompletedAt != nil {
		args = append(args, *q.CompletedAt, q.CompletedAt.Add(-tokenAnalysisUsageLinkCompletionSlack), q.CompletedAt.Add(q.Window),
			q.CompletedAt.Sub(q.StartedAt).Milliseconds()+slackMs)
		where = append(where, "ul.created_at BETWEEN $4 AND $5", "COALESCE(ul.duration_ms, 0) <= $6")
		distance = "ABS(EXTRACT(EPOCH FROM (ul.created_at - $3::timestamptz))), ABS(EXTRACT(EPOCH FROM (" + fs + " - $2::timestamptz)))"
	} else {
		args = append(args, q.StartedAt.Add(q.Window), q.StartedAt.Add(tokenAnalysisUsageLinkMaxDuration))
		where = append(where, fs+" <= $3", "ul.created_at <= $4")
		distance = "ABS(EXTRACT(EPOCH FROM (" + fs + " - $2::timestamptz)))"
	}
	add := func(expr string, value any) string {
		args = append(args, value)
		return fmt.Sprintf(expr, len(args))
	}
	confidence := int16(0)
	if q.UserID != nil {
		where = append(where, add("ul.user_id = $%d", *q.UserID))
		confidence++
	}
	if q.APIKeyID != nil {
		where = append(where, add("ul.api_key_id = $%d", *q.APIKeyID))
		confidence++
	}
	if model := strings.TrimSpace(q.Model); model != "" {
		placeholder := add("$%d", model)
		where = append(where, "(ul.model = "+placeholder+" OR ul.requested_model = "+placeholder+" OR ul.upstream_model = "+placeholder+")")
		confidence++
	}
	if confidence == 0 {
		confidence = 1
	}
	if q.ClientRequestID != "" {
		where = append(where, "strpos(ul.request_id, ':') = 0")
	}
	me := add("$%d", q.ArchiveID)
	where = append(where, tokenAnalysisUsageNotAddOnBill,
		`NOT EXISTS (
        SELECT 1 FROM token_analysis_request_summaries o
        WHERE o.usage_log_id = ul.id AND o.archive_id <> `+me+`)`,
		`NOT EXISTS (
        SELECT 1 FROM token_analysis_request_summaries o
        WHERE o.archive_id <> `+me+` AND (
            (o.client_request_id <> '' AND ul.request_id LIKE 'client:%' AND o.client_request_id = substr(ul.request_id, 8))
            OR (o.response_id <> '' AND o.response_id = ul.request_id)))`,
		fmt.Sprintf(`NOT EXISTS (
        SELECT 1 FROM token_analysis_request_summaries c
        WHERE c.archive_id <> %[1]s
            AND c.user_id = ul.user_id AND c.api_key_id = ul.api_key_id
            AND c.model IN (ul.model, ul.requested_model, ul.upstream_model)
            AND c.endpoint NOT LIKE '%%/count_tokens'
            AND (c.response_status IS NULL OR c.response_status BETWEEN 200 AND 299)
            AND (c.usage_log_id IS NULL OR c.usage_match_method IN ('', 'released'))
            AND (c.client_request_id = '' OR (c.response_id = '' AND strpos(ul.request_id, ':') = 0))
            AND c.event_time >= ul.created_at - INTERVAL '%[2]d milliseconds'
            AND c.event_time <= %[3]s + INTERVAL '%[4]d milliseconds'
            AND CASE WHEN c.completed_at IS NULL
                THEN %[3]s <= c.event_time + INTERVAL '%[5]d milliseconds'
                ELSE ul.created_at BETWEEN c.completed_at - INTERVAL '%[6]d milliseconds' AND c.completed_at + INTERVAL '%[5]d milliseconds'
                    AND COALESCE(ul.duration_ms, 0) <= EXTRACT(EPOCH FROM (c.completed_at - c.event_time)) * 1000 + %[4]d
            END)`, me, maxDurationMs, fs, slackMs, windowMs, completionSlackMs),
	)
	orderBy := distance + ", ul.id"
	if q.OutputTokens != nil {
		orderBy = add("CASE WHEN ul.output_tokens = $%d THEN 0 ELSE 1 END, ", *q.OutputTokens) + orderBy
	}
	query := `
SELECT ` + tokenAnalysisUsageMatchColumns + `
FROM usage_logs ul
WHERE ` + strings.Join(where, "\n    AND ") + `
ORDER BY ` + orderBy + `
LIMIT 1`
	row := r.db.QueryRowContext(ctx, query, args...)
	return scanTokenAnalysisUsageMatch(row, service.TokenAnalysisUsageMatchTiming, confidence)
}

func scanTokenAnalysisUsageMatch(row *sql.Row, method string, confidence int16) (*service.TokenAnalysisUsageMatch, error) {
	match := service.TokenAnalysisUsageMatch{Method: method, MatchConfidence: confidence}
	err := row.Scan(
		&match.UsageLogID,
		&match.InputTokens,
		&match.OutputTokens,
		&match.CacheReadTokens,
		&match.CacheCreationTokens,
		&match.TotalCost,
		&match.ActualCost,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find token analysis usage link: %w", err)
	}
	return &match, nil
}

// 条件更新: 只有关联状态与复核版本仍是查找前读到的值时才写入($7/$8/$9 为读到的
// usage_log_id、usage_match_method 与版本), 陈旧结果既不能覆盖其他索引实例刚写入的
// 关联, 也不能确认其后登记的复核; $10 为 true 时不确认版本(继续等待晚到账单)。
const tokenAnalysisSetUsageLinkSQL = `
UPDATE token_analysis_request_summaries
SET usage_log_id = $2, match_confidence = $3, usage_match_method = $4, risk_score = $5, risk_reasons = $6::jsonb,
    usage_link_checked_version = CASE WHEN $10::boolean THEN usage_link_checked_version ELSE $9 END
WHERE archive_id = $1 AND usage_log_id IS NOT DISTINCT FROM $7 AND usage_match_method = $8 AND usage_link_version = $9`

// SetUsageLink 认领账单时按账单 ID 取事务级咨询锁: 多个索引实例并发认领同一账单
// 时串行执行, 后到者在锁内看到先到者已提交的认领而放弃。精确关联可夺回仅被
// 弱关联(时序/旧窗口)占用的账单, 原占用行置为 released 并登记复核, 由下一轮
// 不受日期范围限制地重关联或解除、重算风险。所有写入按 expected 条件更新。
func (r *tokenAnalysisRepository) SetUsageLink(ctx context.Context, archiveID string, expected service.TokenAnalysisUsageLinkState, match *service.TokenAnalysisUsageMatch, riskScore int, riskReasons []service.TokenAnalysisRiskReason) (service.TokenAnalysisUsageLinkWrite, error) {
	reasons, err := json.Marshal(riskReasons)
	if err != nil {
		return service.TokenAnalysisUsageLinkWrite{}, fmt.Errorf("marshal token analysis risk reasons: %w", err)
	}
	// 关联结果是分类输入: 写入行与被夺回的行在同一事务内加分类版本。
	var write service.TokenAnalysisUsageLinkWrite
	err = r.inTx(ctx, "token analysis usage link", func(tx *sql.Tx) (bool, error) {
		write = service.TokenAnalysisUsageLinkWrite{}
		if match == nil {
			result, err := tx.ExecContext(ctx, tokenAnalysisSetUsageLinkSQL, archiveID, nil, int16(0), "", riskScore, string(reasons),
				nullableInt64(expected.UsageLogID), expected.Method, expected.Version, expected.KeepPending)
			if err != nil {
				return false, fmt.Errorf("clear token analysis usage link: %w", err)
			}
			if updated, err := result.RowsAffected(); err != nil || updated == 0 {
				return false, err
			}
			write.Applied = true
			return true, markTokenAnalysisClassificationDirty(ctx, tx, []string{archiveID})
		}

		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('token_analysis_usage_claim:' || $1::text, 0))`, match.UsageLogID); err != nil {
			return false, fmt.Errorf("lock token analysis usage claim: %w", err)
		}
		rows, err := tx.QueryContext(ctx, `
SELECT usage_match_method FROM token_analysis_request_summaries
WHERE usage_log_id = $1 AND archive_id <> $2
FOR UPDATE`, match.UsageLogID, archiveID)
		if err != nil {
			return false, fmt.Errorf("check token analysis usage claim: %w", err)
		}
		displaced := false
		for rows.Next() {
			var method string
			if err := rows.Scan(&method); err != nil {
				_ = rows.Close()
				return false, fmt.Errorf("scan token analysis usage claim: %w", err)
			}
			if method == service.TokenAnalysisUsageMatchRequestID || match.Method != service.TokenAnalysisUsageMatchRequestID {
				_ = rows.Close()
				return false, nil
			}
			displaced = true
		}
		if err := rows.Close(); err != nil {
			return false, fmt.Errorf("check token analysis usage claim: %w", err)
		}
		result, err := tx.ExecContext(ctx, tokenAnalysisSetUsageLinkSQL, archiveID, match.UsageLogID, match.MatchConfidence, match.Method, riskScore, string(reasons),
			nullableInt64(expected.UsageLogID), expected.Method, expected.Version, expected.KeepPending)
		if err != nil {
			return false, fmt.Errorf("set token analysis usage link: %w", err)
		}
		if updated, err := result.RowsAffected(); err != nil || updated == 0 {
			return false, err
		}
		changed := []string{archiveID}
		if displaced {
			released, err := tx.QueryContext(ctx, `
UPDATE token_analysis_request_summaries
SET usage_log_id = NULL, match_confidence = 0, usage_match_method = 'released',
    `+tokenAnalysisUsageLinkRecheckWhen("", "TRUE")+`
WHERE usage_log_id = $1 AND archive_id <> $2
RETURNING archive_id`, match.UsageLogID, archiveID)
			if err != nil {
				return false, fmt.Errorf("release token analysis usage claim: %w", err)
			}
			ids, err := scanTokenAnalysisArchiveIDs(released)
			if err != nil {
				return false, fmt.Errorf("release token analysis usage claim: %w", err)
			}
			changed = append(changed, ids...)
		}
		if err := markTokenAnalysisClassificationDirty(ctx, tx, changed); err != nil {
			return false, err
		}
		write = service.TokenAnalysisUsageLinkWrite{Applied: true, Displaced: displaced}
		return true, nil
	})
	if err != nil {
		return service.TokenAnalysisUsageLinkWrite{}, err
	}
	return write, nil
}

func (r *tokenAnalysisRepository) ClearUsageLinkPending(ctx context.Context, archiveID string, version int64) error {
	// 关联确认是分类门禁之一: 确认后本行及子孙需重新分类。
	return r.inTx(ctx, "token analysis usage link pending", func(tx *sql.Tx) (bool, error) {
		result, err := tx.ExecContext(ctx, `
UPDATE token_analysis_request_summaries SET usage_link_checked_version = $2
WHERE archive_id = $1 AND usage_link_version = $2 AND usage_link_checked_version <> $2`, archiveID, version)
		if err != nil {
			return false, fmt.Errorf("clear token analysis usage link pending: %w", err)
		}
		updated, err := result.RowsAffected()
		if err != nil || updated == 0 {
			return false, err
		}
		return true, markTokenAnalysisClassificationDirty(ctx, tx, []string{archiveID})
	})
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
	where, args = appendUserEmailFilter(where, args, filters.UserEmail, "u")
	// 邮箱条件引用 users 别名 u: 主查询与每个子查询各自 JOIN, 保持同一筛选集合。
	withUsers := strings.TrimSpace(filters.UserEmail) != ""
	usersJoin := func(alias string) string {
		if !withUsers {
			return ""
		}
		return " LEFT JOIN users u ON u.id = " + alias + ".user_id"
	}
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
    (` + tokenAnalysisHumanInputSQL(where, withUsers) + `) AS human_input_count,
    COALESCE((
        SELECT jsonb_agg(jsonb_build_object('code', code, 'count', count) ORDER BY count DESC, code ASC)
        FROM (
            SELECT reason->>'code' AS code, COUNT(*) AS count
            FROM token_analysis_request_summaries reason_source
            LEFT JOIN usage_logs reason_ul ON reason_ul.id = reason_source.usage_log_id` + usersJoin("reason_source") + `
            CROSS JOIN LATERAL jsonb_array_elements(reason_source.risk_reasons) AS reason
            WHERE ` + strings.Join(rewriteTokenAnalysisAlias(where, "s", "reason_source"), " AND ") + `
              AND reason->>'code' <> ''
            GROUP BY reason->>'code'
            ORDER BY count DESC, code ASC
            LIMIT 8
        ) ranked_reasons
    ), '[]'::jsonb) AS risk_reasons
FROM token_analysis_request_summaries s
LEFT JOIN usage_logs ul ON ul.id = s.usage_log_id` + usersJoin("s") + `
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
		&summary.HumanInputCount,
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

// CountBilledRequests 统计同期 usage_logs 计费请求数, 作为概览归档覆盖率的
// 分母。只应用 usage_logs 上有对应列的过滤(时间/用户/邮箱/密钥/账号/分组/模型);
// endpoint/风险分/项目归因/include_unmatched 是归档侧概念, 不参与。
func (r *tokenAnalysisRepository) CountBilledRequests(ctx context.Context, filters service.TokenAnalysisFilters) (int64, error) {
	where := []string{"1=1"}
	args := make([]any, 0)
	add := func(expr string, value any) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(expr, len(args)))
	}
	if filters.StartTime != nil {
		add("created_at >= $%d", *filters.StartTime)
	}
	if filters.EndTime != nil {
		add("created_at < $%d", *filters.EndTime)
	}
	if filters.UserID != nil {
		add("user_id = $%d", *filters.UserID)
	}
	if email := strings.TrimSpace(filters.UserEmail); email != "" {
		// 与归档侧 appendUserEmailFilter 同一匹配, 归档覆盖率的分子分母共用邮箱口径。
		add("user_id IN (SELECT id FROM users WHERE email ILIKE $%d)", "%"+escapeLikePattern(email)+"%")
	}
	if filters.APIKeyID != nil {
		add("api_key_id = $%d", *filters.APIKeyID)
	}
	if filters.AccountID != nil {
		add("account_id = $%d", *filters.AccountID)
	}
	if filters.GroupID != nil {
		add("group_id = $%d", *filters.GroupID)
	}
	if model := strings.TrimSpace(filters.Model); model != "" {
		args = append(args, model)
		placeholder := fmt.Sprintf("$%d", len(args))
		where = append(where, "(model = "+placeholder+" OR requested_model = "+placeholder+" OR upstream_model = "+placeholder+")")
	}
	var count int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_logs WHERE "+strings.Join(where, " AND "), args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count billed requests: %w", err)
	}
	return count, nil
}

func (r *tokenAnalysisRepository) ListUserUsage(ctx context.Context, filters service.TokenAnalysisFilters, params pagination.PaginationParams) ([]service.TokenAnalysisUserUsage, *pagination.PaginationResult, error) {
	where, args := buildTokenAnalysisWhere(filters, "s")
	where, args = appendUserEmailFilter(where, args, filters.UserEmail, "u")
	whereSQL := "WHERE " + strings.Join(where, " AND ")
	countQuery := `
SELECT COUNT(*) FROM (
    SELECT s.user_id
    FROM token_analysis_request_summaries s
    LEFT JOIN usage_logs ul ON ul.id = s.usage_log_id
    LEFT JOIN users u ON u.id = s.user_id
    ` + whereSQL + `
    GROUP BY s.user_id, u.email
) grouped`
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, nil, fmt.Errorf("count token analysis user usage: %w", err)
	}

	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, params.Limit(), params.Offset())
	orderBy := tokenAnalysisUserOrderBy(params)
	query := `
SELECT agg.*, COALESCE(hi.human_input_count, 0) AS human_input_count
FROM (
SELECT
    s.user_id,
    COALESCE(u.email, '') AS user_email,
    NULL::BIGINT AS api_key_id,
    '' AS api_key_name,
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
` + whereSQL + `
GROUP BY s.user_id, u.email
ORDER BY ` + orderBy + `
LIMIT $` + fmt.Sprint(len(queryArgs)-1) + ` OFFSET $` + fmt.Sprint(len(queryArgs)) + `
) agg
LEFT JOIN (` + tokenAnalysisHumanInputSQL(where, strings.TrimSpace(filters.UserEmail) != "", tokenAnalysisHumanInputKey{"s.user_id", "user_id"}) + `) hi
    ON hi.user_id IS NOT DISTINCT FROM agg.user_id
ORDER BY ` + orderBy

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
			&item.HumanInputCount,
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
	where, args = appendUserEmailFilter(where, args, filters.UserEmail, "u")
	whereSQL := "WHERE " + strings.Join(where, " AND ")
	countQuery := `
SELECT COUNT(*) FROM (
    SELECT s.client_project, s.user_id
    FROM token_analysis_request_summaries s
    LEFT JOIN usage_logs ul ON ul.id = s.usage_log_id
    LEFT JOIN users u ON u.id = s.user_id
    ` + whereSQL + `
    GROUP BY s.client_project, s.user_id
) grouped`
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, nil, fmt.Errorf("count token analysis project usage: %w", err)
	}

	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, params.Limit(), params.Offset())
	orderBy := tokenAnalysisProjectOrderBy(params)
	query := `
SELECT agg.project, agg.user_id, agg.user_email, agg.request_count, agg.matched_request_count,
    agg.total_tokens, agg.input_tokens, agg.output_tokens, agg.cache_read_tokens, agg.cache_creation_tokens,
    agg.actual_cost, agg.last_event_time, COALESCE(hi.human_input_count, 0) AS human_input_count
FROM (
SELECT
    CASE WHEN s.client_project = '' THEN 'unattributed' ELSE s.client_project END AS project,
    s.client_project AS project_key,
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
ORDER BY ` + orderBy + `
LIMIT $` + fmt.Sprint(len(queryArgs)-1) + ` OFFSET $` + fmt.Sprint(len(queryArgs)) + `
) agg
LEFT JOIN (` + tokenAnalysisHumanInputSQL(where, strings.TrimSpace(filters.UserEmail) != "",
		tokenAnalysisHumanInputKey{"s.client_project", "project_key"}, tokenAnalysisHumanInputKey{"s.user_id", "user_id"}) + `) hi
    ON hi.project_key = agg.project_key AND hi.user_id IS NOT DISTINCT FROM agg.user_id
ORDER BY ` + orderBy

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
			&item.HumanInputCount,
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
	where, args = appendUserEmailFilter(where, args, filters.UserEmail, "u")
	whereSQL := "WHERE " + strings.Join(where, " AND ")

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM token_analysis_request_summaries s LEFT JOIN usage_logs ul ON ul.id = s.usage_log_id LEFT JOIN users u ON u.id = s.user_id "+whereSQL, args...).Scan(&total); err != nil {
		return nil, nil, fmt.Errorf("count token analysis requests: %w", err)
	}

	orderBy := tokenAnalysisRequestOrderBy(params)
	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, params.Limit(), params.Offset())
	query := `
SELECT
    s.id, s.archive_id, s.usage_log_id, s.match_confidence, s.usage_match_method, s.completed_at, s.response_status, s.event_time,
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
		// Running 不再按 started_at/finished_at 推断: 中断过的轮次会留下
		// 永远缺 finished_at 的行, 把状态卡成永久运行中。运行中与否由
		// service 层内存标志(s.running)给出, 覆盖手动与自动索引。
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
    last_error = COALESCE(NULLIF(EXCLUDED.last_error, ''), token_analysis_index_state.last_error),
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
	// quality_* 由评估任务回填并绑定内容哈希: 重建索引内容不变时保留, 变了即清空,
	// 避免旧评分挂到新正文上(SET 中的 t.* 均为更新前的旧行)。
	_, err := r.db.ExecContext(ctx, `
INSERT INTO token_analysis_user_inputs AS t (
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
    quality_score = CASE WHEN t.content_sha256 = EXCLUDED.content_sha256 THEN t.quality_score END,
    quality_findings = CASE WHEN t.content_sha256 = EXCLUDED.content_sha256 THEN t.quality_findings END,
    quality_version = CASE WHEN t.content_sha256 = EXCLUDED.content_sha256 THEN t.quality_version ELSE '' END,
    evaluated_at = CASE WHEN t.content_sha256 = EXCLUDED.content_sha256 THEN t.evaluated_at END,
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

func (r *tokenAnalysisRepository) DeleteUserInput(ctx context.Context, archiveID string) error {
	if strings.TrimSpace(archiveID) == "" {
		return nil
	}
	if _, err := r.db.ExecContext(ctx, `DELETE FROM token_analysis_user_inputs WHERE archive_id = $1`, archiveID); err != nil {
		return fmt.Errorf("delete token analysis user input: %w", err)
	}
	return nil
}

func (r *tokenAnalysisRepository) ListStaleClientFamilies(ctx context.Context, version int32, afterID int64, limit int) ([]service.TokenAnalysisClientFamilyRow, error) {
	if limit <= 0 {
		return nil, nil
	}
	// 按主键游标续读, 每遍只扫一次表。
	rows, err := r.db.QueryContext(ctx, `
SELECT id, user_agent, client_family_version
FROM token_analysis_request_summaries
WHERE id > $2 AND client_family_version <> 0 AND client_family_version <> $1
ORDER BY id
LIMIT $3`, version, afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("list stale token analysis client families: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]service.TokenAnalysisClientFamilyRow, 0, limit)
	for rows.Next() {
		var row service.TokenAnalysisClientFamilyRow
		if err := rows.Scan(&row.ID, &row.UserAgent, &row.Version); err != nil {
			return nil, fmt.Errorf("scan stale token analysis client family: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stale token analysis client families: %w", err)
	}
	return out, nil
}

func (r *tokenAnalysisRepository) UpdateClientFamilies(ctx context.Context, version int32, rows []service.TokenAnalysisClientFamilyRow) error {
	if len(rows) == 0 {
		return nil
	}
	ids := make([]int64, len(rows))
	families := make([]string, len(rows))
	userAgents := make([]string, len(rows))
	versions := make([]int64, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
		families[i] = row.Family
		userAgents[i] = row.UserAgent
		versions[i] = int64(row.Version)
	}
	// 只改 UA 与规则版本仍等于读出快照的行: 期间被重新索引或被其他规则实例改过的行
	// 跳过, 由下一遍重新读取计算(版本是规则哈希, 不能按大小比较先后)。
	if _, err := r.db.ExecContext(ctx, `
UPDATE token_analysis_request_summaries s
SET client_family = t.family, client_family_version = $5
FROM (
    SELECT unnest($1::bigint[]) AS id, unnest($2::text[]) AS family,
           unnest($3::text[]) AS user_agent, unnest($4::int[]) AS version
) t
WHERE s.id = t.id AND s.user_agent = t.user_agent AND s.client_family_version = t.version`,
		pq.Array(ids), pq.Array(families), pq.Array(userAgents), pq.Array(versions), version); err != nil {
		return fmt.Errorf("update token analysis client families: %w", err)
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
	var completedAt sql.NullTime
	var responseStatus sql.NullInt32
	var summaryRaw, reasonsRaw []byte
	if err := rows.Scan(
		&item.ID,
		&item.ArchiveID,
		&usageLogID,
		&item.MatchConfidence,
		&item.UsageMatchMethod,
		&completedAt,
		&responseStatus,
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
	if completedAt.Valid {
		item.CompletedAt = &completedAt.Time
	}
	if responseStatus.Valid {
		status := int(responseStatus.Int32)
		item.ResponseStatus = &status
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
	switch filters.Origin {
	case "human", "ai":
		add(prefix+"summary_json->'request_origin'->>'kind' = $%d", filters.Origin)
	case "unknown":
		where = append(where, "COALESCE("+prefix+"summary_json->'request_origin'->>'kind', '') NOT IN ('human', 'ai')")
	}
	if !filters.IncludeUnmatched {
		where = append(where, prefix+"usage_log_id IS NOT NULL")
	}
	return where, args
}

// appendUserEmailFilter 在已 JOIN users 的查询上追加成员邮箱模糊搜索(ILIKE),
// 供项目消耗排行与请求明细按邮箱筛选; alias 为 users 表别名。用户行 email 为空
// (未归因/未匹配)时 ILIKE NULL 不命中, 正是按邮箱搜索应有的语义。
func appendUserEmailFilter(where []string, args []any, email, alias string) ([]string, []any) {
	email = strings.TrimSpace(email)
	if email == "" {
		return where, args
	}
	prefix := strings.TrimSpace(alias)
	if prefix != "" {
		prefix += "."
	}
	args = append(args, "%"+escapeLikePattern(email)+"%")
	where = append(where, fmt.Sprintf(prefix+"email ILIKE $%d", len(args)))
	return where, args
}

// tokenAnalysisHumanInputMergeWindow 是人工输入次数的归并窗口(请求来源设计"人工计数口径")。
const tokenAnalysisHumanInputMergeWindow = 15 * time.Minute

// tokenAnalysisHumanInputKey 是人工输入次数子查询的分组列(表达式与输出别名)。
type tokenAnalysisHumanInputKey struct{ expr, alias string }

// tokenAnalysisHumanInputSQL 是概览与用户/项目排行共用的人工输入次数(估算)子查询:
// 在与主查询相同的筛选集合(同一 where 与占位参数, withUsers 时 JOIN users 以应用邮箱条件)
// 的人工标签行内, 按 (api_key_id, fp_version, fp_state, prefix_end_hash) 分区、
// (event_time, archive_id) 排序取前一行; 前一行相距不超过归并窗口即归并到它, 连续链只
// 计链首, 次数计入链首自身的分组。无指纹、无 Key 的行逐条计数。相同提示词不证明是重试。
// 不带 keys 时返回单列标量子查询。窗口函数只扫描一次筛选集合: 相关子查询逐行探测在长
// 重复桶上慢一个数量级(设计"人工计数口径"的性能记录)。
func tokenAnalysisHumanInputSQL(where []string, withUsers bool, keys ...tokenAnalysisHumanInputKey) string {
	var inner, outer strings.Builder
	for _, key := range keys {
		inner.WriteString(key.expr + " AS " + key.alias + ", ")
		outer.WriteString("h." + key.alias + ", ")
	}
	join := ""
	if withUsers {
		join = " LEFT JOIN users u ON u.id = s.user_id"
	}
	groupBy := ""
	if len(keys) > 0 {
		groupBy = " GROUP BY " + strings.TrimSuffix(outer.String(), ", ")
	}
	return fmt.Sprintf(`SELECT %sCOUNT(*) AS human_input_count FROM (
    SELECT %ss.event_time, s.fp_state, s.prefix_end_hash, s.api_key_id,
           LAG(s.event_time) OVER (PARTITION BY s.api_key_id, s.fp_version, s.fp_state, s.prefix_end_hash ORDER BY s.event_time, s.archive_id) AS prev_time
    FROM token_analysis_request_summaries s%s
    WHERE %s AND s.summary_json->'request_origin'->>'kind' = 'human'
) h
WHERE NOT COALESCE(h.fp_state = '%s' AND h.prefix_end_hash IS NOT NULL AND h.api_key_id IS NOT NULL
    AND h.prev_time >= h.event_time - INTERVAL '%d seconds', FALSE)%s`,
		outer.String(), inner.String(), join, strings.Join(where, " AND "),
		service.TokenAnalysisFingerprintOK, int(tokenAnalysisHumanInputMergeWindow/time.Second), groupBy)
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

// tokenAnalysisUserOrderBy 把用户聚合的排序参数映射到 SELECT 别名,
// switch 白名单防注入; 未识别值回退默认 total_tokens。
func tokenAnalysisUserOrderBy(params pagination.PaginationParams) string {
	order := params.NormalizedSortOrder(pagination.SortOrderDesc)
	switch strings.TrimSpace(params.SortBy) {
	case "actual_cost":
		return "actual_cost " + order + ", total_tokens DESC, request_count DESC"
	case "request_count":
		return "request_count " + order + ", total_tokens DESC, actual_cost DESC"
	case "last_event_time":
		return "last_event_time " + order + ", total_tokens DESC, actual_cost DESC"
	default: // total_tokens
		return "total_tokens " + order + ", actual_cost DESC, request_count DESC"
	}
}

// tokenAnalysisProjectOrderBy 把项目聚合的排序参数映射到 SELECT 别名,
// switch 白名单防注入; 未识别值回退默认 total_tokens。
func tokenAnalysisProjectOrderBy(params pagination.PaginationParams) string {
	order := params.NormalizedSortOrder(pagination.SortOrderDesc)
	switch strings.TrimSpace(params.SortBy) {
	case "request_count":
		return "request_count " + order + ", total_tokens DESC"
	case "actual_cost":
		return "actual_cost " + order + ", total_tokens DESC"
	case "last_event_time":
		return "last_event_time " + order + ", total_tokens DESC"
	default:
		return "total_tokens " + order + ", actual_cost DESC, request_count DESC"
	}
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
