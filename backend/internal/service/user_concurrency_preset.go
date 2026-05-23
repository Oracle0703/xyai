package service

import (
	"context"
	"time"
)

const (
	UserConcurrencyPresetTriggerManual    = "manual"
	UserConcurrencyPresetTriggerScheduled = "scheduled"
	UserConcurrencyPresetRunSuccess       = "success"
	UserConcurrencyPresetRunFailed        = "failed"
)

type UserConcurrencyPreset struct {
	ID                   int64      `json:"id"`
	Name                 string     `json:"name"`
	Description          string     `json:"description"`
	TargetConcurrency    int        `json:"target_concurrency"`
	UserIDs              []int64    `json:"user_ids"`
	ScheduleEnabled      bool       `json:"schedule_enabled"`
	ScheduleTime         string     `json:"schedule_time"`
	LastScheduledRunDate *time.Time `json:"last_scheduled_run_date"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

type UserConcurrencyPresetRun struct {
	ID                int64     `json:"id"`
	PresetID          int64     `json:"preset_id"`
	Trigger           string    `json:"trigger"`
	TargetConcurrency int       `json:"target_concurrency"`
	UserIDs           []int64   `json:"user_ids"`
	AffectedCount     int       `json:"affected_count"`
	Status            string    `json:"status"`
	ErrorMessage      string    `json:"error_message"`
	StartedAt         time.Time `json:"started_at"`
	FinishedAt        time.Time `json:"finished_at"`
	CreatedAt         time.Time `json:"created_at"`
}

type UserConcurrencyPresetRepository interface {
	Create(ctx context.Context, preset *UserConcurrencyPreset) (*UserConcurrencyPreset, error)
	GetByID(ctx context.Context, id int64) (*UserConcurrencyPreset, error)
	List(ctx context.Context) ([]*UserConcurrencyPreset, error)
	ListDue(ctx context.Context, scheduleTime string, runDate time.Time) ([]*UserConcurrencyPreset, error)
	Update(ctx context.Context, preset *UserConcurrencyPreset) (*UserConcurrencyPreset, error)
	Delete(ctx context.Context, id int64) error
	MarkScheduledRun(ctx context.Context, id int64, runDate time.Time) error
	CreateRun(ctx context.Context, run *UserConcurrencyPresetRun) (*UserConcurrencyPresetRun, error)
	ListRuns(ctx context.Context, presetID int64, limit int) ([]*UserConcurrencyPresetRun, error)
}
