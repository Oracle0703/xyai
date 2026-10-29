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

func TestDepartmentMemberManagementIntegration_LegacyRollbackRequiresSourceReset(t *testing.T) {
	s := setupDepartmentMembers(t)
	member := s.f.user("legacy_reset", "xunyou.com", service.RoleUser, nil)
	s.place(member, s.granted)
	in := s.expect(member)
	in.Status = departmentMemberString(service.StatusDisabled)
	in.DepartmentNote = departmentMemberString("retain through source reset")
	_, err := s.svc.UpdateManagedMember(s.leaderCtx, member, in)
	require.NoError(t, err)
	s.f.requireStatus(member, service.StatusDisabled, "department_manager")

	// An old binary repeats the ban without the new explicit source protocol.
	// No trigger can distinguish this omitted source from its inherited value.
	s.f.mustExec(`UPDATE users SET status='disabled' WHERE id=$1`, member)
	s.f.requireStatus(member, service.StatusDisabled, "department_manager")
	beforeDepartment, beforeVersion := s.current(member)

	// Re-upgrade operations must reset existing disabled sources before restoring
	// manager permissions. Restrict this fixture's maintenance SQL to its own user.
	s.f.mustExec(`UPDATE users SET disabled_source='other' WHERE status='disabled' AND id=$1`, member)
	s.f.requireStatus(member, service.StatusDisabled, "other")
	afterDepartment, afterVersion := s.current(member)
	require.Equal(t, beforeDepartment, afterDepartment)
	require.Equal(t, beforeVersion, afterVersion)
	require.Equal(t, "retain through source reset", s.f.note(member))

	in = s.expect(member)
	in.Status = departmentMemberString(service.StatusActive)
	_, err = s.svc.UpdateManagedMember(s.leaderCtx, member, in)
	require.ErrorIs(t, err, service.ErrDepartmentMemberEnableDenied)
	s.f.requireStatus(member, service.StatusDisabled, "other")

	users := NewUserRepository(integrationEntClient, integrationDB)
	user, err := users.GetByID(s.f.ctx, member)
	require.NoError(t, err)
	user.Status = service.StatusActive
	require.NoError(t, users.Update(s.f.ctx, user, service.UserUpdateFields{Status: true}))
	s.f.requireStatus(member, service.StatusActive, "")
	t.Log("legacy repeated ban retains manager source; explicit re-upgrade reset makes manager enable fail, preserves membership/note, and admin enable clears source")
}

func TestDepartmentMemberManagementIntegration_UserServiceReappliesSystemDisable(t *testing.T) {
	s := setupDepartmentMembers(t)
	member := s.f.user("user_service_disable", "xunyou.com", service.RoleUser, nil)
	s.place(member, s.granted)
	in := s.expect(member)
	in.Status = departmentMemberString(service.StatusDisabled)
	_, err := s.svc.UpdateManagedMember(s.leaderCtx, member, in)
	require.NoError(t, err)
	s.f.requireStatus(member, service.StatusDisabled, "department_manager")

	users := service.NewUserService(NewUserRepository(integrationEntClient, integrationDB), nil, nil, nil)
	require.NoError(t, users.UpdateStatus(s.adminCtx, member, service.StatusDisabled))
	s.f.requireStatus(member, service.StatusDisabled, "other")
	in.Status = departmentMemberString(service.StatusActive)
	_, err = s.svc.UpdateManagedMember(s.leaderCtx, member, in)
	require.ErrorIs(t, err, service.ErrDepartmentMemberEnableDenied)
	require.NoError(t, users.UpdateStatus(s.adminCtx, member, service.StatusActive))
	s.f.requireStatus(member, service.StatusActive, "")
}

func TestDepartmentMemberManagementIntegration_RemovalPreservesSubscriptionAndHidesMember(t *testing.T) {
	s := setupDepartmentMembers(t)
	member := s.f.user("subscription_member", "xunyou.com", service.RoleUser, nil)
	s.place(member, s.granted)
	var groupID, subscriptionID int64
	require.NoError(t, integrationDB.QueryRowContext(s.f.ctx,
		`INSERT INTO groups(name,platform,subscription_type) VALUES($1,'openai','subscription') RETURNING id`,
		s.f.prefix+"retained").Scan(&groupID))
	t.Cleanup(func() {
		_, err := integrationDB.ExecContext(context.Background(), `DELETE FROM user_subscriptions WHERE id=$1`, subscriptionID)
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(context.Background(), `DELETE FROM groups WHERE id=$1`, groupID)
		require.NoError(t, err)
	})
	require.NoError(t, integrationDB.QueryRowContext(s.f.ctx, `
INSERT INTO user_subscriptions(user_id,group_id,starts_at,expires_at,status,daily_usage_usd,weekly_usage_usd,monthly_usage_usd)
VALUES($1,$2,NOW()-INTERVAL '1 day',NOW()+INTERVAL '30 days','active',3,4,5) RETURNING id`,
		member, groupID).Scan(&subscriptionID))
	var beforeSubscription, afterSubscription string
	require.NoError(t, integrationDB.QueryRowContext(s.f.ctx,
		`SELECT to_jsonb(s)::text FROM user_subscriptions s WHERE id=$1`, subscriptionID).Scan(&beforeSubscription))
	filter := service.DepartmentListFilter{DepartmentID: strconv.FormatInt(s.granted, 10), Q: s.f.prefix + "subscription_member"}
	beforeList, err := s.svc.ManagedMembers(s.leaderCtx, filter)
	require.NoError(t, err)
	require.Len(t, beforeList.Items, 1)
	require.Equal(t, member, beforeList.Items[0].ID)

	changed, err := s.svc.ManagedAssign(s.leaderCtx, service.DepartmentAssignInput{Members: []service.DepartmentMemberChange{s.change(member)}})
	require.NoError(t, err)
	require.Equal(t, 1, changed)
	require.NoError(t, integrationDB.QueryRowContext(s.f.ctx,
		`SELECT to_jsonb(s)::text FROM user_subscriptions s WHERE id=$1`, subscriptionID).Scan(&afterSubscription))
	require.Equal(t, beforeSubscription, afterSubscription, "removal preserves every subscription field, including expiry and usage")
	s.f.requireStatus(member, service.StatusActive, "")
	afterList, err := s.svc.ManagedMembers(s.leaderCtx, filter)
	require.NoError(t, err)
	require.Empty(t, afterList.Items)
	require.Zero(t, afterList.Total)
}
