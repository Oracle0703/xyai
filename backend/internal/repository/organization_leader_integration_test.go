//go:build integration

package repository

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// Acceptance tests for docs/features/organization-leader-design-cn.md §11.

type organizationLeaderSetup struct {
	f        *departmentMemberFixture
	svc      *service.DepartmentService
	adminID  int64
	adminCtx context.Context
}

func setupOrganizationLeaders(t *testing.T) *organizationLeaderSetup {
	t.Helper()
	f := newDepartmentMemberFixture(t)
	// Registered after the fixture so it runs first: users cannot be deleted
	// while organization grants reference them.
	t.Cleanup(func() {
		_, err := integrationDB.ExecContext(f.ctx, `DELETE FROM organization_access_grants WHERE user_id=ANY($1) OR created_by=ANY($1)`, pq.Array(f.userIDs))
		require.NoError(t, err)
	})
	s := &organizationLeaderSetup{f: f, svc: service.NewDepartmentService(NewDepartmentRepository(integrationDB), nil)}
	s.adminID = f.user("admin", "example.com", service.RoleAdmin, nil)
	s.adminCtx = service.WithDepartmentActor(f.ctx, s.adminID)
	return s
}

func (s *organizationLeaderSetup) access(userID int64) *service.DepartmentAccess {
	s.f.t.Helper()
	access, err := s.svc.GetAccess(s.adminCtx, userID)
	require.NoError(s.f.t, err)
	return access
}

func (s *organizationLeaderSetup) organizationLeader(name string, organizations ...string) (int64, context.Context) {
	s.f.t.Helper()
	id := s.f.user(name, "xunyou.com", service.RoleSubAdmin, nil)
	_, err := s.svc.SetOrganizationAccess(s.adminCtx, id, service.OrganizationAccessInput{Organizations: &organizations, ExpectedVersion: s.access(id).Version})
	require.NoError(s.f.t, err)
	return id, service.WithDepartmentActor(s.f.ctx, id)
}

func (s *organizationLeaderSetup) departmentLeader(name, domain string, departmentIDs []int64, report, reset, manage bool) (int64, context.Context) {
	s.f.t.Helper()
	id := s.f.user(name, domain, service.RoleSubAdmin, nil)
	_, err := s.svc.SetAccess(s.adminCtx, id, service.DepartmentAccessInput{DepartmentIDs: departmentIDs, Report: report, ResetQuota: reset, ManageMembers: &manage, ExpectedVersion: s.access(id).Version})
	require.NoError(s.f.t, err)
	return id, service.WithDepartmentActor(s.f.ctx, id)
}

func (s *organizationLeaderSetup) audits(action string, actorID int64) int {
	s.f.t.Helper()
	var count int
	require.NoError(s.f.t, integrationDB.QueryRowContext(s.f.ctx, `SELECT COUNT(*) FROM audit_logs WHERE action=$1 AND actor_user_id=$2`, action, actorID).Scan(&count))
	return count
}

// expect builds the membership CAS of a member, confirming "unassigned" with an explicit null.
func (s *organizationLeaderSetup) expect(userID int64) service.ManagedMemberUpdateInput {
	s.f.t.Helper()
	var department *int64
	var version int64
	require.NoError(s.f.t, integrationDB.QueryRowContext(s.f.ctx, `SELECT department_id,department_version FROM users WHERE id=$1`, userID).Scan(&department, &version))
	return service.ManagedMemberUpdateInput{ExpectedDepartmentID: department, ExpectedUnassigned: department == nil, ExpectedDepartmentVersion: &version}
}

func (s *organizationLeaderSetup) change(userID int64) service.DepartmentMemberChange {
	in := s.expect(userID)
	return service.DepartmentMemberChange{UserID: userID, ExpectedDepartmentID: in.ExpectedDepartmentID, ExpectedVersion: *in.ExpectedDepartmentVersion}
}

func organizationLeaderInput(ids []int64, report, reset, manage bool, version string) service.OrganizationLeaderInput {
	return service.OrganizationLeaderInput{DepartmentIDs: &ids, Report: &report, ResetQuota: &reset, ManageMembers: &manage, ExpectedVersion: &version}
}

func TestOrganizationLeaderIntegration_GrantLifecycle(t *testing.T) {
	s := setupOrganizationLeaders(t)
	f := s.f
	speedDept := f.department(service.OrganizationWsdashi)
	xunyouDept := f.department(service.OrganizationXunyou)
	leaderID := f.user("leader", "xunyou.com", service.RoleSubAdmin, nil)
	leaderCtx := service.WithDepartmentActor(f.ctx, leaderID)
	access, err := s.svc.SetAccess(s.adminCtx, leaderID, service.DepartmentAccessInput{DepartmentIDs: []int64{speedDept}, Report: true, ExpectedVersion: s.access(leaderID).Version})
	require.NoError(t, err)
	require.Equal(t, service.HashDepartmentScope([]any{leaderID, service.RoleSubAdmin, []int64{speedDept}, []string{service.AdminPermissionOrganizationUsage}}), access.Version,
		"accounts without organization grants keep the pre-existing version input")

	xunyou := []string{service.OrganizationXunyou}
	_, err = s.svc.SetOrganizationAccess(s.adminCtx, leaderID, service.OrganizationAccessInput{ExpectedVersion: access.Version})
	require.ErrorIs(t, err, service.ErrDepartmentInvalid, "an omitted or null organizations field never revokes (C3)")
	_, err = s.svc.SetOrganizationAccess(s.adminCtx, leaderID, service.OrganizationAccessInput{Organizations: &[]string{service.OrganizationOther}, ExpectedVersion: access.Version})
	require.ErrorIs(t, err, service.ErrDepartmentInvalid, "D1: the other organization has no organization leader")
	_, err = s.svc.SetOrganizationAccess(s.adminCtx, leaderID, service.OrganizationAccessInput{Organizations: &xunyou, ExpectedVersion: access.Version})
	require.ErrorIs(t, err, service.ErrOrganizationDepartmentGrantsConfirmation)
	require.Equal(t, access.Version, s.access(leaderID).Version, "an unconfirmed conversion writes nothing (D8)")
	_, err = s.svc.SetOrganizationAccess(s.adminCtx, leaderID, service.OrganizationAccessInput{Organizations: &xunyou, ExpectedVersion: access.Version, ReplaceDepartmentGrants: true})
	require.NoError(t, err)
	access = s.access(leaderID)
	require.Empty(t, access.DepartmentIDs)
	require.Equal(t, xunyou, access.Organizations)
	require.ElementsMatch(t, []string{service.AdminPermissionOrganizationManage, service.AdminPermissionOrganizationUsage,
		service.AdminPermissionDepartmentSubscriptions, service.AdminPermissionDepartmentMembers}, access.Permissions)
	require.Equal(t, 1, s.audits("organization.grant_scope", s.adminID))

	// R1: the former Wsdashi report grant gives the organization leader nothing.
	speed := strconv.FormatInt(speedDept, 10)
	_, err = s.svc.ManagedMembers(leaderCtx, service.DepartmentListFilter{DepartmentID: speed})
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)
	_, err = s.svc.Members(leaderCtx, service.DepartmentListFilter{DepartmentID: speed})
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)
	zero := int64(0)
	_, err = s.svc.Save(leaderCtx, speedDept, service.DepartmentSaveInput{Organization: service.OrganizationXunyou, Name: f.prefix + "x", ExpectedVersion: &zero})
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied, "a foreign department is denied, not reported as stale")
	_, err = s.svc.Save(leaderCtx, speedDept, service.DepartmentSaveInput{Organization: service.OrganizationWsdashi, Name: f.prefix + "x", ExpectedVersion: &zero})
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)
	member := f.user("member", "wsdashi.com", service.RoleUser, nil)
	_, err = s.svc.Assign(leaderCtx, service.DepartmentAssignInput{DepartmentID: &speedDept, Members: []service.DepartmentMemberChange{s.change(member)}})
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)
	_, err = s.svc.QueryScope(leaderCtx, service.AdminPermissionOrganizationUsage, service.DepartmentScopeQuery{Organization: service.OrganizationWsdashi})
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)
	_, err = s.svc.SetAccess(s.adminCtx, leaderID, service.DepartmentAccessInput{DepartmentIDs: []int64{xunyouDept}, Report: true, ExpectedVersion: access.Version})
	require.ErrorIs(t, err, service.ErrOrganizationScopeEndpointRequired)

	// R2: generic edits can neither drop nor add organization management, while
	// a three-capability department leader without an organization grant is legal.
	repo, ok := NewUserRepository(integrationEntClient, integrationDB).(*userRepository)
	require.True(t, ok)
	editor, err := repo.GetByIDWithAdminAccess(s.adminCtx, leaderID)
	require.NoError(t, err)
	editor.AdminPermissions = append([]string{}, service.OrganizationLeaderDepartmentPermissions...)
	err = repo.Update(s.adminCtx, editor, service.UserUpdateFields{AdminPermissions: true, ExpectedAdminAccessVersion: editor.AdminAccessVersion})
	require.ErrorIs(t, err, service.ErrOrganizationScopeEndpointRequired)
	plain := f.user("plain", "xunyou.com", service.RoleSubAdmin, []string{service.AdminPermissionOrganizationUsage})
	plainEditor, err := repo.GetByIDWithAdminAccess(s.adminCtx, plain)
	require.NoError(t, err)
	plainEditor.AdminPermissions = append([]string{service.AdminPermissionOrganizationManage}, service.OrganizationLeaderDepartmentPermissions...)
	err = repo.Update(s.adminCtx, plainEditor, service.UserUpdateFields{AdminPermissions: true, ExpectedAdminAccessVersion: plainEditor.AdminAccessVersion})
	require.ErrorIs(t, err, service.ErrOrganizationScopeEndpointRequired)
	plainEditor.AdminPermissions = append([]string{}, service.OrganizationLeaderDepartmentPermissions...)
	require.NoError(t, repo.Update(s.adminCtx, plainEditor, service.UserUpdateFields{AdminPermissions: true, ExpectedAdminAccessVersion: plainEditor.AdminAccessVersion}))

	// I3 violations (only possible through manual edits) fail closed.
	f.mustExec(`INSERT INTO department_access_grants(user_id,department_id,created_by) VALUES($1,$2,$3)`, leaderID, xunyouDept, s.adminID)
	_, err = s.svc.OrganizationScope(leaderCtx)
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)
	_, err = s.svc.QueryScope(leaderCtx, service.AdminPermissionOrganizationUsage, service.DepartmentScopeQuery{})
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)
	f.mustExec(`DELETE FROM department_access_grants WHERE user_id=$1`, leaderID)

	// Revoking keeps the capabilities without scope; the administrator can then
	// make the account a department leader.
	empty := []string{}
	access, err = s.svc.SetOrganizationAccess(s.adminCtx, leaderID, service.OrganizationAccessInput{Organizations: &empty, ExpectedVersion: s.access(leaderID).Version})
	require.NoError(t, err)
	require.Empty(t, access.Organizations)
	require.ElementsMatch(t, service.OrganizationLeaderDepartmentPermissions, access.Permissions)
	_, err = s.svc.OrganizationScope(leaderCtx)
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)
	access, err = s.svc.SetAccess(s.adminCtx, leaderID, service.DepartmentAccessInput{DepartmentIDs: []int64{xunyouDept}, Report: true, ResetQuota: true, ExpectedVersion: access.Version})
	require.NoError(t, err)
	require.Equal(t, []int64{xunyouDept}, access.DepartmentIDs)

	// Leaving the sub_admin role clears the organization grants.
	second, _ := s.organizationLeader("second", service.OrganizationWsdashi)
	secondEditor, err := repo.GetByIDWithAdminAccess(s.adminCtx, second)
	require.NoError(t, err)
	secondEditor.Role, secondEditor.AdminPermissions = service.RoleUser, []string{}
	require.NoError(t, repo.Update(s.adminCtx, secondEditor, service.UserUpdateFields{Role: true, AdminPermissions: true, ExpectedAdminAccessVersion: secondEditor.AdminAccessVersion}))
	var grants int
	require.NoError(t, integrationDB.QueryRowContext(f.ctx, `SELECT COUNT(*) FROM organization_access_grants WHERE user_id=$1`, second).Scan(&grants))
	require.Zero(t, grants)
	require.Equal(t, 1, s.audits("organization.clear_user_grants", s.adminID))
}

func TestOrganizationLeaderIntegration_UnassignedMembersAndDepartments(t *testing.T) {
	s := setupOrganizationLeaders(t)
	f := s.f
	_, leaderCtx := s.organizationLeader("org", service.OrganizationWsdashi)
	member := f.user("member", "wsdashi.com", service.RoleUser, nil)

	// The leader works before the organization has any department (R4).
	var existing int
	require.NoError(t, integrationDB.QueryRowContext(f.ctx, `SELECT COUNT(*) FROM departments WHERE organization_key='wsdashi'`).Scan(&existing))
	scope, err := s.svc.ManagedScope(leaderCtx)
	require.NoError(t, err)
	require.Equal(t, []string{service.OrganizationWsdashi}, scope.FullOrganizations)
	require.False(t, scope.Unrestricted)
	if existing == 0 {
		require.Empty(t, scope.Departments)
	}
	unassigned := service.DepartmentListFilter{Organization: service.OrganizationWsdashi, DepartmentID: "unassigned", Q: f.prefix}
	list, err := s.svc.ManagedMembers(leaderCtx, unassigned)
	require.NoError(t, err)
	require.Len(t, list.Items, 1)
	require.Nil(t, list.Items[0].DepartmentID)
	require.Nil(t, list.Items[0].DepartmentName)
	require.True(t, list.Items[0].Manageable)

	in := s.expect(member)
	in.ExpectedUnassigned = false
	in.Username = departmentMemberString("renamed")
	_, err = s.svc.UpdateManagedMember(leaderCtx, member, in)
	require.ErrorIs(t, err, service.ErrDepartmentInvalid, "an omitted expected_department_id is rejected")
	in.ExpectedUnassigned = true
	updated, err := s.svc.UpdateManagedMember(leaderCtx, member, in)
	require.NoError(t, err)
	require.Equal(t, "renamed", updated.Username)
	in = s.expect(member)
	in.DepartmentNote = departmentMemberString("note")
	_, err = s.svc.UpdateManagedMember(leaderCtx, member, in)
	require.ErrorIs(t, err, service.ErrDepartmentInvalid, "unassigned members carry no manager note")
	in = s.expect(member)
	in.Status = departmentMemberString(service.StatusDisabled)
	_, err = s.svc.UpdateManagedMember(leaderCtx, member, in)
	require.NoError(t, err)
	f.requireStatus(member, service.StatusDisabled, "organization_manager")
	in = s.expect(member)
	in.Status = departmentMemberString(service.StatusActive)
	_, err = s.svc.UpdateManagedMember(leaderCtx, member, in)
	require.NoError(t, err)

	// D2: departments are created and maintained only inside the organization,
	// and new departments join the scope without any extra grant.
	dept, err := s.svc.Save(leaderCtx, 0, service.DepartmentSaveInput{Organization: service.OrganizationWsdashi, Name: f.prefix + "created"})
	require.NoError(t, err)
	f.departmentIDs = append(f.departmentIDs, dept.ID)
	_, err = s.svc.Save(leaderCtx, 0, service.DepartmentSaveInput{Organization: service.OrganizationXunyou, Name: f.prefix + "foreign"})
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)
	_, err = s.svc.List(leaderCtx, service.DepartmentListFilter{Organization: service.OrganizationXunyou})
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)
	departments, err := s.svc.List(leaderCtx, service.DepartmentListFilter{Organization: service.OrganizationAll, PageSize: 200})
	require.NoError(t, err)
	found := false
	for _, d := range departments.Items {
		require.Equal(t, service.OrganizationWsdashi, d.Organization)
		found = found || d.ID == dept.ID
	}
	require.True(t, found)
	scope, err = s.svc.OrganizationScope(leaderCtx)
	require.NoError(t, err)
	require.ElementsMatch(t, service.OrganizationLeaderDepartmentPermissions, scope.DelegablePermissions)
	require.Contains(t, departmentScopeIDs(scope), dept.ID)
	version := dept.Version
	renamed, err := s.svc.Save(leaderCtx, dept.ID, service.DepartmentSaveInput{Organization: service.OrganizationWsdashi, Name: f.prefix + "renamed", ExpectedVersion: &version})
	require.NoError(t, err)
	require.Equal(t, f.prefix+"renamed", renamed.Name)

	// D3: one-step transfers; leaving a department keeps the member in scope,
	// so a retried removal is a no-op instead of a denial.
	stale := s.expect(member)
	changed, err := s.svc.Assign(leaderCtx, service.DepartmentAssignInput{DepartmentID: &dept.ID, Members: []service.DepartmentMemberChange{s.change(member)}})
	require.NoError(t, err)
	require.Equal(t, 1, changed)
	stale.Username = departmentMemberString("stale")
	_, err = s.svc.UpdateManagedMember(leaderCtx, member, stale)
	require.ErrorIs(t, err, service.ErrDepartmentConflict)
	removal := service.DepartmentAssignInput{Members: []service.DepartmentMemberChange{s.change(member)}}
	changed, err = s.svc.Assign(leaderCtx, removal)
	require.NoError(t, err)
	require.Equal(t, 1, changed)
	changed, err = s.svc.Assign(leaderCtx, removal)
	require.NoError(t, err)
	require.Zero(t, changed)
	_, err = s.svc.UpdateManagedMember(leaderCtx, member, stale)
	require.ErrorIs(t, err, service.ErrDepartmentConflict, "unassigned -> assigned -> unassigned keeps old edits stale")

	// Department managers never manage unassigned members, and privileged
	// members stay read-only for the organization leader (C2).
	managerID, managerCtx := s.departmentLeader("manager", "wsdashi.com", []int64{dept.ID}, true, false, true)
	_, err = s.svc.ManagedMembers(managerCtx, unassigned)
	require.ErrorIs(t, err, service.ErrDepartmentInvalid)
	f.mustExec(`UPDATE users SET department_id=$1 WHERE id=$2`, dept.ID, managerID)
	list, err = s.svc.ManagedMembers(leaderCtx, service.DepartmentListFilter{DepartmentID: strconv.FormatInt(dept.ID, 10), Q: f.prefix})
	require.NoError(t, err)
	require.Len(t, list.Items, 1)
	require.False(t, list.Items[0].Manageable)

	// Reports: the unassigned bucket belongs to the organization leader only.
	report, err := s.svc.QueryScope(leaderCtx, service.AdminPermissionOrganizationUsage, service.DepartmentScopeQuery{Organization: service.OrganizationWsdashi, DepartmentID: "unassigned"})
	require.NoError(t, err)
	require.Contains(t, departmentMemberIDs(report.Members), member)
	_, err = s.svc.QueryScope(managerCtx, service.AdminPermissionOrganizationUsage, service.DepartmentScopeQuery{Organization: service.OrganizationWsdashi, DepartmentID: "unassigned"})
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)
}

func departmentScopeIDs(scope *service.DepartmentScope) []int64 {
	ids := []int64{}
	for _, d := range scope.Departments {
		ids = append(ids, d.ID)
	}
	return ids
}

func departmentMemberIDs(members []service.DepartmentMember) []int64 {
	ids := []int64{}
	for _, m := range members {
		ids = append(ids, m.ID)
	}
	return ids
}

func TestOrganizationLeaderIntegration_AppointUpdateRevoke(t *testing.T) {
	s := setupOrganizationLeaders(t)
	f := s.f
	leaderID, leaderCtx := s.organizationLeader("org", service.OrganizationXunyou, service.OrganizationWsdashi)
	xunyouDept := f.department(service.OrganizationXunyou)
	speedDept := f.department(service.OrganizationWsdashi)
	target := f.user("target", "xunyou.com", service.RoleUser, nil)

	candidates, err := s.svc.OrganizationLeaderCandidates(leaderCtx, service.OrganizationAll, f.prefix+"target")
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	both := []int64{xunyouDept, speedDept}
	for _, invalid := range []service.OrganizationLeaderInput{
		organizationLeaderInput([]int64{}, true, false, false, candidates[0].Version),
		organizationLeaderInput(both, false, false, false, candidates[0].Version),
		{DepartmentIDs: &both},
	} {
		_, err = s.svc.UpdateOrganizationLeader(leaderCtx, target, invalid)
		require.ErrorIs(t, err, service.ErrDepartmentInvalid)
	}
	access, err := s.svc.UpdateOrganizationLeader(leaderCtx, target, organizationLeaderInput(both, true, false, true, candidates[0].Version))
	require.NoError(t, err)
	require.Equal(t, service.RoleSubAdmin, access.Role)
	require.Equal(t, both, access.DepartmentIDs)
	require.ElementsMatch(t, []string{service.AdminPermissionOrganizationUsage, service.AdminPermissionDepartmentMembers}, access.Permissions)
	require.Equal(t, 1, s.audits("organization.leader_appoint", leaderID))

	// R3: the list projects one organization; the edit snapshot holds every grant,
	// so saving it unchanged keeps the other organization's department.
	list, err := s.svc.OrganizationLeaders(leaderCtx, service.DepartmentListFilter{Organization: service.OrganizationXunyou, Q: f.prefix + "target"})
	require.NoError(t, err)
	require.Len(t, list.Items, 1)
	require.True(t, list.Items[0].Managed)
	require.Len(t, list.Items[0].Departments, 1)
	require.Equal(t, xunyouDept, list.Items[0].Departments[0].ID)
	detail, err := s.svc.OrganizationLeader(leaderCtx, target)
	require.NoError(t, err)
	require.Len(t, detail.Departments, 2)
	access, err = s.svc.UpdateOrganizationLeader(leaderCtx, target, organizationLeaderInput(both, true, false, true, detail.Version))
	require.NoError(t, err)
	require.Equal(t, both, access.DepartmentIDs)
	require.Zero(t, s.audits("organization.leader_update", leaderID), "an unchanged save writes no audit")
	_, err = s.svc.UpdateOrganizationLeader(leaderCtx, target, organizationLeaderInput([]int64{xunyouDept}, true, false, true, candidates[0].Version))
	require.ErrorIs(t, err, service.ErrAdminAccessChanged)

	inactive := f.department(service.OrganizationXunyou)
	f.mustExec(`UPDATE departments SET status='inactive' WHERE id=$1`, inactive)
	_, err = s.svc.UpdateOrganizationLeader(leaderCtx, target, organizationLeaderInput([]int64{xunyouDept, speedDept, inactive}, true, false, true, detail.Version))
	require.ErrorIs(t, err, service.ErrDepartmentInactive)
	otherDept := f.department(service.OrganizationOther)
	_, err = s.svc.UpdateOrganizationLeader(leaderCtx, target, organizationLeaderInput([]int64{xunyouDept, otherDept}, true, false, true, detail.Version))
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)

	// Protected targets are denied before any version check.
	disabled := f.user("disabled", "xunyou.com", service.RoleUser, nil)
	f.mustExec(`UPDATE users SET status='disabled' WHERE id=$1`, disabled)
	foreignLeader, _ := s.departmentLeader("foreign", "xunyou.com", []int64{otherDept}, true, false, false)
	peer, _ := s.organizationLeader("peer", service.OrganizationXunyou)
	for name, id := range map[string]int64{
		"self":           leaderID,
		"administrator":  f.user("admin2", "xunyou.com", service.RoleAdmin, nil),
		"other org user": f.user("outsider", "example.com", service.RoleUser, nil),
		"disabled user":  disabled,
		"global leader":  f.user("global", "xunyou.com", service.RoleSubAdmin, []string{service.AdminPermissionUsage}),
		"foreign grants": foreignLeader,
		"peer":           peer,
	} {
		_, err = s.svc.UpdateOrganizationLeader(leaderCtx, id, organizationLeaderInput([]int64{xunyouDept}, true, false, false, "stale"))
		require.ErrorIs(t, err, service.ErrOrganizationLeaderProtected, name)
		_, err = s.svc.OrganizationLeader(leaderCtx, id)
		require.ErrorIs(t, err, service.ErrOrganizationLeaderProtected, name)
	}
	f.mustExec(`UPDATE users SET admin_permissions='["admin.organization_usage","admin.department_members","admin.usage"]'::jsonb WHERE id=$1`, target)
	_, err = s.svc.UpdateOrganizationLeader(leaderCtx, target, organizationLeaderInput([]int64{xunyouDept}, true, false, true, detail.Version))
	require.ErrorIs(t, err, service.ErrOrganizationLeaderProtected, "a target that became protected is a 403, not a 409")
	f.mustExec(`UPDATE users SET admin_permissions='["admin.organization_usage","admin.department_members"]'::jsonb WHERE id=$1`, target)

	// D6: revoking demotes to an ordinary user that a department manager may manage again.
	detail, err = s.svc.OrganizationLeader(leaderCtx, target)
	require.NoError(t, err)
	access, err = s.svc.UpdateOrganizationLeader(leaderCtx, target, organizationLeaderInput([]int64{}, false, false, false, detail.Version))
	require.NoError(t, err)
	require.Equal(t, service.RoleUser, access.Role)
	require.Empty(t, access.DepartmentIDs)
	require.Empty(t, access.Permissions)
	require.Equal(t, 1, s.audits("organization.leader_revoke", leaderID))
	f.mustExec(`UPDATE users SET department_id=$1 WHERE id=$2`, xunyouDept, target)
	_, managerCtx := s.departmentLeader("manager", "xunyou.com", []int64{xunyouDept}, false, false, true)
	members, err := s.svc.ManagedMembers(managerCtx, service.DepartmentListFilter{DepartmentID: strconv.FormatInt(xunyouDept, 10), Q: f.prefix + "target"})
	require.NoError(t, err)
	require.Len(t, members.Items, 1)
	require.True(t, members.Items[0].Manageable)
}

func TestOrganizationLeaderIntegration_DisableSourcesAndSubscriptionTargets(t *testing.T) {
	s := setupOrganizationLeaders(t)
	f := s.f
	leaderID, leaderCtx := s.organizationLeader("org", service.OrganizationXunyou)
	dept := f.department(service.OrganizationXunyou)
	managerID, managerCtx := s.departmentLeader("manager", "xunyou.com", []int64{dept}, false, true, true)
	member := f.user("member", "xunyou.com", service.RoleUser, nil)
	f.mustExec(`UPDATE users SET department_id=$1 WHERE id=ANY($2)`, dept, pq.Array([]int64{member, managerID, leaderID}))
	disabled, active := service.StatusDisabled, service.StatusActive

	// D4: an organization leader takes over a manager disable; managers cannot lift it.
	in := s.expect(member)
	in.Status = &disabled
	_, err := s.svc.UpdateManagedMember(managerCtx, member, in)
	require.NoError(t, err)
	f.requireStatus(member, disabled, "department_manager")
	in = s.expect(member)
	in.Status = &disabled
	_, err = s.svc.UpdateManagedMember(leaderCtx, member, in)
	require.NoError(t, err)
	f.requireStatus(member, disabled, "organization_manager")
	in = s.expect(member)
	in.Status = &active
	_, err = s.svc.UpdateManagedMember(managerCtx, member, in)
	require.ErrorIs(t, err, service.ErrDepartmentMemberEnableDenied)
	_, err = s.svc.UpdateManagedMember(leaderCtx, member, in)
	require.NoError(t, err)
	f.requireStatus(member, active, "")
	f.mustExec(`UPDATE users SET status='disabled',disabled_source='organization_manager' WHERE id=$1`, member)
	requireCheckViolation(t, f.exec(`UPDATE users SET status='disabled',disabled_source='department_manager' WHERE id=$1`, member))
	f.mustExec(`UPDATE users SET status='disabled',disabled_source='other' WHERE id=$1`, member)
	requireCheckViolation(t, f.exec(`UPDATE users SET status='disabled',disabled_source='organization_manager' WHERE id=$1`, member))
	f.mustExec(`UPDATE users SET status='active' WHERE id=$1`, member)

	// D5: scoped subscription access never reaches the actor itself, administrators
	// or leaders outside the actor's management.
	admin := f.user("admin2", "xunyou.com", service.RoleAdmin, nil)
	unassigned := f.user("unassigned", "xunyou.com", service.RoleUser, nil)
	f.mustExec(`UPDATE users SET department_id=$1 WHERE id=$2`, dept, admin)
	var group int64
	require.NoError(t, integrationDB.QueryRowContext(f.ctx, `INSERT INTO groups(name,platform,subscription_type,daily_limit_usd) VALUES($1,'openai','subscription',20) RETURNING id`, f.prefix+"group").Scan(&group))
	t.Cleanup(func() {
		_, e := integrationDB.ExecContext(f.ctx, `DELETE FROM user_subscriptions WHERE group_id=$1`, group)
		require.NoError(t, e)
		_, e = integrationDB.ExecContext(f.ctx, `DELETE FROM groups WHERE id=$1`, group)
		require.NoError(t, e)
	})
	subscriptions := map[int64]int64{}
	for _, userID := range []int64{member, managerID, leaderID, admin, unassigned} {
		var id int64
		require.NoError(t, integrationDB.QueryRowContext(f.ctx, `INSERT INTO user_subscriptions(user_id,group_id,starts_at,expires_at,status,daily_usage_usd)
VALUES($1,$2,NOW()-INTERVAL '1 day',NOW()+INTERVAL '30 days','active',3) RETURNING id`, userID, group).Scan(&id))
		subscriptions[userID] = id
	}
	repo := NewUserSubscriptionRepository(integrationEntClient)
	subs := service.NewSubscriptionService(NewGroupRepository(integrationEntClient, integrationDB), repo, nil, integrationEntClient, nil)
	t.Cleanup(subs.Stop)
	reachable := func(ctx context.Context) []int64 {
		items, _, e := repo.ListAdmin(ctx, pagination.PaginationParams{Page: 1, PageSize: 50}, service.SubscriptionAdminFilter{GroupID: &group, SortBy: "created_at", SortOrder: "desc"}, time.Now())
		require.NoError(t, e)
		ids := []int64{}
		for _, item := range items {
			ids = append(ids, item.UserID)
		}
		return ids
	}
	require.ElementsMatch(t, []int64{member}, reachable(managerCtx))
	require.ElementsMatch(t, []int64{member, managerID, unassigned}, reachable(leaderCtx))
	for name, c := range map[string]struct {
		ctx     context.Context
		userID  int64
		allowed bool
	}{
		"manager resets member":        {managerCtx, member, true},
		"manager resets itself":        {managerCtx, managerID, false},
		"manager resets admin":         {managerCtx, admin, false},
		"manager resets its superior":  {managerCtx, leaderID, false},
		"manager resets unassigned":    {managerCtx, unassigned, false},
		"leader resets member":         {leaderCtx, member, true},
		"leader resets unassigned":     {leaderCtx, unassigned, true},
		"leader resets managed leader": {leaderCtx, managerID, true},
		"leader resets itself":         {leaderCtx, leaderID, false},
		"leader resets admin":          {leaderCtx, admin, false},
	} {
		_, err = subs.AdminResetQuota(c.ctx, subscriptions[c.userID], true, true, true)
		if c.allowed {
			require.NoError(t, err, name)
		} else {
			require.ErrorIs(t, err, service.ErrDepartmentScopeDenied, name)
		}
	}
}

func requireCheckViolation(t *testing.T, err error) {
	t.Helper()
	var pqErr *pq.Error
	require.True(t, errors.As(err, &pqErr), "expected a PostgreSQL error, got %v", err)
	require.Equal(t, pq.ErrorCode("23514"), pqErr.Code)
}

func TestOrganizationLeaderIntegration_ConcurrentDepartmentWrites(t *testing.T) {
	s := setupOrganizationLeaders(t)
	f := s.f
	_, firstCtx := s.organizationLeader("first", service.OrganizationXunyou)
	_, secondCtx := s.organizationLeader("second", service.OrganizationXunyou)
	dept := f.department(service.OrganizationXunyou)

	// R5: two operators editing one department get one success and one
	// version conflict, never a deadlock or a 500.
	for round := 0; round < 5; round++ {
		var version int64
		require.NoError(t, integrationDB.QueryRowContext(f.ctx, `SELECT version FROM departments WHERE id=$1`, dept).Scan(&version))
		start := make(chan struct{})
		results := make(chan error, 2)
		for i, ctx := range []context.Context{firstCtx, secondCtx} {
			status := service.StatusActive
			if i == 1 {
				status = "inactive"
			}
			go func(ctx context.Context, name, status string) {
				<-start
				_, e := s.svc.Save(ctx, dept, service.DepartmentSaveInput{Organization: service.OrganizationXunyou, Name: name, Status: status, ExpectedVersion: &version})
				results <- e
			}(ctx, fmt.Sprintf("%sr%d_%d", f.prefix, round, i), status)
		}
		close(start)
		successes := 0
		for range 2 {
			if e := <-results; e == nil {
				successes++
			} else {
				require.ErrorIs(t, e, service.ErrDepartmentConflict)
			}
		}
		require.Equal(t, 1, successes, "round %d", round)
	}

	// Deactivation committed while a transfer waits: the transfer reads the
	// inactive department and is rejected.
	f.mustExec(`UPDATE departments SET status='active' WHERE id=$1`, dept)
	member := f.user("member", "xunyou.com", service.RoleUser, nil)
	tx, err := integrationDB.BeginTx(f.ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(f.ctx, `UPDATE departments SET status='inactive',version=version+1 WHERE id=$1`, dept)
	require.NoError(t, err)
	queued := make(chan error, 1)
	go func() {
		_, e := s.svc.Assign(firstCtx, service.DepartmentAssignInput{DepartmentID: &dept, Members: []service.DepartmentMemberChange{s.change(member)}})
		queued <- e
	}()
	require.Eventually(t, func() bool {
		var blocked int
		e := integrationDB.QueryRow(`SELECT COUNT(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%FROM departments WHERE id=$1 FOR SHARE%'`).Scan(&blocked)
		return e == nil && blocked > 0
	}, 5*time.Second, 10*time.Millisecond)
	require.NoError(t, tx.Commit())
	select {
	case err = <-queued:
		require.ErrorIs(t, err, service.ErrDepartmentInactive)
	case <-time.After(5 * time.Second):
		t.Fatal("transfer did not finish after the deactivation committed")
	}
}

// Design §14 K9: the maintenance member list marks exactly the rows Assign
// accepts, so a select-page batch of manageable rows never fails on its targets.
func TestOrganizationLeaderIntegration_MemberRowsMatchTransferRules(t *testing.T) {
	s := setupOrganizationLeaders(t)
	f := s.f
	leader, leaderCtx := s.organizationLeader("rows_leader", service.OrganizationXunyou)
	peer, _ := s.organizationLeader("rows_peer", service.OrganizationXunyou)
	department := f.department(service.OrganizationXunyou)
	plain := f.user("rows_plain", "xunyou.com", service.RoleUser, nil)
	admin := f.user("rows_admin", "xunyou.com", service.RoleAdmin, nil)
	managed := f.user("rows_managed", "xunyou.com", service.RoleSubAdmin, []string{service.AdminPermissionOrganizationUsage})
	unmanaged := f.user("rows_unmanaged", "xunyou.com", service.RoleSubAdmin, []string{service.AdminPermissionUsage})
	f.mustExec(`UPDATE users SET department_id=$1 WHERE id=ANY($2)`, department, pq.Array([]int64{leader, peer, plain, admin, managed, unmanaged}))

	manageable := func(ctx context.Context, departmentID string) map[int64]bool {
		t.Helper()
		page, err := s.svc.Members(ctx, service.DepartmentListFilter{Organization: service.OrganizationXunyou, DepartmentID: departmentID, Q: f.prefix + "rows_", Page: 1, PageSize: 50})
		require.NoError(t, err)
		out := map[int64]bool{}
		for _, row := range page.Items {
			out[row.ID] = row.Manageable
		}
		return out
	}
	want := map[int64]bool{leader: false, peer: false, plain: true, admin: false, managed: true, unmanaged: false}
	current := strconv.FormatInt(department, 10)
	require.Equal(t, want, manageable(leaderCtx, current), "current members")
	require.Equal(t, want, manageable(leaderCtx, service.OrganizationAll), "add mode lists the whole organization")
	for id, ok := range manageable(s.adminCtx, current) {
		require.True(t, ok, "admins keep moving anyone: %d", id)
	}

	// A protected ID tampered into the batch is still rejected as a whole.
	_, err := s.svc.Assign(leaderCtx, service.DepartmentAssignInput{Members: []service.DepartmentMemberChange{s.change(plain), s.change(admin)}})
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)
	require.Equal(t, department, *s.expect(plain).ExpectedDepartmentID)
	n, err := s.svc.Assign(leaderCtx, service.DepartmentAssignInput{Members: []service.DepartmentMemberChange{s.change(plain), s.change(managed)}})
	require.NoError(t, err)
	require.Equal(t, 2, n)
}
