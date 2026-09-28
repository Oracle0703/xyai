//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestDepartmentRepositoryIntegration_MembershipAndScope(t *testing.T) {
	ctx := context.Background()
	prefix := "department_" + uuid.NewString()
	userIDs := []int64{}
	departmentIDs := []int64{}
	t.Cleanup(func() {
		_, err := integrationDB.ExecContext(ctx, `DELETE FROM department_access_grants WHERE user_id=ANY($1)`, pq.Array(userIDs))
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(ctx, `UPDATE users SET department_id=NULL WHERE id=ANY($1)`, pq.Array(userIDs))
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(ctx, `DELETE FROM departments WHERE id=ANY($1)`, pq.Array(departmentIDs))
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(ctx, `DELETE FROM audit_logs WHERE actor_user_id=ANY($1) OR (action='department.email_organization_changed' AND extra->>'user_id' IN (SELECT unnest($1::bigint[])::text))`, pq.Array(userIDs))
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(ctx, `DELETE FROM users WHERE id=ANY($1)`, pq.Array(userIDs))
		require.NoError(t, err)
	})
	newUser := func(name, domain, role string, permissions []string) int64 {
		if permissions == nil {
			permissions = []string{}
		}
		raw, err := json.Marshal(permissions)
		require.NoError(t, err)
		var id int64
		err = integrationDB.QueryRowContext(ctx, `INSERT INTO users(email,password_hash,role,status,admin_permissions) VALUES($1,'test',$2,'active',$3) RETURNING id`, prefix+name+"@"+domain, role, string(raw)).Scan(&id)
		require.NoError(t, err)
		userIDs = append(userIDs, id)
		return id
	}
	adminID := newUser("admin", "example.com", service.RoleAdmin, nil)
	memberID := newUser("member", "xunyou.com", service.RoleUser, nil)
	otherID := newUser("other", "wsdashi.com", service.RoleUser, nil)
	leaderID := newUser("leader", "xunyou.com", service.RoleSubAdmin, []string{service.AdminPermissionOrganizationUsage})
	adminCtx := service.WithDepartmentActor(ctx, adminID)
	leaderCtx := service.WithDepartmentActor(ctx, leaderID)
	repo := NewDepartmentRepository(integrationDB)
	svc := service.NewDepartmentService(repo)
	create := func(org string) *service.Department {
		d, err := svc.Save(adminCtx, 0, service.DepartmentSaveInput{Organization: org, Name: prefix + " Engineering"})
		require.NoError(t, err)
		departmentIDs = append(departmentIDs, d.ID)
		return d
	}
	xunyou := create(service.OrganizationXunyou)
	wsdashi := create(service.OrganizationWsdashi)
	_, err := svc.Save(adminCtx, 0, service.DepartmentSaveInput{Organization: service.OrganizationXunyou, Name: "  " + prefix + " ENGINEERING  "})
	require.ErrorIs(t, err, service.ErrDepartmentDuplicate)
	_, err = svc.Save(leaderCtx, 0, service.DepartmentSaveInput{Organization: service.OrganizationXunyou, Name: "forbidden"})
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)

	_, err = svc.Assign(adminCtx, service.DepartmentAssignInput{DepartmentID: &xunyou.ID, Members: []service.DepartmentMemberChange{{UserID: memberID}, {UserID: otherID}}})
	require.ErrorIs(t, err, service.ErrDepartmentMemberOrganization)
	var assigned *int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT department_id FROM users WHERE id=$1`, memberID).Scan(&assigned))
	require.Nil(t, assigned, "cross-organization batch must roll back all members")
	var auditCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_logs WHERE actor_user_id=$1 AND action='department.assign_members'`, adminID).Scan(&auditCount))
	require.Zero(t, auditCount, "a rolled-back batch must not leave a success audit")
	count, err := svc.Assign(adminCtx, service.DepartmentAssignInput{DepartmentID: &xunyou.ID, Members: []service.DepartmentMemberChange{{UserID: memberID}}})
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_logs WHERE actor_user_id=$1 AND action='department.assign_members'`, adminID).Scan(&auditCount))
	require.Equal(t, 1, auditCount)
	count, err = svc.Assign(adminCtx, service.DepartmentAssignInput{DepartmentID: &wsdashi.ID, Members: []service.DepartmentMemberChange{{UserID: otherID}}})
	require.NoError(t, err)
	require.Equal(t, 1, count)

	scope, err := svc.Scope(leaderCtx, service.AdminPermissionOrganizationUsage)
	require.NoError(t, err)
	require.ErrorIs(t, scope.ValidateSelection("all", "all"), service.ErrDepartmentScopeDenied)
	access, err := svc.GetAccess(adminCtx, leaderID)
	require.NoError(t, err)
	access, err = svc.SetAccess(adminCtx, leaderID, service.DepartmentAccessInput{DepartmentIDs: []int64{xunyou.ID}, Report: true, ResetQuota: true, ExpectedVersion: access.Version})
	require.NoError(t, err)
	scope, err = svc.QueryScope(leaderCtx, service.AdminPermissionOrganizationUsage, service.DepartmentScopeQuery{})
	require.NoError(t, err)
	require.Len(t, scope.Departments, 1)
	require.Len(t, scope.Members, 1)
	require.Equal(t, memberID, scope.Members[0].ID)
	_, err = svc.QueryScope(leaderCtx, service.AdminPermissionOrganizationUsage, service.DepartmentScopeQuery{ExpectedVersion: "stale"})
	require.ErrorIs(t, err, service.ErrDepartmentScopeChanged)
	_, err = svc.QueryScope(leaderCtx, service.AdminPermissionOrganizationUsage, service.DepartmentScopeQuery{Organization: service.OrganizationWsdashi, ExpectedVersion: "stale"})
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied, "authorization precedes snapshot validation")
	selected, err := svc.QueryScope(leaderCtx, service.AdminPermissionOrganizationUsage, service.DepartmentScopeQuery{Q: "  " + prefix + "member  "})
	require.NoError(t, err)
	require.Len(t, selected.Members, 1, "SQL is the only member filtering implementation")
	require.ErrorIs(t, scope.ValidateSelection(service.OrganizationWsdashi, "all"), service.ErrDepartmentScopeDenied)
	access, err = svc.SetAccess(adminCtx, leaderID, service.DepartmentAccessInput{DepartmentIDs: []int64{xunyou.ID, wsdashi.ID}, Report: true, ResetQuota: true, ExpectedVersion: access.Version})
	require.NoError(t, err)
	both, err := svc.QueryScope(leaderCtx, service.AdminPermissionOrganizationUsage, service.DepartmentScopeQuery{})
	require.NoError(t, err)
	require.Len(t, both.Departments, 2)
	require.Len(t, both.Members, 2)
	require.Equal(t, "all", both.DefaultOrganization)
	require.Equal(t, "all", both.DefaultDepartment)
	access, err = svc.SetAccess(adminCtx, leaderID, service.DepartmentAccessInput{DepartmentIDs: []int64{xunyou.ID}, Report: true, ResetQuota: true, ExpectedVersion: access.Version})
	require.NoError(t, err)
	includeSubscriptions := false
	listed, paginationResult, err := NewUserRepository(integrationEntClient, integrationDB).ListWithFilters(adminCtx, pagination.PaginationParams{Page: 1, PageSize: 20}, service.UserListFilters{Organization: service.OrganizationXunyou, DepartmentID: strconv.FormatInt(xunyou.ID, 10), Search: prefix, IncludeSubscriptions: &includeSubscriptions})
	require.NoError(t, err)
	require.Equal(t, int64(1), paginationResult.Total)
	require.Equal(t, memberID, listed[0].ID)
	require.Equal(t, &xunyou.ID, listed[0].DepartmentID)
	require.Equal(t, int64(1), listed[0].DepartmentVersion)
	page, err := svc.List(adminCtx, service.DepartmentListFilter{Organization: service.OrganizationXunyou, Q: prefix, Page: 1, PageSize: 1})
	require.NoError(t, err)
	require.Equal(t, int64(1), page.Total)
	require.Equal(t, int64(1), page.Items[0].MemberCount)
	require.Equal(t, leaderID, page.Items[0].Managers[0].ID)
	memberPage, err := svc.Members(adminCtx, service.DepartmentListFilter{DepartmentID: strconv.FormatInt(xunyou.ID, 10), Page: 1, PageSize: 1})
	require.NoError(t, err)
	require.Equal(t, int64(1), memberPage.Total)
	require.Equal(t, memberID, memberPage.Items[0].ID)
	// Inactive departments retain members and existing grants but reject additions.
	inactive, err := svc.Save(adminCtx, xunyou.ID, service.DepartmentSaveInput{Organization: service.OrganizationXunyou, Name: xunyou.Name, Status: "inactive", ExpectedVersion: &xunyou.Version})
	require.NoError(t, err)
	unchanged, err := svc.Assign(adminCtx, service.DepartmentAssignInput{DepartmentID: &xunyou.ID, Members: []service.DepartmentMemberChange{{UserID: memberID, ExpectedDepartmentID: &xunyou.ID, ExpectedVersion: 1}}})
	require.NoError(t, err)
	require.Zero(t, unchanged)
	_, err = svc.Assign(adminCtx, service.DepartmentAssignInput{DepartmentID: &xunyou.ID, Members: []service.DepartmentMemberChange{{UserID: leaderID}}})
	require.ErrorIs(t, err, service.ErrDepartmentInactive)
	_, err = svc.Save(adminCtx, xunyou.ID, service.DepartmentSaveInput{Organization: service.OrganizationXunyou, Name: xunyou.Name, Status: "active", ExpectedVersion: &inactive.Version})
	require.NoError(t, err)
	scope, err = svc.QueryScope(leaderCtx, service.AdminPermissionOrganizationUsage, service.DepartmentScopeQuery{})
	require.NoError(t, err)
	oldVersion := scope.Version
	_, err = svc.Assign(adminCtx, service.DepartmentAssignInput{Members: []service.DepartmentMemberChange{{UserID: memberID, ExpectedDepartmentID: &xunyou.ID, ExpectedVersion: 0}}})
	require.ErrorIs(t, err, service.ErrDepartmentConflict)
	_, err = svc.Assign(adminCtx, service.DepartmentAssignInput{Members: []service.DepartmentMemberChange{{UserID: memberID, ExpectedDepartmentID: &xunyou.ID, ExpectedVersion: 1}}})
	require.NoError(t, err)
	scope, err = svc.QueryScope(leaderCtx, service.AdminPermissionOrganizationUsage, service.DepartmentScopeQuery{})
	require.NoError(t, err)
	require.Empty(t, scope.Members)
	require.NotEqual(t, oldVersion, scope.Version)

	_, err = svc.Assign(adminCtx, service.DepartmentAssignInput{DepartmentID: &xunyou.ID, Members: []service.DepartmentMemberChange{{UserID: memberID, ExpectedVersion: 2}}})
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `UPDATE users SET email=$1 WHERE id=$2`, prefix+"member@wsdashi.com", memberID)
	require.NoError(t, err)
	var version int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT department_id,department_version FROM users WHERE id=$1`, memberID).Scan(&assigned, &version))
	require.Nil(t, assigned)
	require.Equal(t, int64(4), version)
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_logs WHERE action='department.email_organization_changed' AND auth_method='database_guard' AND extra->>'user_id'=$1 AND extra->>'before_department_id'=$2`, strconv.FormatInt(memberID, 10), strconv.FormatInt(xunyou.ID, 10)).Scan(&auditCount))
	require.Equal(t, 1, auditCount, "cross-organization email changes must retain an atomic audit")
	_, err = svc.SetAccess(adminCtx, leaderID, service.DepartmentAccessInput{DepartmentIDs: []int64{}, Report: false, ResetQuota: true, ExpectedVersion: access.Version})
	require.NoError(t, err)
	scope, err = svc.Scope(leaderCtx, service.AdminPermissionDepartmentSubscriptions)
	require.NoError(t, err)
	require.False(t, scope.Unrestricted)
	require.Empty(t, scope.Members)
	require.ErrorIs(t, scope.ValidateSelection("all", "all"), service.ErrDepartmentScopeDenied)
	_, err = svc.Scope(leaderCtx, service.AdminPermissionOrganizationUsage)
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)
}

func TestDepartmentRepositoryIntegration_OnlyNewGrantsLockDepartments(t *testing.T) {
	ctx := context.Background()
	prefix := organizationUsageIntegrationPrefix("grant_locks")
	cleanupOrganizationUsageIntegrationData(t, prefix)
	admin, _ := organizationUsageIntegrationUser(t, prefix+"admin@example.com", service.StatusActive)
	leader, _ := organizationUsageIntegrationUser(t, prefix+"leader@example.com", service.StatusActive)
	_, err := integrationDB.Exec(`UPDATE users SET role=CASE WHEN id=$1 THEN 'admin' ELSE 'sub_admin' END WHERE id=ANY($2)`, admin.ID, pq.Array([]int64{admin.ID, leader.ID}))
	require.NoError(t, err)
	adminCtx := service.WithDepartmentActor(ctx, admin.ID)
	setup := service.NewDepartmentService(NewDepartmentRepository(integrationDB))
	ids := []int64{}
	t.Cleanup(func() {
		_, e := integrationDB.Exec(`DELETE FROM department_access_grants WHERE user_id=$1`, leader.ID)
		require.NoError(t, e)
		_, e = integrationDB.Exec(`DELETE FROM departments WHERE id=ANY($1)`, pq.Array(ids))
		require.NoError(t, e)
		_, e = integrationDB.Exec(`DELETE FROM audit_logs WHERE actor_user_id=$1`, admin.ID)
		require.NoError(t, e)
	})
	for _, name := range []string{"retained", "new-one", "new-two"} {
		d, e := setup.Save(adminCtx, 0, service.DepartmentSaveInput{Organization: service.OrganizationXunyou, Name: prefix + name})
		require.NoError(t, e)
		ids = append(ids, d.ID)
	}
	access, err := setup.GetAccess(adminCtx, leader.ID)
	require.NoError(t, err)
	db, counter := newDepartmentCountingDB(t)
	svc := service.NewDepartmentService(NewDepartmentRepository(db))
	counter.calls.Store(0)
	access, err = svc.SetAccess(adminCtx, leader.ID, service.DepartmentAccessInput{DepartmentIDs: ids[:1], Report: true, ExpectedVersion: access.Version})
	require.NoError(t, err)
	oneAdditionCalls := counter.calls.Load()
	var createdAt time.Time
	require.NoError(t, integrationDB.QueryRow(`SELECT created_at FROM department_access_grants WHERE user_id=$1 AND department_id=$2`, leader.ID, ids[0]).Scan(&createdAt))
	_, err = integrationDB.Exec(`UPDATE departments SET status='inactive' WHERE id=$1`, ids[0])
	require.NoError(t, err)

	blocker, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = blocker.Rollback() }()
	_, err = blocker.Exec(`SELECT id FROM departments WHERE id=$1 FOR UPDATE`, ids[0])
	require.NoError(t, err)
	writeCtx, cancel := context.WithTimeout(adminCtx, 3*time.Second)
	defer cancel()
	counter.calls.Store(0)
	access, err = svc.SetAccess(writeCtx, leader.ID, service.DepartmentAccessInput{DepartmentIDs: ids, Report: true, ExpectedVersion: access.Version})
	// Always release the blocker before assertions and cleanup, including failures.
	require.NoError(t, blocker.Rollback())
	require.NoError(t, err, "retaining an inactive grant must not lock its department again")
	require.Equal(t, oneAdditionCalls, counter.calls.Load(), "adding multiple grants uses one status query and one INSERT")
	var after time.Time
	require.NoError(t, integrationDB.QueryRow(`SELECT created_at FROM department_access_grants WHERE user_id=$1 AND department_id=$2`, leader.ID, ids[0]).Scan(&after))
	require.Equal(t, createdAt, after, "retained grants keep their creation history")

	access, err = svc.SetAccess(adminCtx, leader.ID, service.DepartmentAccessInput{DepartmentIDs: ids[1:], Report: true, ExpectedVersion: access.Version})
	require.NoError(t, err)
	_, err = svc.SetAccess(adminCtx, leader.ID, service.DepartmentAccessInput{DepartmentIDs: ids, Report: true, ExpectedVersion: access.Version})
	require.ErrorIs(t, err, service.ErrDepartmentInactive, "re-adding an inactive grant must still fail")
	_, err = svc.SetAccess(adminCtx, leader.ID, service.DepartmentAccessInput{DepartmentIDs: append(append([]int64{}, ids[1:]...), 9223372036854775807), Report: true, ExpectedVersion: access.Version})
	require.ErrorIs(t, err, service.ErrDepartmentInvalid)
	current, err := setup.GetAccess(adminCtx, leader.ID)
	require.NoError(t, err)
	require.Equal(t, access, current, "rejected additions must not partially update grants or permissions")
}
