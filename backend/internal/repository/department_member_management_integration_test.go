//go:build integration && deptmembers_pending

package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// Locked acceptance tests for the manager member-management API (phase 2 of
// docs/features/department-member-management-implementation-plan-cn.md).
// At the start of phase 2 remove "&& deptmembers_pending" from the build line; change nothing else.

type departmentMemberSetup struct {
	f                   *departmentMemberFixture
	svc                 *service.DepartmentService
	adminID, leaderID   int64
	adminCtx, leaderCtx context.Context
	granted, other      int64
}

func setupDepartmentMembers(t *testing.T) *departmentMemberSetup {
	t.Helper()
	f := newDepartmentMemberFixture(t)
	s := &departmentMemberSetup{f: f, svc: service.NewDepartmentService(NewDepartmentRepository(integrationDB), nil)}
	s.adminID = f.user("admin", "example.com", service.RoleAdmin, nil)
	s.leaderID = f.user("leader", "xunyou.com", service.RoleSubAdmin, nil)
	s.adminCtx = service.WithDepartmentActor(f.ctx, s.adminID)
	s.leaderCtx = service.WithDepartmentActor(f.ctx, s.leaderID)
	s.granted = f.department(service.OrganizationXunyou)
	s.other = f.department(service.OrganizationXunyou)
	s.grant(true, s.granted)
	return s
}

func (s *departmentMemberSetup) grant(manage bool, departmentIDs ...int64) *service.DepartmentAccess {
	s.f.t.Helper()
	access, err := s.svc.GetAccess(s.adminCtx, s.leaderID)
	require.NoError(s.f.t, err)
	if departmentIDs == nil {
		departmentIDs = []int64{}
	}
	access, err = s.svc.SetAccess(s.adminCtx, s.leaderID, service.DepartmentAccessInput{DepartmentIDs: departmentIDs, ManageMembers: &manage, ExpectedVersion: access.Version})
	require.NoError(s.f.t, err)
	return access
}

func (s *departmentMemberSetup) place(userID, departmentID int64) {
	s.f.t.Helper()
	s.f.mustExec(`UPDATE users SET department_id=$1 WHERE id=$2`, departmentID, userID)
}

func (s *departmentMemberSetup) current(userID int64) (*int64, int64) {
	s.f.t.Helper()
	var department sql.NullInt64
	var version int64
	require.NoError(s.f.t, integrationDB.QueryRowContext(s.f.ctx, `SELECT department_id,department_version FROM users WHERE id=$1`, userID).Scan(&department, &version))
	if !department.Valid {
		return nil, version
	}
	return &department.Int64, version
}

func (s *departmentMemberSetup) change(userID int64) service.DepartmentMemberChange {
	department, version := s.current(userID)
	return service.DepartmentMemberChange{UserID: userID, ExpectedDepartmentID: department, ExpectedVersion: version}
}

func (s *departmentMemberSetup) expect(userID int64) service.ManagedMemberUpdateInput {
	department, version := s.current(userID)
	return service.ManagedMemberUpdateInput{ExpectedDepartmentID: department, ExpectedDepartmentVersion: &version}
}

func (s *departmentMemberSetup) audits(action string, actorID int64) int {
	s.f.t.Helper()
	var count int
	require.NoError(s.f.t, integrationDB.QueryRowContext(s.f.ctx, `SELECT COUNT(*) FROM audit_logs WHERE action=$1 AND actor_user_id=$2`, action, actorID).Scan(&count))
	return count
}

func departmentMemberString(value string) *string { return &value }

func TestDepartmentMemberManagementIntegration_AccessPermissionContract(t *testing.T) {
	s := setupDepartmentMembers(t)
	access, err := s.svc.GetAccess(s.adminCtx, s.leaderID)
	require.NoError(t, err)
	require.Equal(t, []string{service.AdminPermissionDepartmentMembers}, access.Permissions, "manage_members alone grants no report or quota permission")

	access, err = s.svc.SetAccess(s.adminCtx, s.leaderID, service.DepartmentAccessInput{DepartmentIDs: []int64{s.granted}, Report: true, ExpectedVersion: access.Version})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{service.AdminPermissionOrganizationUsage, service.AdminPermissionDepartmentMembers}, access.Permissions, "omitting manage_members keeps it")

	manage := false
	access, err = s.svc.SetAccess(s.adminCtx, s.leaderID, service.DepartmentAccessInput{DepartmentIDs: []int64{s.granted}, Report: true, ManageMembers: &manage, ExpectedVersion: access.Version})
	require.NoError(t, err)
	require.Equal(t, []string{service.AdminPermissionOrganizationUsage}, access.Permissions, "explicit false revokes it")
	_, err = s.svc.ManagedMembers(s.leaderCtx, service.DepartmentListFilter{DepartmentID: "all"})
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied, "a report grant does not imply member management")

	s.grant(true)
	_, err = s.svc.ManagedMembers(s.leaderCtx, service.DepartmentListFilter{DepartmentID: "all"})
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied, "the permission without grants manages nothing")
}

func TestDepartmentMemberManagementIntegration_AssignAndRemove(t *testing.T) {
	s := setupDepartmentMembers(t)
	f := s.f
	member := f.user("member", "xunyou.com", service.RoleUser, nil)

	stale := s.change(member)
	n, err := s.svc.ManagedAssign(s.leaderCtx, service.DepartmentAssignInput{DepartmentID: &s.granted, Members: []service.DepartmentMemberChange{stale}})
	require.NoError(t, err)
	require.Equal(t, 1, n)
	n, err = s.svc.ManagedAssign(s.leaderCtx, service.DepartmentAssignInput{DepartmentID: &s.granted, Members: []service.DepartmentMemberChange{stale}})
	require.NoError(t, err, "repeating an add into the same granted department is an idempotent success")
	require.Zero(t, n)
	require.Equal(t, 1, s.audits("department.manager_assign_members", s.leaderID), "a no-op does not write a change audit")

	// R4: once removed the member is no longer manageable, so a retried removal is denied.
	removal := s.change(member)
	n, err = s.svc.ManagedAssign(s.leaderCtx, service.DepartmentAssignInput{Members: []service.DepartmentMemberChange{removal}})
	require.NoError(t, err)
	require.Equal(t, 1, n)
	_, err = s.svc.ManagedAssign(s.leaderCtx, service.DepartmentAssignInput{Members: []service.DepartmentMemberChange{removal}})
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)

	// Members of another department cannot be pulled in, even when both departments are granted.
	s.grant(true, s.granted, s.other)
	transfer := f.user("transfer", "xunyou.com", service.RoleUser, nil)
	s.place(transfer, s.other)
	_, err = s.svc.ManagedAssign(s.leaderCtx, service.DepartmentAssignInput{DepartmentID: &s.granted, Members: []service.DepartmentMemberChange{s.change(transfer)}})
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)
	department, _ := s.current(transfer)
	require.Equal(t, s.other, *department)

	foreign := f.user("foreign", "wsdashi.com", service.RoleUser, nil)
	subAdmin := f.user("subadmin", "xunyou.com", service.RoleSubAdmin, nil)
	valid := f.user("valid", "xunyou.com", service.RoleUser, nil)
	for _, target := range []int64{foreign, subAdmin} {
		_, err = s.svc.ManagedAssign(s.leaderCtx, service.DepartmentAssignInput{DepartmentID: &s.granted, Members: []service.DepartmentMemberChange{s.change(target)}})
		require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)
	}
	_, err = s.svc.ManagedAssign(s.leaderCtx, service.DepartmentAssignInput{DepartmentID: &s.granted, Members: []service.DepartmentMemberChange{s.change(valid), s.change(foreign)}})
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied, "authorization is checked for the whole batch first")
	department, _ = s.current(valid)
	require.Nil(t, department, "a rejected batch writes nothing")

	// A stale CAS on a still-addable user is a conflict, not a denial.
	staleValid := s.change(valid)
	s.place(valid, s.other)
	f.mustExec(`UPDATE users SET department_id=NULL WHERE id=$1`, valid)
	_, err = s.svc.ManagedAssign(s.leaderCtx, service.DepartmentAssignInput{DepartmentID: &s.granted, Members: []service.DepartmentMemberChange{staleValid}})
	require.ErrorIs(t, err, service.ErrDepartmentConflict)

	// Inactive departments reject additions but still allow removals.
	s.place(member, s.granted)
	f.mustExec(`UPDATE departments SET status='inactive' WHERE id=$1`, s.granted)
	_, err = s.svc.ManagedAssign(s.leaderCtx, service.DepartmentAssignInput{DepartmentID: &s.granted, Members: []service.DepartmentMemberChange{s.change(valid)}})
	require.ErrorIs(t, err, service.ErrDepartmentInactive)
	n, err = s.svc.ManagedAssign(s.leaderCtx, service.DepartmentAssignInput{Members: []service.DepartmentMemberChange{s.change(member)}})
	require.NoError(t, err)
	require.Equal(t, 1, n)
}

func TestDepartmentMemberManagementIntegration_UpdateMember(t *testing.T) {
	s := setupDepartmentMembers(t)
	f := s.f
	disabled, active := service.StatusDisabled, service.StatusActive
	member := f.user("member", "xunyou.com", service.RoleUser, nil)
	s.place(member, s.granted)

	in := s.expect(member)
	in.Status = &disabled
	updated, err := s.svc.UpdateManagedMember(s.leaderCtx, member, in)
	require.NoError(t, err)
	require.Equal(t, service.StatusDisabled, updated.Status)
	require.True(t, updated.CanEnable)
	f.requireStatus(member, "disabled", "department_manager")
	require.Equal(t, 1, s.audits("department.manager_update_member", s.leaderID))

	in = s.expect(member)
	in.Status = &active
	_, err = s.svc.UpdateManagedMember(s.leaderCtx, member, in)
	require.NoError(t, err)
	f.requireStatus(member, "active", "")
	_, err = s.svc.UpdateManagedMember(s.leaderCtx, member, in)
	require.NoError(t, err, "enabling an active member is an idempotent success")
	require.Equal(t, 2, s.audits("department.manager_update_member", s.leaderID), "a no-op does not write a change audit")

	// R1: an admin re-disable takes over a manager disable.
	in.Status = &disabled
	_, err = s.svc.UpdateManagedMember(s.leaderCtx, member, in)
	require.NoError(t, err)
	users := NewUserRepository(integrationEntClient, integrationDB)
	snapshot, err := users.GetByID(f.ctx, member)
	require.NoError(t, err)
	snapshot.Status = service.StatusDisabled
	require.NoError(t, users.Update(f.ctx, snapshot, service.UserUpdateFields{Status: true}))
	in.Status = &active
	_, err = s.svc.UpdateManagedMember(s.leaderCtx, member, in)
	require.ErrorIs(t, err, service.ErrDepartmentMemberEnableDenied)
	f.requireStatus(member, "disabled", "other")

	// Admin actors using this API never create a manager-enable-able disable.
	adminTarget := f.user("admintarget", "xunyou.com", service.RoleUser, nil)
	s.place(adminTarget, s.granted)
	in = s.expect(adminTarget)
	in.Status = &disabled
	_, err = s.svc.UpdateManagedMember(s.adminCtx, adminTarget, in)
	require.NoError(t, err)
	f.requireStatus(adminTarget, "disabled", "other")

	// R2: leaving and returning to the same department invalidates the old page.
	roundTrip := f.user("roundtrip", "xunyou.com", service.RoleUser, nil)
	s.place(roundTrip, s.granted)
	stale := s.expect(roundTrip)
	_, err = s.svc.Assign(s.adminCtx, service.DepartmentAssignInput{Members: []service.DepartmentMemberChange{s.change(roundTrip)}})
	require.NoError(t, err)
	_, err = s.svc.Assign(s.adminCtx, service.DepartmentAssignInput{DepartmentID: &s.granted, Members: []service.DepartmentMemberChange{s.change(roundTrip)}})
	require.NoError(t, err)
	stale.Status = &disabled
	_, err = s.svc.UpdateManagedMember(s.leaderCtx, roundTrip, stale)
	require.ErrorIs(t, err, service.ErrDepartmentConflict)
	f.requireStatus(roundTrip, "active", "")
	missing := s.expect(roundTrip)
	missing.ExpectedDepartmentVersion = nil
	missing.Status = &disabled
	_, err = s.svc.UpdateManagedMember(s.leaderCtx, roundTrip, missing)
	require.ErrorIs(t, err, service.ErrDepartmentInvalid, "the department version is required")

	outside := f.user("outside", "xunyou.com", service.RoleUser, nil)
	s.place(outside, s.other)
	peer := f.user("peer", "xunyou.com", service.RoleSubAdmin, nil)
	s.place(peer, s.granted)
	for _, target := range []int64{outside, peer, s.leaderID} {
		in = s.expect(target)
		in.Status = &disabled
		_, err = s.svc.UpdateManagedMember(s.leaderCtx, target, in)
		require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)
	}
	f.requireStatus(peer, "active", "")

	profileTarget := f.user("profile", "xunyou.com", service.RoleUser, nil)
	s.place(profileTarget, s.granted)
	f.mustExec(`UPDATE users SET notes='ADMIN SECRET', balance=12.5 WHERE id=$1`, profileTarget)
	// Usernames keep the existing Ent MaxLen(100) rule: 100 UTF-8 bytes ("名" is 3 bytes).
	for _, username := range []string{strings.Repeat("a", 100), strings.Repeat("名", 33)} {
		in = s.expect(profileTarget)
		in.Username = departmentMemberString(username)
		updated, err = s.svc.UpdateManagedMember(s.leaderCtx, profileTarget, in)
		require.NoError(t, err, "a username of %d bytes is accepted", len(username))
		require.Equal(t, username, updated.Username)
	}
	// Manager notes are limited to 1000 Unicode characters.
	in = s.expect(profileTarget)
	in.DepartmentNote = departmentMemberString(strings.Repeat("备", 1000))
	_, err = s.svc.UpdateManagedMember(s.leaderCtx, profileTarget, in)
	require.NoError(t, err)
	in = s.expect(profileTarget)
	in.DepartmentNote = departmentMemberString("  leader note  ")
	updated, err = s.svc.UpdateManagedMember(s.leaderCtx, profileTarget, in)
	require.NoError(t, err)
	require.Equal(t, "leader note", updated.DepartmentNote)
	var notes string
	var balance float64
	require.NoError(t, integrationDB.QueryRowContext(f.ctx, `SELECT notes,balance FROM users WHERE id=$1`, profileTarget).Scan(&notes, &balance))
	require.Equal(t, "ADMIN SECRET", notes, "the admin note is never written by managers")
	require.Equal(t, 12.5, balance)
	for _, invalid := range []service.ManagedMemberUpdateInput{
		{Username: departmentMemberString(strings.Repeat("a", 101))},
		{Username: departmentMemberString(strings.Repeat("名", 34))},
		{DepartmentNote: departmentMemberString(strings.Repeat("备", 1001))},
		{Status: departmentMemberString("banned")},
	} {
		in = s.expect(profileTarget)
		in.Username, in.DepartmentNote, in.Status = invalid.Username, invalid.DepartmentNote, invalid.Status
		_, err = s.svc.UpdateManagedMember(s.leaderCtx, profileTarget, in)
		require.ErrorIs(t, err, service.ErrDepartmentInvalid)
	}

	s.grant(true)
	in = s.expect(profileTarget)
	in.Status = &disabled
	_, err = s.svc.UpdateManagedMember(s.leaderCtx, profileTarget, in)
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied, "revoked grants take effect on the next write")
}

func TestDepartmentMemberManagementIntegration_ListAndCandidates(t *testing.T) {
	s := setupDepartmentMembers(t)
	f := s.f
	member := f.user("member", "xunyou.com", service.RoleUser, nil)
	s.place(member, s.granted)
	f.mustExec(`UPDATE users SET notes='ADMIN SECRET', department_note='visible note', status='disabled' WHERE id=$1`, member)
	peer := f.user("peer", "xunyou.com", service.RoleSubAdmin, nil)
	s.place(peer, s.granted)
	f.mustExec(`UPDATE users SET department_note='peer note' WHERE id=$1`, peer)
	outside := f.user("outside", "xunyou.com", service.RoleUser, nil)
	s.place(outside, s.other)

	list, err := s.svc.ManagedMembers(s.leaderCtx, service.DepartmentListFilter{DepartmentID: "all", Page: 1, PageSize: 50})
	require.NoError(t, err)
	byID := map[int64]service.ManagedDepartmentMember{}
	for _, item := range list.Items {
		byID[item.ID] = item
	}
	require.Contains(t, byID, member, "disabled members stay listed so they can be managed")
	require.Contains(t, byID, peer)
	require.NotContains(t, byID, outside)
	require.True(t, byID[member].Manageable)
	require.False(t, byID[member].CanEnable, "an implicit disable is not enable-able by managers")
	require.Equal(t, "visible note", byID[member].DepartmentNote)
	require.False(t, byID[peer].Manageable)
	require.Empty(t, byID[peer].DepartmentNote, "read-only rows do not expose notes")
	encoded, err := json.Marshal(list)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "ADMIN SECRET")
	require.NotContains(t, string(encoded), "peer note")
	_, err = s.svc.ManagedMembers(s.leaderCtx, service.DepartmentListFilter{DepartmentID: strconv.FormatInt(s.other, 10)})
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)

	candidate := f.user("cand", "xunyou.com", service.RoleUser, nil)
	f.user("cand_foreign", "wsdashi.com", service.RoleUser, nil)
	f.user("cand_sub", "xunyou.com", service.RoleSubAdmin, nil)
	assigned := f.user("cand_assigned", "xunyou.com", service.RoleUser, nil)
	s.place(assigned, s.other)
	found, err := s.svc.ManagedCandidates(s.leaderCtx, s.granted, " "+f.prefix+"cand ")
	require.NoError(t, err)
	require.Len(t, found, 1)
	require.Equal(t, candidate, found[0].ID)
	for i := 0; i < 21; i++ {
		f.user("bulk"+strconv.Itoa(i), "xunyou.com", service.RoleUser, nil)
	}
	found, err = s.svc.ManagedCandidates(s.leaderCtx, s.granted, f.prefix+"bulk")
	require.NoError(t, err)
	require.Len(t, found, 20)
	_, err = s.svc.ManagedCandidates(s.leaderCtx, s.granted, " a ")
	require.ErrorIs(t, err, service.ErrDepartmentInvalid)
	_, err = s.svc.ManagedCandidates(s.leaderCtx, s.other, f.prefix+"cand")
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)
}
