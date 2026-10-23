//go:build integration

package repository

import (
	"context"
	"os"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// The rollup state is a single shared row; every test leaves it uninitialized (raw only).
func resetOrganizationUsageRollupsForTest(t *testing.T) {
	t.Helper()
	reset := func() {
		_, err := integrationDB.ExecContext(context.Background(), `DELETE FROM usage_user_daily_rollups;
DELETE FROM usage_user_rollup_dirty_days;
UPDATE usage_user_rollup_state SET closed_before = NULL WHERE id = 1`)
		require.NoError(t, err)
	}
	reset()
	t.Cleanup(reset)
}

func publishOrganizationUsageRollupsForTest(t *testing.T, from string, days int) {
	t.Helper()
	ctx := context.Background()
	_, err := integrationDB.ExecContext(ctx, `UPDATE usage_user_rollup_state SET closed_before = $1::date WHERE id = 1`, from)
	require.NoError(t, err)
	repo := NewOrganizationUsageRollupRepository(integrationDB)
	for i := 0; i < days; i++ {
		closed, err := repo.CloseNextOrganizationUsageRollupDay(ctx)
		require.NoError(t, err)
		require.True(t, closed)
	}
}

func drainOrganizationUsageDirtyDaysForTest(t *testing.T) {
	t.Helper()
	repo := NewOrganizationUsageRollupRepository(integrationDB)
	for {
		rebuilt, err := repo.RebuildNextDirtyOrganizationUsageRollupDay(context.Background())
		require.NoError(t, err)
		if !rebuilt {
			return
		}
	}
}

func organizationUsageDirtyDayExists(t *testing.T, day string) bool {
	t.Helper()
	var exists bool
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		`SELECT EXISTS(SELECT 1 FROM usage_user_rollup_dirty_days WHERE bucket_date = $1::date)`, day).Scan(&exists))
	return exists
}

// requireOrganizationUsageSourcesMatch runs every report surface through the raw-only and the
// rollup-backed repository and requires identical results.
func requireOrganizationUsageSourcesMatch(t *testing.T, ctx context.Context, label, prefix string, platform string) {
	t.Helper()
	raw := &organizationUsageRepository{db: integrationDB, rollupDisabled: true}
	rolled := &organizationUsageRepository{db: integrationDB}
	filters := service.OrganizationUsageDepartmentFilters{Platform: platform}
	asOf := time.Date(2026, 3, 8, 6, 30, 0, 0, time.UTC) // 14:30 Beijing, inside a published day

	summaryRanges := []service.OrganizationUsageSummaryRepositoryParams{
		organizationUsageSummaryIntegrationParams(prefix, "2026-03-01", "2026-03-31"),
		organizationUsageSummaryIntegrationParams(prefix, "2026-03-03", "2026-03-09"),
	}
	clipped := organizationUsageSummaryIntegrationParams(prefix, "2026-03-01", "2026-03-31")
	clipped.EndTime = asOf
	summaryRanges = append(summaryRanges, clipped)
	for _, params := range summaryRanges {
		for _, sortBy := range []string{"total_tokens", "actual_cost", "peak_week_tokens"} {
			params.SortBy = sortBy
			params.Filters = filters
			want, err := raw.Summary(ctx, params)
			require.NoError(t, err)
			got, err := rolled.Summary(ctx, params)
			require.NoError(t, err)
			require.Equal(t, want, got, "%s summary %s %s", label, params.StartDate.Format("01-02"), sortBy)
		}
		for _, granularity := range []string{service.OrganizationUsageGranularityDay, service.OrganizationUsageGranularityWeek, service.OrganizationUsageGranularityMonth} {
			periods := organizationUsagePeriodsIntegrationParams(prefix, params.StartDate.Format("2006-01-02"), params.EndDate.Format("2006-01-02"), granularity)
			periods.EndTime, periods.Filters = params.EndTime, filters
			want, err := raw.Periods(ctx, periods)
			require.NoError(t, err)
			got, err := rolled.Periods(ctx, periods)
			require.NoError(t, err)
			require.Equal(t, want, got, "%s periods %s", label, granularity)

			dataThrough := "2026-03-20"
			if params.EndTime.Equal(asOf) {
				dataThrough = "2026-03-08"
			} else if end := params.EndDate.Format("2006-01-02"); end < dataThrough {
				dataThrough = end
			}
			trend := organizationUsageTrendIntegrationParams(prefix, params.StartDate.Format("2006-01-02"), params.EndDate.Format("2006-01-02"), dataThrough, service.OrganizationAll, granularity)
			if params.EndTime.Before(trend.EndTime) {
				trend.EndTime = params.EndTime
			}
			trend.Filters = filters
			wantTrend, err := raw.Trend(ctx, trend)
			require.NoError(t, err)
			gotTrend, err := rolled.Trend(ctx, trend)
			require.NoError(t, err)
			require.Equal(t, wantTrend, gotTrend, "%s trend %s", label, granularity)
		}
	}
}

func TestOrganizationUsageRollupIntegration_ReportsMatchRawAggregation(t *testing.T) {
	resetOrganizationUsageRollupsForTest(t)
	ctx := context.Background()
	prefix := organizationUsageIntegrationPrefix("rollup")
	account := organizationUsageIntegrationAccount(t, prefix)
	grok := organizationUsageIntegrationAccount(t, prefix+"grok")
	_, err := integrationDB.ExecContext(ctx, `UPDATE accounts SET platform='grok' WHERE id=$1`, grok.ID)
	require.NoError(t, err)
	cleanupOrganizationUsageIntegrationData(t, prefix)
	admin, _ := organizationUsageIntegrationUser(t, prefix+"admin@example.com", service.StatusActive)
	_, err = integrationDB.ExecContext(ctx, `UPDATE users SET role='admin' WHERE id=$1`, admin.ID)
	require.NoError(t, err)
	alice, aliceKey := organizationUsageIntegrationUser(t, prefix+"alice@xunyou.com", service.StatusActive)
	bob, bobKey := organizationUsageIntegrationUser(t, prefix+"bob@wsdashi.com", service.StatusActive)
	organizationUsageIntegrationUser(t, prefix+"zero@example.com", service.StatusActive)

	beijing := time.FixedZone("Asia/Shanghai", 8*60*60)
	for day := 2; day <= 16; day++ {
		date := time.Date(2026, 3, day, 0, 0, 0, 0, beijing)
		// Both edges of the Beijing day, plus a second account/platform and fractional costs.
		insertOrganizationUsageIntegrationLog(t, alice.ID, aliceKey.ID, account.ID, 100*day, 10, 3, 7, 0.1234567891*float64(day), date.Add(5*time.Minute))
		insertOrganizationUsageIntegrationLog(t, alice.ID, aliceKey.ID, grok.ID, 50, 5*day, 1, 2, 0.3333333333, date.Add(23*time.Hour+59*time.Minute))
		insertOrganizationUsageIntegrationLog(t, bob.ID, bobKey.ID, grok.ID, 70*day, 20, 0, 9, 0.0000000007*float64(day), date.Add(12*time.Hour))
	}
	insertOrganizationUsageIntegrationLog(t, alice.ID, aliceKey.ID, account.ID, 999, 1, 1, 1, 9.99, time.Date(2026, 2, 28, 23, 0, 0, 0, beijing))

	requireOrganizationUsageSourcesMatch(t, ctx, "unpublished", prefix, "all")

	// Publish 2026-03-01..03-12; 03-13..03-16 stay unpublished and are read raw.
	publishOrganizationUsageRollupsForTest(t, "2026-03-01", 12)
	var rollupRows int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM usage_user_daily_rollups WHERE user_id IN ($1, $2)`, alice.ID, bob.ID).Scan(&rollupRows))
	require.Equal(t, 11*3, rollupRows) // 03-02..03-12: alice × 2 accounts + bob × 1
	requireOrganizationUsageSourcesMatch(t, ctx, "published", prefix, "all")

	// Control: the rollup-backed path really reads the published rollup rows.
	planned, err := (&organizationUsageRepository{db: integrationDB}).withRollupPlan(ctx,
		time.Date(2026, 3, 1, 0, 0, 0, 0, beijing), time.Date(2026, 3, 1, 0, 0, 0, 0, beijing), time.Date(2026, 4, 1, 0, 0, 0, 0, beijing))
	require.NoError(t, err)
	require.Equal(t, "2026-03-13", planned.plan.rollupEnd)
	corruption, err := integrationDB.ExecContext(ctx, `UPDATE usage_user_daily_rollups SET requests = requests + 100 WHERE user_id = $1 AND bucket_date = '2026-03-04'`, alice.ID)
	require.NoError(t, err)
	corruptedRows, err := corruption.RowsAffected()
	require.NoError(t, err)
	require.Equal(t, int64(2), corruptedRows)
	corrupted, err := (&organizationUsageRepository{db: integrationDB}).Summary(ctx, organizationUsageSummaryIntegrationParams(prefix, "2026-03-01", "2026-03-31"))
	require.NoError(t, err)
	reference, err := (&organizationUsageRepository{db: integrationDB, rollupDisabled: true}).Summary(ctx, organizationUsageSummaryIntegrationParams(prefix, "2026-03-01", "2026-03-31"))
	require.NoError(t, err)
	require.Equal(t, reference.Overview.Requests+100*corruptedRows, corrupted.Overview.Requests)
	_, err = integrationDB.ExecContext(ctx, `INSERT INTO usage_user_rollup_dirty_days (bucket_date) VALUES ('2026-03-04')`)
	require.NoError(t, err)
	requireOrganizationUsageSourcesMatch(t, ctx, "dirty corrupted day", prefix, "all")
	drainOrganizationUsageDirtyDaysForTest(t)

	// A late insert on a published day is read raw until the day is rebuilt.
	insertOrganizationUsageIntegrationLog(t, bob.ID, bobKey.ID, account.ID, 5000, 5000, 0, 0, 12.5, time.Date(2026, 3, 5, 10, 0, 0, 0, beijing))
	require.True(t, organizationUsageDirtyDayExists(t, "2026-03-05"))
	requireOrganizationUsageSourcesMatch(t, ctx, "late insert", prefix, "all")
	drainOrganizationUsageDirtyDaysForTest(t)
	require.False(t, organizationUsageDirtyDayExists(t, "2026-03-05"))
	requireOrganizationUsageSourcesMatch(t, ctx, "rebuilt", prefix, "all")

	// Deletes and platform changes keep matching the current-value semantics.
	_, err = integrationDB.ExecContext(ctx, `DELETE FROM usage_logs WHERE user_id=$1 AND created_at >= $2 AND created_at < $3`,
		alice.ID, time.Date(2026, 3, 6, 0, 0, 0, 0, beijing), time.Date(2026, 3, 6, 1, 0, 0, 0, beijing))
	require.NoError(t, err)
	require.True(t, organizationUsageDirtyDayExists(t, "2026-03-06"))
	requireOrganizationUsageSourcesMatch(t, ctx, "delete", prefix, "all")
	drainOrganizationUsageDirtyDaysForTest(t)
	_, err = integrationDB.ExecContext(ctx, `UPDATE accounts SET platform='openai' WHERE id=$1`, grok.ID)
	require.NoError(t, err)

	adminCtx := service.WithDepartmentActor(ctx, admin.ID)
	for _, platform := range []string{"all", "openai", "grok", "unknown"} {
		requireOrganizationUsageSourcesMatch(t, adminCtx, "scoped "+platform, prefix, platform)
	}
}

func TestOrganizationUsageRollupIntegration_TriggersMarkOnlyPastDays(t *testing.T) {
	resetOrganizationUsageRollupsForTest(t)
	ctx := context.Background()
	prefix := organizationUsageIntegrationPrefix("rollup-trigger")
	account := organizationUsageIntegrationAccount(t, prefix)
	cleanupOrganizationUsageIntegrationData(t, prefix)
	user, key := organizationUsageIntegrationUser(t, prefix+"user@xunyou.com", service.StatusActive)

	var nowBeijing time.Time
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&nowBeijing))
	nowBeijing = nowBeijing.In(organizationUsageRollupLocation)
	today := nowBeijing.Format(organizationUsageDateLayout)
	insertOrganizationUsageIntegrationLog(t, user.ID, key.ID, account.ID, 1, 1, 1, 1, 1, nowBeijing)
	nearMidnight := nowBeijing.Add(10*time.Minute).Format(organizationUsageDateLayout) != today
	if !nearMidnight {
		require.False(t, organizationUsageDirtyDayExists(t, today), "same-day writes outside the midnight window are not marked")
	}

	past := time.Date(2026, 1, 15, 9, 0, 0, 0, organizationUsageRollupLocation)
	insertOrganizationUsageIntegrationLog(t, user.ID, key.ID, account.ID, 1, 1, 1, 1, 1, past)
	require.True(t, organizationUsageDirtyDayExists(t, "2026-01-15"))

	_, err := integrationDB.ExecContext(ctx, `DELETE FROM usage_user_rollup_dirty_days`)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `UPDATE usage_logs SET created_at = $1 WHERE user_id = $2 AND created_at = $3`,
		time.Date(2026, 1, 20, 9, 0, 0, 0, organizationUsageRollupLocation), user.ID, past)
	require.NoError(t, err)
	require.True(t, organizationUsageDirtyDayExists(t, "2026-01-15"))
	require.True(t, organizationUsageDirtyDayExists(t, "2026-01-20"))

	_, err = integrationDB.ExecContext(ctx, `DELETE FROM usage_user_rollup_dirty_days`)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `UPDATE usage_logs SET model = 'renamed' WHERE user_id = $1`, user.ID)
	require.NoError(t, err)
	var marks int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM usage_user_rollup_dirty_days`).Scan(&marks))
	require.Zero(t, marks, "updates of unrelated columns do not mark")

	_, err = integrationDB.ExecContext(ctx, `DELETE FROM usage_logs WHERE user_id = $1 AND created_at < $2`, user.ID, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.True(t, organizationUsageDirtyDayExists(t, "2026-01-20"))
}

func TestOrganizationUsageRollupIntegration_SyncLifecycleAndReconcile(t *testing.T) {
	resetOrganizationUsageRollupsForTest(t)
	ctx := context.Background()
	prefix := organizationUsageIntegrationPrefix("rollup-sync")
	account := organizationUsageIntegrationAccount(t, prefix)
	cleanupOrganizationUsageIntegrationData(t, prefix)
	user, key := organizationUsageIntegrationUser(t, prefix+"user@xunyou.com", service.StatusActive)
	beijing := organizationUsageRollupLocation
	insertOrganizationUsageIntegrationLog(t, user.ID, key.ID, account.ID, 10, 10, 0, 0, 1.5, time.Date(2026, 4, 2, 10, 0, 0, 0, beijing))
	insertOrganizationUsageIntegrationLog(t, user.ID, key.ID, account.ID, 20, 20, 0, 0, 2.5, time.Date(2026, 4, 3, 10, 0, 0, 0, beijing))

	repo := NewOrganizationUsageRollupRepository(integrationDB)
	initialized, err := repo.InitializeOrganizationUsageRollups(ctx)
	require.NoError(t, err)
	require.True(t, initialized)
	initialized, err = repo.InitializeOrganizationUsageRollups(ctx)
	require.NoError(t, err)
	require.False(t, initialized, "initialization runs once")

	// Jump the watermark next to the fixture instead of replaying the shared database history.
	_, err = integrationDB.ExecContext(ctx, `UPDATE usage_user_rollup_state SET closed_before = '2026-04-02' WHERE id = 1`)
	require.NoError(t, err)
	publishOrganizationUsageRollupsForTest(t, "2026-04-02", 2)
	var requests int64
	var cost float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT SUM(requests), SUM(actual_cost)::double precision FROM usage_user_daily_rollups WHERE user_id = $1`, user.ID).Scan(&requests, &cost))
	require.Equal(t, int64(2), requests)
	require.InDelta(t, 4.0, cost, 1e-12)

	marked, err := repo.ReconcileOrganizationUsageRollups(ctx, 1, time.Hour)
	require.NoError(t, err)
	require.Empty(t, marked)

	// Corrupt a published day: reconciliation marks it and the rebuild repairs it.
	_, err = integrationDB.ExecContext(ctx, `UPDATE usage_user_daily_rollups SET requests = requests + 1 WHERE user_id = $1 AND bucket_date = '2026-04-03'`, user.ID)
	require.NoError(t, err)
	marked, err = repo.ReconcileOrganizationUsageRollups(ctx, 1, time.Hour)
	require.NoError(t, err)
	require.Empty(t, marked, "the shared hourly cadence was already claimed")
	marked, err = repo.ReconcileOrganizationUsageRollups(ctx, 1, 0)
	require.NoError(t, err)
	require.Equal(t, []string{"2026-04-03"}, marked)
	drainOrganizationUsageDirtyDaysForTest(t)
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT SUM(requests) FROM usage_user_daily_rollups WHERE user_id = $1`, user.ID).Scan(&requests))
	require.Equal(t, int64(2), requests)

	// TRUNCATE is not used here because the database is shared; the reset trigger is covered by the migration test.
	svc := service.NewOrganizationUsageRollupService(repo, nil, nil)
	runCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	require.NoError(t, svc.RunOnce(runCtx))
}

// publishOrganizationUsageRollupsForPerformance publishes every closable day from `from` when
// ORGANIZATION_USAGE_PERF_ROLLUP=1, so the opt-in performance tests measure the rollup path;
// otherwise they keep measuring raw usage_logs aggregation.
func publishOrganizationUsageRollupsForPerformance(t *testing.T, from time.Time) {
	t.Helper()
	resetOrganizationUsageRollupsForTest(t)
	if os.Getenv("ORGANIZATION_USAGE_PERF_ROLLUP") != "1" {
		t.Log("ORG_USAGE_ROLLUP disabled; set ORGANIZATION_USAGE_PERF_ROLLUP=1 to measure the rollup path")
		return
	}
	ctx := context.Background()
	_, err := integrationDB.ExecContext(ctx, `UPDATE usage_user_rollup_state SET closed_before = $1::date WHERE id = 1`, from.Format("2006-01-02"))
	require.NoError(t, err)
	repo := NewOrganizationUsageRollupRepository(integrationDB)
	started := time.Now()
	closed := 0
	for {
		done, err := repo.CloseNextOrganizationUsageRollupDay(ctx)
		require.NoError(t, err)
		if !done {
			break
		}
		closed++
	}
	t.Logf("ORG_USAGE_ROLLUP published_days=%d elapsed_ms=%d", closed, time.Since(started).Milliseconds())
	_, err = integrationDB.ExecContext(ctx, `ANALYZE usage_user_daily_rollups`)
	require.NoError(t, err)
}

// TestOrganizationUsageRollupIntegration_ProductionShapedBenchmark mirrors the 2026-10-23
// production shape: 135 active users, ~47k usage logs per day and ~400 rollup rows per day.
func TestOrganizationUsageRollupIntegration_ProductionShapedBenchmark(t *testing.T) {
	if os.Getenv("ORGANIZATION_USAGE_ROLLUP_BENCH") != "1" {
		t.Skip("set ORGANIZATION_USAGE_ROLLUP_BENCH=1")
	}
	resetOrganizationUsageRollupsForTest(t)
	ctx := context.Background()
	prefix := organizationUsageIntegrationPrefix("bench")
	cleanupOrganizationUsageIntegrationData(t, prefix)
	accountIDs := make([]int64, 0, 3)
	for i, platform := range []string{"anthropic", "openai", "gemini"} {
		account := organizationUsageIntegrationAccount(t, prefix+strconv.Itoa(i))
		_, err := integrationDB.ExecContext(ctx, `UPDATE accounts SET platform=$1 WHERE id=$2`, platform, account.ID)
		require.NoError(t, err)
		accountIDs = append(accountIDs, account.ID)
	}
	admin, _ := organizationUsageIntegrationUser(t, prefix+"admin@example.com", service.StatusActive)
	_, err := integrationDB.ExecContext(ctx, `UPDATE users SET role='admin' WHERE id=$1`, admin.ID)
	require.NoError(t, err)
	userIDs, keyIDs := make([]int64, 0, 135), make([]int64, 0, 135)
	for i := 0; i < 135; i++ {
		domain := []string{"xunyou.com", "wsdashi.com", "example.com"}[i%3]
		user, key := organizationUsageIntegrationUser(t, prefix+"u"+strconv.Itoa(i)+"@"+domain, service.StatusActive)
		userIDs, keyIDs = append(userIDs, user.ID), append(keyIDs, key.ID)
	}

	const days, perDay = 30, 47000
	first := time.Date(2026, 6, 1, 0, 0, 0, 0, organizationUsageRollupLocation)
	seeded := time.Now()
	for day := 0; day < days; day++ {
		_, err := integrationDB.ExecContext(ctx, `INSERT INTO usage_logs (
    user_id, api_key_id, account_id, request_id, model,
    input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens, actual_cost, created_at
)
SELECT u.user_id, u.key_id, acc.id, $1 || $2::int || '-' || g, 'organization-usage-bench',
    g % 1000, g % 300, g % 50, g % 2000, ((g % 997) / 10000.0)::numeric(20,10),
    $3::timestamptz + (g * 86400.0 / $4) * INTERVAL '1 second'
FROM generate_series(0, $4 - 1) AS g
JOIN unnest($5::bigint[], $6::bigint[]) WITH ORDINALITY AS u(user_id, key_id, ord) ON u.ord = g % 135 + 1
JOIN unnest($7::bigint[]) WITH ORDINALITY AS acc(id, ord) ON acc.ord = (g / 135) % 3 + 1`,
			prefix, day, first.AddDate(0, 0, day), perDay, pq.Array(userIDs), pq.Array(keyIDs), pq.Array(accountIDs))
		require.NoError(t, err)
	}
	for _, table := range []string{"users", "usage_logs"} {
		_, err = integrationDB.ExecContext(ctx, "ANALYZE "+table)
		require.NoError(t, err)
	}
	t.Logf("ORG_USAGE_BENCH seeded_rows=%d elapsed_ms=%d", days*perDay, time.Since(seeded).Milliseconds())

	adminCtx := service.WithDepartmentActor(ctx, admin.ID)
	summary := organizationUsageSummaryIntegrationParams(prefix, "2026-06-01", "2026-06-30")
	summary.PageSize = 20
	summary.EndTime = time.Date(2026, 6, 30, 15, 0, 0, 0, organizationUsageRollupLocation) // month to date, today until 15:00
	trend := organizationUsageTrendIntegrationParams(prefix, "2026-06-01", "2026-06-30", "2026-06-30", service.OrganizationAll, service.OrganizationUsageGranularityDay)
	trend.EndTime = summary.EndTime

	measure := func(label string, repo *organizationUsageRepository) (*service.OrganizationUsageSummaryRepositoryResult, *service.OrganizationUsageTrendRepositoryResult) {
		var summaryResult *service.OrganizationUsageSummaryRepositoryResult
		var trendResult *service.OrganizationUsageTrendRepositoryResult
		for _, surface := range []string{"summary", "trend"} {
			samples := make([]time.Duration, 0, 7)
			for i := 0; i < 8; i++ {
				started := time.Now()
				var err error
				if surface == "summary" {
					summaryResult, err = repo.Summary(adminCtx, summary)
				} else {
					trendResult, err = repo.Trend(adminCtx, trend)
				}
				require.NoError(t, err)
				if i > 0 { // the first pass warms caches
					samples = append(samples, time.Since(started))
				}
			}
			sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
			t.Logf("ORG_USAGE_BENCH source=%s surface=%s samples=%d median_ms=%d max_ms=%d", label, surface, len(samples), samples[len(samples)/2].Milliseconds(), samples[len(samples)-1].Milliseconds())
		}
		return summaryResult, trendResult
	}

	rawSummary, rawTrend := measure("raw", &organizationUsageRepository{db: integrationDB, rollupDisabled: true})
	published := time.Now()
	publishOrganizationUsageRollupsForTest(t, "2026-06-01", 29)
	_, err = integrationDB.ExecContext(ctx, `ANALYZE usage_user_daily_rollups`)
	require.NoError(t, err)
	var rollupRows int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM usage_user_daily_rollups WHERE bucket_date >= '2026-06-01' AND bucket_date < '2026-06-30'`).Scan(&rollupRows))
	t.Logf("ORG_USAGE_BENCH published_days=29 rollup_rows=%d elapsed_ms=%d", rollupRows, time.Since(published).Milliseconds())
	rolledSummary, rolledTrend := measure("rollup", &organizationUsageRepository{db: integrationDB})
	require.Equal(t, rawSummary, rolledSummary)
	require.Equal(t, rawTrend, rolledTrend)
}

// A writer that marks an already-dirty day holds the mark until it commits, so the job cannot
// rebuild the day from a snapshot that still contains the writer's uncommitted change.
func TestOrganizationUsageRollupIntegration_RebuildWaitsForWriterHoldingDirtyMark(t *testing.T) {
	resetOrganizationUsageRollupsForTest(t)
	ctx := context.Background()
	prefix := organizationUsageIntegrationPrefix("rollup-race")
	account := organizationUsageIntegrationAccount(t, prefix)
	cleanupOrganizationUsageIntegrationData(t, prefix)
	user, key := organizationUsageIntegrationUser(t, prefix+"user@xunyou.com", service.StatusActive)
	day := time.Date(2026, 5, 6, 10, 0, 0, 0, organizationUsageRollupLocation)
	insertOrganizationUsageIntegrationLog(t, user.ID, key.ID, account.ID, 1, 1, 1, 1, 1, day)
	insertOrganizationUsageIntegrationLog(t, user.ID, key.ID, account.ID, 1, 1, 1, 1, 1, day.Add(time.Hour))
	publishOrganizationUsageRollupsForTest(t, "2026-05-06", 1)
	// An earlier change left a committed mark for the day.
	_, err := integrationDB.ExecContext(ctx, `INSERT INTO usage_user_rollup_dirty_days (bucket_date) VALUES ('2026-05-06')`)
	require.NoError(t, err)

	writer, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = writer.Rollback() }()
	_, err = writer.ExecContext(ctx, `DELETE FROM usage_logs WHERE user_id = $1 AND created_at = $2`, user.ID, day)
	require.NoError(t, err)
	var marker string
	require.NoError(t, writer.QueryRowContext(ctx, `SELECT current_setting('sub2api.user_rollup_dirty_20260506', true)`).Scan(&marker))
	require.Equal(t, "t", marker, "the day is marked once per transaction")

	rebuilt := make(chan error, 1)
	go func() {
		_, err := NewOrganizationUsageRollupRepository(integrationDB).RebuildNextDirtyOrganizationUsageRollupDay(ctx)
		rebuilt <- err
	}()
	select {
	case err := <-rebuilt:
		t.Fatalf("rebuild finished while the writer still held the mark: %v", err)
	case <-time.After(500 * time.Millisecond):
	}
	require.NoError(t, writer.Commit())
	select {
	case err := <-rebuilt:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("rebuild did not resume after the writer committed")
	}
	var requests int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COALESCE(SUM(requests), 0) FROM usage_user_daily_rollups WHERE user_id = $1 AND bucket_date = '2026-05-06'`, user.ID).Scan(&requests))
	require.Equal(t, int64(1), requests, "the rebuild saw the committed delete")
	require.False(t, organizationUsageDirtyDayExists(t, "2026-05-06"))
}

// Cross-partition UPDATEs become DELETE+INSERT; only the statement-level UPDATE trigger sees
// the new day. usage_logs is not partitioned in tests, so a partitioned probe reuses the functions.
func TestOrganizationUsageRollupIntegration_MarksBothDaysForCrossPartitionUpdate(t *testing.T) {
	resetOrganizationUsageRollupsForTest(t)
	ctx := context.Background()
	_, err := integrationDB.ExecContext(ctx, `DROP TABLE IF EXISTS usage_rollup_partition_probe;
CREATE TABLE usage_rollup_partition_probe (
    id BIGINT NOT NULL, created_at TIMESTAMPTZ NOT NULL, user_id BIGINT, group_id BIGINT, account_id BIGINT,
    input_tokens INT, output_tokens INT, cache_creation_tokens INT, cache_read_tokens INT, actual_cost DECIMAL(20,10)
) PARTITION BY RANGE (created_at);
CREATE TABLE usage_rollup_partition_probe_05 PARTITION OF usage_rollup_partition_probe FOR VALUES FROM ('2026-05-01') TO ('2026-06-01');
CREATE TABLE usage_rollup_partition_probe_06 PARTITION OF usage_rollup_partition_probe FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');
CREATE TRIGGER probe_update_statement AFTER UPDATE ON usage_rollup_partition_probe
    REFERENCING OLD TABLE AS old_usage_logs NEW TABLE AS new_usage_logs
    FOR EACH STATEMENT EXECUTE FUNCTION mark_user_usage_rollup_dirty_after_update();
CREATE TRIGGER probe_delete AFTER DELETE ON usage_rollup_partition_probe
    FOR EACH ROW EXECUTE FUNCTION mark_user_usage_rollup_dirty_row();`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := integrationDB.ExecContext(context.Background(), `DROP TABLE IF EXISTS usage_rollup_partition_probe`)
		require.NoError(t, err)
	})
	_, err = integrationDB.ExecContext(ctx, `INSERT INTO usage_rollup_partition_probe VALUES (1, '2026-05-31 12:00:00+08', 1, NULL, 1, 1, 1, 1, 1, 1)`)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `UPDATE usage_rollup_partition_probe SET created_at = '2026-06-01 12:00:00+08' WHERE id = 1`)
	require.NoError(t, err)
	require.True(t, organizationUsageDirtyDayExists(t, "2026-05-31"))
	require.True(t, organizationUsageDirtyDayExists(t, "2026-06-01"))
}

func TestOrganizationUsageRollupIntegration_PartitionDropMarksWholeMonth(t *testing.T) {
	resetOrganizationUsageRollupsForTest(t)
	ctx := context.Background()
	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	// usage_logs partitions are UTC months; they touch Beijing 05-01 through 06-01.
	require.NoError(t, markOrganizationUsageRollupRangeDirty(ctx, tx, time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)))
	require.NoError(t, tx.Commit())
	var count int
	var first, last string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COUNT(*), MIN(bucket_date)::text, MAX(bucket_date)::text FROM usage_user_rollup_dirty_days`).Scan(&count, &first, &last))
	require.Equal(t, 32, count)
	require.Equal(t, "2026-05-01", first)
	require.Equal(t, "2026-06-01", last)
}
