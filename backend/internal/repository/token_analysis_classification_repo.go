package repository

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/shopspring/decimal"
)

// tokenAnalysisClassificationSelect 读取分类事实: 摘要行加当前关联、且仍存在的账单与
// 最终服务账号的平台。实际发送模型口径与用量报表一致(设计 6.2)。
const tokenAnalysisClassificationSelect = `
SELECT s.id, s.archive_id, s.event_time, s.completed_at, s.response_status, s.endpoint,
    s.usage_link_version, s.usage_link_checked_version, s.fp_version, s.has_continuation_ref,
    s.cand_input_version, s.cand_checked_version, s.cand_state, s.cand_overflow,
    s.forward_path, s.upstream_cache_key_hash, s.usage_cache_source, s.client_cache_key_hash,
    s.image_count, s.cls_input_version, s.cls_dep_version, s.upstream_items_hash, s.upstream_root_hash, s.items_end_hash,
    u.id, u.account_id, COALESCE(a.platform, ''),
    COALESCE(NULLIF(TRIM(u.upstream_model), ''), u.model), COALESCE(u.billing_mode, ''),
    u.input_tokens, u.cache_read_tokens, u.cache_creation_tokens, u.image_input_tokens,
    u.input_cost::text, u.cache_read_cost::text, u.cache_creation_cost::text, u.image_input_cost::text
FROM token_analysis_request_summaries s
LEFT JOIN usage_logs u ON u.id = s.usage_log_id
LEFT JOIN accounts a ON a.id = u.account_id`

func (r *tokenAnalysisRepository) ListClassificationDue(ctx context.Context, afterID int64, limit int) ([]service.TokenAnalysisClassificationRow, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx, tokenAnalysisClassificationSelect+`
WHERE (s.cls_input_version <> s.cls_checked_version OR s.cls_dep_version <> s.cls_dep_checked_version) AND s.id > $1
ORDER BY s.id
LIMIT $2`, afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("list token analysis classification due: %w", err)
	}
	return scanTokenAnalysisClassificationRows(rows)
}

func (r *tokenAnalysisRepository) LoadClassificationRows(ctx context.Context, archiveIDs []string) ([]service.TokenAnalysisClassificationRow, error) {
	if len(archiveIDs) == 0 {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx, tokenAnalysisClassificationSelect+`
WHERE s.archive_id = ANY($1::text[])`, pq.Array(archiveIDs))
	if err != nil {
		return nil, fmt.Errorf("load token analysis classification rows: %w", err)
	}
	return scanTokenAnalysisClassificationRows(rows)
}

func scanTokenAnalysisClassificationRows(rows *sql.Rows) ([]service.TokenAnalysisClassificationRow, error) {
	defer func() { _ = rows.Close() }()
	var out []service.TokenAnalysisClassificationRow
	for rows.Next() {
		var (
			row                                           service.TokenAnalysisClassificationRow
			completedAt                                   sql.NullTime
			status                                        sql.NullInt64
			usageID, accountID                            sql.NullInt64
			platform, sentModel, billingMode              sql.NullString
			inputTokens, readTokens, writeTokens, imgToks sql.NullInt64
			inputCost, readCost, writeCost, imgCost       sql.NullString
		)
		if err := rows.Scan(&row.ID, &row.ArchiveID, &row.EventTime, &completedAt, &status, &row.Endpoint,
			&row.UsageLinkVersion, &row.UsageLinkCheckedVersion, &row.FPVersion, &row.HasContinuationRef,
			&row.CandInputVersion, &row.CandCheckedVersion, &row.CandState, &row.CandOverflow,
			&row.ForwardPath, &row.UpstreamCacheKeyHash, &row.UsageCacheSource, &row.ClientCacheKeyHash,
			&row.ImageCount, &row.ClsInputVersion, &row.ClsDepVersion, &row.UpstreamItemsHash, &row.UpstreamRootHash, &row.ItemsEndHash,
			&usageID, &accountID, &platform, &sentModel, &billingMode,
			&inputTokens, &readTokens, &writeTokens, &imgToks,
			&inputCost, &readCost, &writeCost, &imgCost); err != nil {
			return nil, fmt.Errorf("scan token analysis classification row: %w", err)
		}
		if completedAt.Valid {
			t := completedAt.Time
			row.CompletedAt = &t
		}
		if status.Valid {
			v := int(status.Int64)
			row.ResponseStatus = &v
		}
		if usageID.Valid {
			usage := &service.TokenAnalysisClassificationUsage{
				UsageLogID: usageID.Int64, AccountID: accountID.Int64, Platform: platform.String,
				SentModel: sentModel.String, BillingMode: billingMode.String,
				InputTokens: inputTokens.Int64, CacheReadTokens: readTokens.Int64,
				CacheCreationTokens: writeTokens.Int64, ImageInputTokens: imgToks.Int64,
			}
			var err error
			for _, field := range []struct {
				dst *decimal.Decimal
				src sql.NullString
			}{{&usage.InputCost, inputCost}, {&usage.CacheReadCost, readCost}, {&usage.CacheCreationCost, writeCost}, {&usage.ImageInputCost, imgCost}} {
				if *field.dst, err = decimal.NewFromString(field.src.String); err != nil {
					return nil, fmt.Errorf("parse token analysis usage cost: %w", err)
				}
			}
			row.Usage = usage
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate token analysis classification rows: %w", err)
	}
	return out, nil
}

func (r *tokenAnalysisRepository) LoadPrefixEdges(ctx context.Context, childArchiveIDs []string) ([]service.TokenAnalysisStoredEdge, error) {
	if len(childArchiveIDs) == 0 {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT child_archive_id, parent_archive_id, kind, matched_items
FROM token_analysis_prefix_edges
WHERE child_archive_id = ANY($1::text[])`, pq.Array(childArchiveIDs))
	if err != nil {
		return nil, fmt.Errorf("load token analysis prefix edges: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []service.TokenAnalysisStoredEdge
	for rows.Next() {
		var e service.TokenAnalysisStoredEdge
		if err := rows.Scan(&e.ChildArchiveID, &e.ParentArchiveID, &e.Kind, &e.MatchedItems); err != nil {
			return nil, fmt.Errorf("scan token analysis prefix edge: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate token analysis prefix edges: %w", err)
	}
	return out, nil
}

// tokenAnalysisClassificationWrite 是写回一行分类结果的 JSON 记录(jsonb_to_recordset 输入)。
type tokenAnalysisClassificationWrite struct {
	ArchiveID          string  `json:"archive_id"`
	ClsInputVersion    int64   `json:"cls_input_version"`
	ClsDepVersion      int64   `json:"cls_dep_version"`
	CandCheckedVersion int64   `json:"cand_checked_version"`
	ParentArchiveID    string  `json:"parent_archive_id"`
	ParentClsVersion   *int64  `json:"parent_cls_version"`
	RuleVersion        int32   `json:"rule_version"`
	ClsState           string  `json:"cls_state"`
	CacheObsState      string  `json:"cache_obs_state"`
	Cause              string  `json:"cause"`
	PrefixEvidence     string  `json:"prefix_evidence"`
	CacheEligibility   string  `json:"cache_eligibility"`
	PriceEvidence      string  `json:"price_evidence"`
	ReusableTokens     *int64  `json:"reusable_tokens"`
	RecoverableTokens  *int64  `json:"recoverable_tokens"`
	Amount             *string `json:"amount"`
	ColdInputCost      *string `json:"cold_input_cost"`
	AmountInputsHash   *string `json:"amount_inputs_hash"`
}

func (r *tokenAnalysisRepository) WriteClassifications(ctx context.Context, results []service.TokenAnalysisClassificationResult) (int, error) {
	if len(results) == 0 {
		return 0, nil
	}
	writes := make([]tokenAnalysisClassificationWrite, len(results))
	for i, res := range results {
		w := tokenAnalysisClassificationWrite{
			ArchiveID: res.ArchiveID, ClsInputVersion: res.ClsInputVersion, ClsDepVersion: res.ClsDepVersion, CandCheckedVersion: res.CandCheckedVersion,
			ParentArchiveID: res.ParentArchiveID, ParentClsVersion: res.ParentClsVersion, RuleVersion: res.RuleVersion,
			ClsState: res.ClsState, CacheObsState: res.CacheObsState, Cause: res.Cause,
			PrefixEvidence: res.PrefixEvidence, CacheEligibility: res.CacheEligibility, PriceEvidence: res.PriceEvidence,
			ReusableTokens: res.ReusableTokens, RecoverableTokens: res.RecoverableTokens,
		}
		if res.Amount != nil {
			v := res.Amount.String()
			w.Amount = &v
		}
		if res.ColdInputCost != nil {
			v := res.ColdInputCost.String()
			w.ColdInputCost = &v
		}
		if len(res.AmountInputsHash) > 0 {
			v := hex.EncodeToString(res.AmountInputsHash)
			w.AmountInputsHash = &v
		}
		writes[i] = w
	}
	payload, err := json.Marshal(writes)
	if err != nil {
		return 0, fmt.Errorf("marshal token analysis classifications: %w", err)
	}
	// 子请求两个版本未变; 有父请求时父请求的分类输入版本也未变。SKIP LOCKED: 正被源写入
	// 事务持有的行直接跳过(它们的版本即将变化, 下一轮重算), 写回从不等待行锁, 因而不会
	// 与源写入形成死锁。
	result, err := r.db.ExecContext(ctx, `
WITH v AS (
    SELECT * FROM jsonb_to_recordset($1::jsonb) AS x(
        archive_id text, cls_input_version bigint, cls_dep_version bigint, cand_checked_version bigint,
        parent_archive_id text, parent_cls_version bigint, rule_version integer,
        cls_state text, cache_obs_state text, cause text,
        prefix_evidence text, cache_eligibility text, price_evidence text,
        reusable_tokens bigint, recoverable_tokens bigint,
        amount numeric, cold_input_cost numeric, amount_inputs_hash text)
), locked AS (
    SELECT s.id, v.*
    FROM token_analysis_request_summaries s
    JOIN v ON v.archive_id = s.archive_id
    WHERE s.cls_input_version = v.cls_input_version AND s.cls_dep_version = v.cls_dep_version
        AND s.cand_checked_version = v.cand_checked_version
        AND (v.parent_archive_id = '' OR EXISTS (
            SELECT 1 FROM token_analysis_request_summaries p
            WHERE p.archive_id = v.parent_archive_id AND p.cls_input_version = v.parent_cls_version))
    ORDER BY s.id
    FOR UPDATE OF s SKIP LOCKED
)
UPDATE token_analysis_request_summaries s
SET cls_checked_version = l.cls_input_version,
    cls_dep_checked_version = l.cls_dep_version,
    cls_rule_version = l.rule_version,
    cls_state = l.cls_state,
    cache_obs_state = l.cache_obs_state,
    cause = l.cause,
    prefix_evidence = l.prefix_evidence,
    cache_eligibility = l.cache_eligibility,
    price_evidence = l.price_evidence,
    parent_archive_id = l.parent_archive_id,
    parent_cls_version_used = l.parent_cls_version,
    reusable_tokens = l.reusable_tokens,
    recoverable_tokens = l.recoverable_tokens,
    amount = l.amount,
    cold_input_cost = l.cold_input_cost,
    amount_inputs_hash = decode(l.amount_inputs_hash, 'hex'),
    classified_at = NOW()
FROM locked l
WHERE s.id = l.id AND s.cls_input_version = l.cls_input_version`, string(payload))
	if err != nil {
		return 0, fmt.Errorf("write token analysis classifications: %w", err)
	}
	written, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("write token analysis classifications: %w", err)
	}
	return int(written), nil
}

func (r *tokenAnalysisRepository) MaxSummaryID(ctx context.Context) (int64, error) {
	var id int64
	if err := r.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(id), 0) FROM token_analysis_request_summaries`).Scan(&id); err != nil {
		return 0, fmt.Errorf("max token analysis summary id: %w", err)
	}
	return id, nil
}

// MarkClassificationStale 只处理规则版本: 范围内全部行都会重算, 不需要沿边传播; 只改依赖
// 版本, 不改子请求比较的输入版本。账单删除由触发器登记(migration 256), 见 DrainClassificationInvalidations。
func (r *tokenAnalysisRepository) MarkClassificationStale(ctx context.Context, ruleVersion int32, fromID, toID int64) (int64, error) {
	if toID <= fromID {
		return 0, nil
	}
	result, err := r.db.ExecContext(ctx, `
UPDATE token_analysis_request_summaries
SET cls_dep_version = cls_dep_version + 1
WHERE id > $1 AND id <= $2 AND cls_input_version = cls_checked_version AND cls_dep_version = cls_dep_checked_version
    AND cls_rule_version <> $3`,
		fromID, toID, ruleVersion)
	if err != nil {
		return 0, fmt.Errorf("mark token analysis classification outdated: %w", err)
	}
	marked, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("mark token analysis classification outdated: %w", err)
	}
	return marked, nil
}

// DrainClassificationInvalidations 处理一批失效登记: 源写入(及账单删除触发器)已在各自事务
// 内给登记行加了输入版本; 这里在源事务提交后沿前缀边给子孙加依赖版本, 再按读取时的 seq
// 删除登记。处理期间又被登记(seq 已变)的行保留到下一批。读取不加锁: 过期或重复的消费者
// 只会让子孙多重算一次, 不会改变任何行的输入版本, 因此不会留下父版本失配。
func (r *tokenAnalysisRepository) DrainClassificationInvalidations(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		return 0, nil
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT archive_id, seq FROM token_analysis_cls_invalidations
ORDER BY created_at, archive_id
LIMIT $1`, limit)
	if err != nil {
		return 0, fmt.Errorf("list token analysis classification invalidations: %w", err)
	}
	var ids []string
	var seqs []int64
	for rows.Next() {
		var id string
		var seq int64
		if err := rows.Scan(&id, &seq); err != nil {
			_ = rows.Close()
			return 0, fmt.Errorf("scan token analysis classification invalidation: %w", err)
		}
		ids, seqs = append(ids, id), append(seqs, seq)
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("list token analysis classification invalidations: %w", err)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	err = r.inTx(ctx, "token analysis classification invalidations", func(tx *sql.Tx) (bool, error) {
		if err := propagateTokenAnalysisClassification(ctx, tx, ids); err != nil {
			return false, err
		}
		if _, err := tx.ExecContext(ctx, `
DELETE FROM token_analysis_cls_invalidations q
USING (SELECT unnest($1::text[]) AS archive_id, unnest($2::bigint[]) AS seq) d
WHERE q.archive_id = d.archive_id AND q.seq = d.seq`, pq.Array(ids), pq.Array(seqs)); err != nil {
			return false, fmt.Errorf("delete token analysis classification invalidations: %w", err)
		}
		return true, nil
	})
	if err != nil {
		return 0, err
	}
	return len(ids), nil
}
