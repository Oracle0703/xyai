package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type userConcurrencyPresetRepository struct {
	db *sql.DB
}

func NewUserConcurrencyPresetRepository(db *sql.DB) service.UserConcurrencyPresetRepository {
	return &userConcurrencyPresetRepository{db: db}
}

func (r *userConcurrencyPresetRepository) Create(ctx context.Context, preset *service.UserConcurrencyPreset) (*service.UserConcurrencyPreset, error) {
	userIDs, err := marshalUserIDs(preset.UserIDs)
	if err != nil {
		return nil, err
	}
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO user_concurrency_presets (name, description, target_concurrency, user_ids, schedule_enabled, schedule_time, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
		RETURNING id, name, description, target_concurrency, user_ids, schedule_enabled, schedule_time, last_scheduled_run_date, created_at, updated_at
	`, preset.Name, preset.Description, preset.TargetConcurrency, userIDs, preset.ScheduleEnabled, preset.ScheduleTime)
	return scanUserConcurrencyPreset(row)
}

func (r *userConcurrencyPresetRepository) GetByID(ctx context.Context, id int64) (*service.UserConcurrencyPreset, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, name, description, target_concurrency, user_ids, schedule_enabled, schedule_time, last_scheduled_run_date, created_at, updated_at
		FROM user_concurrency_presets WHERE id = $1
	`, id)
	return scanUserConcurrencyPreset(row)
}

func (r *userConcurrencyPresetRepository) List(ctx context.Context) ([]*service.UserConcurrencyPreset, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, description, target_concurrency, user_ids, schedule_enabled, schedule_time, last_scheduled_run_date, created_at, updated_at
		FROM user_concurrency_presets
		ORDER BY created_at DESC, id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanUserConcurrencyPresets(rows)
}

func (r *userConcurrencyPresetRepository) ListDue(ctx context.Context, scheduleTime string, runDate time.Time) ([]*service.UserConcurrencyPreset, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, description, target_concurrency, user_ids, schedule_enabled, schedule_time, last_scheduled_run_date, created_at, updated_at
		FROM user_concurrency_presets
		WHERE schedule_enabled = true
		  AND schedule_time = $1
		  AND (last_scheduled_run_date IS NULL OR last_scheduled_run_date < $2::date)
		ORDER BY id ASC
	`, scheduleTime, runDate)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanUserConcurrencyPresets(rows)
}

func (r *userConcurrencyPresetRepository) Update(ctx context.Context, preset *service.UserConcurrencyPreset) (*service.UserConcurrencyPreset, error) {
	userIDs, err := marshalUserIDs(preset.UserIDs)
	if err != nil {
		return nil, err
	}
	row := r.db.QueryRowContext(ctx, `
		UPDATE user_concurrency_presets
		SET name = $2, description = $3, target_concurrency = $4, user_ids = $5,
		    schedule_enabled = $6, schedule_time = $7, updated_at = NOW()
		WHERE id = $1
		RETURNING id, name, description, target_concurrency, user_ids, schedule_enabled, schedule_time, last_scheduled_run_date, created_at, updated_at
	`, preset.ID, preset.Name, preset.Description, preset.TargetConcurrency, userIDs, preset.ScheduleEnabled, preset.ScheduleTime)
	return scanUserConcurrencyPreset(row)
}

func (r *userConcurrencyPresetRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM user_concurrency_presets WHERE id = $1`, id)
	return err
}

func (r *userConcurrencyPresetRepository) MarkScheduledRun(ctx context.Context, id int64, runDate time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE user_concurrency_presets
		SET last_scheduled_run_date = $2::date, updated_at = NOW()
		WHERE id = $1
	`, id, runDate)
	return err
}

func (r *userConcurrencyPresetRepository) CreateRun(ctx context.Context, run *service.UserConcurrencyPresetRun) (*service.UserConcurrencyPresetRun, error) {
	userIDs, err := marshalUserIDs(run.UserIDs)
	if err != nil {
		return nil, err
	}
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO user_concurrency_preset_runs (preset_id, trigger, target_concurrency, user_ids, affected_count, status, error_message, started_at, finished_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
		RETURNING id, preset_id, trigger, target_concurrency, user_ids, affected_count, status, error_message, started_at, finished_at, created_at
	`, run.PresetID, run.Trigger, run.TargetConcurrency, userIDs, run.AffectedCount, run.Status, run.ErrorMessage, run.StartedAt, run.FinishedAt)
	return scanUserConcurrencyPresetRun(row)
}

func (r *userConcurrencyPresetRepository) ListRuns(ctx context.Context, presetID int64, limit int) ([]*service.UserConcurrencyPresetRun, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, preset_id, trigger, target_concurrency, user_ids, affected_count, status, error_message, started_at, finished_at, created_at
		FROM user_concurrency_preset_runs
		WHERE preset_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2
	`, presetID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	runs := make([]*service.UserConcurrencyPresetRun, 0)
	for rows.Next() {
		run, err := scanUserConcurrencyPresetRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func marshalUserIDs(userIDs []int64) ([]byte, error) {
	if userIDs == nil {
		userIDs = []int64{}
	}
	out, err := json.Marshal(userIDs)
	if err != nil {
		return nil, fmt.Errorf("marshal user ids: %w", err)
	}
	return out, nil
}

func scanUserConcurrencyPreset(row scannable) (*service.UserConcurrencyPreset, error) {
	var userIDsRaw []byte
	var lastScheduledRunDate sql.NullTime
	preset := &service.UserConcurrencyPreset{}
	if err := row.Scan(
		&preset.ID,
		&preset.Name,
		&preset.Description,
		&preset.TargetConcurrency,
		&userIDsRaw,
		&preset.ScheduleEnabled,
		&preset.ScheduleTime,
		&lastScheduledRunDate,
		&preset.CreatedAt,
		&preset.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(userIDsRaw, &preset.UserIDs); err != nil {
		return nil, fmt.Errorf("unmarshal preset user ids: %w", err)
	}
	if lastScheduledRunDate.Valid {
		preset.LastScheduledRunDate = &lastScheduledRunDate.Time
	}
	return preset, nil
}

func scanUserConcurrencyPresets(rows *sql.Rows) ([]*service.UserConcurrencyPreset, error) {
	presets := make([]*service.UserConcurrencyPreset, 0)
	for rows.Next() {
		preset, err := scanUserConcurrencyPreset(rows)
		if err != nil {
			return nil, err
		}
		presets = append(presets, preset)
	}
	return presets, rows.Err()
}

func scanUserConcurrencyPresetRun(row scannable) (*service.UserConcurrencyPresetRun, error) {
	var userIDsRaw []byte
	run := &service.UserConcurrencyPresetRun{}
	if err := row.Scan(
		&run.ID,
		&run.PresetID,
		&run.Trigger,
		&run.TargetConcurrency,
		&userIDsRaw,
		&run.AffectedCount,
		&run.Status,
		&run.ErrorMessage,
		&run.StartedAt,
		&run.FinishedAt,
		&run.CreatedAt,
	); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(userIDsRaw, &run.UserIDs); err != nil {
		return nil, fmt.Errorf("unmarshal run user ids: %w", err)
	}
	return run, nil
}
