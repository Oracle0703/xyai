//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

const organizationUsageExplainPrefix = "org-usage-perf-"

type organizationUsageExplainEnvelope struct {
	Plan          organizationUsageExplainPlan `json:"Plan"`
	PlanningTime  float64                      `json:"Planning Time"`
	ExecutionTime float64                      `json:"Execution Time"`
}

type organizationUsageExplainPlan struct {
	NodeType            string                         `json:"Node Type"`
	JoinType            string                         `json:"Join Type"`
	Strategy            string                         `json:"Strategy"`
	RelationName        string                         `json:"Relation Name"`
	IndexName           string                         `json:"Index Name"`
	CTEName             string                         `json:"CTE Name"`
	SortMethod          string                         `json:"Sort Method"`
	ActualStartupTime   float64                        `json:"Actual Startup Time"`
	ActualTotalTime     float64                        `json:"Actual Total Time"`
	ActualRows          float64                        `json:"Actual Rows"`
	ActualLoops         float64                        `json:"Actual Loops"`
	SharedHitBlocks     int64                          `json:"Shared Hit Blocks"`
	SharedReadBlocks    int64                          `json:"Shared Read Blocks"`
	TempReadBlocks      int64                          `json:"Temp Read Blocks"`
	TempWrittenBlocks   int64                          `json:"Temp Written Blocks"`
	LocalHitBlocks      int64                          `json:"Local Hit Blocks"`
	LocalReadBlocks     int64                          `json:"Local Read Blocks"`
	LocalWrittenBlocks  int64                          `json:"Local Written Blocks"`
	SharedWrittenBlocks int64                          `json:"Shared Written Blocks"`
	Plans               []organizationUsageExplainPlan `json:"Plans"`
}

func TestOrganizationUsagePostgresEnvironment(t *testing.T) {
	if os.Getenv("ORGANIZATION_USAGE_RUN_EXPLAIN") != "1" {
		t.Skip("set ORGANIZATION_USAGE_RUN_EXPLAIN=1 to inspect the postgres environment")
	}
	var version, workMem, sharedBuffers, effectiveCacheSize, maxParallelWorkers string
	require.NoError(t, integrationDB.QueryRowContext(context.Background(), `
SELECT
    current_setting('server_version'),
    current_setting('work_mem'),
    current_setting('shared_buffers'),
    current_setting('effective_cache_size'),
    current_setting('max_parallel_workers_per_gather')`).Scan(
		&version, &workMem, &sharedBuffers, &effectiveCacheSize, &maxParallelWorkers,
	))
	t.Logf(
		"ORG_USAGE_EXPLAIN_ENV server_version=%s work_mem=%s shared_buffers=%s effective_cache_size=%s max_parallel_workers_per_gather=%s",
		version, workMem, sharedBuffers, effectiveCacheSize, maxParallelWorkers,
	)
}

func TestOrganizationUsageRepositoryExplainAnalyze(t *testing.T) {
	if os.Getenv("ORGANIZATION_USAGE_RUN_EXPLAIN") != "1" {
		t.Skip("set ORGANIZATION_USAGE_RUN_EXPLAIN=1 to seed and analyze organization usage queries")
	}

	ctx := context.Background()
	cleanupOrganizationUsageExplainData(t, ctx)
	seedOrganizationUsageExplainData(t, ctx)
	t.Cleanup(func() { cleanupOrganizationUsageExplainData(t, context.Background()) })

	var activeUsers, usageLogs int64
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users WHERE email LIKE $1`, organizationUsageExplainPrefix+"%",
	).Scan(&activeUsers))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM usage_logs ul
JOIN users u ON u.id = ul.user_id
WHERE u.email LIKE $1`, organizationUsageExplainPrefix+"%",
	).Scan(&usageLogs))
	require.Equal(t, int64(600), activeUsers)
	require.Equal(t, int64(219600), usageLogs)
	t.Logf("ORG_USAGE_EXPLAIN dataset active_users=%d usage_logs=%d", activeUsers, usageLogs)

	_, err := integrationDB.ExecContext(ctx, `ANALYZE users`)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `ANALYZE usage_logs`)
	require.NoError(t, err)

	endDate := time.Date(2026, 6, 30, 0, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60))
	for _, days := range organizationUsageExplainDays(t) {
		startDate := endDate.AddDate(0, 0, -(days - 1))
		startTime := startDate.UTC()
		endTime := endDate.AddDate(0, 0, 1).UTC()
		pattern := organizationUsageSearchPattern(organizationUsageExplainPrefix)
		startRaw := startDate.Format("2006-01-02")
		endRaw := endDate.Format("2006-01-02")

		queries := []struct {
			name  string
			query string
			args  []any
		}{
			{name: "summary-organizations", query: organizationUsageOrganizationsQuery(), args: []any{startTime, endTime, pattern}},
			{name: "summary-count", query: organizationUsageSummaryItemsCountQuery(), args: []any{pattern, service.OrganizationAll}},
			{name: "summary-items", query: organizationUsageSummaryItemsQuery("total_tokens DESC, user_id ASC"), args: []any{startTime, endTime, pattern, service.OrganizationAll, startRaw, endRaw, 500, 0}},
			{name: "summary-items-materialized-peaks", query: organizationUsageMaterializedPeaksCandidateQuery(), args: []any{startTime, endTime, pattern, service.OrganizationAll, startRaw, endRaw, 500, 0}},
			{name: "summary-champions", query: organizationUsageChampionsQuery(), args: []any{startTime, endTime, pattern, service.OrganizationAll, startRaw, endRaw}},
		}
		for _, granularity := range []string{
			service.OrganizationUsageGranularityDay,
			service.OrganizationUsageGranularityWeek,
			service.OrganizationUsageGranularityMonth,
		} {
			countQuery, countErr := organizationUsagePeriodsCountQuery(granularity)
			require.NoError(t, countErr)
			dataQuery, dataErr := organizationUsagePeriodsQuery(granularity)
			require.NoError(t, dataErr)
			queries = append(queries,
				struct {
					name  string
					query string
					args  []any
				}{name: "periods-" + granularity + "-count", query: countQuery, args: []any{startTime, endTime, pattern, service.OrganizationAll}},
				struct {
					name  string
					query string
					args  []any
				}{name: "periods-" + granularity + "-data", query: dataQuery, args: []any{startTime, endTime, pattern, service.OrganizationAll, startRaw, endRaw, 500, 0}},
			)
		}

		for _, query := range queries {
			if filter := os.Getenv("ORGANIZATION_USAGE_EXPLAIN_QUERY"); filter != "" && filter != query.name {
				continue
			}
			// First pass warms PostgreSQL and filesystem caches; the second pass is recorded.
			explainOrganizationUsageQuery(t, ctx, query.query, query.args...)
			plan := explainOrganizationUsageQuery(t, ctx, query.query, query.args...)
			t.Logf(
				"ORG_USAGE_EXPLAIN days=%d query=%s planning_ms=%.3f execution_ms=%.3f rows=%.0f shared_hit=%d shared_read=%d temp_read=%d temp_written=%d",
				days, query.name, plan.PlanningTime, plan.ExecutionTime, plan.Plan.ActualRows,
				plan.Plan.SharedHitBlocks, plan.Plan.SharedReadBlocks, plan.Plan.TempReadBlocks, plan.Plan.TempWrittenBlocks,
			)
			if plan.ExecutionTime >= 1000 {
				logOrganizationUsageSlowPlan(t, query.name, plan.Plan, 0)
			}
		}
	}
}

func organizationUsageMaterializedPeaksCandidateQuery() string {
	return strings.NewReplacer(
		"day_peak AS (", "day_peak AS MATERIALIZED (",
		"week_peak AS (", "week_peak AS MATERIALIZED (",
		"month_peak AS (", "month_peak AS MATERIALIZED (",
	).Replace(organizationUsageSummaryItemsQuery("total_tokens DESC, user_id ASC"))
}

func organizationUsageExplainDays(t *testing.T) []int {
	t.Helper()
	raw := os.Getenv("ORGANIZATION_USAGE_EXPLAIN_DAYS")
	if raw == "" {
		return []int{30, 90, 366}
	}
	days, err := strconv.Atoi(raw)
	require.NoError(t, err)
	require.Contains(t, []int{30, 90, 366}, days)
	return []int{days}
}

func seedOrganizationUsageExplainData(t *testing.T, ctx context.Context) {
	t.Helper()
	var accountID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
INSERT INTO accounts (name, platform, type, credentials, extra, status)
VALUES ($1, 'anthropic', 'oauth', '{}', '{}', 'active')
RETURNING id`, organizationUsageExplainPrefix+"account").Scan(&accountID))

	_, err := integrationDB.ExecContext(ctx, `
INSERT INTO users (email, password_hash, role, status, concurrency)
SELECT
    $1 || n || CASE n % 3 WHEN 0 THEN '@xunyou.com' WHEN 1 THEN '@wsdashi.com' ELSE '@example.com' END,
    'organization-usage-performance', 'user', 'active', 5
FROM generate_series(1, 600) AS n`, organizationUsageExplainPrefix)
	require.NoError(t, err)

	_, err = integrationDB.ExecContext(ctx, `
INSERT INTO api_keys (user_id, key, name, status)
SELECT id, 'sk-perf-' || md5($1 || id::text), 'organization-usage-performance', 'active'
FROM users
WHERE email LIKE $2`, organizationUsageExplainPrefix, organizationUsageExplainPrefix+"%")
	require.NoError(t, err)

	_, err = integrationDB.ExecContext(ctx, `
INSERT INTO usage_logs (
    user_id, api_key_id, account_id, request_id, model,
    input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
    actual_cost, created_at
)
SELECT
    u.id,
    k.id,
    $1,
    md5($2 || u.id::text || '-' || day_offset::text),
    'organization-usage-performance',
    100 + (u.id % 17)::int,
    40 + (u.id % 11)::int,
    20 + (u.id % 7)::int,
    10 + (u.id % 5)::int,
    0.01 + ((u.id % 13)::numeric / 1000),
    '2025-07-01 00:00:00+08'::timestamptz
        + day_offset * interval '1 day'
        + (u.id % 24) * interval '1 hour'
FROM users u
JOIN api_keys k ON k.user_id = u.id AND k.deleted_at IS NULL
CROSS JOIN generate_series(0, 365) AS day_offset
WHERE u.email LIKE $3`, accountID, organizationUsageExplainPrefix, organizationUsageExplainPrefix+"%")
	require.NoError(t, err)
}

func cleanupOrganizationUsageExplainData(t *testing.T, ctx context.Context) {
	t.Helper()
	_, err := integrationDB.ExecContext(ctx, `DELETE FROM users WHERE email LIKE $1`, organizationUsageExplainPrefix+"%")
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `DELETE FROM accounts WHERE name = $1`, organizationUsageExplainPrefix+"account")
	require.NoError(t, err)
}

func explainOrganizationUsageQuery(t *testing.T, ctx context.Context, query string, args ...any) organizationUsageExplainEnvelope {
	t.Helper()
	var raw []byte
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) "+query,
		args...,
	).Scan(&raw))

	var plans []organizationUsageExplainEnvelope
	require.NoError(t, json.Unmarshal(raw, &plans))
	require.Len(t, plans, 1)
	return plans[0]
}

func logOrganizationUsageSlowPlan(t *testing.T, query string, plan organizationUsageExplainPlan, depth int) {
	t.Helper()
	elapsed := plan.ActualTotalTime * plan.ActualLoops
	if depth <= 1 || elapsed >= 50 || plan.TempReadBlocks > 0 || plan.TempWrittenBlocks > 0 {
		t.Logf(
			"ORG_USAGE_SLOW_PLAN query=%s depth=%d node=%s join=%s strategy=%s relation=%s cte=%s sort=%s elapsed_ms=%.3f rows=%.0f loops=%.0f temp_read=%d temp_written=%d",
			query, depth, plan.NodeType, plan.JoinType, plan.Strategy, plan.RelationName, plan.CTEName, plan.SortMethod,
			elapsed, plan.ActualRows, plan.ActualLoops, plan.TempReadBlocks, plan.TempWrittenBlocks,
		)
	}
	for _, child := range plan.Plans {
		logOrganizationUsageSlowPlan(t, query, child, depth+1)
	}
}
