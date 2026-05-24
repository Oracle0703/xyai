package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUserConcurrencyPresetRepositoryCreateAndGet(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := NewUserConcurrencyPresetRepository(db)
	now := time.Date(2026, 5, 23, 9, 0, 0, 0, time.UTC)

	mock.ExpectQuery("INSERT INTO user_concurrency_presets").
		WithArgs("daytime", "work hours", 12, []byte(`[11,22]`), true, "09:00").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "description", "target_concurrency", "user_ids",
			"schedule_enabled", "schedule_time", "last_scheduled_run_date", "created_at", "updated_at",
		}).AddRow(int64(1), "daytime", "work hours", 12, []byte(`[11,22]`), true, "09:00", nil, now, now))

	created, err := repo.Create(context.Background(), &service.UserConcurrencyPreset{
		Name:              "daytime",
		Description:       "work hours",
		TargetConcurrency: 12,
		UserIDs:           []int64{11, 22},
		ScheduleEnabled:   true,
		ScheduleTime:      "09:00",
	})

	require.NoError(t, err)
	require.Equal(t, int64(1), created.ID)
	require.Equal(t, []int64{11, 22}, created.UserIDs)
	require.True(t, created.ScheduleEnabled)
	require.Nil(t, created.LastScheduledRunDate)

	mock.ExpectQuery("FROM user_concurrency_presets WHERE id = \\$1").
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "description", "target_concurrency", "user_ids",
			"schedule_enabled", "schedule_time", "last_scheduled_run_date", "created_at", "updated_at",
		}).AddRow(int64(1), "daytime", "work hours", 12, []byte(`[11,22]`), true, "09:00", nil, now, now))

	got, err := repo.GetByID(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, "daytime", got.Name)
	require.Equal(t, []int64{11, 22}, got.UserIDs)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUserConcurrencyPresetRepositoryListDueUsesLocalDate(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := NewUserConcurrencyPresetRepository(db)
	runDate := time.Date(2026, 5, 23, 9, 0, 0, 0, time.UTC)

	mock.ExpectQuery("WHERE schedule_enabled = true").
		WithArgs("09:00", runDate).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "description", "target_concurrency", "user_ids",
			"schedule_enabled", "schedule_time", "last_scheduled_run_date", "created_at", "updated_at",
		}).AddRow(int64(2), "daytime", "", 12, []byte(`[33]`), true, "09:00", nil, runDate, runDate))

	due, err := repo.ListDue(context.Background(), "09:00", runDate)
	require.NoError(t, err)
	require.Len(t, due, 1)
	require.Equal(t, int64(2), due[0].ID)
	require.Equal(t, []int64{33}, due[0].UserIDs)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUserConcurrencyPresetRepositoryListEmptyReturnsNonNilSlice(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := NewUserConcurrencyPresetRepository(db)
	mock.ExpectQuery("FROM user_concurrency_presets").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "description", "target_concurrency", "user_ids",
			"schedule_enabled", "schedule_time", "last_scheduled_run_date", "created_at", "updated_at",
		}))

	presets, err := repo.List(context.Background())
	require.NoError(t, err)
	require.NotNil(t, presets)
	require.Empty(t, presets)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUserConcurrencyPresetRepositoryRunLifecycle(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := NewUserConcurrencyPresetRepository(db)
	started := time.Date(2026, 5, 23, 9, 0, 0, 0, time.UTC)
	finished := started.Add(100 * time.Millisecond)

	mock.ExpectQuery("INSERT INTO user_concurrency_preset_runs").
		WithArgs(int64(2), "manual", 12, []byte(`[11,22]`), 2, "success", "", started, finished).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "preset_id", "trigger", "target_concurrency", "user_ids", "affected_count",
			"status", "error_message", "started_at", "finished_at", "created_at",
		}).AddRow(int64(9), int64(2), "manual", 12, []byte(`[11,22]`), 2, "success", "", started, finished, finished))

	run, err := repo.CreateRun(context.Background(), &service.UserConcurrencyPresetRun{
		PresetID:          2,
		Trigger:           "manual",
		TargetConcurrency: 12,
		UserIDs:           []int64{11, 22},
		AffectedCount:     2,
		Status:            "success",
		StartedAt:         started,
		FinishedAt:        finished,
	})
	require.NoError(t, err)
	require.Equal(t, int64(9), run.ID)
	require.Equal(t, []int64{11, 22}, run.UserIDs)

	mock.ExpectExec("UPDATE user_concurrency_presets").
		WithArgs(int64(2), started).
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, repo.MarkScheduledRun(context.Background(), 2, started))

	mock.ExpectQuery("FROM user_concurrency_preset_runs").
		WithArgs(int64(2), 20).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "preset_id", "trigger", "target_concurrency", "user_ids", "affected_count",
			"status", "error_message", "started_at", "finished_at", "created_at",
		}).AddRow(int64(9), int64(2), "manual", 12, []byte(`[11,22]`), 2, "success", "", started, finished, finished))

	runs, err := repo.ListRuns(context.Background(), 2, 20)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, int64(9), runs[0].ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUserConcurrencyPresetRepositoryGetByIDNotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := NewUserConcurrencyPresetRepository(db)
	mock.ExpectQuery("FROM user_concurrency_presets WHERE id = \\$1").
		WithArgs(int64(404)).
		WillReturnError(sql.ErrNoRows)

	got, err := repo.GetByID(context.Background(), 404)
	require.Error(t, err)
	require.Nil(t, got)
	require.NoError(t, mock.ExpectationsWereMet())
}
