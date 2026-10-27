package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
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

	// 新行在事务内写入: 先锁定读取旧指纹(不存在), 再写入。重新索引请求行即重置
	// 关联方式, 由扫描后的关联轮次重新关联。不可比较的指纹不需要计算候选。
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT fp_version, prefix_end_hash, prefix_tail1_hash, items_end_hash, cand_input_version, cand_checked_version\s+FROM token_analysis_request_summaries WHERE archive_id = \$1 FOR UPDATE`).
		WithArgs("arch-1").WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(`INSERT INTO token_analysis_request_summaries[\s\S]*usage_match_method = '',\s+client_request_id = EXCLUDED.client_request_id`).
		WithArgs(
			"arch-1", usageID, int16(3), now,
			userID, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			"gpt-4.1", "/v1/chat/completions", "POST",
			int64(1200), false, "abc",
			2, 10, 20, "hello", 1, 0,
			sqlmock.AnyArg(), 50, sqlmock.AnyArg(), "2026-05-19.jsonl", sqlmock.AnyArg(),
			"e:/code/demo", "demo", "main", "cc_system_prompt",
			"cid-1", "Codex Desktop/0.160.0", "codex", int32(7),
			nil, nil,
			int32(0), "unsupported:body_truncated", nil, nil, nil, nil, 0, false,
			int64(0), int64(0), "unsupported:body_truncated", false,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectTokenAnalysisClassificationDirty(mock, `{"arch-1"}`)
	mock.ExpectCommit()

	upserted, err := repo.UpsertRequestSummary(context.Background(), &service.TokenAnalysisRequestSummary{
		ArchiveID: "arch-1", UsageLogID: &usageID, MatchConfidence: 3, EventTime: now,
		UserID: &userID, Model: "gpt-4.1", Endpoint: "/v1/chat/completions", Method: "POST",
		RequestBodySize: 1200, BodySHA256: "abc", MessageCount: 2, SystemChars: 10,
		UserChars: 20, LastUserPreview: "hello", ToolsCount: 1, RiskScore: 50,
		RiskReasons: []service.TokenAnalysisRiskReason{{Code: "x", Message: "y", Score: 50}},
		SummaryJSON: map[string]any{"shape": "chat"}, SourceFile: "2026-05-19.jsonl",
		ClientWorkdir: "e:/code/demo", ClientProject: "demo", ClientBranch: "main", AttributionSource: "cc_system_prompt",
		ClientRequestID: "cid-1", UserAgent: "Codex Desktop/0.160.0", ClientFamily: "codex", ClientFamilyVersion: 7,
		Fingerprint: service.TokenAnalysisFingerprint{State: "unsupported:body_truncated"},
	})

	require.NoError(t, err)
	require.False(t, upserted.CandidatesDue)
	require.NoError(t, mock.ExpectationsWereMet())
}

// 来源筛选: human/ai 精确匹配 kind; unknown 同时包含未写入来源的旧行, 与页面 "—" 一致。
func TestBuildTokenAnalysisWhereOriginFilter(t *testing.T) {
	where, args := buildTokenAnalysisWhere(service.TokenAnalysisFilters{Origin: "human", IncludeUnmatched: true}, "s")
	require.Contains(t, where, "s.summary_json->'request_origin'->>'kind' = $1")
	require.Equal(t, []any{"human"}, args)

	where, args = buildTokenAnalysisWhere(service.TokenAnalysisFilters{Origin: "unknown", IncludeUnmatched: true}, "s")
	require.Contains(t, where, "COALESCE(s.summary_json->'request_origin'->>'kind', '') NOT IN ('human', 'ai')")
	require.Empty(t, args)

	where, _ = buildTokenAnalysisWhere(service.TokenAnalysisFilters{IncludeUnmatched: true}, "s")
	require.Equal(t, []string{"1=1"}, where)
}

var tokenAnalysisUsageMatchRow = []string{"id", "input_tokens", "output_tokens", "cache_read_tokens", "cache_creation_tokens", "total_cost", "actual_cost"}

// 精确关联键: client:<id> 与上游 response.id; 只被精确关联占用的账单才挡住认领。
func TestTokenAnalysisRepositoryFindUsageLinkByRequestID(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	repo := NewTokenAnalysisRepository(db)
	apiKeyID := int64(33)

	mock.ExpectQuery(`WHERE ul.api_key_id = \$1 AND ul.request_id IN \(\$3, \$4\)[\s\S]*o.archive_id <> \$2 AND o.usage_match_method = 'request_id'`).
		WithArgs(apiKeyID, "arch-1", "client:cid-1", "resp_1").
		WillReturnRows(sqlmock.NewRows(tokenAnalysisUsageMatchRow).AddRow(int64(99), 294, 547, 156288, 0, 0.18, 0.186578))

	got, err := repo.FindUsageLink(context.Background(), service.TokenAnalysisUsageLinkQuery{
		ArchiveID: "arch-1", ClientRequestID: "cid-1", ResponseID: "resp_1", APIKeyID: &apiKeyID, ExactOnly: true,
	})

	require.NoError(t, err)
	require.Equal(t, &service.TokenAnalysisUsageMatch{
		UsageLogID: 99, MatchConfidence: 3, Method: service.TokenAnalysisUsageMatchRequestID,
		InputTokens: 294, OutputTokens: 547, CacheReadTokens: 156288, TotalCost: 0.18, ActualCost: 0.186578,
	}, got)
	require.NoError(t, mock.ExpectationsWereMet())
}

// 只接受精确证据时, 精确未命中不再做时序兜底。
func TestTokenAnalysisRepositoryFindUsageLinkExactOnlyMiss(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	repo := NewTokenAnalysisRepository(db)
	apiKeyID := int64(33)

	mock.ExpectQuery(`ul.request_id IN \(\$3\)`).WithArgs(apiKeyID, "arch-1", "client:cid-1").WillReturnError(sql.ErrNoRows)

	got, err := repo.FindUsageLink(context.Background(), service.TokenAnalysisUsageLinkQuery{
		ArchiveID: "arch-1", ClientRequestID: "cid-1", APIKeyID: &apiKeyID, ExactOnly: true,
	})

	require.NoError(t, err)
	require.Nil(t, got)
	require.NoError(t, mock.ExpectationsWereMet())
}

// 有关联键且已完成: 精确未命中后按完成时间兜底, 只看上游原生 ID 账单, 排除附加计费、
// 他行占用、他行精确账单, 并要求没有其他未确认请求能解释同一账单。
func TestTokenAnalysisRepositoryFindUsageLinkTimingWithCompletion(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	repo := NewTokenAnalysisRepository(db)
	userID, apiKeyID, output := int64(22), int64(33), int64(547)
	start := time.Date(2026, 10, 24, 6, 11, 12, 654557512, time.UTC)
	done := start.Add(18547 * time.Millisecond)

	mock.ExpectQuery(`ul.request_id IN \(\$3\)`).WithArgs(apiKeyID, "arch-1", "client:cid-1").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`ul.created_at BETWEEN \$4 AND \$5\s+AND COALESCE\(ul.duration_ms, 0\) <= \$6[\s\S]*`+
		`strpos\(ul.request_id, ':'\) = 0\s+AND ul.request_id NOT LIKE ALL[\s\S]*`+
		`o.usage_log_id = ul.id AND o.archive_id <> \$10[\s\S]*o.client_request_id = substr\(ul.request_id, 8\)[\s\S]*`+
		`c.usage_log_id IS NULL OR c.usage_match_method IN \('', 'released'\)[\s\S]*`+
		`ORDER BY CASE WHEN ul.output_tokens = \$11 THEN 0 ELSE 1 END, ABS`).
		WithArgs(start.Add(-500*time.Millisecond), start, done, done.Add(-2*time.Second), done.Add(10*time.Second), int64(18547+500),
			userID, apiKeyID, "gpt-6-astra", "arch-1", output).
		WillReturnRows(sqlmock.NewRows(tokenAnalysisUsageMatchRow).AddRow(int64(7), 294, 547, 156288, 0, 0.18, 0.186578))

	got, err := repo.FindUsageLink(context.Background(), service.TokenAnalysisUsageLinkQuery{
		ArchiveID: "arch-1", ClientRequestID: "cid-1", StartedAt: start, CompletedAt: &done, OutputTokens: &output,
		UserID: &userID, APIKeyID: &apiKeyID, Model: "gpt-6-astra", Window: 10 * time.Second,
	})

	require.NoError(t, err)
	require.Equal(t, int64(7), got.UsageLogID)
	require.Equal(t, service.TokenAnalysisUsageMatchTiming, got.Method)
	require.Equal(t, int16(3), got.MatchConfidence)
	require.NoError(t, mock.ExpectationsWereMet())
}

// 存量行: 无关联键/完成时间, 转发须在开始后窗口内启动; 竞争请求的合理性按其自身有无完成时间判断。
func TestTokenAnalysisRepositoryFindUsageLinkLegacyTiming(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	repo := NewTokenAnalysisRepository(db)
	start := time.Date(2026, 10, 24, 6, 11, 12, 0, time.UTC)

	mock.ExpectQuery(`<= \$3\s+AND ul.created_at <= \$4\s+AND ul.request_id NOT LIKE ALL[\s\S]*CASE WHEN c.completed_at IS NULL`).
		WithArgs(start.Add(-500*time.Millisecond), start, start.Add(10*time.Second), start.Add(2*time.Hour), "arch-1").
		WillReturnError(sql.ErrNoRows)

	got, err := repo.FindUsageLink(context.Background(), service.TokenAnalysisUsageLinkQuery{
		ArchiveID: "arch-1", StartedAt: start, Window: 10 * time.Second,
	})

	require.NoError(t, err)
	require.Nil(t, got)
	require.NoError(t, mock.ExpectationsWereMet())
}

// 请求行尚未入库的 response 暂存; 合并后删除已合并与过期暂存。
func TestTokenAnalysisRepositoryRecordResponseStashesUntilRequestArrives(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	repo := NewTokenAnalysisRepository(db)
	done := time.Date(2026, 10, 24, 6, 11, 31, 0, time.UTC)
	output := int64(547)

	mock.ExpectBegin()
	mock.ExpectExec(`SET completed_at = \$2, response_status = \$3, response_output_tokens = \$4, response_id = \$5,\s+`+
		`forward_path = \$6, cache_key_handling = \$7, upstream_cache_key_hash = \$8, usage_cache_source = \$9,\s+`+
		`upstream_items_hash = \$10, upstream_root_hash = \$11,\s+`+
		`usage_link_version = usage_link_version \+ CASE WHEN \(completed_at IS DISTINCT FROM \$2 OR response_status IS DISTINCT FROM \$3 OR response_id <> \$5\)\s+AND usage_match_method <> 'request_id'[\s\S]*`+
		`usage_link_retry_until = CASE WHEN [\s\S]* THEN NOW\(\) \+ INTERVAL '172800 seconds' ELSE usage_link_retry_until END`).
		WithArgs("arch-1", done, 200, output, "resp_1", "openai_responses", "scoped", "0123456789abcdef0123456789abcdef", "upstream_parsed", "11111111111111111111111111111111", "22222222222222222222222222222222").WillReturnResult(sqlmock.NewResult(0, 1))
	// 完成事实是分类输入: 同一事务内给本行及其子孙加分类版本。
	expectTokenAnalysisClassificationDirty(mock, `{"arch-1"}`)
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec(`SET completed_at`).WithArgs("arch-2", done, nil, nil, "", "", "", "", "", "", "").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`INSERT INTO token_analysis_pending_responses[\s\S]*forward_path = EXCLUDED.forward_path[\s\S]*upstream_root_hash = EXCLUDED.upstream_root_hash`).WithArgs("arch-2", done, nil, nil, "", "", "", "", "", "", "").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectQuery(`UPDATE token_analysis_request_summaries s[\s\S]*forward_path = p.forward_path[\s\S]*usage_link_version = s.usage_link_version \+ CASE WHEN \(s.completed_at IS DISTINCT FROM p.completed_at[\s\S]*FROM token_analysis_pending_responses p[\s\S]*DELETE FROM token_analysis_pending_responses p\s+WHERE p.archive_id IN \(SELECT archive_id FROM merged\) OR p.recorded_at < \$1\s+\)\s+SELECT archive_id FROM merged`).
		WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"archive_id"}).AddRow("arch-2"))
	expectTokenAnalysisClassificationDirty(mock, `{"arch-2"}`)
	mock.ExpectCommit()

	ctx := context.Background()
	require.NoError(t, repo.RecordResponse(ctx, service.TokenAnalysisResponseFacts{
		ArchiveID: "arch-1", CompletedAt: done, Status: 200, OutputTokens: &output, ResponseID: "resp_1",
		Forward: service.TokenAnalysisForwardFacts{
			ForwardPath: "openai_responses", CacheKeyHandling: "scoped",
			UpstreamCacheKeyHash: "0123456789abcdef0123456789abcdef", UsageCacheSource: "upstream_parsed",
			UpstreamItemsHash: "11111111111111111111111111111111", UpstreamRootHash: "22222222222222222222222222222222",
		},
	}))
	require.NoError(t, repo.RecordResponse(ctx, service.TokenAnalysisResponseFacts{ArchiveID: "arch-2", CompletedAt: done}))
	require.NoError(t, repo.RecordResponse(ctx, service.TokenAnalysisResponseFacts{}))
	require.NoError(t, repo.MergePendingResponses(ctx))
	require.NoError(t, mock.ExpectationsWereMet())
}

// 认领在事务内按账单加咨询锁; 写入按读到的关联状态与复核版本条件更新。精确关联
// 夺回弱关联占用的账单并为原行登记复核; 时序关联遇到占用、或本行已被改过时放弃。
func TestTokenAnalysisRepositorySetUsageLinkClaimsAtomically(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	repo := NewTokenAnalysisRepository(db)
	ctx := context.Background()
	exact := &service.TokenAnalysisUsageMatch{UsageLogID: 99, MatchConfidence: 3, Method: service.TokenAnalysisUsageMatchRequestID}
	timing := &service.TokenAnalysisUsageMatch{UsageLogID: 99, MatchConfidence: 3, Method: service.TokenAnalysisUsageMatchTiming}
	readV2 := service.TokenAnalysisUsageLinkState{Version: 2}
	holders := func(methods ...string) *sqlmock.Rows {
		rows := sqlmock.NewRows([]string{"usage_match_method"})
		for _, method := range methods {
			rows.AddRow(method)
		}
		return rows
	}
	const setLink = `SET usage_log_id = \$2, match_confidence = \$3, usage_match_method = \$4, risk_score = \$5, risk_reasons = \$6::jsonb,\s+` +
		`usage_link_checked_version = CASE WHEN \$10::boolean THEN usage_link_checked_version ELSE \$9 END\s+` +
		`WHERE archive_id = \$1 AND usage_log_id IS NOT DISTINCT FROM \$7 AND usage_match_method = \$8 AND usage_link_version = \$9`

	mock.ExpectBegin()
	mock.ExpectExec(`pg_advisory_xact_lock\(hashtextextended\('token_analysis_usage_claim:' \|\| \$1::text, 0\)\)`).WithArgs(int64(99)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`WHERE usage_log_id = \$1 AND archive_id <> \$2\s+FOR UPDATE`).WithArgs(int64(99), "arch-1").WillReturnRows(holders(service.TokenAnalysisUsageMatchTiming))
	mock.ExpectExec(setLink).
		WithArgs("arch-1", int64(99), int16(3), service.TokenAnalysisUsageMatchRequestID, 45, sqlmock.AnyArg(), nil, "", int64(2), false).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SET usage_log_id = NULL, match_confidence = 0, usage_match_method = 'released',\s+usage_link_version = usage_link_version \+ CASE WHEN TRUE THEN 1 ELSE 0 END[\s\S]*RETURNING archive_id`).
		WithArgs(int64(99), "arch-1").WillReturnRows(sqlmock.NewRows([]string{"archive_id"}).AddRow("arch-0"))
	// 写入行与被夺回的行在同一事务内加分类版本。
	expectTokenAnalysisClassificationDirty(mock, `{"arch-1","arch-0"}`)
	mock.ExpectCommit()
	write, err := repo.SetUsageLink(ctx, "arch-1", readV2, exact, 45, []service.TokenAnalysisRiskReason{{Code: "huge_input_tiny_output", Score: 45}})
	require.NoError(t, err)
	require.Equal(t, service.TokenAnalysisUsageLinkWrite{Applied: true, Displaced: true}, write)

	mock.ExpectBegin()
	mock.ExpectExec(`pg_advisory_xact_lock`).WithArgs(int64(99)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`FOR UPDATE`).WithArgs(int64(99), "arch-2").WillReturnRows(holders(service.TokenAnalysisUsageMatchRequestID))
	mock.ExpectRollback()
	write, err = repo.SetUsageLink(ctx, "arch-2", readV2, timing, 0, []service.TokenAnalysisRiskReason{})
	require.NoError(t, err)
	require.False(t, write.Applied)

	// 读到的状态或版本已过期(其他实例刚写入或登记了新复核): 条件更新 0 行, 不提交。
	mock.ExpectBegin()
	mock.ExpectExec(`pg_advisory_xact_lock`).WithArgs(int64(99)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`FOR UPDATE`).WithArgs(int64(99), "arch-4").WillReturnRows(holders())
	mock.ExpectExec(setLink).WithArgs("arch-4", int64(99), int16(3), service.TokenAnalysisUsageMatchTiming, 0, "[]", nil, "", int64(2), false).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	write, err = repo.SetUsageLink(ctx, "arch-4", readV2, timing, 0, []service.TokenAnalysisRiskReason{})
	require.NoError(t, err)
	require.False(t, write.Applied)

	// 解除关联但继续等待晚到账单: 不确认版本; 确认按读到的版本条件更新。
	stale := int64(41)
	mock.ExpectBegin()
	mock.ExpectExec(setLink).WithArgs("arch-3", nil, int16(0), "", 0, "[]", stale, "", int64(5), true).WillReturnResult(sqlmock.NewResult(0, 1))
	expectTokenAnalysisClassificationDirty(mock, `{"arch-3"}`)
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec(`SET usage_link_checked_version = \$2\s+WHERE archive_id = \$1 AND usage_link_version = \$2 AND usage_link_checked_version <> \$2`).WithArgs("arch-3", int64(5)).WillReturnResult(sqlmock.NewResult(0, 1))
	expectTokenAnalysisClassificationDirty(mock, `{"arch-3"}`)
	mock.ExpectCommit()
	write, err = repo.SetUsageLink(ctx, "arch-3", service.TokenAnalysisUsageLinkState{UsageLogID: &stale, Version: 5, KeepPending: true}, nil, 0, []service.TokenAnalysisRiskReason{})
	require.NoError(t, err)
	require.True(t, write.Applied)
	require.NoError(t, repo.ClearUsageLinkPending(ctx, "arch-3", 5))
	require.NoError(t, mock.ExpectationsWereMet())
}

// 候选含未关联/旧窗口/被夺回的行, 以及可被精确证据纠正的时序关联行; 旧行缺重复数时为 nil。
func TestTokenAnalysisRepositoryListUsageLinkCandidates(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	repo := NewTokenAnalysisRepository(db)
	start := time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	event := start.Add(6 * time.Hour)

	mock.ExpectQuery(`WHERE s.id > \$3\s+AND \(s.usage_link_version <> s.usage_link_checked_version OR \(s.event_time >= \$1 AND s.event_time < \$2\)\)\s+AND s.endpoint NOT LIKE '%/count_tokens'\s+`+
		`AND \(s.usage_link_version <> s.usage_link_checked_version OR s.usage_match_method IN \('', 'released'\)\s+OR \(s.usage_match_method = 'timing' AND \(s.client_request_id <> '' OR s.response_id <> ''\)\)\)\s+ORDER BY s.id`).
		WithArgs(start, end, int64(0), 500).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "archive_id", "client_request_id", "response_id", "event_time", "completed_at", "response_status",
			"response_output_tokens", "user_id", "api_key_id", "model", "usage_log_id", "usage_match_method",
			"usage_link_version", "usage_link_pending", "usage_link_retry_until", "body_sha256",
			"system_chars", "user_chars", "tools_count", "tool_message_count", "tool_output_bytes", "max_tool_output_bytes", "same_body",
		}).
			AddRow(int64(5), "legacy", "", "", event, nil, nil, nil, int64(2), int64(9), "gpt-6-astra", int64(41), "", int64(0), false, nil, "sha", 10, 20, 3, int64(0), int64(0), int64(0), nil).
			AddRow(int64(6), "new", "cid-1", "resp_1", event, event.Add(time.Minute), int32(502), int64(547), int64(2), int64(9), "gpt-6-astra", nil, "released", int64(3), true, event.Add(48*time.Hour), "sha2", 0, 5, 0, int64(12), int64(600000), int64(524288), int32(2)))

	got, err := repo.ListUsageLinkCandidates(context.Background(), start, end, 0, 500)

	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, int64(41), *got[0].UsageLogID)
	require.Nil(t, got[0].CompletedAt)
	require.Nil(t, got[0].ResponseStatus)
	require.Nil(t, got[0].SameBodyRecentCount)
	require.Equal(t, 3, got[0].Body.ToolsCount)
	require.Equal(t, "cid-1", got[1].ClientRequestID)
	require.Equal(t, "resp_1", got[1].ResponseID)
	require.Equal(t, 502, *got[1].ResponseStatus)
	require.Equal(t, service.TokenAnalysisUsageMatchReleased, got[1].Method)
	require.True(t, got[1].LinkPending)
	require.Equal(t, int64(3), got[1].LinkVersion)
	require.Equal(t, event.Add(48*time.Hour), *got[1].RetryUntil)
	require.False(t, got[0].LinkPending)
	require.Nil(t, got[0].RetryUntil)
	require.Equal(t, int64(547), *got[1].OutputTokens)
	require.Equal(t, 2, *got[1].SameBodyRecentCount)
	require.Equal(t, 12, got[1].Body.ToolMessageCount)
	require.Equal(t, int64(524288), got[1].Body.MaxToolOutputBytes)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTokenAnalysisRepositoryListUserUsageAggregatesByUserOnly(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := NewTokenAnalysisRepository(db)
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	userID := int64(7)

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM \([\s\S]*SELECT s\.user_id[\s\S]*GROUP BY s\.user_id, u\.email\s*\) grouped`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	// 默认按 total_tokens 降序(与项目排行一致); 次级 actual_cost / request_count。
	mock.ExpectQuery(`SELECT[\s\S]*NULL::BIGINT AS api_key_id[\s\S]*'' AS api_key_name[\s\S]*GROUP BY s\.user_id, u\.email[\s\S]*ORDER BY total_tokens desc, actual_cost DESC, request_count DESC`).
		WillReturnRows(sqlmock.NewRows([]string{
			"user_id", "user_email", "api_key_id", "api_key_name", "request_count", "risky_request_count",
			"total_tokens", "input_tokens", "output_tokens", "cache_read_tokens", "cache_creation_tokens",
			"actual_cost", "risky_cost", "last_event_time",
		}).AddRow(userID, "dev@example.com", nil, "", int64(5), int64(2), int64(1200), int64(700), int64(100), int64(300), int64(100), 1.5, 0.7, now))

	items, result, err := repo.ListUserUsage(context.Background(), service.TokenAnalysisFilters{IncludeUnmatched: true},
		pagination.PaginationParams{Page: 1, PageSize: 20, SortBy: "total_tokens", SortOrder: "desc"})

	require.NoError(t, err)
	require.Equal(t, int64(1), result.Total)
	require.Len(t, items, 1)
	require.Equal(t, userID, *items[0].UserID)
	require.Equal(t, "dev@example.com", items[0].UserEmail)
	require.Nil(t, items[0].APIKeyID)
	require.Empty(t, items[0].APIKeyName)
	require.Equal(t, int64(1200), items[0].TotalTokens)
	require.InDelta(t, 300.0/1100.0, items[0].CacheHitRate, 0.0001)
	require.InDelta(t, 0.4, items[0].RiskRatio, 0.0001)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTokenAnalysisRepositoryListUserUsageSortableByActualCost(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := NewTokenAnalysisRepository(db)

	mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	// 显式 sort_by=actual_cost 时走费用主序。
	mock.ExpectQuery("ORDER BY actual_cost asc, total_tokens DESC, request_count DESC").
		WillReturnRows(sqlmock.NewRows([]string{
			"user_id", "user_email", "api_key_id", "api_key_name", "request_count", "risky_request_count",
			"total_tokens", "input_tokens", "output_tokens", "cache_read_tokens", "cache_creation_tokens",
			"actual_cost", "risky_cost", "last_event_time",
		}))

	_, _, err = repo.ListUserUsage(context.Background(), service.TokenAnalysisFilters{IncludeUnmatched: true},
		pagination.PaginationParams{Page: 1, PageSize: 20, SortBy: "actual_cost", SortOrder: "asc"})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
func TestTokenAnalysisRepositoryListProjectUsageSortable(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := NewTokenAnalysisRepository(db)
	now := time.Date(2026, 6, 10, 10, 0, 0, 0, time.UTC)

	mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	// 排序参数经白名单映射进 ORDER BY(SELECT 别名), asc 生效(归一化为小写)。
	mock.ExpectQuery("ORDER BY request_count asc, total_tokens DESC").
		WillReturnRows(sqlmock.NewRows([]string{
			"project", "user_id", "user_email", "request_count", "matched_request_count",
			"total_tokens", "input_tokens", "output_tokens", "cache_read_tokens", "cache_creation_tokens",
			"actual_cost", "last_event_time",
		}).AddRow("demo", int64(7), "a@b.c", int64(3), int64(2), int64(900), int64(500), int64(100), int64(200), int64(100), 0.5, now))

	items, result, err := repo.ListProjectUsage(context.Background(), service.TokenAnalysisFilters{IncludeUnmatched: true},
		pagination.PaginationParams{Page: 1, PageSize: 20, SortBy: "request_count", SortOrder: "asc"})

	require.NoError(t, err)
	require.Equal(t, int64(1), result.Total)
	require.Len(t, items, 1)
	require.Equal(t, "demo", items[0].Project)
	require.Equal(t, int64(3), items[0].RequestCount)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTokenAnalysisRepositoryListProjectUsageRejectsUnknownSort(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := NewTokenAnalysisRepository(db)

	mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	// 未识别排序字段(潜在注入)回退默认 total_tokens, 排序方向保留请求值。
	mock.ExpectQuery("ORDER BY total_tokens asc, actual_cost DESC, request_count DESC").
		WillReturnRows(sqlmock.NewRows([]string{
			"project", "user_id", "user_email", "request_count", "matched_request_count",
			"total_tokens", "input_tokens", "output_tokens", "cache_read_tokens", "cache_creation_tokens",
			"actual_cost", "last_event_time",
		}))

	_, _, err = repo.ListProjectUsage(context.Background(), service.TokenAnalysisFilters{IncludeUnmatched: true},
		pagination.PaginationParams{Page: 1, PageSize: 20, SortBy: "1; DROP TABLE users", SortOrder: "asc"})

	require.NoError(t, err)
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

func TestTokenAnalysisRepositoryUpdateIndexStateKeepsLastError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := NewTokenAnalysisRepository(db)

	// 空 last_error 不得清掉已有错误(成功行/收尾更新都传空),
	// 否则诊断信息在一轮索引结束后必然丢失。
	mock.ExpectExec(`INSERT INTO token_analysis_index_state[\s\S]*last_error = COALESCE\(NULLIF\(EXCLUDED\.last_error, ''\), token_analysis_index_state\.last_error\)`).
		WithArgs("2026-06-09.jsonl", int64(100), "arch-1", int64(1), int64(0), "", nil, nil).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = repo.UpdateIndexState(context.Background(), service.TokenAnalysisIndexState{
		SourceFile: "2026-06-09.jsonl", LastOffset: 100, LastArchiveID: "arch-1", ProcessedRows: 1,
	})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTokenAnalysisRepositoryGetIndexStatusIgnoresStaleUnfinishedRows(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := NewTokenAnalysisRepository(db)
	now := time.Date(2026, 6, 10, 10, 0, 0, 0, time.UTC)

	// 中断过的轮次留下 started_at 非空、finished_at 为空的行,
	// 不得据此判定 Running(否则状态永久卡在运行中, 轮询停不下来)。
	mock.ExpectQuery("FROM token_analysis_index_state").
		WillReturnRows(sqlmock.NewRows([]string{
			"source_file", "last_offset", "last_archive_id", "processed_rows", "failed_rows", "last_error", "started_at", "finished_at", "updated_at",
		}).AddRow("2026-06-02.jsonl", int64(10), "a1", int64(5), int64(1), "boom", now, nil, now))

	status, err := repo.GetIndexStatus(context.Background())

	require.NoError(t, err)
	require.False(t, status.Running)
	require.Equal(t, "boom", status.LastError)
	require.Equal(t, int64(1), status.FailedRows)
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

func TestTokenAnalysisRepositoryCountBilledRequests(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := NewTokenAnalysisRepository(db)
	start := time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	userID := int64(22)

	// 模型过滤与 FindUsageLink 同口径: model/requested_model/upstream_model 任一命中。
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM usage_logs WHERE[\s\S]*\(model = \$4 OR requested_model = \$4 OR upstream_model = \$4\)`).
		WithArgs(start, end, userID, "gpt-5.5").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(42)))

	got, err := repo.CountBilledRequests(context.Background(), service.TokenAnalysisFilters{
		StartTime: &start, EndTime: &end, UserID: &userID, Model: "gpt-5.5",
		// 归档侧维度不应进入 usage_logs 查询条件。
		Endpoint: "/v1/responses", RiskMin: 50, Project: "demo", IncludeUnmatched: true,
	})

	require.NoError(t, err)
	require.Equal(t, int64(42), got)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTokenAnalysisRepositoryUpsertUserInputPreservesQuality(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := NewTokenAnalysisRepository(db)
	userID := int64(7)
	now := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)

	// 冲突更新刷新内容字段; quality_* 只在内容哈希不变时保留(语义由集成测试覆盖)。
	mock.ExpectExec(`INSERT INTO token_analysis_user_inputs[\s\S]*ON CONFLICT \(archive_id\) DO UPDATE SET[\s\S]*truncated = EXCLUDED\.truncated,[\s\S]*quality_score = CASE WHEN t\.content_sha256 = EXCLUDED\.content_sha256`).
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

func TestTokenAnalysisRepositoryDeleteUserInput(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := NewTokenAnalysisRepository(db)
	mock.ExpectExec(`DELETE FROM token_analysis_user_inputs WHERE archive_id = \$1`).
		WithArgs("arch-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, repo.DeleteUserInput(context.Background(), "arch-1"))
	// 空 archive_id 不发 SQL。
	require.NoError(t, repo.DeleteUserInput(context.Background(), " "))
	require.NoError(t, mock.ExpectationsWereMet())
}

// expectTokenAnalysisClassificationDirty 期望源写入事务给本行加分类版本并登记失效(子孙
// 由作业在提交后传播)。ids 是 pq 数组的文本形式。
func expectTokenAnalysisClassificationDirty(mock sqlmock.Sqlmock, ids string) {
	mock.ExpectExec(`WITH locked AS \([\s\S]*ORDER BY id\s+FOR UPDATE\s+\)\s+UPDATE token_analysis_request_summaries s\s+SET cls_input_version = s.cls_input_version \+ 1`).
		WithArgs(ids).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO token_analysis_cls_invalidations \(archive_id\)\s+SELECT unnest\(\$1::text\[\]\)\s+ON CONFLICT \(archive_id\) DO UPDATE SET seq = nextval\('token_analysis_cls_invalidation_seq'\)`).
		WithArgs(ids).WillReturnResult(sqlmock.NewResult(0, 1))
}

// 源写入事务遇到死锁时整体重试(审核 R3): 第一次在语句处被 PG 选为死锁受害者, 第二次成功。
func TestTokenAnalysisRepositoryRetriesDeadlockedTransaction(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	repo := NewTokenAnalysisRepository(db)

	const clear = `SET usage_link_checked_version = \$2\s+WHERE archive_id = \$1 AND usage_link_version = \$2 AND usage_link_checked_version <> \$2`
	mock.ExpectBegin()
	mock.ExpectExec(clear).WithArgs("arch-1", int64(3)).WillReturnError(&pq.Error{Code: "40P01", Message: "deadlock detected"})
	mock.ExpectRollback()
	mock.ExpectBegin()
	mock.ExpectExec(clear).WithArgs("arch-1", int64(3)).WillReturnResult(sqlmock.NewResult(0, 1))
	expectTokenAnalysisClassificationDirty(mock, `{"arch-1"}`)
	mock.ExpectCommit()
	require.NoError(t, repo.ClearUsageLinkPending(context.Background(), "arch-1", 3))

	// 非冲突错误不重试。
	mock.ExpectBegin()
	mock.ExpectExec(clear).WithArgs("arch-1", int64(4)).WillReturnError(&pq.Error{Code: "23505", Message: "unique violation"})
	mock.ExpectRollback()
	require.Error(t, repo.ClearUsageLinkPending(context.Background(), "arch-1", 4))
	require.NoError(t, mock.ExpectationsWereMet())
}
