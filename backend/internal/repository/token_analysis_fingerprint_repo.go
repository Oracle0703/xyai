package repository

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

// tokenAnalysisFingerprintState 是写入前锁定读出的旧指纹与候选版本。
type tokenAnalysisFingerprintState struct {
	exists         bool
	fpVersion      int32
	prefixEndHash  []byte
	prefixTailHash []byte
	itemsEndHash   []byte
	inputVersion   int64
	checkedVersion int64
}

func lockTokenAnalysisFingerprintState(ctx context.Context, tx *sql.Tx, archiveID string) (tokenAnalysisFingerprintState, error) {
	var state tokenAnalysisFingerprintState
	err := tx.QueryRowContext(ctx, `
SELECT fp_version, prefix_end_hash, prefix_tail1_hash, items_end_hash, cand_input_version, cand_checked_version
FROM token_analysis_request_summaries WHERE archive_id = $1 FOR UPDATE`, archiveID).
		Scan(&state.fpVersion, &state.prefixEndHash, &state.prefixTailHash, &state.itemsEndHash, &state.inputVersion, &state.checkedVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return tokenAnalysisFingerprintState{}, nil
	}
	if err != nil {
		return tokenAnalysisFingerprintState{}, fmt.Errorf("lock token analysis fingerprint state: %w", err)
	}
	state.exists = true
	return state, nil
}

// tokenAnalysisCandidatePlan 是本次写入后的候选快照状态。
type tokenAnalysisCandidatePlan struct {
	inputVersion   int64
	checkedVersion int64
	state          string
	// keepState 表示指纹未变, 保留已有的 cand_state/cand_overflow。
	keepState bool
	// clearEdges 表示本行不再可比较, 删除它作为子请求的候选边。
	clearEdges bool
	// fingerprintChanged 表示指纹新增或变化, 需要标记可能漏选本行的后续行。
	fingerprintChanged bool
	// bumpChildren 表示旧指纹已被子请求引用为父请求, 指纹变化或失效后这些边可能不再成立。
	bumpChildren bool
}

func planTokenAnalysisCandidateVersions(prev tokenAnalysisFingerprintState, fp service.TokenAnalysisFingerprint) tokenAnalysisCandidatePlan {
	if !fp.Comparable() {
		// 不可比较的行没有候选可算: 版本对齐, 状态记为指纹状态。
		return tokenAnalysisCandidatePlan{
			inputVersion: prev.inputVersion, checkedVersion: prev.inputVersion,
			state: fp.State, clearEdges: prev.exists, bumpChildren: len(prev.prefixEndHash) > 0,
		}
	}
	changed := !prev.exists || prev.fpVersion != fp.Version ||
		!bytes.Equal(prev.prefixEndHash, fp.PrefixEndHash) ||
		!bytes.Equal(prev.prefixTailHash, fp.PrefixTail1Hash) ||
		!bytes.Equal(prev.itemsEndHash, fp.ItemsEndHash)
	if changed {
		return tokenAnalysisCandidatePlan{
			inputVersion: prev.inputVersion + 1, checkedVersion: prev.checkedVersion,
			fingerprintChanged: true, bumpChildren: len(prev.prefixEndHash) > 0,
		}
	}
	return tokenAnalysisCandidatePlan{inputVersion: prev.inputVersion, checkedVersion: prev.checkedVersion, keepState: true}
}

func nullableBytes(value []byte) any {
	if len(value) == 0 {
		return nil
	}
	return value
}

// FindPrefixCandidates 在连接上查找候选(不取候选锁), 供测试与诊断; 分类用的候选集合
// 只经 ReplacePrefixEdges 在锁内查找。
func (r *tokenAnalysisRepository) FindPrefixCandidates(ctx context.Context, query service.TokenAnalysisPrefixCandidateQuery) ([]service.TokenAnalysisPrefixCandidate, error) {
	return findTokenAnalysisPrefixCandidates(ctx, r.db, query)
}

type tokenAnalysisQueryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// findTokenAnalysisPrefixCandidates 按三类匹配分别取条目数最多的 K+1 行(多取 1 行用于判断
// 并列溢出), 每行只归入优先级最高的一类(完整前缀 > 去尾前缀 > 关联), 与选边规则一致;
// 结果行数有界, 同 Key 大量相同前缀时也不会把全部历史行读出。
func findTokenAnalysisPrefixCandidates(ctx context.Context, db tokenAnalysisQueryer, query service.TokenAnalysisPrefixCandidateQuery) ([]service.TokenAnalysisPrefixCandidate, error) {
	if len(query.PrefixHashes) == 0 && len(query.ItemsHashes) == 0 {
		return nil, nil
	}
	const scope = `api_key_id = $1 AND fp_version = $2 AND event_time >= $3 AND event_time < $4 AND archive_id <> $5`
	const cols = `archive_id, event_time, prefix_items, prefix_end_hash, prefix_tail1_hash, items_end_hash`
	const order = ` ORDER BY prefix_items DESC, event_time DESC, archive_id LIMIT $8`
	rows, err := db.QueryContext(ctx, `
(SELECT `+cols+` FROM token_analysis_request_summaries
 WHERE `+scope+` AND prefix_end_hash = ANY($6::bytea[])`+order+`)
UNION ALL
(SELECT `+cols+` FROM token_analysis_request_summaries
 WHERE `+scope+` AND prefix_tail1_hash = ANY($6::bytea[]) AND NOT (prefix_end_hash = ANY($6::bytea[]))`+order+`)
UNION ALL
(SELECT `+cols+` FROM token_analysis_request_summaries
 WHERE `+scope+` AND items_end_hash = ANY($7::bytea[])
     AND NOT (prefix_end_hash = ANY($6::bytea[])) AND NOT (COALESCE(prefix_tail1_hash = ANY($6::bytea[]), FALSE))`+order+`)`,
		query.APIKeyID, query.FPVersion, query.From, query.To, query.ArchiveID,
		pq.ByteaArray(query.PrefixHashes), pq.ByteaArray(query.ItemsHashes), service.TokenAnalysisCandidateMaxTies+1)
	if err != nil {
		return nil, fmt.Errorf("find token analysis prefix candidates: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []service.TokenAnalysisPrefixCandidate
	for rows.Next() {
		var c service.TokenAnalysisPrefixCandidate
		if err := rows.Scan(&c.ArchiveID, &c.EventTime, &c.PrefixItems, &c.PrefixEndHash, &c.PrefixTail1Hash, &c.ItemsEndHash); err != nil {
			return nil, fmt.Errorf("scan token analysis prefix candidate: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate token analysis prefix candidates: %w", err)
	}
	return out, nil
}

func (r *tokenAnalysisRepository) ReplacePrefixEdges(ctx context.Context, write service.TokenAnalysisPrefixEdgeWrite) (bool, error) {
	var applied bool
	err := r.inTx(ctx, "token analysis prefix edges", func(tx *sql.Tx) (bool, error) {
		applied = false
		if write.Query != nil {
			// 候选锁先于行锁(与摘要写入同序)。
			if err := lockTokenAnalysisCandidates(ctx, tx, write.Query.APIKeyID); err != nil {
				return false, err
			}
		}
		var current int64
		err := tx.QueryRowContext(ctx, `SELECT cand_input_version FROM token_analysis_request_summaries WHERE archive_id = $1 FOR UPDATE`, write.ArchiveID).Scan(&current)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("lock token analysis candidate version: %w", err)
		}
		if current != write.ExpectedCandVersion {
			// 计算期间出现了新的候选或指纹变化: 丢弃本次结果, 由重新匹配任务再算。
			return false, nil
		}
		edges, state, overflow := write.Edges, write.State, write.Overflow
		if write.Query != nil {
			candidates, err := findTokenAnalysisPrefixCandidates(ctx, tx, *write.Query)
			if err != nil {
				return false, err
			}
			edges, overflow = nil, false
			if write.Select != nil {
				edges, overflow = write.Select(candidates)
			}
			if overflow {
				state = service.TokenAnalysisCandidateStateTieOverflow
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM token_analysis_prefix_edges WHERE child_archive_id = $1`, write.ArchiveID); err != nil {
			return false, fmt.Errorf("delete token analysis prefix edges: %w", err)
		}
		if len(edges) > 0 {
			parents := make([]string, len(edges))
			kinds := make([]string, len(edges))
			matched := make([]int64, len(edges))
			for i, edge := range edges {
				parents[i], kinds[i], matched[i] = edge.ParentArchiveID, edge.Kind, int64(edge.MatchedItems)
			}
			if _, err := tx.ExecContext(ctx, `
INSERT INTO token_analysis_prefix_edges (child_archive_id, parent_archive_id, kind, matched_items)
SELECT $1, unnest($2::text[]), unnest($3::text[]), unnest($4::int[])`,
				write.ArchiveID, pq.Array(parents), pq.Array(kinds), pq.Array(matched)); err != nil {
				return false, fmt.Errorf("insert token analysis prefix edges: %w", err)
			}
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE token_analysis_request_summaries
SET cand_checked_version = $2, cand_state = $3, cand_overflow = $4
WHERE archive_id = $1`, write.ArchiveID, write.ExpectedCandVersion, state, overflow); err != nil {
			return false, fmt.Errorf("confirm token analysis candidate version: %w", err)
		}
		// 候选边是本行(以及回溯经过本行的子孙)的分类输入。
		if err := markTokenAnalysisClassificationDirty(ctx, tx, []string{write.ArchiveID}); err != nil {
			return false, err
		}
		applied = true
		return true, nil
	})
	if err != nil {
		return false, err
	}
	return applied, nil
}

func (r *tokenAnalysisRepository) ListCandidateRematch(ctx context.Context, limit int) ([]service.TokenAnalysisCandidateRematchRow, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT archive_id, cand_input_version, source_file, source_offset, source_line_start, source_line_length, body_sha256,
    fp_version, prefix_end_hash, prefix_tail1_hash, items_end_hash
FROM token_analysis_request_summaries
WHERE cand_input_version <> cand_checked_version
ORDER BY id
LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list token analysis candidate rematch: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []service.TokenAnalysisCandidateRematchRow
	for rows.Next() {
		var row service.TokenAnalysisCandidateRematchRow
		var offset, lineStart, lineLength sql.NullInt64
		if err := rows.Scan(&row.ArchiveID, &row.CandInputVersion, &row.SourceFile, &offset, &lineStart, &lineLength, &row.BodySHA256,
			&row.FPVersion, &row.PrefixEndHash, &row.PrefixTail1Hash, &row.ItemsEndHash); err != nil {
			return nil, fmt.Errorf("scan token analysis candidate rematch: %w", err)
		}
		row.SourceOffset = tokenAnalysisNullInt64Ptr(offset)
		row.SourceLineStart = tokenAnalysisNullInt64Ptr(lineStart)
		row.SourceLineLength = tokenAnalysisNullInt64Ptr(lineLength)
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate token analysis candidate rematch: %w", err)
	}
	return out, nil
}

func tokenAnalysisNullInt64Ptr(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	v := value.Int64
	return &v
}

type tokenAnalysisQueryExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// propagateTokenAnalysisClassification 沿前缀边(不含关联边)给 archiveIDs 最多 D 层的子孙加
// 依赖版本 cls_dep_version, 由处理失效登记的事务调用(源事务提交之后, 设计 10.2)。子孙的
// 分类可能依赖这些行: 直接作为父请求, 或在回溯中作为祖先。只改依赖版本: 子请求比较的是
// 父请求的 cls_input_version(本行自身的事实), 传播(包括过期的消费者)不会让它们的父版本
// 失配。登记行自身也加依赖版本(它可能同时是另一登记行的子孙, 无害)。逐层查找并去重,
// 不按 L 截断(只会多算); 按 id 顺序加锁。
func propagateTokenAnalysisClassification(ctx context.Context, db tokenAnalysisQueryExecer, archiveIDs []string) error {
	seen := make(map[string]struct{}, len(archiveIDs))
	var affected, frontier []string
	for _, id := range archiveIDs {
		if _, ok := seen[id]; ok || id == "" {
			continue
		}
		seen[id] = struct{}{}
		frontier = append(frontier, id)
	}
	affected = append(affected, frontier...)
	for depth := 0; depth < service.TokenAnalysisClassificationMaxDepth && len(frontier) > 0; depth++ {
		rows, err := db.QueryContext(ctx, `
SELECT DISTINCT child_archive_id FROM token_analysis_prefix_edges
WHERE parent_archive_id = ANY($1::text[]) AND kind <> $2`, pq.Array(frontier), service.TokenAnalysisEdgeItemsLink)
		if err != nil {
			return fmt.Errorf("list token analysis classification dependents: %w", err)
		}
		children, err := scanTokenAnalysisArchiveIDs(rows)
		if err != nil {
			return fmt.Errorf("list token analysis classification dependents: %w", err)
		}
		frontier = frontier[:0:0]
		for _, id := range children {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			frontier = append(frontier, id)
		}
		affected = append(affected, frontier...)
	}
	if len(affected) == 0 {
		return nil
	}
	if _, err := db.ExecContext(ctx, `
WITH locked AS (
    SELECT id FROM token_analysis_request_summaries
    WHERE archive_id = ANY($1::text[])
    ORDER BY id
    FOR UPDATE
)
UPDATE token_analysis_request_summaries s
SET cls_dep_version = s.cls_dep_version + 1
FROM locked WHERE s.id = locked.id`, pq.Array(affected)); err != nil {
		return fmt.Errorf("propagate token analysis classification: %w", err)
	}
	return nil
}

// 源写入事务遇到死锁或序列化冲突时整体重试的次数。这些事务只读写数据库、结果是输入的
// 纯函数, 整体重放是安全的; 索引出错会推进水位, 不能只记日志放弃(设计 10.2)。
const tokenAnalysisTxAttempts = 3

// inTx 在事务内执行 fn; fn 返回 commit=false 时回滚(条件不满足、无需写入)。死锁(40P01)
// 与序列化失败(40001)按 tokenAnalysisTxAttempts 整体重试。
func (r *tokenAnalysisRepository) inTx(ctx context.Context, op string, fn func(tx *sql.Tx) (bool, error)) error {
	for attempt := 1; ; attempt++ {
		err := r.runTx(ctx, op, fn)
		if err == nil || attempt >= tokenAnalysisTxAttempts || !isTokenAnalysisRetryableTxError(err) || ctx.Err() != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(time.Duration(attempt) * 20 * time.Millisecond):
		}
	}
}

func (r *tokenAnalysisRepository) runTx(ctx context.Context, op string, fn func(tx *sql.Tx) (bool, error)) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin %s: %w", op, err)
	}
	defer func() { _ = tx.Rollback() }()
	commit, err := fn(tx)
	if err != nil || !commit {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit %s: %w", op, err)
	}
	return nil
}

func isTokenAnalysisRetryableTxError(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && (pqErr.Code == "40P01" || pqErr.Code == "40001")
}

// lockTokenAnalysisCandidates 取同一 API Key 的候选事务级咨询锁: 摘要写入(可能成为新的
// 父请求)与候选查找+写边串行, 新父请求不会在并发提交时漏出子请求的候选集合。必须是
// 事务里的第一把锁, 之后才锁摘要行, 保持统一的加锁顺序。
func lockTokenAnalysisCandidates(ctx context.Context, tx *sql.Tx, apiKeyID int64) error {
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('token_analysis_candidates:' || $1::text, 0))`, apiKeyID); err != nil {
		return fmt.Errorf("lock token analysis candidates: %w", err)
	}
	return nil
}

// markTokenAnalysisClassificationDirty 在源写入事务内给 archiveIDs 自身加分类版本并登记
// 失效(每次登记从全局序列取新的 seq, 删除后重建也不会复用旧值); 子孙由分类作业在源事务
// 提交后沿前缀边传播(设计 10.2)。改输入版本必须同时登记, 否则依赖它的子请求收不到通知。
// 提交后再查边才不会漏: 任何可能读到旧事实的子请求, 其边在源事务提交前已提交。
func markTokenAnalysisClassificationDirty(ctx context.Context, db tokenAnalysisQueryExecer, archiveIDs []string) error {
	seen := make(map[string]struct{}, len(archiveIDs))
	ids := make([]string, 0, len(archiveIDs))
	for _, id := range archiveIDs {
		if _, ok := seen[id]; ok || id == "" {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil
	}
	if _, err := db.ExecContext(ctx, `
WITH locked AS (
    SELECT id FROM token_analysis_request_summaries
    WHERE archive_id = ANY($1::text[])
    ORDER BY id
    FOR UPDATE
)
UPDATE token_analysis_request_summaries s
SET cls_input_version = s.cls_input_version + 1
FROM locked WHERE s.id = locked.id`, pq.Array(ids)); err != nil {
		return fmt.Errorf("mark token analysis classification dirty: %w", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO token_analysis_cls_invalidations (archive_id)
SELECT unnest($1::text[])
ON CONFLICT (archive_id) DO UPDATE SET seq = nextval('token_analysis_cls_invalidation_seq')`, pq.Array(ids)); err != nil {
		return fmt.Errorf("register token analysis classification invalidation: %w", err)
	}
	return nil
}
