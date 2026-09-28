//go:build integration

package repository

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestOrganizationUsageDepartmentIntegration_AllSurfacesUseTheSameScope(t *testing.T) {
	ctx := context.Background()
	prefix := organizationUsageIntegrationPrefix("departments")
	account := organizationUsageIntegrationAccount(t, prefix)
	grok := organizationUsageIntegrationAccount(t, prefix+"grok")
	_, err := integrationDB.ExecContext(ctx, `UPDATE accounts SET platform='grok' WHERE id=$1`, grok.ID)
	require.NoError(t, err)
	cleanupOrganizationUsageIntegrationData(t, prefix)
	admin, _ := organizationUsageIntegrationUser(t, prefix+"admin@example.com", service.StatusActive)
	leader, _ := organizationUsageIntegrationUser(t, prefix+"leader@xunyou.com", service.StatusActive)
	member, key := organizationUsageIntegrationUser(t, prefix+"member@xunyou.com", service.StatusActive)
	zero, _ := organizationUsageIntegrationUser(t, prefix+"zero@xunyou.com", service.StatusActive)
	outsider, outKey := organizationUsageIntegrationUser(t, prefix+"outsider@wsdashi.com", service.StatusActive)
	_, err = integrationDB.ExecContext(ctx, `UPDATE users SET role='admin' WHERE id=$1`, admin.ID)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `UPDATE users SET role='sub_admin' WHERE id=$1`, leader.ID)
	require.NoError(t, err)
	adminCtx := service.WithDepartmentActor(ctx, admin.ID)
	leaderCtx := service.WithDepartmentActor(ctx, leader.ID)
	departments := service.NewDepartmentService(NewDepartmentRepository(integrationDB))
	dept, err := departments.Save(adminCtx, 0, service.DepartmentSaveInput{Organization: service.OrganizationXunyou, Name: prefix})
	require.NoError(t, err)
	other, err := departments.Save(adminCtx, 0, service.DepartmentSaveInput{Organization: service.OrganizationWsdashi, Name: prefix})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, e := integrationDB.ExecContext(ctx, `DELETE FROM department_access_grants WHERE user_id=$1`, leader.ID)
		require.NoError(t, e)
		_, e = integrationDB.ExecContext(ctx, `UPDATE users SET department_id=NULL WHERE department_id=ANY($1)`, pq.Array([]int64{dept.ID, other.ID}))
		require.NoError(t, e)
		_, e = integrationDB.ExecContext(ctx, `DELETE FROM departments WHERE id=ANY($1)`, pq.Array([]int64{dept.ID, other.ID}))
		require.NoError(t, e)
		_, e = integrationDB.ExecContext(ctx, `DELETE FROM audit_logs WHERE actor_user_id=$1`, admin.ID)
		require.NoError(t, e)
	})
	_, err = departments.Assign(adminCtx, service.DepartmentAssignInput{DepartmentID: &dept.ID, Members: []service.DepartmentMemberChange{{UserID: member.ID}, {UserID: zero.ID}}})
	require.NoError(t, err)
	_, err = departments.Assign(adminCtx, service.DepartmentAssignInput{DepartmentID: &other.ID, Members: []service.DepartmentMemberChange{{UserID: outsider.ID}}})
	require.NoError(t, err)
	access, err := departments.GetAccess(adminCtx, leader.ID)
	require.NoError(t, err)
	_, err = departments.SetAccess(adminCtx, leader.ID, service.DepartmentAccessInput{DepartmentIDs: []int64{dept.ID}, Report: true, ExpectedVersion: access.Version})
	require.NoError(t, err)
	at := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	insertOrganizationUsageIntegrationLog(t, member.ID, key.ID, account.ID, 10, 0, 0, 0, 1, at)
	insertOrganizationUsageIntegrationLog(t, member.ID, key.ID, grok.ID, 20, 0, 0, 0, 2, at.Add(time.Hour))
	insertOrganizationUsageIntegrationLog(t, outsider.ID, outKey.ID, account.ID, 1000, 0, 0, 0, 100, at)

	repo := NewOrganizationUsageRepository(integrationDB)
	p := organizationUsageSummaryIntegrationParams(prefix, "2026-01-01", "2026-01-03")
	p.Organization = service.OrganizationXunyou
	p.Filters = service.OrganizationUsageDepartmentFilters{DepartmentID: strconv.FormatInt(dept.ID, 10)}
	summary, err := repo.Summary(leaderCtx, p)
	require.NoError(t, err)
	require.Equal(t, int64(2), summary.Overview.ActiveUsers)
	require.Equal(t, int64(1), summary.Overview.UsedUsers)
	require.Equal(t, int64(30), summary.Overview.TotalTokens)
	require.InDelta(t, 3, summary.Overview.ActualCost, 1e-8)
	require.Len(t, summary.Organizations, 1)
	require.Len(t, summary.Departments, 1)
	require.Len(t, summary.Platforms, 2)
	require.Equal(t, member.ID, summary.Champions.Day.UserID)
	require.NotEmpty(t, summary.ScopeVersion)
	// Only the selected report projection belongs to the version, not the whole catalog.
	adminSnapshot, err := repo.Summary(adminCtx, p)
	require.NoError(t, err)
	globalParams := p
	globalParams.Organization = service.OrganizationAll
	globalParams.Filters.DepartmentID = "all"
	globalSnapshot, err := repo.Summary(adminCtx, globalParams)
	require.NoError(t, err)
	organizationUsageIntegrationUser(t, prefix+"new@wsdashi.com", service.StatusActive)
	_, err = integrationDB.ExecContext(ctx, `UPDATE users SET username='unrelated profile edit' WHERE id=ANY($1)`, pq.Array([]int64{member.ID, outsider.ID}))
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `UPDATE departments SET sort_order=10,version=version+1,updated_at=NOW() WHERE id=ANY($1)`, pq.Array([]int64{dept.ID, other.ID}))
	require.NoError(t, err)
	selectedParams := p
	selectedParams.Filters.ScopeVersion = adminSnapshot.ScopeVersion
	stable, err := repo.Summary(adminCtx, selectedParams)
	require.NoError(t, err)
	require.Equal(t, adminSnapshot.ScopeVersion, stable.ScopeVersion)
	globalParams.Filters.ScopeVersion = globalSnapshot.ScopeVersion
	_, err = repo.Summary(adminCtx, globalParams)
	require.ErrorIs(t, err, service.ErrDepartmentScopeChanged, "new members legitimately change a global report")
	_, err = integrationDB.ExecContext(ctx, `UPDATE users SET email=$1 WHERE id=$2`, prefix+"renamed@xunyou.com", member.ID)
	require.NoError(t, err)
	_, err = repo.Summary(adminCtx, selectedParams)
	require.ErrorIs(t, err, service.ErrDepartmentScopeChanged, "email is part of the exported projection")
	_, err = integrationDB.ExecContext(ctx, `UPDATE users SET email=$1 WHERE id=$2`, member.Email, member.ID)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `UPDATE departments SET name=$1 WHERE id=$2`, prefix+" renamed", dept.ID)
	require.NoError(t, err)
	_, err = repo.Summary(adminCtx, selectedParams)
	require.ErrorIs(t, err, service.ErrDepartmentScopeChanged, "department labels must not mix across export pages")
	_, err = integrationDB.ExecContext(ctx, `UPDATE departments SET name=$1 WHERE id=$2`, dept.Name, dept.ID)
	require.NoError(t, err)
	for _, item := range summary.Items {
		require.NotEqual(t, outsider.ID, item.UserID)
		require.Equal(t, &dept.ID, item.DepartmentID)
	}

	periodParams := organizationUsagePeriodsIntegrationParams(prefix, "2026-01-01", "2026-01-03", "day")
	periodParams.Organization = p.Organization
	periodParams.Filters = p.Filters
	periodParams.Filters.ScopeVersion = summary.ScopeVersion
	periods, err := repo.Periods(leaderCtx, periodParams)
	require.NoError(t, err)
	require.Len(t, periods.Items, 1)
	require.Equal(t, int64(30), periods.Items[0].TotalTokens)
	require.Equal(t, &dept.ID, periods.Items[0].DepartmentID)
	trend, err := repo.Trend(leaderCtx, service.OrganizationUsageTrendRepositoryParams{StartTime: p.StartTime, EndTime: p.EndTime, StartDate: p.StartDate, EndDate: p.EndDate, DataThrough: p.EndDate, Organization: p.Organization, Q: p.Q, Granularity: "day", Filters: periodParams.Filters})
	require.NoError(t, err)
	require.Len(t, trend.Points, 3)
	var tokens int64
	for _, point := range trend.Points {
		tokens += point.TotalTokens
	}
	require.Equal(t, int64(30), tokens)
	// Composite routing reports the concrete account platform, even after soft deletion.
	var compositeID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO groups(name,platform) VALUES($1,'composite') RETURNING id`, prefix+"composite").Scan(&compositeID))
	t.Cleanup(func() {
		_, e := integrationDB.ExecContext(ctx, `UPDATE usage_logs SET group_id=NULL WHERE group_id=$1`, compositeID)
		require.NoError(t, e)
		_, e = integrationDB.ExecContext(ctx, `DELETE FROM groups WHERE id=$1`, compositeID)
		require.NoError(t, e)
	})
	_, err = integrationDB.ExecContext(ctx, `UPDATE usage_logs SET group_id=$1 WHERE user_id=$2 AND account_id=$3`, compositeID, member.ID, grok.ID)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `UPDATE accounts SET deleted_at=NOW() WHERE id=$1`, grok.ID)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `UPDATE groups SET deleted_at=NOW() WHERE id=$1`, compositeID)
	require.NoError(t, err)
	composite, err := repo.Summary(leaderCtx, p)
	require.NoError(t, err)
	require.Equal(t, int64(30), composite.Overview.TotalTokens)
	require.Equal(t, summary.Champions, composite.Champions)
	_, err = integrationDB.ExecContext(ctx, `UPDATE accounts SET platform='' WHERE id=$1`, account.ID)
	require.NoError(t, err)
	p.Filters.Platform = "unknown"
	unknown, err := repo.Summary(leaderCtx, p)
	require.NoError(t, err)
	require.Equal(t, int64(10), unknown.Overview.TotalTokens)
	require.Equal(t, "unknown", unknown.Platforms[0].Platform)
	future, err := repo.Trend(leaderCtx, service.OrganizationUsageTrendRepositoryParams{StartTime: p.StartTime, EndTime: p.EndTime, StartDate: p.StartDate, EndDate: p.EndDate, DataThrough: p.StartDate.AddDate(0, 0, -1), Organization: p.Organization, Q: p.Q, Granularity: "month", Filters: p.Filters})
	require.NoError(t, err)
	require.Empty(t, future.Points)
	require.NotEmpty(t, future.ScopeVersion)
	p.Filters.Platform = service.PlatformGrok
	filtered, err := repo.Summary(leaderCtx, p)
	require.NoError(t, err)
	require.Equal(t, int64(2), filtered.Overview.ActiveUsers)
	require.Equal(t, int64(1), filtered.Overview.UsedUsers)
	require.Equal(t, int64(20), filtered.Overview.TotalTokens)
	require.Len(t, filtered.Platforms, 1)
	p.Organization = service.OrganizationWsdashi
	p.Filters.DepartmentID = strconv.FormatInt(other.ID, 10)
	_, err = repo.Summary(leaderCtx, p)
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)
	p.Organization = service.OrganizationXunyou
	p.Filters.DepartmentID = strconv.FormatInt(dept.ID, 10)
	p.Filters.ScopeVersion = summary.ScopeVersion
	_, err = departments.Assign(adminCtx, service.DepartmentAssignInput{Members: []service.DepartmentMemberChange{{UserID: member.ID, ExpectedDepartmentID: &dept.ID, ExpectedVersion: 1}}})
	require.NoError(t, err)
	_, err = repo.Summary(leaderCtx, p)
	require.ErrorIs(t, err, service.ErrDepartmentScopeChanged)
	p.Filters.ScopeVersion = ""
	p.Filters.Platform = "all"
	after, err := repo.Summary(leaderCtx, p)
	require.NoError(t, err)
	require.Equal(t, int64(1), after.Overview.ActiveUsers)
	require.Zero(t, after.Overview.Requests)
	require.Nil(t, after.Champions.Day)
}
