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

// 展示配置是单例行，测试直接使用真实库并在结束时恢复，不能与其他修改该表的测试并行。
func TestGPTQuotaDisplayRepositoryRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := NewGPTQuotaDisplayRepository(integrationDB)
	client := testEntClient(t)

	parent := mustCreateAccount(t, client, &service.Account{Name: "gptq-parent", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Credentials: map[string]any{}})
	c1 := mustCreateAccount(t, client, &service.Account{Name: "c-1 owner@example.com", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Credentials: map[string]any{"auth_mode": "chatgpt", "access_token": "secret"}})
	d1 := mustCreateAccount(t, client, &service.Account{Name: "d-1", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Credentials: map[string]any{"openai_auth_mode": "personal_access_token"}})
	shadow := mustCreateAccount(t, client, &service.Account{Name: "c-2", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Credentials: map[string]any{}, ParentAccountID: &parent.ID, QuotaDimension: service.QuotaDimensionSpark})
	ids := []int64{parent.ID, c1.ID, d1.ID, shadow.ID}

	var original struct {
		enabled  bool
		interval int
	}
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT enabled, interval_minutes FROM gpt_quota_display_config WHERE id = TRUE`).Scan(&original.enabled, &original.interval))
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM gpt_quota_display_entries WHERE account_id = ANY($1)`, pq.Array(ids))
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM gpt_quota_display_snapshots WHERE account_id = ANY($1)`, pq.Array(ids))
		// 影子引用母账号（ON DELETE RESTRICT），先删影子。
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM accounts WHERE id = $1`, shadow.ID)
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM accounts WHERE id = ANY($1)`, pq.Array(ids))
		_, _ = integrationDB.ExecContext(ctx, `UPDATE gpt_quota_display_config SET enabled = $1, interval_minutes = $2, last_slot_at = NULL WHERE id = TRUE`, original.enabled, original.interval)
	})

	// 配置：默认时段文本、版本冲突、事务内替换选择。
	cfg, err := repo.GetConfig(ctx)
	require.NoError(t, err)
	require.Equal(t, "09:30", cfg.StartTime)
	require.Equal(t, "18:00", cfg.EndTime)
	_, err = repo.SaveConfig(ctx, *cfg, nil, cfg.Version+100, 1, nil)
	require.ErrorIs(t, err, service.ErrGPTQuotaConfigConflict)

	_, err = integrationDB.ExecContext(ctx, `UPDATE gpt_quota_display_config SET last_slot_at = NULL WHERE id = TRUE`)
	require.NoError(t, err)
	floor := time.Now().UTC().Truncate(time.Microsecond)
	cfg.Enabled, cfg.IntervalMinutes = true, 60
	saved, err := repo.SaveConfig(ctx, *cfg, []service.GPTQuotaDisplaySelection{{AccountID: c1.ID, DisplayName: "迅游一号"}, {AccountID: d1.ID}, {AccountID: shadow.ID}}, cfg.Version, 1, &floor)
	require.NoError(t, err)
	require.Equal(t, cfg.Version+1, saved.Version)
	require.True(t, saved.Enabled)
	require.Equal(t, 60, saved.IntervalMinutes)
	require.NotNil(t, saved.LastSlotAt, "slot floor raises a NULL last_slot_at")
	require.WithinDuration(t, floor, *saved.LastSlotAt, time.Microsecond)

	entries, err := repo.ListSelectedEntries(ctx)
	require.NoError(t, err)
	byAccount := map[int64]service.GPTQuotaDisplayEntry{}
	for _, e := range entries {
		byAccount[e.AccountID] = e
	}
	require.Equal(t, "迅游一号", byAccount[c1.ID].DisplayName)
	require.Equal(t, "chatgpt", byAccount[c1.ID].AuthMode)
	require.Equal(t, "personal_access_token", byAccount[d1.ID].LegacyAuthMode)
	require.NotNil(t, byAccount[shadow.ID].ParentAccountID)
	require.True(t, byAccount[c1.ID].AccountExists)

	// 软删除账号仍在条目里，但标记为不存在。
	_, err = integrationDB.ExecContext(ctx, `UPDATE accounts SET deleted_at = NOW() WHERE id = $1`, d1.ID)
	require.NoError(t, err)
	entries, err = repo.ListSelectedEntries(ctx)
	require.NoError(t, err)
	for _, e := range entries {
		if e.AccountID == d1.ID {
			require.False(t, e.AccountExists)
		}
	}

	earlier := floor.Add(-time.Hour)
	resaved, err := repo.SaveConfig(ctx, *saved, []service.GPTQuotaDisplaySelection{{AccountID: c1.ID}}, saved.Version, 1, &earlier)
	require.NoError(t, err)
	require.WithinDuration(t, floor, *resaved.LastSlotAt, time.Microsecond, "slot floor never moves last_slot_at backwards")
	entries, err = repo.ListSelectedEntries(ctx)
	require.NoError(t, err)
	var selectedIDs []int64
	for _, e := range entries {
		if e.AccountID == c1.ID || e.AccountID == d1.ID || e.AccountID == shadow.ID {
			selectedIDs = append(selectedIDs, e.AccountID)
		}
	}
	require.Equal(t, []int64{c1.ID}, selectedIDs, "save replaces the previous selection")

	// 采集尝试：跨实例冷却与 Retry-After 退避。
	attempt := time.Now().UTC().Truncate(time.Microsecond)
	ok, err := repo.ClaimAttempt(ctx, c1.ID, attempt, time.Minute)
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = repo.ClaimAttempt(ctx, c1.ID, attempt.Add(30*time.Second), time.Minute)
	require.NoError(t, err)
	require.False(t, ok, "cooldown")
	retryAt := attempt.Add(10 * time.Minute)
	require.NoError(t, repo.FinishAttempt(ctx, c1.ID, attempt, service.GPTQuotaStatusRateLimited, &retryAt))
	ok, err = repo.ClaimAttempt(ctx, c1.ID, attempt.Add(2*time.Minute), time.Minute)
	require.NoError(t, err)
	require.False(t, ok, "retry-after backoff")

	snaps, err := repo.ListSnapshots(ctx, []int64{c1.ID})
	require.NoError(t, err)
	require.Equal(t, service.GPTQuotaStatusRateLimited, snaps[c1.ID].LastAttemptStatus)
	require.Nil(t, snaps[c1.ID].SampledAt)

	// 成功快照：缺失窗口写 SQL NULL；旧采样不能覆盖新采样；晚返回的旧尝试不改状态。
	attempt2 := retryAt.Add(time.Second)
	ok, err = repo.ClaimAttempt(ctx, c1.ID, attempt2, time.Minute)
	require.NoError(t, err)
	require.True(t, ok)
	sampled := attempt2.Add(2 * time.Second)
	reset := sampled.Add(time.Hour)
	published, err := repo.PublishSnapshot(ctx, &service.GPTQuotaDisplaySnapshot{AccountID: c1.ID, FiveHour: &service.GPTQuotaWindow{RemainingPercent: 75, ResetAt: &reset, ResetTimeSource: "reset_at"}, SampledAt: &sampled, Source: "active"}, attempt2)
	require.NoError(t, err)
	require.True(t, published)
	var sevenDayIsNull bool
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT seven_day IS NULL FROM gpt_quota_display_snapshots WHERE account_id = $1`, c1.ID).Scan(&sevenDayIsNull))
	require.True(t, sevenDayIsNull)

	older := sampled.Add(-time.Minute)
	published, err = repo.PublishSnapshot(ctx, &service.GPTQuotaDisplaySnapshot{AccountID: c1.ID, FiveHour: &service.GPTQuotaWindow{RemainingPercent: 1}, SampledAt: &older, Source: "active"}, attempt)
	require.NoError(t, err)
	require.False(t, published)
	require.NoError(t, repo.FinishAttempt(ctx, c1.ID, attempt, service.GPTQuotaStatusTimeout, nil))

	snaps, err = repo.ListSnapshots(ctx, []int64{c1.ID, d1.ID})
	require.NoError(t, err)
	snap := snaps[c1.ID]
	require.Equal(t, service.GPTQuotaStatusOK, snap.LastAttemptStatus)
	require.Nil(t, snap.RetryAfter)
	require.Equal(t, 75.0, snap.FiveHour.RemainingPercent)
	require.WithinDuration(t, reset, *snap.FiveHour.ResetAt, time.Second)
	require.Nil(t, snap.SevenDay)
	require.WithinDuration(t, sampled, *snap.SampledAt, time.Microsecond)
	require.NotContains(t, snaps, d1.ID)

	// 计划槽位：同一槽位只领取一次。
	_, err = integrationDB.ExecContext(ctx, `UPDATE gpt_quota_display_config SET last_slot_at = NULL WHERE id = TRUE`)
	require.NoError(t, err)
	slot := time.Date(2026, 9, 24, 1, 30, 0, 0, time.UTC)
	ok, err = repo.ClaimSlot(ctx, slot)
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = repo.ClaimSlot(ctx, slot)
	require.NoError(t, err)
	require.False(t, ok)
	ok, err = repo.ClaimSlot(ctx, slot.Add(30*time.Minute))
	require.NoError(t, err)
	require.True(t, ok)
}

func TestGPTQuotaDisplayRepositoryListCandidates(t *testing.T) {
	ctx := context.Background()
	repo := NewGPTQuotaDisplayRepository(integrationDB)
	client := testEntClient(t)
	// 用独特前缀隔离其他测试遗留的账号。
	mk := func(name string, credentials map[string]any) *service.Account {
		return mustCreateAccount(t, client, &service.Account{Name: name, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Credentials: credentials})
	}
	c10 := mk("c-10 gqcand", map[string]any{})
	c2 := mk("C-2 gqcand", map[string]any{})
	c002 := mk("c-002 gqcand", map[string]any{})
	d1 := mk("d-1 gqcand", map[string]any{"auth_mode": "personal_access_token"})
	other := mk("team gqcand", map[string]any{})
	literal := mk("c-9 gq%_cand", map[string]any{})
	deleted := mk("c-1 gqcand", map[string]any{})
	apiKey := mustCreateAccount(t, client, &service.Account{Name: "c-3 gqcand", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Credentials: map[string]any{"api_key": "x"}})
	ids := []int64{c10.ID, c2.ID, c002.ID, d1.ID, other.ID, literal.ID, deleted.ID, apiKey.ID}
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM accounts WHERE id = ANY($1)`, pq.Array(ids))
	})
	_, err := integrationDB.ExecContext(ctx, `UPDATE accounts SET deleted_at = NOW() WHERE id = $1`, deleted.ID)
	require.NoError(t, err)

	items, total, err := repo.ListCandidates(ctx, "gqcand", 10, 0)
	require.NoError(t, err)
	require.Equal(t, 5, total, "only undeleted OpenAI OAuth accounts; literal %/_ must not act as wildcards")
	var names []string
	for _, it := range items {
		names = append(names, it.AccountName)
	}
	require.Equal(t, []string{"c-002 gqcand", "C-2 gqcand", "c-10 gqcand", "d-1 gqcand", "team gqcand"}, names)
	require.Equal(t, "personal_access_token", items[3].AuthMode)

	items, total, err = repo.ListCandidates(ctx, "gqcand", 2, 2)
	require.NoError(t, err)
	require.Equal(t, 5, total)
	require.Len(t, items, 2)
	require.Equal(t, "c-10 gqcand", items[0].AccountName)

	items, total, err = repo.ListCandidates(ctx, "gq%_cand", 10, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Equal(t, literal.ID, items[0].AccountID)

	items, total, err = repo.ListCandidates(ctx, "gqcand", 0, 0)
	require.NoError(t, err)
	require.Equal(t, 5, total)
	require.Empty(t, items)
}

// 多实例并发：同一槽位、同一账号尝试只能被领取一次。
func TestGPTQuotaDisplayRepositoryConcurrentClaims(t *testing.T) {
	ctx := context.Background()
	repo := NewGPTQuotaDisplayRepository(integrationDB)
	account := mustCreateAccount(t, testEntClient(t), &service.Account{Name: "c-77 gqconc", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Credentials: map[string]any{}})
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM accounts WHERE id = $1`, account.ID)
		_, _ = integrationDB.ExecContext(ctx, `UPDATE gpt_quota_display_config SET last_slot_at = NULL WHERE id = TRUE`)
	})
	_, err := integrationDB.ExecContext(ctx, `UPDATE gpt_quota_display_config SET last_slot_at = NULL WHERE id = TRUE`)
	require.NoError(t, err)

	const workers = 16
	slot := time.Date(2026, 9, 24, 2, 0, 0, 0, time.UTC)
	attempt := time.Now().UTC().Truncate(time.Microsecond)
	var slotWins, attemptWins atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			if ok, err := repo.ClaimSlot(ctx, slot); err == nil && ok {
				slotWins.Add(1)
			} else if err != nil {
				t.Errorf("claim slot: %v", err)
			}
			// 各实例时钟略有差异，仍在 60 秒冷却内。
			at := attempt.Add(time.Duration(i) * time.Millisecond)
			if ok, err := repo.ClaimAttempt(ctx, account.ID, at, time.Minute); err == nil && ok {
				attemptWins.Add(1)
			} else if err != nil {
				t.Errorf("claim attempt: %v", err)
			}
		}(i)
	}
	close(start)
	wg.Wait()
	require.EqualValues(t, 1, slotWins.Load())
	require.EqualValues(t, 1, attemptWins.Load())
}
