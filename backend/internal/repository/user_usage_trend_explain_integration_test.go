//go:build integration

package repository

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestSelectedUserUsageTrendExplainAnalyze(t *testing.T) {
	if os.Getenv("USER_USAGE_TREND_RUN_EXPLAIN") != "1" {
		t.Skip("set USER_USAGE_TREND_RUN_EXPLAIN=1 to analyze selected-user trend")
	}

	ctx := context.Background()
	cleanupOrganizationUsageExplainData(t, ctx)
	seedOrganizationUsageExplainData(t, ctx)
	t.Cleanup(func() { cleanupOrganizationUsageExplainData(t, context.Background()) })

	_, err := integrationDB.ExecContext(ctx, `ANALYZE users`)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `ANALYZE usage_logs`)
	require.NoError(t, err)

	rows, err := integrationDB.QueryContext(ctx, `
		SELECT id
		FROM users
		WHERE email LIKE $1
		ORDER BY id
		LIMIT 5`, organizationUsageExplainPrefix+"%")
	require.NoError(t, err)
	defer rows.Close()

	userIDs := make([]int64, 0, 5)
	for rows.Next() {
		var id int64
		require.NoError(t, rows.Scan(&id))
		userIDs = append(userIDs, id)
	}
	require.NoError(t, rows.Err())
	require.Len(t, userIDs, 5)

	loc := time.FixedZone("Asia/Shanghai", 8*60*60)
	dayEnd := time.Date(2026, 7, 1, 0, 0, 0, 0, loc)
	tests := []struct {
		name        string
		granularity string
		users       []int64
		start       time.Time
		end         time.Time
	}{
		{"day-1-user", "day", userIDs[:1], dayEnd.AddDate(0, 0, -90), dayEnd},
		{"day-5-users", "day", userIDs, dayEnd.AddDate(0, 0, -90), dayEnd},
		{"hour-1-user", "hour", userIDs[:1], dayEnd.AddDate(0, 0, -1), dayEnd},
		{"hour-5-users", "hour", userIDs, dayEnd.AddDate(0, 0, -1), dayEnd},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := selectedUserUsageTrendQuery(tt.granularity)
			args := []any{pq.Array(tt.users), tt.start.UTC(), tt.end.UTC()}
			// Warm PostgreSQL and filesystem caches before recording the plan.
			explainOrganizationUsageQuery(t, ctx, query, args...)
			plan := explainOrganizationUsageQuery(t, ctx, query, args...)
			require.True(t, selectedTrendPlanUsesUserCreatedIndex(plan.Plan), "selected-user trend must use idx_usage_logs_user_created")
			t.Logf(
				"USER_TREND_EXPLAIN case=%s planning_ms=%.3f execution_ms=%.3f rows=%.0f shared_hit=%d shared_read=%d temp_read=%d temp_written=%d",
				tt.name,
				plan.PlanningTime,
				plan.ExecutionTime,
				plan.Plan.ActualRows,
				plan.Plan.SharedHitBlocks,
				plan.Plan.SharedReadBlocks,
				plan.Plan.TempReadBlocks,
				plan.Plan.TempWrittenBlocks,
			)
		})
	}
}

func selectedTrendPlanUsesUserCreatedIndex(plan organizationUsageExplainPlan) bool {
	if strings.Contains(plan.NodeType, "Index") && plan.IndexName == "idx_usage_logs_user_created" {
		return true
	}
	for _, child := range plan.Plans {
		if selectedTrendPlanUsesUserCreatedIndex(child) {
			return true
		}
	}
	return false
}
