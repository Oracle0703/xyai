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

func TestTokenAnalysisRepositoryUpsertRequestSummary(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := NewTokenAnalysisRepository(db)
	usageID := int64(11)
	userID := int64(22)
	now := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)

	mock.ExpectExec("INSERT INTO token_analysis_request_summaries").
		WithArgs(
			"arch-1", usageID, int16(3), now,
			userID, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			"gpt-4.1", "/v1/chat/completions", "POST",
			int64(1200), false, "abc",
			2, 10, 20, "hello", 1, 0,
			sqlmock.AnyArg(), 50, sqlmock.AnyArg(), "2026-05-19.jsonl", sqlmock.AnyArg(),
			"e:/code/demo", "demo", "main", "cc_system_prompt",
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = repo.UpsertRequestSummary(context.Background(), &service.TokenAnalysisRequestSummary{
		ArchiveID: "arch-1", UsageLogID: &usageID, MatchConfidence: 3, EventTime: now,
		UserID: &userID, Model: "gpt-4.1", Endpoint: "/v1/chat/completions", Method: "POST",
		RequestBodySize: 1200, BodySHA256: "abc", MessageCount: 2, SystemChars: 10,
		UserChars: 20, LastUserPreview: "hello", ToolsCount: 1, RiskScore: 50,
		RiskReasons: []service.TokenAnalysisRiskReason{{Code: "x", Message: "y", Score: 50}},
		SummaryJSON: map[string]any{"shape": "chat"}, SourceFile: "2026-05-19.jsonl",
		ClientWorkdir: "e:/code/demo", ClientProject: "demo", ClientBranch: "main", AttributionSource: "cc_system_prompt",
	})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTokenAnalysisRepositoryFindNearestUsageLog(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := NewTokenAnalysisRepository(db)
	userID := int64(22)
	apiKeyID := int64(33)
	now := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)

	mock.ExpectQuery("FROM usage_logs").
		WithArgs(now.Add(-10*time.Second), now.Add(10*time.Second), now, userID, apiKeyID, "gpt-4.1").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "input_tokens", "output_tokens", "cache_read_tokens", "cache_creation_tokens", "total_cost", "actual_cost", "confidence",
		}).AddRow(int64(99), 1000, 20, 500, 100, 0.5, 0.6, int16(3)))

	got, err := repo.FindNearestUsageLog(context.Background(), now, &userID, &apiKeyID, "gpt-4.1", 10*time.Second)

	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, int64(99), got.UsageLogID)
	require.Equal(t, int16(3), got.MatchConfidence)
	require.Equal(t, 500, got.CacheReadTokens)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTokenAnalysisRepositoryFindNearestUsageLogNoRows(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := NewTokenAnalysisRepository(db)
	now := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)

	mock.ExpectQuery("FROM usage_logs").
		WithArgs(now.Add(-10*time.Second), now.Add(10*time.Second), now).
		WillReturnError(sql.ErrNoRows)

	got, err := repo.FindNearestUsageLog(context.Background(), now, nil, nil, "", 10*time.Second)

	require.NoError(t, err)
	require.Nil(t, got)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTokenAnalysisRepositoryGetIndexState(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := NewTokenAnalysisRepository(db)
	now := time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)

	mock.ExpectQuery("FROM token_analysis_index_state").
		WithArgs("data/request-archive/2026-05-21.jsonl").
		WillReturnRows(sqlmock.NewRows([]string{
			"source_file", "last_offset", "last_archive_id", "processed_rows", "failed_rows", "last_error", "started_at", "finished_at", "updated_at",
		}).AddRow("data/request-archive/2026-05-21.jsonl", int64(1234), "arch-1", int64(2), int64(1), "bad line", now, now, now))

	state, err := repo.GetIndexState(context.Background(), "data/request-archive/2026-05-21.jsonl")

	require.NoError(t, err)
	require.NotNil(t, state)
	require.Equal(t, int64(1234), state.LastOffset)
	require.Equal(t, "arch-1", state.LastArchiveID)
	require.Equal(t, int64(2), state.ProcessedRows)
	require.Equal(t, int64(1), state.FailedRows)
	require.Equal(t, "bad line", state.LastError)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTokenAnalysisRepositoryGetSummaryIncludesUnmatchedAndRiskReasons(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := NewTokenAnalysisRepository(db)
	start := time.Date(2026, 5, 21, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)

	mock.ExpectQuery("COUNT\\(\\*\\) AS total_requests").
		WithArgs(start, end).
		WillReturnRows(sqlmock.NewRows([]string{
			"total_requests", "matched_requests", "total_input_tokens", "total_output_tokens", "cache_read_tokens", "cache_creation_tokens",
			"total_tokens", "total_cost", "total_actual_cost", "risky_requests", "risky_cost", "risk_reasons",
		}).AddRow(int64(10), int64(7), int64(1000), int64(100), int64(300), int64(200), int64(1600), 1.2, 1.1, int64(4), 0.6, []byte(`[{"code":"huge_input_tiny_output","count":3},{"code":"low_cache_hit_large_input","count":1}]`)))

	got, err := repo.GetSummary(context.Background(), service.TokenAnalysisFilters{StartTime: &start, EndTime: &end})

	require.NoError(t, err)
	require.Equal(t, int64(10), got.TotalRequests)
	require.Equal(t, int64(7), got.MatchedRequests)
	require.Equal(t, int64(3), got.UnmatchedRequests)
	require.Equal(t, 0.3, got.UnmatchedRate)
	require.Equal(t, 0.4, got.RiskRequestRate)
	require.Len(t, got.RiskReasons, 2)
	require.Equal(t, "huge_input_tiny_output", got.RiskReasons[0].Code)
	require.Equal(t, int64(3), got.RiskReasons[0].Count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTokenAnalysisRepositoryUpsertUserInputPreservesQuality(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := NewTokenAnalysisRepository(db)
	userID := int64(7)
	now := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)

	// 冲突更新只触内容字段, 不应出现 quality_ 列。
	mock.ExpectExec(`INSERT INTO token_analysis_user_inputs[\s\S]*ON CONFLICT \(archive_id\) DO UPDATE SET[\s\S]*truncated = EXCLUDED\.truncated`).
		WithArgs("arch-1", now, userID, "line one\nline two", "deadbeef", 17, false).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = repo.UpsertUserInput(context.Background(), &service.TokenAnalysisUserInput{
		ArchiveID: "arch-1", EventTime: now, UserID: &userID,
		Content: "line one\nline two", ContentSHA256: "deadbeef", Chars: 17,
	})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTokenAnalysisRepositoryGetUserInput(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := NewTokenAnalysisRepository(db)
	now := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)
	score := int16(85)

	mock.ExpectQuery("FROM token_analysis_user_inputs").
		WithArgs("arch-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "archive_id", "event_time", "user_id", "content", "content_sha256", "chars", "truncated",
			"quality_score", "quality_findings", "quality_version", "evaluated_at",
		}).AddRow(int64(1), "arch-1", now, int64(7), "hello", "deadbeef", 5, false, score, []byte(`{"clarity":"ok"}`), "v1", now))

	got, err := repo.GetUserInput(context.Background(), "arch-1")

	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "hello", got.Content)
	require.Equal(t, int16(85), *got.QualityScore)
	require.Equal(t, "v1", got.QualityVersion)
	require.Equal(t, "ok", got.QualityFindings["clarity"])

	// 无记录返回 nil, nil。
	mock.ExpectQuery("FROM token_analysis_user_inputs").
		WithArgs("missing").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "archive_id", "event_time", "user_id", "content", "content_sha256", "chars", "truncated",
			"quality_score", "quality_findings", "quality_version", "evaluated_at",
		}))

	missing, err := repo.GetUserInput(context.Background(), "missing")
	require.NoError(t, err)
	require.Nil(t, missing)
	require.NoError(t, mock.ExpectationsWereMet())
}
