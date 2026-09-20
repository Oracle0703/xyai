//go:build integration

package repository

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// Uses the same repeatable fixture as the original SQL baseline, but exercises
// authenticated scopes, complete summaries and export queries as shipped.
func TestDepartmentUsagePerformanceIntegration(t *testing.T) {
	if os.Getenv("DEPARTMENT_USAGE_RUN_PERFORMANCE") != "1" {
		t.Skip("set DEPARTMENT_USAGE_RUN_PERFORMANCE=1")
	}
	ctx := context.Background()
	cleanupOrganizationUsageExplainData(t, ctx)
	seedOrganizationUsageExplainData(t, ctx)
	var adminID, leaderID, departmentID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO users(email,password_hash,role,status,admin_permissions) VALUES('department-perf-admin@bench.invalid','test','admin','active','[]') RETURNING id`).Scan(&adminID))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO users(email,password_hash,role,status,admin_permissions) VALUES('department-perf-leader@bench.invalid','test','sub_admin','active','["admin.organization_usage"]') RETURNING id`).Scan(&leaderID))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO departments(organization_key,name) VALUES('xunyou','department-performance-fixture') RETURNING id`).Scan(&departmentID))
	_, err := integrationDB.ExecContext(ctx, `UPDATE users SET department_id=$1 WHERE email LIKE $2 AND lower(split_part(email,'@',2))='xunyou.com'`, departmentID, organizationUsageExplainPrefix+"%")
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `INSERT INTO department_access_grants(user_id,department_id,created_by) VALUES($1,$2,$3)`, leaderID, departmentID, adminID)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, e := integrationDB.ExecContext(ctx, `DELETE FROM department_access_grants WHERE user_id=$1`, leaderID)
		require.NoError(t, e)
		cleanupOrganizationUsageExplainData(t, ctx)
		_, e = integrationDB.ExecContext(ctx, `DELETE FROM departments WHERE id=$1`, departmentID)
		require.NoError(t, e)
		_, e = integrationDB.ExecContext(ctx, `DELETE FROM users WHERE id IN ($1,$2)`, leaderID, adminID)
		require.NoError(t, e)
	})
	for _, table := range []string{"users", "usage_logs", "departments", "department_access_grants"} {
		_, err = integrationDB.ExecContext(ctx, "ANALYZE "+table)
		require.NoError(t, err)
	}
	adminCtx := service.WithDepartmentActor(ctx, adminID)
	leaderCtx := service.WithDepartmentActor(ctx, leaderID)
	repo := NewOrganizationUsageRepository(integrationDB)
	end := time.Date(2026, 6, 30, 0, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*3600))
	for _, days := range []int{30, 90, 366} {
		start := end.AddDate(0, 0, -days+1)
		for _, mode := range []string{"all", "organization", "department", "platform"} {
			t.Run(fmt.Sprintf("%d/%s", days, mode), func(t *testing.T) {
				p := organizationUsageSummaryIntegrationParams(organizationUsageExplainPrefix, start.Format("2006-01-02"), end.Format("2006-01-02"))
				p.PageSize = 20
				queryCtx := adminCtx
				if mode != "all" {
					p.Organization = "xunyou"
				}
				if mode == "department" || mode == "platform" {
					queryCtx = leaderCtx
					p.Filters.DepartmentID = strconv.FormatInt(departmentID, 10)
				}
				if mode == "platform" {
					p.Filters.Platform = "anthropic"
				}
				_, err := repo.Summary(queryCtx, p)
				require.NoError(t, err)
				samples := make([]time.Duration, 0, 10)
				for i := 0; i < 10; i++ {
					started := time.Now()
					result, e := repo.Summary(queryCtx, p)
					require.NoError(t, e)
					samples = append(samples, time.Since(started))
					expected := int64(200)
					if mode == "all" {
						expected = 600
					}
					require.Equal(t, expected, result.Total)
				}
				sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
				p95 := samples[len(samples)-1]
				t.Logf("DEPARTMENT_PERF days=%d mode=%s samples=10 median_ms=%d p95_ms=%d", days, mode, samples[4].Milliseconds(), p95.Milliseconds())
				require.Less(t, p95, 3*time.Second, "interactive summary p95 target")
				// Record a real scoped query plan and the last exported page shape.
				concrete := repo.(*organizationUsageRepository)
				q, tx, _, e := concrete.beginScoped(queryCtx, p.Organization, p.Q, p.Filters)
				require.NoError(t, e)
				query, args := q.scopedQuery(organizationUsageSummaryItemsSQL("total_tokens DESC, user_id ASC", true), []any{p.StartTime, p.EndTime, organizationUsageSearchPattern(p.Q), p.Organization, p.StartDate.Format("2006-01-02"), p.EndDate.Format("2006-01-02"), 500, 0})
				var raw []byte
				require.NoError(t, tx.QueryRowContext(queryCtx, "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) "+query, args...).Scan(&raw))
				require.NoError(t, tx.Rollback())
				t.Logf("DEPARTMENT_PLAN days=%d mode=%s %s", days, mode, raw)
				if mode == "department" {
					for _, granularity := range []string{"day", "week", "month"} {
						pp := organizationUsagePeriodsIntegrationParams(organizationUsageExplainPrefix, p.StartDate.Format("2006-01-02"), p.EndDate.Format("2006-01-02"), granularity)
						pp.Organization = p.Organization
						pp.Filters = p.Filters
						pp.PageSize = 500
						started := time.Now()
						periods, e := repo.Periods(queryCtx, pp)
						require.NoError(t, e)
						if periods.Total > 500 {
							pp.Page = int((periods.Total + 499) / 500)
							_, e = repo.Periods(queryCtx, pp)
							require.NoError(t, e)
						}
						elapsed := time.Since(started)
						require.Less(t, elapsed, 6*time.Second, "first and last export pages should each meet the interactive query budget")
						t.Logf("DEPARTMENT_EXPORT_QUERY days=%d granularity=%s first_and_last_ms=%d total_rows=%d", days, granularity, elapsed.Milliseconds(), periods.Total)
					}
				}
			})
		}
	}
}
