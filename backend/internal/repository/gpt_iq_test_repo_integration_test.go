//go:build integration

package repository

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// 降智测试配置是单例行，测试直接使用真实库并在结束时恢复，不能与其他修改该表的测试并行。
func TestGPTIQTestRepositoryRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := NewGPTIQTestRepository(integrationDB)
	client := testEntClient(t)

	acct := mustCreateAccount(t, client, &service.Account{Name: "c-91", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Credentials: map[string]any{}})
	var entryID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO gpt_quota_display_entries (account_id, selected) VALUES ($1, TRUE) RETURNING id`, acct.ID).Scan(&entryID))
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM gpt_iq_test_batches`)
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM gpt_iq_test_cases`)
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM gpt_iq_test_daily_usage`)
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM gpt_iq_test_backoff`)
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM gpt_quota_display_entries WHERE id = $1`, entryID)
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM accounts WHERE id = $1`, acct.ID)
		_, _ = integrationDB.ExecContext(ctx, `UPDATE gpt_iq_test_config SET enabled = FALSE, schedule_enabled = FALSE, run_time = '08:30', last_run_date = NULL WHERE id = TRUE`)
	})

	// 配置：默认值、版本冲突、日期只进不退。
	cfg, err := repo.GetConfig(ctx)
	require.NoError(t, err)
	require.Equal(t, "08:30", cfg.RunTime)
	require.Equal(t, 300, cfg.DailyCallBudget)
	_, err = repo.SaveConfig(ctx, *cfg, cfg.Version+100, 1, "")
	require.ErrorIs(t, err, service.ErrGPTIQConfigConflict)
	cfg.Enabled, cfg.ScheduleEnabled, cfg.RunTime = true, true, "09:15"
	saved, err := repo.SaveConfig(ctx, *cfg, cfg.Version, 1, "2026-11-23")
	require.NoError(t, err)
	require.Equal(t, "09:15", saved.RunTime)
	require.Equal(t, "2026-11-23", saved.LastRunDate)
	saved, err = repo.SaveConfig(ctx, *saved, saved.Version, 1, "2026-11-01")
	require.NoError(t, err)
	require.Equal(t, "2026-11-23", saved.LastRunDate, "mark never moves the date backwards")
	claimed, err := repo.ClaimRunDate(ctx, "2026-11-23")
	require.NoError(t, err)
	require.False(t, claimed)
	claimed, err = repo.ClaimRunDate(ctx, "2026-11-24")
	require.NoError(t, err)
	require.True(t, claimed)

	// 测试项：只改可变属性不建新版本；改判分建新版本；删除乐观锁。
	in, spec, err := service.NormalizeGPTIQCaseInput(service.GPTIQCaseTemplates()[1].Input)
	require.NoError(t, err)
	created, err := repo.CreateCase(ctx, in, spec, 1)
	require.NoError(t, err)
	require.Equal(t, 1, created.Revision)
	require.Equal(t, spec.Prompt, created.Prompt)
	in.Name = "糖果题二"
	renamed, err := repo.UpdateCase(ctx, created.ID, created.Version, in, spec, 1)
	require.NoError(t, err)
	require.Equal(t, created.RevisionID, renamed.RevisionID)
	require.Equal(t, "糖果题二", renamed.Name)
	_, err = repo.UpdateCase(ctx, created.ID, created.Version, in, spec, 1)
	require.ErrorIs(t, err, service.ErrGPTIQCaseConflict)
	spec2 := spec
	spec2.ScoringMode = service.GPTIQScoringNumberAny
	revised, err := repo.UpdateCase(ctx, created.ID, renamed.Version, in, spec2, 1)
	require.NoError(t, err)
	require.Equal(t, 2, revised.Revision)
	require.NotEqual(t, created.RevisionID, revised.RevisionID)
	revs, err := repo.ListRevisions(ctx, []int64{created.RevisionID, revised.RevisionID})
	require.NoError(t, err)
	require.Equal(t, service.GPTIQScoringFinalLine, revs[created.RevisionID].ScoringMode, "old revision is immutable")
	require.Equal(t, service.GPTIQScoringNumberAny, revs[revised.RevisionID].ScoringMode)

	// 批次：全站只允许一个运行中批次；执行权令牌栅栏。
	batch, err := repo.CreateBatch(ctx, &service.GPTIQTestBatch{Trigger: service.GPTIQTriggerManual, RunDate: "2026-11-24", ScopeGroups: []string{"xunyou"}, EntryIDs: []int64{entryID}, CaseIDs: []int64{created.ID}, LeaseToken: "lease-a", TotalCalls: 3})
	require.NoError(t, err)
	require.Equal(t, service.GPTIQBatchRunning, batch.Status)
	require.Equal(t, []string{"xunyou"}, batch.ScopeGroups)
	_, err = repo.CreateBatch(ctx, &service.GPTIQTestBatch{Trigger: service.GPTIQTriggerSchedule, RunDate: "2026-11-24", LeaseToken: "lease-b"})
	require.ErrorIs(t, err, service.ErrGPTIQBatchRunning)

	start := time.Now().UTC().Truncate(time.Microsecond)
	res := &service.GPTIQTestResult{BatchID: batch.ID, RunDate: "2026-11-24", Trigger: batch.Trigger, EntryID: entryID, AccountID: acct.ID, CaseID: created.ID, RevisionID: revised.RevisionID, RunIndex: 1, Status: service.GPTIQResultRunning, ModelRequested: "gpt-6.1-sol", StartedAt: start}
	_, err = repo.InsertResult(ctx, "wrong-lease", res)
	require.ErrorIs(t, err, service.ErrGPTIQLeaseLost)
	res.ID, err = repo.InsertResult(ctx, "lease-a", res)
	require.NoError(t, err)
	correct, tokens, finished := true, 700, start.Add(3*time.Second)
	res.Status, res.Correct, res.Attempts, res.AnswerText, res.ReasoningTokens, res.FinishedAt = service.GPTIQResultOK, &correct, 2, "最终答案＝21\x00", &tokens, &finished
	scored := revised.RevisionID
	res.ScoredRevisionID = &scored
	require.NoError(t, repo.FinishResult(ctx, "lease-a", res))
	require.ErrorIs(t, repo.FinishResult(ctx, "lease-a", res), service.ErrGPTIQLeaseLost, "a finished row cannot be overwritten")

	alive, cancel, err := repo.HeartbeatBatch(ctx, batch.ID, "lease-a")
	require.NoError(t, err)
	require.True(t, alive)
	require.False(t, cancel)
	reserved, err := repo.ReserveDispatch(ctx, batch.ID, "lease-a", "2026-11-24", 10)
	require.NoError(t, err)
	require.True(t, reserved)
	ok, err := repo.RequestCancel(ctx, batch.ID)
	require.NoError(t, err)
	require.True(t, ok)
	_, cancel, _ = repo.HeartbeatBatch(ctx, batch.ID, "lease-a")
	require.True(t, cancel)
	_, err = repo.ReserveDispatch(ctx, batch.ID, "lease-a", "2026-11-24", 10)
	require.ErrorIs(t, err, service.ErrGPTIQBatchCancelRequested, "no new dispatch after cancel was requested")

	results, err := repo.ListBatchResults(ctx, batch.ID, 4)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "最终答案", results[0].AnswerText, "answer limit truncates in SQL; NUL removed")
	require.Equal(t, 2, results[0].Attempts)
	require.Equal(t, 700, *results[0].ReasoningTokens)
	require.Nil(t, results[0].InputTokens)
	require.True(t, *results[0].Correct)
	exists, err := repo.RecentRunExists(ctx, entryID, created.ID, start.Add(-time.Minute))
	require.NoError(t, err)
	require.True(t, exists)

	statuses, err := repo.ListBatchStatuses(ctx, []int64{batch.ID, 999999999})
	require.NoError(t, err)
	require.Equal(t, map[int64]string{batch.ID: service.GPTIQBatchRunning}, statuses)

	// 派发预占绑定执行权：错误令牌拒绝，已请求取消拒绝。
	_, err = repo.ReserveDispatch(ctx, batch.ID, "wrong-lease", "2026-11-24", 10)
	require.ErrorIs(t, err, service.ErrGPTIQLeaseLost)

	summaries, err := repo.ListBatchSummaries(ctx, 5, nil)
	require.NoError(t, err)
	require.Equal(t, 1, summaries[0].OK)
	require.Equal(t, 1, summaries[0].Correct)
	scoped, err := repo.ListBatchSummaries(ctx, 5, []int64{entryID + 100000})
	require.NoError(t, err)
	require.Empty(t, scoped, "batches without the scoped entries are not listed")
	scoped, err = repo.ListBatchSummaries(ctx, 5, []int64{entryID})
	require.NoError(t, err)
	require.Len(t, scoped, 1)
	require.Equal(t, 1, scoped[0].OK)

	// 重评候选只取同题面；截断标记带回。
	candidates, err := repo.ListRejudgeCandidates(ctx, created.ID, revised.PromptSHA256)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	wrong := false
	require.NoError(t, repo.UpdateCorrect(ctx, []service.GPTIQCorrectUpdate{{ResultID: res.ID, Correct: &wrong, ScoredRevisionID: revised.RevisionID}}))
	// 版本单调：较早版本的重评不能覆盖较新版本的分数。
	require.NoError(t, repo.UpdateCorrect(ctx, []service.GPTIQCorrectUpdate{{ResultID: res.ID, Correct: &correct, ScoredRevisionID: created.RevisionID}}))
	entry, err := repo.ListEntryResults(ctx, entryID, created.ID, "2026-11-01")
	require.NoError(t, err)
	require.False(t, *entry[0].Correct)
	require.Equal(t, "最终答案＝21", entry[0].AnswerText)
	// 回报的更新数按实际以该版本判分的行计：被挡下的旧版本重评计 0。
	applied, err := repo.CountScoredWith(ctx, []int64{res.ID}, revised.RevisionID)
	require.NoError(t, err)
	require.Equal(t, 1, applied)
	applied, err = repo.CountScoredWith(ctx, []int64{res.ID}, created.RevisionID)
	require.NoError(t, err)
	require.Zero(t, applied)

	// 同题面但执行时是 chat 的历史结果不进入重评候选。
	var chatRev int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
INSERT INTO gpt_iq_test_case_revisions (case_id, revision, kind, prompt, prompt_sha256, model, runs, timeout_seconds, scoring_mode)
VALUES ($1, 99, 'chat', $2, $3, 'gpt-5.4', 1, 60, 'none') RETURNING id`, created.ID, revised.Prompt, revised.PromptSHA256).Scan(&chatRev))
	_, err = integrationDB.ExecContext(ctx, `
INSERT INTO gpt_iq_test_results (batch_id, run_date, trigger, entry_id, account_id, case_id, revision_id, run_index, status, answer_text)
VALUES ($1, '2026-11-24', 'manual', $2, $3, $4, $5, 9, 'ok', 'hello')`, batch.ID, entryID, acct.ID, created.ID, chatRev)
	require.NoError(t, err)
	candidates, err = repo.ListRejudgeCandidates(ctx, created.ID, revised.PromptSHA256)
	require.NoError(t, err)
	require.Len(t, candidates, 1, "chat executions are never re-scored")

	// 心跳超时的批次被回收，运行中的结果标 abandoned，旧执行者无法再写。
	res2 := *res
	res2.RunIndex, res2.Status, res2.Correct, res2.ScoredRevisionID = 2, service.GPTIQResultRunning, nil, nil
	res2.ID, err = repo.InsertResult(ctx, "lease-a", &res2)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `UPDATE gpt_iq_test_batches SET heartbeat_at = NOW() - INTERVAL '11 minutes' WHERE id = $1`, batch.ID)
	require.NoError(t, err)
	reaped, err := repo.ReapAbandoned(ctx, 10*time.Minute)
	require.NoError(t, err)
	require.Equal(t, 1, reaped)
	alive, _, err = repo.HeartbeatBatch(ctx, batch.ID, "lease-a")
	require.NoError(t, err)
	require.False(t, alive)
	res2.Status = service.GPTIQResultOK
	require.ErrorIs(t, repo.FinishResult(ctx, "lease-a", &res2), service.ErrGPTIQLeaseLost)
	require.ErrorIs(t, repo.FinishBatch(ctx, batch.ID, "lease-a", service.GPTIQBatchFinished), service.ErrGPTIQLeaseLost)
	got, err := repo.GetBatch(ctx, batch.ID)
	require.NoError(t, err)
	require.Equal(t, service.GPTIQBatchAbandoned, got.Status)
	var abandoned string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT status FROM gpt_iq_test_results WHERE id = $1`, res2.ID).Scan(&abandoned))
	require.Equal(t, service.GPTIQResultAbandoned, abandoned)
	next, err := repo.CreateBatch(ctx, &service.GPTIQTestBatch{Trigger: service.GPTIQTriggerManual, RunDate: "2026-11-24", LeaseToken: "lease-c"})
	require.NoError(t, err, "abandoning releases the global running slot")
	require.NoError(t, repo.CloseBatch(ctx, next.ID, "lease-c", service.GPTIQBatchAborted, service.GPTIQBatchFailurePersistFailed))
	closed, err := repo.GetBatch(ctx, next.ID)
	require.NoError(t, err)
	require.Equal(t, service.GPTIQBatchFailurePersistFailed, closed.FailureReason)

	// 退避。
	require.NoError(t, repo.SetBackoff(ctx, acct.ID, time.Now().Add(time.Hour), "rate_limited"))
	backoff, err := repo.ListBackoff(ctx, []int64{acct.ID})
	require.NoError(t, err)
	require.Contains(t, backoff, acct.ID)

	// 清理按 run_date。
	removed, err := repo.Cleanup(ctx, "2026-11-25")
	require.NoError(t, err)
	require.EqualValues(t, 3, removed, "two runs of the graded revision plus the historical chat run")

	require.NoError(t, repo.DeleteCase(ctx, created.ID, revised.Version, 1))
	_, err = repo.GetCase(ctx, created.ID)
	require.ErrorIs(t, err, service.ErrGPTIQCaseNotFound)
	require.ErrorIs(t, repo.DeleteCase(ctx, created.ID, revised.Version+1, 1), service.ErrGPTIQCaseNotFound)
}

// 16 路并发争最后的派发名额：恰好 budget 个成功。
func TestGPTIQTestRepositoryReserveDispatchIsAtomic(t *testing.T) {
	ctx := context.Background()
	repo := NewGPTIQTestRepository(integrationDB)
	date := "2031-01-01"
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM gpt_iq_test_daily_usage WHERE run_date = $1::date`, date)
	})

	batch, err := repo.CreateBatch(ctx, &service.GPTIQTestBatch{Trigger: service.GPTIQTriggerManual, RunDate: date, LeaseToken: "lease-r"})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM gpt_iq_test_batches WHERE id = $1`, batch.ID)
	})
	var wins atomic.Int32
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := repo.ReserveDispatch(ctx, batch.ID, "lease-r", date, 5)
			require.NoError(t, err)
			if ok {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	require.EqualValues(t, 5, wins.Load())
	n, err := repo.DispatchedOn(ctx, date)
	require.NoError(t, err)
	require.Equal(t, 5, n)
	require.NoError(t, repo.ReleaseDispatch(ctx, date))
	n, _ = repo.DispatchedOn(ctx, date)
	require.Equal(t, 4, n)
	ok, err := repo.ReserveDispatch(ctx, batch.ID, "lease-r", date, 0)
	require.NoError(t, err)
	require.False(t, ok, "zero budget never dispatches")
}

func TestGPTIQTestRepositoryLoadActor(t *testing.T) {
	ctx := context.Background()
	repo := NewGPTIQTestRepository(integrationDB)
	insertUser := func(email, role string, permissions string) int64 {
		var id int64
		require.NoError(t, integrationDB.QueryRowContext(ctx, `
INSERT INTO users (email, password_hash, role, status, admin_permissions, created_at, updated_at)
VALUES ($1, 'x', $2, 'active', $3::jsonb, NOW(), NOW()) RETURNING id`, email, role, permissions).Scan(&id))
		return id
	}
	admin := insertUser("iq-admin@example.test", service.RoleAdmin, `[]`)
	explicit := insertUser("iq-sub@example.test", service.RoleSubAdmin, `["admin.gpt_iq_test"]`)
	leader := insertUser("iq-leader@example.test", service.RoleSubAdmin, `["admin.organization_usage","admin.department_subscriptions","admin.department_members","admin.organization_manage"]`)
	dual := insertUser("iq-dual@example.test", service.RoleSubAdmin, `["admin.organization_usage","admin.department_subscriptions","admin.department_members","admin.organization_manage","admin.gpt_iq_test"]`)
	plain := insertUser("iq-user@example.test", service.RoleUser, `[]`)
	other := insertUser("iq-usage@example.test", service.RoleSubAdmin, `["admin.usage"]`)
	_, err := integrationDB.ExecContext(ctx, `INSERT INTO organization_access_grants (user_id, organization_key, created_by) VALUES ($1, 'wsdashi', $3), ($2, 'xunyou', $3)`, leader, dual, admin)
	require.NoError(t, err)
	ids := []int64{admin, explicit, leader, dual, plain, other}
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM organization_access_grants WHERE user_id = ANY($1)`, pq.Array(ids))
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM users WHERE id = ANY($1)`, pq.Array(ids))
	})

	load := func(id int64) (*service.GPTIQTestActor, error) {
		return repo.LoadActor(service.WithDepartmentActor(ctx, id))
	}
	a, err := load(admin)
	require.NoError(t, err)
	require.True(t, a.FullScope)
	require.True(t, a.CanConfigure)
	a, err = load(explicit)
	require.NoError(t, err)
	require.True(t, a.FullScope)
	require.True(t, a.CanConfigure)
	a, err = load(leader)
	require.NoError(t, err)
	require.False(t, a.FullScope)
	require.False(t, a.CanConfigure, "organization permission alone cannot change site-wide settings")
	require.Equal(t, []string{"wsdashi"}, a.Groups)
	a, err = load(dual)
	require.NoError(t, err)
	require.True(t, a.CanConfigure)
	require.False(t, a.FullScope, "the extra permission does not widen the leader's run scope")
	require.Equal(t, []string{"xunyou"}, a.Groups)
	_, err = load(plain)
	require.ErrorIs(t, err, service.ErrGPTIQScopeDenied)
	_, err = load(other)
	require.ErrorIs(t, err, service.ErrGPTIQScopeDenied)
	_, err = repo.LoadActor(ctx)
	require.ErrorIs(t, err, service.ErrGPTIQScopeDenied)
}
