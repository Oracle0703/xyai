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

type gptQuotaDisplayRepository struct{ db *sql.DB }

// gptQuotaLikeEscaper 转义 ILIKE 通配符，搜索词按字面匹配（默认转义字符为反斜杠）。
var gptQuotaLikeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func NewGPTQuotaDisplayRepository(db *sql.DB) service.GPTQuotaDisplayRepository {
	return &gptQuotaDisplayRepository{db: db}
}

const gptQuotaDisplayConfigColumns = `enabled, interval_minutes, substr(start_time::text, 1, 5), substr(end_time::text, 1, 5), version, last_slot_at, updated_by, updated_at`

type gptQuotaRowScanner interface {
	Scan(dest ...any) error
}

func scanGPTQuotaDisplayConfig(row gptQuotaRowScanner) (*service.GPTQuotaDisplayConfig, error) {
	c := &service.GPTQuotaDisplayConfig{}
	var slot sql.NullTime
	var updatedBy sql.NullInt64
	if err := row.Scan(&c.Enabled, &c.IntervalMinutes, &c.StartTime, &c.EndTime, &c.Version, &slot, &updatedBy, &c.UpdatedAt); err != nil {
		return nil, err
	}
	if slot.Valid {
		t := slot.Time
		c.LastSlotAt = &t
	}
	if updatedBy.Valid {
		id := updatedBy.Int64
		c.UpdatedBy = &id
	}
	return c, nil
}

func (r *gptQuotaDisplayRepository) GetConfig(ctx context.Context) (*service.GPTQuotaDisplayConfig, error) {
	c, err := scanGPTQuotaDisplayConfig(r.db.QueryRowContext(ctx, `SELECT `+gptQuotaDisplayConfigColumns+` FROM gpt_quota_display_config WHERE id = TRUE`))
	if err != nil {
		return nil, fmt.Errorf("get gpt quota display config: %w", err)
	}
	return c, nil
}

func (r *gptQuotaDisplayRepository) SaveConfig(ctx context.Context, cfg service.GPTQuotaDisplayConfig, selections []service.GPTQuotaDisplaySelection, expectedVersion, updatedBy int64, slotFloor *time.Time) (*service.GPTQuotaDisplayConfig, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var uid sql.NullInt64
	if updatedBy > 0 {
		uid = sql.NullInt64{Int64: updatedBy, Valid: true}
	}
	out, err := scanGPTQuotaDisplayConfig(tx.QueryRowContext(ctx, `
UPDATE gpt_quota_display_config
SET enabled = $1, interval_minutes = $2, start_time = $3::time, end_time = $4::time,
    version = version + 1, updated_by = $5, updated_at = NOW(),
    last_slot_at = GREATEST(last_slot_at, $7::timestamptz)
WHERE id = TRUE AND version = $6
RETURNING `+gptQuotaDisplayConfigColumns,
		cfg.Enabled, cfg.IntervalMinutes, cfg.StartTime, cfg.EndTime, uid, expectedVersion, slotFloor))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrGPTQuotaConfigConflict
	}
	if err != nil {
		return nil, fmt.Errorf("update gpt quota display config: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE gpt_quota_display_entries SET selected = FALSE, updated_at = NOW() WHERE selected`); err != nil {
		return nil, fmt.Errorf("reset gpt quota display entries: %w", err)
	}
	for _, sel := range selections {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO gpt_quota_display_entries (account_id, display_name, selected, updated_at)
VALUES ($1, $2, TRUE, NOW())
ON CONFLICT (account_id) DO UPDATE SET display_name = EXCLUDED.display_name, selected = TRUE, updated_at = NOW()`,
			sel.AccountID, sel.DisplayName); err != nil {
			return nil, fmt.Errorf("save gpt quota display entry %d: %w", sel.AccountID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

// ListSelectedEntries 只读取资格判断需要的账号字段；软删除账号视为不存在。
func (r *gptQuotaDisplayRepository) ListSelectedEntries(ctx context.Context) ([]service.GPTQuotaDisplayEntry, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT e.id, e.account_id, e.display_name, a.id IS NOT NULL,
       COALESCE(a.name, ''), COALESCE(a.platform, ''), COALESCE(a.type, ''), a.parent_account_id,
       COALESCE(a.credentials->>'auth_mode', ''), COALESCE(a.credentials->>'openai_auth_mode', '')
FROM gpt_quota_display_entries e
LEFT JOIN accounts a ON a.id = e.account_id AND a.deleted_at IS NULL
WHERE e.selected
ORDER BY e.id`)
	if err != nil {
		return nil, fmt.Errorf("list gpt quota display entries: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []service.GPTQuotaDisplayEntry{}
	for rows.Next() {
		var e service.GPTQuotaDisplayEntry
		var parentID sql.NullInt64
		if err := rows.Scan(&e.ID, &e.AccountID, &e.DisplayName, &e.AccountExists, &e.AccountName, &e.Platform, &e.AccountType, &parentID, &e.AuthMode, &e.LegacyAuthMode); err != nil {
			return nil, err
		}
		if parentID.Valid {
			id := parentID.Int64
			e.ParentAccountID = &id
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListCandidates 在数据库分页：先 COUNT，再按"前缀匹配 → 前缀 → 编号数值 → 规范化名称 → ID"排序取一页。
// 资格判断在服务层按返回的 parent/auth_mode 字段计算；limit<=0 时只返回总数。
func (r *gptQuotaDisplayRepository) ListCandidates(ctx context.Context, search string, limit, offset int) ([]service.GPTQuotaDisplayEntry, int, error) {
	where := `a.deleted_at IS NULL AND a.platform = $1 AND a.type = $2`
	args := []any{service.PlatformOpenAI, service.AccountTypeOAuth}
	if search != "" {
		where += ` AND a.name ILIKE $3`
		args = append(args, "%"+gptQuotaLikeEscaper.Replace(search)+"%")
	}
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM accounts a WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count gpt quota candidates: %w", err)
	}
	out := []service.GPTQuotaDisplayEntry{}
	if limit <= 0 || offset >= total {
		return out, total, nil
	}
	limitArg, offsetArg := len(args)+1, len(args)+2
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`
SELECT a.id, a.name, a.platform, a.type, a.parent_account_id,
       COALESCE(a.credentials->>'auth_mode', ''), COALESCE(a.credentials->>'openai_auth_mode', '')
FROM accounts a
WHERE %s
ORDER BY (a.parent_account_id IS NULL AND lower(btrim(a.name)) ~ '^[cd]-') DESC,
         lower(left(btrim(a.name), 2)),
         NULLIF(substring(btrim(a.name) from '^[cCdD]-([0-9]+)'), '')::numeric ASC NULLS LAST,
         lower(btrim(a.name)),
         a.id
LIMIT $%d OFFSET $%d`, where, limitArg, offsetArg), append(args, limit, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("list gpt quota candidates: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		e := service.GPTQuotaDisplayEntry{AccountExists: true}
		var parentID sql.NullInt64
		if err := rows.Scan(&e.AccountID, &e.AccountName, &e.Platform, &e.AccountType, &parentID, &e.AuthMode, &e.LegacyAuthMode); err != nil {
			return nil, 0, err
		}
		if parentID.Valid {
			id := parentID.Int64
			e.ParentAccountID = &id
		}
		out = append(out, e)
	}
	return out, total, rows.Err()
}

func (r *gptQuotaDisplayRepository) ListSnapshots(ctx context.Context, accountIDs []int64) (map[int64]*service.GPTQuotaDisplaySnapshot, error) {
	out := map[int64]*service.GPTQuotaDisplaySnapshot{}
	if len(accountIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT s.account_id, s.five_hour::text, s.seven_day::text, s.sampled_at, s.last_attempt_at,
       s.last_attempt_status, s.retry_after, s.source, s.updated_at
FROM gpt_quota_display_snapshots s
WHERE s.account_id = ANY($1)`, pq.Array(accountIDs))
	if err != nil {
		return nil, fmt.Errorf("list gpt quota display snapshots: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		snap := &service.GPTQuotaDisplaySnapshot{}
		var fiveHour, sevenDay sql.NullString
		var sampled, attempt, retry sql.NullTime
		if err := rows.Scan(&snap.AccountID, &fiveHour, &sevenDay, &sampled, &attempt, &snap.LastAttemptStatus, &retry, &snap.Source, &snap.UpdatedAt); err != nil {
			return nil, err
		}
		if snap.FiveHour, err = decodeGPTQuotaWindow(fiveHour); err != nil {
			return nil, fmt.Errorf("decode five_hour for account %d: %w", snap.AccountID, err)
		}
		if snap.SevenDay, err = decodeGPTQuotaWindow(sevenDay); err != nil {
			return nil, fmt.Errorf("decode seven_day for account %d: %w", snap.AccountID, err)
		}
		snap.SampledAt = nullTimePtr(sampled)
		snap.LastAttemptAt = nullTimePtr(attempt)
		snap.RetryAfter = nullTimePtr(retry)
		out[snap.AccountID] = snap
	}
	return out, rows.Err()
}

func (r *gptQuotaDisplayRepository) ClaimAttempt(ctx context.Context, accountID int64, attemptAt time.Time, cooldown time.Duration) (bool, error) {
	var claimed int
	err := r.db.QueryRowContext(ctx, `
INSERT INTO gpt_quota_display_snapshots (account_id, last_attempt_at, last_attempt_status, updated_at)
VALUES ($1, $2, 'running', NOW())
ON CONFLICT (account_id) DO UPDATE
SET last_attempt_at = EXCLUDED.last_attempt_at, last_attempt_status = 'running', updated_at = NOW()
WHERE (gpt_quota_display_snapshots.last_attempt_at IS NULL
       OR gpt_quota_display_snapshots.last_attempt_at <= EXCLUDED.last_attempt_at - $3::double precision * INTERVAL '1 second')
  AND (gpt_quota_display_snapshots.retry_after IS NULL
       OR gpt_quota_display_snapshots.retry_after <= EXCLUDED.last_attempt_at)
RETURNING 1`, accountID, attemptAt, cooldown.Seconds()).Scan(&claimed)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claim gpt quota attempt for account %d: %w", accountID, err)
	}
	return true, nil
}

func (r *gptQuotaDisplayRepository) FinishAttempt(ctx context.Context, accountID int64, attemptAt time.Time, status string, retryAfter *time.Time) error {
	_, err := r.db.ExecContext(ctx, `
UPDATE gpt_quota_display_snapshots
SET last_attempt_status = $3, retry_after = $4, updated_at = NOW()
WHERE account_id = $1 AND last_attempt_at = $2`, accountID, attemptAt, status, retryAfter)
	if err != nil {
		return fmt.Errorf("finish gpt quota attempt for account %d: %w", accountID, err)
	}
	return nil
}

func (r *gptQuotaDisplayRepository) PublishSnapshot(ctx context.Context, snap *service.GPTQuotaDisplaySnapshot, attemptAt time.Time) (bool, error) {
	if snap == nil || snap.SampledAt == nil {
		return false, errors.New("publish gpt quota snapshot: sampled_at is required")
	}
	fiveHour, err := encodeGPTQuotaWindow(snap.FiveHour)
	if err != nil {
		return false, err
	}
	sevenDay, err := encodeGPTQuotaWindow(snap.SevenDay)
	if err != nil {
		return false, err
	}
	res, err := r.db.ExecContext(ctx, `
UPDATE gpt_quota_display_snapshots
SET five_hour = $2::jsonb, seven_day = $3::jsonb, sampled_at = $4, source = $5, updated_at = NOW(),
    last_attempt_status = CASE WHEN last_attempt_at = $6 THEN 'ok' ELSE last_attempt_status END,
    retry_after = CASE WHEN last_attempt_at = $6 THEN NULL ELSE retry_after END
WHERE account_id = $1 AND (sampled_at IS NULL OR sampled_at < $4)`,
		snap.AccountID, fiveHour, sevenDay, *snap.SampledAt, snap.Source, attemptAt)
	if err != nil {
		return false, fmt.Errorf("publish gpt quota snapshot for account %d: %w", snap.AccountID, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (r *gptQuotaDisplayRepository) ClaimSlot(ctx context.Context, slot time.Time) (bool, error) {
	var claimed int
	err := r.db.QueryRowContext(ctx, `
UPDATE gpt_quota_display_config SET last_slot_at = $1
WHERE id = TRUE AND (last_slot_at IS NULL OR last_slot_at < $1)
RETURNING 1`, slot).Scan(&claimed)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claim gpt quota slot: %w", err)
	}
	return true, nil
}

// encodeGPTQuotaWindow 缺失窗口写 SQL NULL，而不是 JSON null。
func encodeGPTQuotaWindow(w *service.GPTQuotaWindow) (sql.NullString, error) {
	if w == nil {
		return sql.NullString{}, nil
	}
	raw, err := json.Marshal(w)
	if err != nil {
		return sql.NullString{}, fmt.Errorf("encode gpt quota window: %w", err)
	}
	return sql.NullString{String: string(raw), Valid: true}, nil
}

func decodeGPTQuotaWindow(raw sql.NullString) (*service.GPTQuotaWindow, error) {
	if !raw.Valid || raw.String == "" || raw.String == "null" {
		return nil, nil
	}
	var w service.GPTQuotaWindow
	if err := json.Unmarshal([]byte(raw.String), &w); err != nil {
		return nil, err
	}
	return &w, nil
}

func nullTimePtr(v sql.NullTime) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time
	return &t
}
