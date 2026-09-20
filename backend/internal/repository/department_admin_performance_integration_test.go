//go:build integration

package repository

import (
	"context"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// Kept compatible with a9c53a9f6 so exactly this fixture can measure both trees.
func TestDepartmentAdminPerformanceIntegration(t *testing.T) {
	if os.Getenv("DEPARTMENT_USAGE_RUN_PERFORMANCE") != "1" {
		t.Skip("set DEPARTMENT_USAGE_RUN_PERFORMANCE=1")
	}
	ctx := context.Background()
	prefix := organizationUsageIntegrationPrefix("admin_perf")
	cleanupOrganizationUsageIntegrationData(t, prefix)
	admin, _ := organizationUsageIntegrationUser(t, prefix+"admin@example.com", service.StatusActive)
	_, err := integrationDB.Exec(`UPDATE users SET role='admin' WHERE id=$1`, admin.ID)
	require.NoError(t, err)
	departmentIDs := []int64{}
	groupID := int64(0)
	t.Cleanup(func() {
		_, e := integrationDB.Exec(`DELETE FROM user_subscriptions WHERE group_id=$1`, groupID)
		require.NoError(t, e)
		_, e = integrationDB.Exec(`DELETE FROM groups WHERE id=$1`, groupID)
		require.NoError(t, e)
		_, e = integrationDB.Exec(`UPDATE users SET department_id=NULL WHERE department_id=ANY($1)`, pq.Array(departmentIDs))
		require.NoError(t, e)
		_, e = integrationDB.Exec(`DELETE FROM departments WHERE id=ANY($1)`, pq.Array(departmentIDs))
		require.NoError(t, e)
		_, e = integrationDB.Exec(`DELETE FROM audit_logs WHERE actor_user_id=$1`, admin.ID)
		require.NoError(t, e)
	})
	rows, err := integrationDB.Query(`INSERT INTO departments(organization_key,name) SELECT 'xunyou',$1||n::text FROM generate_series(1,40)n RETURNING id`, prefix)
	require.NoError(t, err)
	for rows.Next() {
		var id int64
		require.NoError(t, rows.Scan(&id))
		departmentIDs = append(departmentIDs, id)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	_, err = integrationDB.Exec(`INSERT INTO users(email,password_hash,role,status,department_id) SELECT $1||n::text||'@xunyou.com','test','user','active',($2::bigint[])[((n-1)%40)+1] FROM generate_series(1,600)n`, prefix, pq.Array(departmentIDs))
	require.NoError(t, err)
	require.NoError(t, integrationDB.QueryRow(`INSERT INTO groups(name,platform,subscription_type,daily_limit_usd) VALUES($1,'openai','subscription',20) RETURNING id`, prefix).Scan(&groupID))
	_, err = integrationDB.Exec(`INSERT INTO user_subscriptions(user_id,group_id,starts_at,expires_at,status,daily_usage_usd) SELECT id,$1,NOW()-INTERVAL '1 day',NOW()+INTERVAL '30 days','active',3 FROM users WHERE email LIKE $2 AND role='user'`, groupID, prefix+"%")
	require.NoError(t, err)
	var subscriptionID int64
	require.NoError(t, integrationDB.QueryRow(`SELECT MIN(id) FROM user_subscriptions WHERE group_id=$1`, groupID).Scan(&subscriptionID))
	for _, table := range []string{"users", "departments", "user_subscriptions", "groups"} {
		_, err = integrationDB.Exec("ANALYZE " + table)
		require.NoError(t, err)
	}
	adminCtx := service.WithDepartmentActor(ctx, admin.ID)
	departments := service.NewDepartmentService(NewDepartmentRepository(integrationDB))
	subs := NewUserSubscriptionRepository(integrationEntClient)
	measure := func(name string, action func() error) {
		t.Helper()
		require.NoError(t, action())
		samples := make([]time.Duration, 50)
		for i := range samples {
			start := time.Now()
			require.NoError(t, action())
			samples[i] = time.Since(start)
		}
		sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
		t.Logf("ADMIN_PERF name=%s members=600 departments=40 subscriptions=600 samples=50 median_us=%d p95_us=%d", name, ((samples[24] + samples[25]) / 2).Microseconds(), samples[47].Microseconds())
	}
	measure("departments", func() error {
		page, e := departments.List(adminCtx, service.DepartmentListFilter{Organization: service.OrganizationXunyou, Q: prefix, Page: 1, PageSize: 20})
		if e == nil && (page.Total != 40 || len(page.Items) != 20) {
			return fmt.Errorf("incorrect department page")
		}
		return e
	})
	measure("members", func() error {
		page, e := departments.Members(adminCtx, service.DepartmentListFilter{Organization: service.OrganizationXunyou, Q: prefix, Page: 1, PageSize: 20})
		if e == nil && (page.Total != 600 || len(page.Items) != 20) {
			return fmt.Errorf("incorrect member page")
		}
		return e
	})
	measure("subscription_list", func() error {
		items, page, e := subs.ListAdmin(adminCtx, pagination.PaginationParams{Page: 1, PageSize: 20}, service.SubscriptionAdminFilter{GroupID: &groupID, Status: "active", SortBy: "created_at", SortOrder: "desc"}, time.Now())
		if e == nil && (page.Total != 600 || len(items) != 20) {
			return fmt.Errorf("incorrect subscription page")
		}
		return e
	})
	measure("subscription_detail", func() error { _, e := subs.GetByID(adminCtx, subscriptionID); return e })
	measure("subscription_reset", func() error {
		return subs.ResetUsageWindows(adminCtx, subscriptionID, true, false, false, time.Now(), time.Now())
	})
}
