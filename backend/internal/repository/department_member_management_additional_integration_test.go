//go:build integration

package repository

import (
	"context"
	"strconv"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestDepartmentMemberManagementIntegration_AdditionalReadAndWriteBoundaries(t *testing.T) {
	s := setupDepartmentMembers(t)
	f := s.f
	member := f.user("profile", "xunyou.com", service.RoleUser, nil)
	s.place(member, s.granted)
	f.mustExec(`UPDATE users SET username=$1,department_note='keep note',status='disabled' WHERE id=$2`, f.prefix+"member_name", member)
	list, err := s.svc.ManagedMembers(s.leaderCtx, service.DepartmentListFilter{
		DepartmentID: strconv.FormatInt(s.granted, 10), Q: f.prefix + "member_name", Status: service.StatusDisabled,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, list.Total)
	require.Len(t, list.Items, 1)
	require.Equal(t, member, list.Items[0].ID)
	require.Equal(t, "keep note", list.Items[0].DepartmentNote)

	missingDepartment := s.expect(member)
	missingDepartment.ExpectedDepartmentID = nil
	missingDepartment.Username = departmentMemberString("missing department must not write")
	_, err = s.svc.UpdateManagedMember(s.leaderCtx, member, missingDepartment)
	require.ErrorIs(t, err, service.ErrDepartmentInvalid, "an authorized member still requires the expected department id")

	in := s.expect(member)
	in.DepartmentNote = departmentMemberString("administrator overwrite")
	_, err = s.svc.UpdateManagedMember(s.adminCtx, member, in)
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)
	_, err = s.svc.UpdateManagedMember(service.WithDepartmentAdminAPIKey(f.ctx, s.leaderID), member, in)
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)
	require.Equal(t, "keep note", f.note(member))

	candidate := f.user("literal", "xunyou.com", service.RoleUser, nil)
	f.mustExec(`UPDATE users SET username=$1,status='disabled' WHERE id=$2`, f.prefix+"name%_", candidate)
	decoy := f.user("decoy", "xunyou.com", service.RoleUser, nil)
	f.mustExec(`UPDATE users SET username=$1 WHERE id=$2`, f.prefix+"nameABC", decoy)
	found, err := s.svc.ManagedCandidates(s.leaderCtx, s.granted, f.prefix+"name%_")
	require.NoError(t, err)
	require.Len(t, found, 1, "search must match username and treat LIKE metacharacters literally")
	require.Equal(t, candidate, found[0].ID)
	require.Equal(t, service.StatusDisabled, found[0].Status, "disabled unassigned users remain candidates")

	outside := f.user("api_key_target", "xunyou.com", service.RoleUser, nil)
	s.place(outside, s.other)
	in = s.expect(outside)
	in.Status = departmentMemberString(service.StatusDisabled)
	_, err = s.svc.UpdateManagedMember(service.WithDepartmentAdminAPIKey(f.ctx, s.leaderID), outside, in)
	require.NoError(t, err, "an Admin API Key has independent full-admin scope")
	f.requireStatus(outside, service.StatusDisabled, "other")

	f.mustExec(`UPDATE departments SET status='inactive' WHERE id=$1`, s.granted)
	_, err = s.svc.ManagedCandidates(s.leaderCtx, s.granted, f.prefix+"name")
	require.ErrorIs(t, err, service.ErrDepartmentInactive)
	s.grant(true)
	_, err = s.svc.ManagedScope(s.leaderCtx)
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)
}

func TestDepartmentMemberManagementIntegration_BatchAuthorizationAndAuditRollback(t *testing.T) {
	s := setupDepartmentMembers(t)
	f := s.f
	valid := f.user("valid", "xunyou.com", service.RoleUser, nil)
	stale := s.change(valid)
	stale.ExpectedVersion++
	outside := f.user("outside", "xunyou.com", service.RoleUser, nil)
	s.place(outside, s.other)
	_, err := s.svc.ManagedAssign(s.leaderCtx, service.DepartmentAssignInput{
		DepartmentID: &s.granted, Members: []service.DepartmentMemberChange{stale, s.change(outside)},
	})
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied, "an earlier stale CAS must not precede the whole batch authorization check")
	department, _ := s.current(valid)
	require.Nil(t, department)

	member := f.user("rollback", "xunyou.com", service.RoleUser, nil)
	s.place(member, s.granted)
	f.mustExec(`UPDATE users SET department_note='keep note' WHERE id=$1`, member)
	functionName := pq.QuoteIdentifier(f.prefix + "reject_audit")
	triggerName := pq.QuoteIdentifier(f.prefix + "reject_audit_trigger")
	f.mustExec(`CREATE FUNCTION ` + functionName + `() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.actor_user_id=` + strconv.FormatInt(s.leaderID, 10) + ` AND NEW.action IN ('department.manager_assign_members','department.manager_update_member') THEN
        RAISE EXCEPTION 'department member test audit failure';
    END IF;
    RETURN NEW;
END;
$$`)
	t.Cleanup(func() {
		_, dropErr := integrationDB.ExecContext(context.Background(), `DROP TRIGGER IF EXISTS `+triggerName+` ON audit_logs`)
		require.NoError(t, dropErr)
		_, dropErr = integrationDB.ExecContext(context.Background(), `DROP FUNCTION IF EXISTS `+functionName+`() `)
		require.NoError(t, dropErr)
	})
	f.mustExec(`CREATE TRIGGER ` + triggerName + ` BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION ` + functionName + `()`)

	_, err = s.svc.ManagedAssign(s.leaderCtx, service.DepartmentAssignInput{Members: []service.DepartmentMemberChange{s.change(member)}})
	require.Error(t, err)
	department, _ = s.current(member)
	require.NotNil(t, department)
	require.Equal(t, s.granted, *department)
	require.Equal(t, "keep note", f.note(member), "audit failure rolls back the note-clearing trigger as well")
	require.Empty(t, f.noteClearedAudits(member))

	in := s.expect(member)
	in.DepartmentNote = departmentMemberString("new note")
	in.Status = departmentMemberString(service.StatusDisabled)
	_, err = s.svc.UpdateManagedMember(s.leaderCtx, member, in)
	require.Error(t, err)
	f.requireStatus(member, service.StatusActive, "")
	require.Equal(t, "keep note", f.note(member))
	require.Zero(t, s.audits("department.manager_update_member", s.leaderID))
	require.Zero(t, s.audits("department.manager_assign_members", s.leaderID))
}
