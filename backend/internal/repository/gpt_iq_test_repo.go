package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

// GPT 降智测试仓储。结果写入、心跳与批次终态都校验批次执行权令牌（lease_token），
// 被回收的旧执行者无法回写。设计见 docs/features/gpt-iq-test-design-cn.md。

type gptIQTestRepository struct{ db *sql.DB }

func NewGPTIQTestRepository(db *sql.DB) service.GPTIQTestRepository {
	return &gptIQTestRepository{db: db}
}

const gptIQConfigColumns = `enabled, schedule_enabled, substr(run_time::text, 1, 5), COALESCE(to_char(last_run_date, 'YYYY-MM-DD'), ''),
min_accuracy_percent, min_graded_runs, reasoning_ratio_percent, min_five_hour_percent, min_seven_day_percent,
daily_call_budget, retention_days, version, updated_by, updated_at`

func scanGPTIQConfig(row gptQuotaRowScanner) (*service.GPTIQTestConfig, error) {
	c := &service.GPTIQTestConfig{}
	var updatedBy sql.NullInt64
	if err := row.Scan(&c.Enabled, &c.ScheduleEnabled, &c.RunTime, &c.LastRunDate,
		&c.MinAccuracyPercent, &c.MinGradedRuns, &c.ReasoningRatioPercent, &c.MinFiveHourPercent, &c.MinSevenDayPercent,
		&c.DailyCallBudget, &c.RetentionDays, &c.Version, &updatedBy, &c.UpdatedAt); err != nil {
		return nil, err
	}
	if updatedBy.Valid {
		id := updatedBy.Int64
		c.UpdatedBy = &id
	}
	return c, nil
}

func (r *gptIQTestRepository) GetConfig(ctx context.Context) (*service.GPTIQTestConfig, error) {
	c, err := scanGPTIQConfig(r.db.QueryRowContext(ctx, `SELECT `+gptIQConfigColumns+` FROM gpt_iq_test_config WHERE id = TRUE`))
	if err != nil {
		return nil, fmt.Errorf("get gpt iq test config: %w", err)
	}
	return c, nil
}

func (r *gptIQTestRepository) SaveConfig(ctx context.Context, cfg service.GPTIQTestConfig, expectedVersion, updatedBy int64, markRunDate string) (*service.GPTIQTestConfig, error) {
	out, err := scanGPTIQConfig(r.db.QueryRowContext(ctx, `
UPDATE gpt_iq_test_config
SET enabled = $1, schedule_enabled = $2, run_time = $3::time,
    min_accuracy_percent = $4, min_graded_runs = $5, reasoning_ratio_percent = $6,
    min_five_hour_percent = $7, min_seven_day_percent = $8, daily_call_budget = $9, retention_days = $10,
    last_run_date = CASE
        WHEN $13::date IS NULL THEN last_run_date
        WHEN last_run_date IS NULL OR last_run_date < $13::date THEN $13::date
        ELSE last_run_date END,
    version = version + 1, updated_by = $11, updated_at = NOW()
WHERE id = TRUE AND version = $12
RETURNING `+gptIQConfigColumns,
		cfg.Enabled, cfg.ScheduleEnabled, cfg.RunTime,
		cfg.MinAccuracyPercent, cfg.MinGradedRuns, cfg.ReasoningRatioPercent,
		cfg.MinFiveHourPercent, cfg.MinSevenDayPercent, cfg.DailyCallBudget, cfg.RetentionDays,
		gptIQNullInt64(updatedBy), expectedVersion, gptIQNullString(markRunDate)))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrGPTIQConfigConflict
	}
	if err != nil {
		return nil, fmt.Errorf("save gpt iq test config: %w", err)
	}
	return out, nil
}

// ---- 测试项与版本 ----

const gptIQCaseColumns = `c.id, c.name, c.enabled, c.sort_order, c.public_prompt, c.public_answer, c.version, c.updated_at,
v.id, v.revision, v.kind, v.prompt, v.prompt_sha256, v.model, v.reasoning_effort, v.runs, v.timeout_seconds, v.scoring_mode, v.scoring_expected`

func scanGPTIQCase(row gptQuotaRowScanner) (*service.GPTIQTestCase, error) {
	c := &service.GPTIQTestCase{}
	if err := row.Scan(&c.ID, &c.Name, &c.Enabled, &c.SortOrder, &c.PublicPrompt, &c.PublicAnswer, &c.Version, &c.UpdatedAt,
		&c.RevisionID, &c.Revision, &c.Kind, &c.Prompt, &c.PromptSHA256, &c.Model, &c.ReasoningEffort, &c.Runs, &c.TimeoutSeconds,
		&c.ScoringMode, &c.ScoringExpected); err != nil {
		return nil, err
	}
	return c, nil
}

func (r *gptIQTestRepository) ListCases(ctx context.Context) ([]service.GPTIQTestCase, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+gptIQCaseColumns+`
FROM gpt_iq_test_cases c
JOIN gpt_iq_test_case_revisions v ON v.id = c.current_revision_id
WHERE c.deleted_at IS NULL
ORDER BY c.sort_order, c.id`)
	if err != nil {
		return nil, fmt.Errorf("list gpt iq test cases: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []service.GPTIQTestCase{}
	for rows.Next() {
		c, err := scanGPTIQCase(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func (r *gptIQTestRepository) GetCase(ctx context.Context, id int64) (*service.GPTIQTestCase, error) {
	c, err := scanGPTIQCase(r.db.QueryRowContext(ctx, `SELECT `+gptIQCaseColumns+`
FROM gpt_iq_test_cases c
JOIN gpt_iq_test_case_revisions v ON v.id = c.current_revision_id
WHERE c.id = $1 AND c.deleted_at IS NULL`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrGPTIQCaseNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get gpt iq test case %d: %w", id, err)
	}
	return c, nil
}

func insertGPTIQRevision(ctx context.Context, tx *sql.Tx, caseID int64, revision int, spec service.GPTIQRevisionSpec, actorID int64) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `
INSERT INTO gpt_iq_test_case_revisions
    (case_id, revision, kind, prompt, prompt_sha256, model, reasoning_effort, runs, timeout_seconds, scoring_mode, scoring_expected, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING id`,
		caseID, revision, spec.Kind, spec.Prompt, spec.PromptSHA256, spec.Model, spec.ReasoningEffort, spec.Runs, spec.TimeoutSeconds,
		spec.ScoringMode, spec.ScoringExpected, gptIQNullInt64(actorID)).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert gpt iq test revision: %w", err)
	}
	return id, nil
}

func (r *gptIQTestRepository) CreateCase(ctx context.Context, in service.GPTIQCaseInput, spec service.GPTIQRevisionSpec, actorID int64) (*service.GPTIQTestCase, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var id int64
	if err := tx.QueryRowContext(ctx, `
INSERT INTO gpt_iq_test_cases (name, enabled, sort_order, public_prompt, public_answer, created_by, updated_by)
VALUES ($1, $2, $3, $4, $5, $6, $6)
RETURNING id`, in.Name, in.Enabled, in.SortOrder, in.PublicPrompt, in.PublicAnswer, gptIQNullInt64(actorID)).Scan(&id); err != nil {
		return nil, fmt.Errorf("insert gpt iq test case: %w", err)
	}
	revID, err := insertGPTIQRevision(ctx, tx, id, 1, spec, actorID)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE gpt_iq_test_cases SET current_revision_id = $2 WHERE id = $1`, id, revID); err != nil {
		return nil, fmt.Errorf("link gpt iq test revision: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetCase(ctx, id)
}

func (r *gptIQTestRepository) UpdateCase(ctx context.Context, id, expectedVersion int64, in service.GPTIQCaseInput, spec service.GPTIQRevisionSpec, actorID int64) (*service.GPTIQTestCase, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	cur, err := scanGPTIQCase(tx.QueryRowContext(ctx, `SELECT `+gptIQCaseColumns+`
FROM gpt_iq_test_cases c
JOIN gpt_iq_test_case_revisions v ON v.id = c.current_revision_id
WHERE c.id = $1 AND c.deleted_at IS NULL
FOR UPDATE OF c`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrGPTIQCaseNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock gpt iq test case %d: %w", id, err)
	}
	if cur.Version != expectedVersion {
		return nil, service.ErrGPTIQCaseConflict
	}
	revID := cur.RevisionID
	if !spec.SameAs(cur.GPTIQRevisionSpec) {
		var next int
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(revision), 0) + 1 FROM gpt_iq_test_case_revisions WHERE case_id = $1`, id).Scan(&next); err != nil {
			return nil, fmt.Errorf("next gpt iq test revision: %w", err)
		}
		if revID, err = insertGPTIQRevision(ctx, tx, id, next, spec, actorID); err != nil {
			return nil, err
		}
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE gpt_iq_test_cases
SET name = $2, enabled = $3, sort_order = $4, public_prompt = $5, public_answer = $6,
    current_revision_id = $7, version = version + 1, updated_by = $8, updated_at = NOW()
WHERE id = $1`, id, in.Name, in.Enabled, in.SortOrder, in.PublicPrompt, in.PublicAnswer, revID, gptIQNullInt64(actorID)); err != nil {
		return nil, fmt.Errorf("update gpt iq test case %d: %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetCase(ctx, id)
}

func (r *gptIQTestRepository) DeleteCase(ctx context.Context, id, expectedVersion, actorID int64) error {
	res, err := r.db.ExecContext(ctx, `
UPDATE gpt_iq_test_cases
SET deleted_at = NOW(), enabled = FALSE, version = version + 1, updated_by = $3, updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL AND version = $2`, id, expectedVersion, gptIQNullInt64(actorID))
	if err != nil {
		return fmt.Errorf("delete gpt iq test case %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	var exists bool
	if err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM gpt_iq_test_cases WHERE id = $1 AND deleted_at IS NULL)`, id).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return service.ErrGPTIQCaseConflict
	}
	return service.ErrGPTIQCaseNotFound
}

func (r *gptIQTestRepository) ListRevisions(ctx context.Context, ids []int64) (map[int64]*service.GPTIQTestRevision, error) {
	out := map[int64]*service.GPTIQTestRevision{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT id, case_id, revision, kind, prompt, prompt_sha256, model, reasoning_effort, runs, timeout_seconds, scoring_mode, scoring_expected
FROM gpt_iq_test_case_revisions WHERE id = ANY($1)`, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("list gpt iq test revisions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		v := &service.GPTIQTestRevision{}
		if err := rows.Scan(&v.ID, &v.CaseID, &v.Revision, &v.Kind, &v.Prompt, &v.PromptSHA256, &v.Model, &v.ReasoningEffort,
			&v.Runs, &v.TimeoutSeconds, &v.ScoringMode, &v.ScoringExpected); err != nil {
			return nil, err
		}
		out[v.ID] = v
	}
	return out, rows.Err()
}

// ---- 批次 ----

const gptIQBatchColumns = `b.id, b.trigger, to_char(b.run_date, 'YYYY-MM-DD'), b.triggered_by, b.scope_groups, b.entry_ids, b.case_ids,
b.status, b.lease_token, b.cancel_requested, b.heartbeat_at, b.total_calls, b.failure_reason, b.started_at, b.finished_at`

func scanGPTIQBatch(row gptQuotaRowScanner, extra ...any) (*service.GPTIQTestBatch, error) {
	b := &service.GPTIQTestBatch{}
	var triggeredBy sql.NullInt64
	var finished sql.NullTime
	var groups pq.StringArray
	var entries, cases pq.Int64Array
	dest := []any{&b.ID, &b.Trigger, &b.RunDate, &triggeredBy, &groups, &entries, &cases,
		&b.Status, &b.LeaseToken, &b.CancelRequested, &b.HeartbeatAt, &b.TotalCalls, &b.FailureReason, &b.StartedAt, &finished}
	if err := row.Scan(append(dest, extra...)...); err != nil {
		return nil, err
	}
	if triggeredBy.Valid {
		id := triggeredBy.Int64
		b.TriggeredBy = &id
	}
	b.FinishedAt = nullTimePtr(finished)
	b.ScopeGroups = append([]string{}, groups...)
	b.EntryIDs = append([]int64{}, entries...)
	b.CaseIDs = append([]int64{}, cases...)
	return b, nil
}

func (r *gptIQTestRepository) CreateBatch(ctx context.Context, batch *service.GPTIQTestBatch) (*service.GPTIQTestBatch, error) {
	var triggeredBy sql.NullInt64
	if batch.TriggeredBy != nil {
		triggeredBy = sql.NullInt64{Int64: *batch.TriggeredBy, Valid: true}
	}
	out, err := scanGPTIQBatch(r.db.QueryRowContext(ctx, `
WITH b AS (
    INSERT INTO gpt_iq_test_batches (trigger, run_date, triggered_by, scope_groups, entry_ids, case_ids, status, lease_token, total_calls)
    VALUES ($1, $2::date, $3, $4, $5, $6, 'running', $7, $8)
    RETURNING *
)
SELECT `+gptIQBatchColumns+` FROM b`,
		// nil 切片会被编码成 SQL NULL，而三列都是 NOT NULL，统一换成空数组。
		batch.Trigger, batch.RunDate, triggeredBy, pq.StringArray(append([]string{}, batch.ScopeGroups...)),
		pq.Int64Array(append([]int64{}, batch.EntryIDs...)), pq.Int64Array(append([]int64{}, batch.CaseIDs...)),
		batch.LeaseToken, batch.TotalCalls))
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return nil, service.ErrGPTIQBatchRunning
		}
		return nil, fmt.Errorf("create gpt iq test batch: %w", err)
	}
	return out, nil
}

func (r *gptIQTestRepository) ClaimRunDate(ctx context.Context, date string) (bool, error) {
	var claimed int
	err := r.db.QueryRowContext(ctx, `
UPDATE gpt_iq_test_config SET last_run_date = $1::date
WHERE id = TRUE AND (last_run_date IS NULL OR last_run_date < $1::date)
RETURNING 1`, date).Scan(&claimed)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claim gpt iq test run date: %w", err)
	}
	return true, nil
}

func (r *gptIQTestRepository) FinishBatch(ctx context.Context, id int64, token, status string) error {
	return r.CloseBatch(ctx, id, token, status, "")
}

func (r *gptIQTestRepository) CloseBatch(ctx context.Context, id int64, token, status, failureReason string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `
UPDATE gpt_iq_test_batches SET status = $3, failure_reason = $4, finished_at = NOW()
WHERE id = $1 AND lease_token = $2 AND status = 'running'`, id, token, status, failureReason)
	if err != nil {
		return fmt.Errorf("finish gpt iq test batch %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return service.ErrGPTIQLeaseLost
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE gpt_iq_test_results SET status = 'cancelled', reason = 'cancelled', finished_at = NOW()
WHERE batch_id = $1 AND status = 'running'`, id); err != nil {
		return fmt.Errorf("close gpt iq test results for batch %d: %w", id, err)
	}
	return tx.Commit()
}

func (r *gptIQTestRepository) HeartbeatBatch(ctx context.Context, id int64, token string) (bool, bool, error) {
	var cancelRequested bool
	err := r.db.QueryRowContext(ctx, `
UPDATE gpt_iq_test_batches SET heartbeat_at = NOW()
WHERE id = $1 AND lease_token = $2 AND status = 'running'
RETURNING cancel_requested`, id, token).Scan(&cancelRequested)
	if errors.Is(err, sql.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("heartbeat gpt iq test batch %d: %w", id, err)
	}
	return true, cancelRequested, nil
}

func (r *gptIQTestRepository) RequestCancel(ctx context.Context, id int64) (bool, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE gpt_iq_test_batches SET cancel_requested = TRUE WHERE id = $1 AND status = 'running'`, id)
	if err != nil {
		return false, fmt.Errorf("cancel gpt iq test batch %d: %w", id, err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *gptIQTestRepository) ReapAbandoned(ctx context.Context, staleAfter time.Duration) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `
UPDATE gpt_iq_test_batches SET status = 'abandoned', finished_at = NOW()
WHERE status = 'running' AND heartbeat_at < NOW() - $1::double precision * INTERVAL '1 second'
RETURNING id`, staleAfter.Seconds())
	if err != nil {
		return 0, fmt.Errorf("reap gpt iq test batches: %w", err)
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE gpt_iq_test_results SET status = 'abandoned', reason = 'abandoned', finished_at = NOW()
WHERE batch_id = ANY($1) AND status = 'running'`, pq.Array(ids)); err != nil {
		return 0, fmt.Errorf("abandon gpt iq test results: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(ids), nil
}

func (r *gptIQTestRepository) GetBatch(ctx context.Context, id int64) (*service.GPTIQTestBatch, error) {
	b, err := scanGPTIQBatch(r.db.QueryRowContext(ctx, `SELECT `+gptIQBatchColumns+` FROM gpt_iq_test_batches b WHERE b.id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrGPTIQBatchNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get gpt iq test batch %d: %w", id, err)
	}
	return b, nil
}

func (r *gptIQTestRepository) ListBatchStatuses(ctx context.Context, ids []int64) (map[int64]string, error) {
	out := map[int64]string{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id, status FROM gpt_iq_test_batches WHERE id = ANY($1)`, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("list gpt iq test batch statuses: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int64
		var status string
		if err := rows.Scan(&id, &status); err != nil {
			return nil, err
		}
		out[id] = status
	}
	return out, rows.Err()
}

func (r *gptIQTestRepository) ListBatchSummaries(ctx context.Context, limit int, entryIDs []int64) ([]service.GPTIQTestBatchSummary, error) {
	// entryIDs 为 nil 表示全站；非 nil（可为空）时只看包含这些条目的批次，计数也只统计这些条目。
	scoped := entryIDs != nil
	scope := pq.Int64Array(append([]int64{}, entryIDs...))
	rows, err := r.db.QueryContext(ctx, `
SELECT `+gptIQBatchColumns+`,
       COALESCE(s.completed, 0), COALESCE(s.ok, 0), COALESCE(s.errors, 0), COALESCE(s.skipped, 0), COALESCE(s.interrupted, 0),
       COALESCE(s.graded, 0), COALESCE(s.correct, 0)
FROM gpt_iq_test_batches b
LEFT JOIN LATERAL (
    SELECT COUNT(*) FILTER (WHERE status <> 'running') AS completed,
           COUNT(*) FILTER (WHERE status = 'ok') AS ok,
           COUNT(*) FILTER (WHERE status = 'error') AS errors,
           COUNT(*) FILTER (WHERE status = 'skipped') AS skipped,
           COUNT(*) FILTER (WHERE status IN ('cancelled', 'abandoned')) AS interrupted,
           COUNT(*) FILTER (WHERE status = 'ok' AND correct IS NOT NULL) AS graded,
           COUNT(*) FILTER (WHERE status = 'ok' AND correct) AS correct
    FROM gpt_iq_test_results WHERE batch_id = b.id AND (NOT $2 OR entry_id = ANY($3))
) s ON TRUE
WHERE NOT $2 OR b.entry_ids && $3
ORDER BY b.id DESC
LIMIT $1`, limit, scoped, scope)
	if err != nil {
		return nil, fmt.Errorf("list gpt iq test batches: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []service.GPTIQTestBatchSummary{}
	for rows.Next() {
		var s service.GPTIQTestBatchSummary
		b, err := scanGPTIQBatch(rows, &s.Completed, &s.OK, &s.Errors, &s.Skipped, &s.Interrupted, &s.Graded, &s.Correct)
		if err != nil {
			return nil, err
		}
		s.GPTIQTestBatch = *b
		out = append(out, s)
	}
	return out, rows.Err()
}

// ---- 每日派发预算 ----

func (r *gptIQTestRepository) ReserveDispatch(ctx context.Context, batchID int64, token, date string, budget int) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	// FOR SHARE 锁住批次行：回收器把批次标 abandoned 要等本事务结束，预占与执行权校验原子完成。
	var cancelRequested bool
	err = tx.QueryRowContext(ctx, `SELECT cancel_requested FROM gpt_iq_test_batches WHERE id = $1 AND lease_token = $2 AND status = 'running' FOR SHARE`, batchID, token).Scan(&cancelRequested)
	if errors.Is(err, sql.ErrNoRows) {
		return false, service.ErrGPTIQLeaseLost
	}
	if err != nil {
		return false, fmt.Errorf("check gpt iq test batch lease: %w", err)
	}
	if cancelRequested {
		return false, service.ErrGPTIQBatchCancelRequested
	}
	if budget <= 0 {
		return false, nil
	}
	var dispatched int
	err = tx.QueryRowContext(ctx, `
INSERT INTO gpt_iq_test_daily_usage (run_date, dispatched) VALUES ($1::date, 1)
ON CONFLICT (run_date) DO UPDATE SET dispatched = gpt_iq_test_daily_usage.dispatched + 1
WHERE gpt_iq_test_daily_usage.dispatched < $2
RETURNING dispatched`, date, budget).Scan(&dispatched)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("reserve gpt iq test dispatch: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (r *gptIQTestRepository) ReleaseDispatch(ctx context.Context, date string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE gpt_iq_test_daily_usage SET dispatched = GREATEST(dispatched - 1, 0) WHERE run_date = $1::date`, date)
	if err != nil {
		return fmt.Errorf("release gpt iq test dispatch: %w", err)
	}
	return nil
}

func (r *gptIQTestRepository) DispatchedOn(ctx context.Context, date string) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COALESCE((SELECT dispatched FROM gpt_iq_test_daily_usage WHERE run_date = $1::date), 0)`, date).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("get gpt iq test dispatched: %w", err)
	}
	return n, nil
}

// ---- 结果 ----

func (r *gptIQTestRepository) InsertResult(ctx context.Context, token string, res *service.GPTIQTestResult) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
INSERT INTO gpt_iq_test_results
    (batch_id, run_date, trigger, entry_id, account_id, case_id, revision_id, run_index, status, reason,
     model_requested, reasoning_effort, started_at, finished_at)
SELECT b.id, $2::date, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
FROM gpt_iq_test_batches b
WHERE b.id = $1 AND b.lease_token = $15 AND b.status = 'running'
RETURNING id`,
		res.BatchID, res.RunDate, res.Trigger, res.EntryID, res.AccountID, res.CaseID, res.RevisionID, res.RunIndex,
		res.Status, res.Reason, res.ModelRequested, res.ReasoningEffort, res.StartedAt, gptIQNullTime(res.FinishedAt), token).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, service.ErrGPTIQLeaseLost
	}
	if err != nil {
		return 0, fmt.Errorf("insert gpt iq test result: %w", err)
	}
	return id, nil
}

func (r *gptIQTestRepository) FinishResult(ctx context.Context, token string, res *service.GPTIQTestResult) error {
	out, err := r.db.ExecContext(ctx, `
UPDATE gpt_iq_test_results AS t
SET status = $3, reason = $4, error_summary = $5, http_status = $6, correct = $7, scored_revision_id = $8,
    attempts = $9, model_requested = $10, model_responded = $11, answer_text = $12, answer_truncated = $13,
    input_tokens = $14, cached_tokens = $15, output_tokens = $16, reasoning_tokens = $17,
    ttft_ms = $18, duration_ms = $19, finished_at = $20
FROM gpt_iq_test_batches b
WHERE t.id = $1 AND t.status = 'running' AND b.id = t.batch_id AND b.lease_token = $2 AND b.status = 'running'`,
		res.ID, token, res.Status, res.Reason, gptIQDBText(res.ErrorSummary), gptIQNullIntPtr(res.HTTPStatus), gptIQNullBool(res.Correct),
		gptIQNullInt64Ptr(res.ScoredRevisionID), res.Attempts, gptIQDBText(res.ModelRequested), gptIQDBText(res.ModelResponded),
		gptIQDBText(res.AnswerText), res.AnswerTruncated,
		gptIQNullIntPtr(res.InputTokens), gptIQNullIntPtr(res.CachedTokens), gptIQNullIntPtr(res.OutputTokens), gptIQNullIntPtr(res.ReasoningTokens),
		gptIQNullIntPtr(res.TTFTMs), gptIQNullIntPtr(res.DurationMs), gptIQNullTime(res.FinishedAt))
	if err != nil {
		return fmt.Errorf("finish gpt iq test result %d: %w", res.ID, err)
	}
	if n, _ := out.RowsAffected(); n == 0 {
		return service.ErrGPTIQLeaseLost
	}
	return nil
}

func gptIQResultColumns(answerLimit int) string {
	answer := "answer_text"
	if answerLimit > 0 {
		answer = fmt.Sprintf("left(answer_text, %d)", answerLimit)
	}
	return `id, batch_id, to_char(run_date, 'YYYY-MM-DD'), trigger, entry_id, account_id, case_id, revision_id, scored_revision_id,
run_index, attempts, status, reason, error_summary, http_status, correct, model_requested, model_responded, reasoning_effort,
` + answer + `, answer_truncated, input_tokens, cached_tokens, output_tokens, reasoning_tokens, ttft_ms, duration_ms, started_at, finished_at`
}

func scanGPTIQResults(rows *sql.Rows) ([]service.GPTIQTestResult, error) {
	defer func() { _ = rows.Close() }()
	out := []service.GPTIQTestResult{}
	for rows.Next() {
		var res service.GPTIQTestResult
		var scored, httpStatus, input, cached, output, reasoning, ttft, duration sql.NullInt64
		var correct sql.NullBool
		var finished sql.NullTime
		if err := rows.Scan(&res.ID, &res.BatchID, &res.RunDate, &res.Trigger, &res.EntryID, &res.AccountID, &res.CaseID, &res.RevisionID, &scored,
			&res.RunIndex, &res.Attempts, &res.Status, &res.Reason, &res.ErrorSummary, &httpStatus, &correct, &res.ModelRequested, &res.ModelResponded,
			&res.ReasoningEffort, &res.AnswerText, &res.AnswerTruncated, &input, &cached, &output, &reasoning, &ttft, &duration,
			&res.StartedAt, &finished); err != nil {
			return nil, err
		}
		if scored.Valid {
			v := scored.Int64
			res.ScoredRevisionID = &v
		}
		if correct.Valid {
			v := correct.Bool
			res.Correct = &v
		}
		res.HTTPStatus = gptIQIntPtr(httpStatus)
		res.InputTokens, res.CachedTokens = gptIQIntPtr(input), gptIQIntPtr(cached)
		res.OutputTokens, res.ReasoningTokens = gptIQIntPtr(output), gptIQIntPtr(reasoning)
		res.TTFTMs, res.DurationMs = gptIQIntPtr(ttft), gptIQIntPtr(duration)
		res.FinishedAt = nullTimePtr(finished)
		out = append(out, res)
	}
	return out, rows.Err()
}

func (r *gptIQTestRepository) ListBatchResults(ctx context.Context, batchID int64, answerLimit int) ([]service.GPTIQTestResult, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+gptIQResultColumns(answerLimit)+`
FROM gpt_iq_test_results WHERE batch_id = $1 ORDER BY entry_id, case_id, run_index`, batchID)
	if err != nil {
		return nil, fmt.Errorf("list gpt iq test batch results: %w", err)
	}
	return scanGPTIQResults(rows)
}

func (r *gptIQTestRepository) ListResultsSince(ctx context.Context, since string, entryIDs []int64, answerLimit int) ([]service.GPTIQTestResult, error) {
	if len(entryIDs) == 0 {
		return []service.GPTIQTestResult{}, nil
	}
	rows, err := r.db.QueryContext(ctx, `SELECT `+gptIQResultColumns(answerLimit)+`
FROM gpt_iq_test_results WHERE run_date >= $1::date AND entry_id = ANY($2)
ORDER BY batch_id, entry_id, case_id, run_index`, since, pq.Array(entryIDs))
	if err != nil {
		return nil, fmt.Errorf("list gpt iq test results: %w", err)
	}
	return scanGPTIQResults(rows)
}

func (r *gptIQTestRepository) ListEntryResults(ctx context.Context, entryID, caseID int64, since string) ([]service.GPTIQTestResult, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+gptIQResultColumns(0)+`
FROM gpt_iq_test_results
WHERE entry_id = $1 AND ($2::bigint = 0 OR case_id = $2::bigint) AND run_date >= $3::date
ORDER BY batch_id DESC, case_id, run_index`, entryID, caseID, since)
	if err != nil {
		return nil, fmt.Errorf("list gpt iq test entry results: %w", err)
	}
	return scanGPTIQResults(rows)
}

func (r *gptIQTestRepository) RecentRunExists(ctx context.Context, entryID, caseID int64, since time.Time) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `
SELECT EXISTS(SELECT 1 FROM gpt_iq_test_results
WHERE entry_id = $1 AND case_id = $2 AND started_at >= $3 AND status IN ('running', 'ok', 'error'))`, entryID, caseID, since).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check gpt iq test cooldown: %w", err)
	}
	return exists, nil
}

// ---- 退避 ----

func (r *gptIQTestRepository) ListBackoff(ctx context.Context, accountIDs []int64) (map[int64]time.Time, error) {
	out := map[int64]time.Time{}
	if len(accountIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx, `SELECT account_id, retry_after FROM gpt_iq_test_backoff WHERE account_id = ANY($1) AND retry_after > NOW()`, pq.Array(accountIDs))
	if err != nil {
		return nil, fmt.Errorf("list gpt iq test backoff: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int64
		var until time.Time
		if err := rows.Scan(&id, &until); err != nil {
			return nil, err
		}
		out[id] = until
	}
	return out, rows.Err()
}

func (r *gptIQTestRepository) SetBackoff(ctx context.Context, accountID int64, until time.Time, reason string) error {
	_, err := r.db.ExecContext(ctx, `
INSERT INTO gpt_iq_test_backoff (account_id, retry_after, reason, updated_at) VALUES ($1, $2, $3, NOW())
ON CONFLICT (account_id) DO UPDATE SET retry_after = EXCLUDED.retry_after, reason = EXCLUDED.reason, updated_at = NOW()`, accountID, until, reason)
	if err != nil {
		return fmt.Errorf("set gpt iq test backoff for account %d: %w", accountID, err)
	}
	return nil
}

// ---- 重新判分与清理 ----

// ListRejudgeCandidates 只取同题面、执行时就是判分题（kind=graded）的成功回答；执行时为 chat 的连通性
// 结果即使题面相同也不纳入，历史连通性记录保持未评分。
func (r *gptIQTestRepository) ListRejudgeCandidates(ctx context.Context, caseID int64, promptSHA256 string) ([]service.GPTIQTestResult, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT r.id, r.answer_text, r.answer_truncated, r.correct, r.scored_revision_id
FROM gpt_iq_test_results r
JOIN gpt_iq_test_case_revisions v ON v.id = r.revision_id
WHERE r.case_id = $1 AND r.status = 'ok' AND v.prompt_sha256 = $2 AND v.kind = 'graded'
ORDER BY r.id`, caseID, promptSHA256)
	if err != nil {
		return nil, fmt.Errorf("list gpt iq test rejudge candidates: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []service.GPTIQTestResult{}
	for rows.Next() {
		var res service.GPTIQTestResult
		var correct sql.NullBool
		var scored sql.NullInt64
		if err := rows.Scan(&res.ID, &res.AnswerText, &res.AnswerTruncated, &correct, &scored); err != nil {
			return nil, err
		}
		if correct.Valid {
			v := correct.Bool
			res.Correct = &v
		}
		if scored.Valid {
			v := scored.Int64
			res.ScoredRevisionID = &v
		}
		out = append(out, res)
	}
	return out, rows.Err()
}

func (r *gptIQTestRepository) UpdateCorrect(ctx context.Context, updates []service.GPTIQCorrectUpdate) error {
	if len(updates) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, u := range updates {
		// 版本单调：已被更新版本判过分的结果不允许被较早的重评覆盖（版本 ID 递增）。
		if _, err := tx.ExecContext(ctx, `UPDATE gpt_iq_test_results SET correct = $2, scored_revision_id = $3
WHERE id = $1 AND status = 'ok' AND (scored_revision_id IS NULL OR scored_revision_id <= $3)`,
			u.ResultID, gptIQNullBool(u.Correct), u.ScoredRevisionID); err != nil {
			return fmt.Errorf("rejudge gpt iq test result %d: %w", u.ResultID, err)
		}
	}
	return tx.Commit()
}

// CountScoredWith 统计 ids 中当前以 revisionID 判分的结果数。
func (r *gptIQTestRepository) CountScoredWith(ctx context.Context, ids []int64, revisionID int64) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	var n int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM gpt_iq_test_results WHERE id = ANY($1) AND scored_revision_id = $2`, pq.Array(ids), revisionID).Scan(&n); err != nil {
		return 0, fmt.Errorf("count gpt iq test rejudged results: %w", err)
	}
	return n, nil
}

func (r *gptIQTestRepository) Cleanup(ctx context.Context, before string) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `DELETE FROM gpt_iq_test_results WHERE run_date < $1::date`, before)
	if err != nil {
		return 0, fmt.Errorf("cleanup gpt iq test results: %w", err)
	}
	removed, _ := res.RowsAffected()
	for _, stmt := range []string{
		`DELETE FROM gpt_iq_test_batches WHERE run_date < $1::date AND status <> 'running'`,
		`DELETE FROM gpt_iq_test_daily_usage WHERE run_date < $1::date`,
	} {
		if _, err := tx.ExecContext(ctx, stmt, before); err != nil {
			return 0, fmt.Errorf("cleanup gpt iq test: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM gpt_iq_test_backoff WHERE retry_after < NOW()`); err != nil {
		return 0, fmt.Errorf("cleanup gpt iq test backoff: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return removed, nil
}

// ---- 操作者 ----

// LoadActor 按认证中间件写入的操作者 ID 重读用户：完整管理员（含完整管理员 API Key）可配置、全站范围；
// 组织负责人（admin.organization_manage）运行与查看限其组织授权，另持 admin.gpt_iq_test 时才可配置；
// 非负责人的子管理员持 admin.gpt_iq_test 时可配置、全站范围。
func (r *gptIQTestRepository) LoadActor(ctx context.Context) (*service.GPTIQTestActor, error) {
	id := service.DepartmentActorID(ctx)
	if id <= 0 {
		return nil, service.ErrGPTIQScopeDenied
	}
	if service.DepartmentActorIsAdminAPIKey(ctx) {
		return &service.GPTIQTestActor{UserID: id, CanConfigure: true, FullScope: true}, nil
	}
	var user service.User
	var permissions []byte
	err := r.db.QueryRowContext(ctx, `SELECT id, role, admin_permissions, status FROM users WHERE id = $1 AND deleted_at IS NULL`, id).
		Scan(&user.ID, &user.Role, &permissions, &user.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrGPTIQScopeDenied
	}
	if err != nil {
		return nil, fmt.Errorf("load gpt iq test actor: %w", err)
	}
	if len(permissions) > 0 {
		if err := json.Unmarshal(permissions, &user.AdminPermissions); err != nil {
			return nil, fmt.Errorf("decode gpt iq test actor permissions: %w", err)
		}
	}
	if user.Status != service.StatusActive {
		return nil, service.ErrGPTIQScopeDenied
	}
	configure := service.HasAdminPermission(&user, service.AdminPermissionGPTIQTest)
	switch {
	case user.Role == service.RoleAdmin:
		return &service.GPTIQTestActor{UserID: id, CanConfigure: true, FullScope: true}, nil
	case user.Role != service.RoleSubAdmin:
		return nil, service.ErrGPTIQScopeDenied
	case service.HasAdminPermission(&user, service.AdminPermissionOrganizationManage):
		// 组织负责人先按组织授权限定运行与查看范围；另持 admin.gpt_iq_test 只增加配置能力，不扩大范围。
		groups, err := loadOrganizationGrants(ctx, r.db, id)
		if err != nil {
			return nil, err
		}
		if len(groups) == 0 {
			return nil, service.ErrGPTIQScopeDenied
		}
		return &service.GPTIQTestActor{UserID: id, CanConfigure: configure, Groups: groups}, nil
	case configure:
		return &service.GPTIQTestActor{UserID: id, CanConfigure: true, FullScope: true}, nil
	}
	return nil, service.ErrGPTIQScopeDenied
}

// ---- 空值辅助 ----

// gptIQDBText 去掉 PostgreSQL text 不接受的 NUL 与非法 UTF-8（服务层已清理，这里兜底）。
func gptIQDBText(v string) string {
	return strings.ToValidUTF8(strings.ReplaceAll(v, string(rune(0)), ""), "")
}

func gptIQNullInt64(v int64) sql.NullInt64 {
	return sql.NullInt64{Int64: v, Valid: v > 0}
}

func gptIQNullInt64Ptr(v *int64) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *v, Valid: true}
}

func gptIQNullIntPtr(v *int) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*v), Valid: true}
}

func gptIQIntPtr(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	n := int(v.Int64)
	return &n
}

func gptIQNullBool(v *bool) sql.NullBool {
	if v == nil {
		return sql.NullBool{}
	}
	return sql.NullBool{Bool: *v, Valid: true}
}

func gptIQNullString(v string) sql.NullString {
	return sql.NullString{String: v, Valid: v != ""}
}

func gptIQNullTime(v *time.Time) sql.NullTime {
	if v == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *v, Valid: true}
}
