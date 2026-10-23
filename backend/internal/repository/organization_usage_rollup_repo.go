package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type organizationUsageRollupRepository struct {
	db *sql.DB
}

func NewOrganizationUsageRollupRepository(db *sql.DB) service.OrganizationUsageRollupRepository {
	return &organizationUsageRollupRepository{db: db}
}

// A day closes only 10 minutes after it ends; the insert trigger marks same-day rows written
// in the last 10 minutes of the day, so a write transaction must stay open for over 20 minutes
// across midnight before a row can be missed, and hourly reconciliation catches even that.
const organizationUsageRollupClosableDateSQL = `((clock_timestamp() - INTERVAL '10 minutes') AT TIME ZONE 'Asia/Shanghai')::date`

// Every step runs in its own short READ COMMITTED transaction: the dirty mark is claimed
// before the day is re-aggregated, so a late row committed after the aggregation statement
// keeps its own mark and is rebuilt by the next run.
func (r *organizationUsageRollupRepository) inTx(ctx context.Context, fn func(*sql.Tx) (bool, error)) (bool, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return false, err
	}
	done, err := fn(tx)
	if err != nil {
		_ = tx.Rollback()
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return done, nil
}

func lockOrganizationUsageRollupState(ctx context.Context, tx *sql.Tx) (sql.NullString, error) {
	if _, err := tx.ExecContext(ctx, `INSERT INTO usage_user_rollup_state (id) VALUES (1) ON CONFLICT (id) DO NOTHING`); err != nil {
		return sql.NullString{}, fmt.Errorf("ensure organization usage rollup state: %w", err)
	}
	var closedBefore sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT closed_before::text FROM usage_user_rollup_state WHERE id = 1 FOR UPDATE`).Scan(&closedBefore); err != nil {
		return sql.NullString{}, fmt.Errorf("lock organization usage rollup state: %w", err)
	}
	return closedBefore, nil
}

// InitializeOrganizationUsageRollups starts publishing from the earliest retained usage day.
func (r *organizationUsageRollupRepository) InitializeOrganizationUsageRollups(ctx context.Context) (bool, error) {
	return r.inTx(ctx, func(tx *sql.Tx) (bool, error) {
		closedBefore, err := lockOrganizationUsageRollupState(ctx, tx)
		if err != nil || closedBefore.Valid {
			return false, err
		}
		var start string
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(
    (MIN(created_at) AT TIME ZONE 'Asia/Shanghai')::date,
    (clock_timestamp() AT TIME ZONE 'Asia/Shanghai')::date
)::text FROM usage_logs`).Scan(&start); err != nil {
			return false, fmt.Errorf("read earliest usage day: %w", err)
		}
		// Nothing is published yet, so every pending mark is covered by closing its day.
		if _, err := tx.ExecContext(ctx, `DELETE FROM usage_user_daily_rollups`); err != nil {
			return false, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM usage_user_rollup_dirty_days`); err != nil {
			return false, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE usage_user_rollup_state SET closed_before = $1::date, updated_at = NOW() WHERE id = 1`, start); err != nil {
			return false, err
		}
		return true, nil
	})
}

// CloseNextOrganizationUsageRollupDay publishes the oldest unpublished day once it is closable.
func (r *organizationUsageRollupRepository) CloseNextOrganizationUsageRollupDay(ctx context.Context) (bool, error) {
	return r.inTx(ctx, func(tx *sql.Tx) (bool, error) {
		closedBefore, err := lockOrganizationUsageRollupState(ctx, tx)
		if err != nil || !closedBefore.Valid {
			return false, err
		}
		var closable bool
		if err := tx.QueryRowContext(ctx, `SELECT $1::date < `+organizationUsageRollupClosableDateSQL, closedBefore.String).Scan(&closable); err != nil {
			return false, err
		}
		if !closable {
			return false, nil
		}
		if err := rebuildOrganizationUsageRollupDay(ctx, tx, closedBefore.String); err != nil {
			return false, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE usage_user_rollup_state SET closed_before = closed_before + 1, updated_at = NOW() WHERE id = 1`); err != nil {
			return false, err
		}
		return true, nil
	})
}

// RebuildNextDirtyOrganizationUsageRollupDay re-aggregates the oldest published dirty day.
// Marks on unpublished days stay until their day is closed.
func (r *organizationUsageRollupRepository) RebuildNextDirtyOrganizationUsageRollupDay(ctx context.Context) (bool, error) {
	return r.inTx(ctx, func(tx *sql.Tx) (bool, error) {
		closedBefore, err := lockOrganizationUsageRollupState(ctx, tx)
		if err != nil || !closedBefore.Valid {
			return false, err
		}
		var day string
		err = tx.QueryRowContext(ctx, `SELECT bucket_date::text FROM usage_user_rollup_dirty_days
WHERE bucket_date < $1::date ORDER BY bucket_date LIMIT 1`, closedBefore.String).Scan(&day)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return true, rebuildOrganizationUsageRollupDay(ctx, tx, day)
	})
}

func rebuildOrganizationUsageRollupDay(ctx context.Context, tx *sql.Tx, day string) error {
	start, err := time.ParseInLocation(organizationUsageDateLayout, day, organizationUsageRollupLocation)
	if err != nil {
		return fmt.Errorf("parse organization usage rollup day %q: %w", day, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM usage_user_rollup_dirty_days WHERE bucket_date = $1::date`, day); err != nil {
		return fmt.Errorf("claim organization usage dirty day %s: %w", day, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM usage_user_daily_rollups WHERE bucket_date = $1::date`, day); err != nil {
		return fmt.Errorf("clear organization usage rollup day %s: %w", day, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO usage_user_daily_rollups (
    bucket_date, user_id, group_id, account_id, requests,
    input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens, actual_cost, computed_at
)
SELECT $1::date, ul.user_id, COALESCE(ul.group_id, 0), ul.account_id, COUNT(*),
    COALESCE(SUM(ul.input_tokens), 0), COALESCE(SUM(ul.output_tokens), 0),
    COALESCE(SUM(ul.cache_creation_tokens), 0), COALESCE(SUM(ul.cache_read_tokens), 0),
    COALESCE(SUM(ul.actual_cost), 0), NOW()
FROM usage_logs ul
WHERE ul.created_at >= $2 AND ul.created_at < $3
GROUP BY ul.user_id, COALESCE(ul.group_id, 0), ul.account_id`, day, start.UTC(), start.AddDate(0, 0, 1).UTC()); err != nil {
		return fmt.Errorf("rebuild organization usage rollup day %s: %w", day, err)
	}
	return nil
}

// markOrganizationUsageRollupRangeDirty marks every Beijing day touching [start, end). Dropping a
// usage_logs partition fires no row trigger, so the retention path marks the range itself.
func markOrganizationUsageRollupRangeDirty(ctx context.Context, tx *sql.Tx, start, end time.Time) error {
	_, err := tx.ExecContext(ctx, `SELECT mark_user_usage_rollup_dirty_day(day::date)
FROM generate_series(
    ($1::timestamptz AT TIME ZONE 'Asia/Shanghai')::date,
    (($2::timestamptz - INTERVAL '1 microsecond') AT TIME ZONE 'Asia/Shanghai')::date,
    INTERVAL '1 day'
) AS day`, start.UTC(), end.UTC())
	if err != nil {
		return fmt.Errorf("mark organization usage rollup range dirty: %w", err)
	}
	return nil
}

// AnalyzeOrganizationUsageRollups refreshes planner statistics after a large backfill.
func (r *organizationUsageRollupRepository) AnalyzeOrganizationUsageRollups(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `ANALYZE usage_user_daily_rollups`)
	return err
}

// ReconcileOrganizationUsageRollups compares the last published days against usage_logs in a
// single statement snapshot and marks every mismatching day dirty. A late row is always
// visible together with its own mark, so marked days are skipped rather than reported.
// The cadence is claimed on the shared state row, so it holds across instances.
func (r *organizationUsageRollupRepository) ReconcileOrganizationUsageRollups(ctx context.Context, days int, every time.Duration) ([]string, error) {
	if days <= 0 {
		return nil, nil
	}
	var closedBefore sql.NullString
	if err := r.db.QueryRowContext(ctx, `UPDATE usage_user_rollup_state SET reconciled_at = NOW()
WHERE id = 1 AND closed_before IS NOT NULL
  AND (reconciled_at IS NULL OR reconciled_at <= NOW() - $1 * INTERVAL '1 second')
RETURNING closed_before::text`, int64(every/time.Second)).Scan(&closedBefore); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	upper, err := time.ParseInLocation(organizationUsageDateLayout, closedBefore.String, organizationUsageRollupLocation)
	if err != nil {
		return nil, err
	}
	lower := upper.AddDate(0, 0, -days)
	rows, err := r.db.QueryContext(ctx, `WITH rollup AS (
    SELECT bucket_date, SUM(requests) AS requests, SUM(input_tokens) AS input_tokens,
        SUM(output_tokens) AS output_tokens, SUM(cache_creation_tokens) AS cache_creation_tokens,
        SUM(cache_read_tokens) AS cache_read_tokens, SUM(actual_cost) AS actual_cost
    FROM usage_user_daily_rollups
    WHERE bucket_date >= $1::date AND bucket_date < $2::date
    GROUP BY bucket_date
),
raw AS (
    SELECT (created_at AT TIME ZONE 'Asia/Shanghai')::date AS bucket_date, COUNT(*)::numeric AS requests,
        SUM(input_tokens)::numeric AS input_tokens, SUM(output_tokens)::numeric AS output_tokens,
        SUM(cache_creation_tokens)::numeric AS cache_creation_tokens, SUM(cache_read_tokens)::numeric AS cache_read_tokens,
        SUM(actual_cost) AS actual_cost
    FROM usage_logs
    WHERE created_at >= $3 AND created_at < $4
    GROUP BY 1
),
mismatched AS (
    SELECT COALESCE(rollup.bucket_date, raw.bucket_date) AS bucket_date
    FROM rollup FULL JOIN raw ON raw.bucket_date = rollup.bucket_date
    WHERE (rollup.requests, rollup.input_tokens, rollup.output_tokens, rollup.cache_creation_tokens, rollup.cache_read_tokens, rollup.actual_cost)
        IS DISTINCT FROM (raw.requests, raw.input_tokens, raw.output_tokens, raw.cache_creation_tokens, raw.cache_read_tokens, raw.actual_cost)
)
INSERT INTO usage_user_rollup_dirty_days (bucket_date)
SELECT m.bucket_date FROM mismatched m
WHERE NOT EXISTS (SELECT 1 FROM usage_user_rollup_dirty_days d WHERE d.bucket_date = m.bucket_date)
ON CONFLICT (bucket_date) DO NOTHING
RETURNING bucket_date::text`, lower.Format(organizationUsageDateLayout), upper.Format(organizationUsageDateLayout), lower.UTC(), upper.UTC())
	if err != nil {
		return nil, fmt.Errorf("reconcile organization usage rollups: %w", err)
	}
	defer func() { _ = rows.Close() }()
	marked := []string{}
	for rows.Next() {
		var day string
		if err := rows.Scan(&day); err != nil {
			return nil, err
		}
		marked = append(marked, day)
	}
	return marked, rows.Err()
}
