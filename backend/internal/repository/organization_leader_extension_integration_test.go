//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// Acceptance tests for the 2026-11-04 extension of
// docs/features/organization-leader-design-cn.md (§4.8 D11, §4.9 D12, §11).

func (s *organizationLeaderSetup) subscriptionService(t *testing.T) *service.SubscriptionService {
	t.Helper()
	subs := service.NewSubscriptionService(NewGroupRepository(integrationEntClient, integrationDB), NewUserSubscriptionRepository(integrationEntClient), nil, integrationEntClient, nil)
	t.Cleanup(subs.Stop)
	return subs
}

func (s *organizationLeaderSetup) subscriptionGroup(t *testing.T) int64 {
	t.Helper()
	var group int64
	require.NoError(t, integrationDB.QueryRowContext(s.f.ctx, `INSERT INTO groups(name,platform,subscription_type,daily_limit_usd) VALUES($1,'openai','subscription',20) RETURNING id`, s.f.prefix+"group").Scan(&group))
	t.Cleanup(func() {
		_, err := integrationDB.ExecContext(s.f.ctx, `DELETE FROM user_subscriptions WHERE group_id=$1 OR user_id=ANY($2)`, group, pq.Array(s.f.userIDs))
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(s.f.ctx, `DELETE FROM groups WHERE id=$1`, group)
		require.NoError(t, err)
	})
	return group
}

func (s *organizationLeaderSetup) subscriptionOf(t *testing.T, userID, group int64, includeDeleted bool) int64 {
	t.Helper()
	query := `SELECT COALESCE(MAX(id),0) FROM user_subscriptions WHERE user_id=$1 AND group_id=$2`
	if !includeDeleted {
		query += ` AND deleted_at IS NULL`
	}
	var id int64
	require.NoError(t, integrationDB.QueryRowContext(s.f.ctx, query, userID, group).Scan(&id))
	return id
}

// The admin middleware marks every sub-admin request; scoped writes depend on it.
func subAdminRequest(ctx context.Context) context.Context {
	return service.WithSubAdminAccountRestrictions(ctx)
}

func TestOrganizationLeaderIntegration_SubscriptionWrites(t *testing.T) {
	s := setupOrganizationLeaders(t)
	f := s.f
	leaderID, leaderCtx := s.organizationLeader("org", service.OrganizationXunyou)
	_, otherLeaderCtx := s.organizationLeader("peer", service.OrganizationXunyou)
	otherLeaderID := service.DepartmentActorID(otherLeaderCtx)
	dept := f.department(service.OrganizationXunyou)
	managerID, managerCtx := s.departmentLeader("manager", "xunyou.com", []int64{dept}, false, true, true)
	member := f.user("member", "xunyou.com", service.RoleUser, nil)
	unassigned := f.user("unassigned", "xunyou.com", service.RoleUser, nil)
	disabledUser := f.user("disabled", "xunyou.com", service.RoleUser, nil)
	admin := f.user("admin2", "xunyou.com", service.RoleAdmin, nil)
	outsider := f.user("outsider", "wsdashi.com", service.RoleUser, nil)
	operator := f.user("operator", "xunyou.com", service.RoleSubAdmin, []string{service.AdminPermissionAccountsOperator})
	f.mustExec(`UPDATE users SET department_id=$1 WHERE id=ANY($2)`, dept, pq.Array([]int64{member, managerID, admin}))
	f.mustExec(`UPDATE users SET status='disabled',disabled_source='other' WHERE id=$1`, disabledUser)
	group := s.subscriptionGroup(t)
	subs := s.subscriptionService(t)
	writeCtx := subAdminRequest(leaderCtx)
	assign := func(ctx context.Context, userID int64) error {
		_, err := subs.AssignSubscription(ctx, &service.AssignSubscriptionInput{UserID: userID, GroupID: group, ValidityDays: 30, AssignedBy: service.DepartmentActorID(ctx)})
		return err
	}

	// D11 write targets: the leader, active normal users and managed leaders.
	for name, c := range map[string]struct {
		userID  int64
		allowed bool
	}{
		"member":         {member, true},
		"unassigned":     {unassigned, true},
		"itself":         {leaderID, true},
		"managed leader": {managerID, true},
		"admin":          {admin, false},
		"peer leader":    {otherLeaderID, false},
		"unmanaged sub":  {operator, false},
		"other org user": {outsider, false},
		"disabled user":  {disabledUser, false},
		"missing user":   {999999999, false},
	} {
		err := assign(writeCtx, c.userID)
		if c.allowed {
			require.NoError(t, err, name)
		} else {
			require.ErrorIs(t, err, service.ErrDepartmentScopeDenied, name)
		}
	}
	require.Equal(t, 0, s.subscriptionCount(t, admin, group), "a rejected write leaves nothing behind")
	require.ErrorIs(t, assign(subAdminRequest(managerCtx), member), service.ErrDepartmentScopeDenied, "department leaders never write")

	var selfAudits int
	require.NoError(t, integrationDB.QueryRowContext(f.ctx, `SELECT COUNT(*) FROM audit_logs WHERE action='organization.subscription_assign' AND actor_user_id=$1 AND extra->>'self'='true' AND extra->>'actor_scope'='organization'`, leaderID).Scan(&selfAudits))
	require.Equal(t, 1, selfAudits)

	// G13: one out-of-scope target rejects the whole batch with no partial write.
	fresh := f.user("fresh", "xunyou.com", service.RoleUser, nil)
	_, err := subs.BulkAssignSubscription(writeCtx, &service.BulkAssignSubscriptionInput{UserIDs: []int64{fresh, admin}, GroupID: group, ValidityDays: 30})
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)
	require.Equal(t, 0, s.subscriptionCount(t, fresh, group))
	result, err := subs.BulkAssignSubscription(writeCtx, &service.BulkAssignSubscriptionInput{UserIDs: []int64{fresh, member}, GroupID: group, ValidityDays: 30})
	require.NoError(t, err)
	require.Equal(t, 2, result.SuccessCount)
	require.Equal(t, "created", result.Statuses[fresh])
	require.Equal(t, "reused", result.Statuses[member])

	// can_manage and assignment candidates use the same predicate (K14, G18).
	manageable, scoped, err := subs.ManageableSubscriptionUsers(writeCtx, []int64{member, leaderID, managerID, admin, otherLeaderID, disabledUser, outsider})
	require.NoError(t, err)
	require.True(t, scoped)
	require.Equal(t, map[int64]bool{member: true, leaderID: true, managerID: true}, manageable)
	manageable, scoped, err = subs.ManageableSubscriptionUsers(subAdminRequest(managerCtx), []int64{member})
	require.NoError(t, err)
	require.True(t, scoped)
	require.Empty(t, manageable)
	_, scoped, err = subs.ManageableSubscriptionUsers(s.adminCtx, []int64{member})
	require.NoError(t, err)
	require.False(t, scoped, "administrators keep unrestricted writes")
	candidates, err := s.svc.QueryScope(leaderCtx, service.AdminPermissionDepartmentSubscriptions, service.DepartmentScopeQuery{Q: f.prefix, Limit: 30, AssignTargets: true})
	require.NoError(t, err)
	candidateIDs := []int64{}
	for _, m := range candidates.Members {
		candidateIDs = append(candidateIDs, m.ID)
	}
	require.ElementsMatch(t, []int64{leaderID, managerID, member, unassigned, fresh}, candidateIDs)
	resetTargets, err := s.svc.QueryScope(leaderCtx, service.AdminPermissionDepartmentSubscriptions, service.DepartmentScopeQuery{Q: f.prefix, Limit: 30})
	require.NoError(t, err)
	require.Len(t, resetTargets.Members, 8, "without purpose the search still lists every reset target, admins included")
	_, err = s.svc.QueryScope(managerCtx, service.AdminPermissionDepartmentSubscriptions, service.DepartmentScopeQuery{Q: f.prefix, Limit: 30, AssignTargets: true})
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)

	// Extend, revoke and restore authorize the subscription owner; a missing
	// subscription is indistinguishable from an out-of-scope one (K16).
	memberSub := s.subscriptionOf(t, member, group, false)
	adminSub := s.seedSubscription(t, admin, group)
	_, err = subs.ExtendSubscription(writeCtx, memberSub, 5)
	require.NoError(t, err)
	_, err = subs.ExtendSubscription(writeCtx, adminSub, 5)
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)
	_, err = subs.ExtendSubscription(writeCtx, 999999999, 5)
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)
	require.ErrorIs(t, subs.RevokeSubscription(writeCtx, adminSub), service.ErrDepartmentScopeDenied)
	require.NoError(t, subs.RevokeSubscription(writeCtx, memberSub))
	require.ErrorIs(t, subs.RevokeSubscription(writeCtx, memberSub), service.ErrDepartmentScopeDenied, "an already revoked subscription is not found")
	_, err = subs.RestoreSubscription(writeCtx, memberSub)
	require.NoError(t, err)
	for _, action := range []string{"organization.subscription_extend", "organization.subscription_revoke", "organization.subscription_restore"} {
		require.Equal(t, 1, s.audits(action, leaderID), action)
	}

	// G17: the replay precheck follows current grants.
	require.NoError(t, subs.PrecheckSubscriptionWrite(writeCtx, memberSub))
	require.ErrorIs(t, subs.PrecheckSubscriptionWrite(writeCtx, adminSub), service.ErrDepartmentScopeDenied)
	f.mustExec(`UPDATE users SET role='admin',admin_permissions='[]' WHERE id=$1`, member)
	require.ErrorIs(t, subs.PrecheckSubscriptionWrite(writeCtx, memberSub), service.ErrDepartmentScopeDenied, "a target promoted after the first write is no longer writable")
	f.mustExec(`UPDATE users SET role='user' WHERE id=$1`, member)
	empty := []string{}
	_, err = s.svc.SetOrganizationAccess(s.adminCtx, leaderID, service.OrganizationAccessInput{Organizations: &empty, ExpectedVersion: s.access(leaderID).Version})
	require.NoError(t, err)
	require.ErrorIs(t, subs.PrecheckSubscriptionWrite(writeCtx, memberSub), service.ErrDepartmentScopeDenied, "revoked leaders cannot replay")
}

func (s *organizationLeaderSetup) subscriptionCount(t *testing.T, userID, group int64) int {
	t.Helper()
	var count int
	require.NoError(t, integrationDB.QueryRowContext(s.f.ctx, `SELECT COUNT(*) FROM user_subscriptions WHERE user_id=$1 AND group_id=$2`, userID, group).Scan(&count))
	return count
}

func (s *organizationLeaderSetup) seedSubscription(t *testing.T, userID, group int64) int64 {
	t.Helper()
	var id int64
	require.NoError(t, integrationDB.QueryRowContext(s.f.ctx, `INSERT INTO user_subscriptions(user_id,group_id,starts_at,expires_at,status)
VALUES($1,$2,NOW()-INTERVAL '1 day',NOW()+INTERVAL '30 days','active') RETURNING id`, userID, group).Scan(&id))
	return id
}

func TestOrganizationLeaderIntegration_UserManagement(t *testing.T) {
	s := setupOrganizationLeaders(t)
	f := s.f
	leaderID, leaderCtx := s.organizationLeader("org", service.OrganizationXunyou)
	dept := f.department(service.OrganizationXunyou)
	managerID, managerCtx := s.departmentLeader("manager", "xunyou.com", []int64{dept}, false, true, true)
	member := f.user("member", "xunyou.com", service.RoleUser, nil)
	victim := f.user("victim", "xunyou.com", service.RoleUser, nil)
	admin := f.user("admin2", "xunyou.com", service.RoleAdmin, nil)
	outsider := f.user("outsider", "wsdashi.com", service.RoleUser, nil)
	f.mustExec(`UPDATE users SET department_id=$1 WHERE id=ANY($2)`, dept, pq.Array([]int64{member, victim, managerID}))
	f.mustExec(`UPDATE users SET status='disabled',disabled_source='other' WHERE id=$1`, victim)
	t.Cleanup(func() {
		_, err := integrationDB.ExecContext(f.ctx, `DELETE FROM api_keys WHERE user_id=ANY($1)`, pq.Array(f.userIDs))
		require.NoError(t, err)
	})
	hash := func(id int64) string {
		var value string
		require.NoError(t, integrationDB.QueryRowContext(f.ctx, `SELECT password_hash FROM users WHERE id=$1`, id).Scan(&value))
		return value
	}

	// Password reset: organization normal users only, never the leader itself.
	require.ErrorIs(t, s.svc.ResetOrganizationUserPassword(leaderCtx, member, "short"), service.ErrDepartmentInvalid)
	require.NoError(t, s.svc.ResetOrganizationUserPassword(leaderCtx, member, "new-password-1"))
	updated := &service.User{PasswordHash: hash(member)}
	require.True(t, updated.CheckPassword("new-password-1"))
	require.Equal(t, 1, s.audits("organization.user_password_reset", leaderID))
	var leaked int
	require.NoError(t, integrationDB.QueryRowContext(f.ctx, `SELECT COUNT(*) FROM audit_logs WHERE actor_user_id=$1 AND extra::text LIKE '%new-password-1%'`, leaderID).Scan(&leaked))
	require.Zero(t, leaked, "the password never reaches the audit log")
	require.NoError(t, s.svc.ResetOrganizationUserPassword(leaderCtx, victim, "new-password-2"), "disabled users are manageable too")
	for name, c := range map[string]struct {
		ctx    context.Context
		target int64
	}{
		"leader itself":     {leaderCtx, leaderID},
		"department leader": {leaderCtx, managerID},
		"admin":             {leaderCtx, admin},
		"other org user":    {leaderCtx, outsider},
		"missing user":      {leaderCtx, 999999999},
		"by department":     {managerCtx, member},
		"by administrator":  {s.adminCtx, member},
	} {
		before := ""
		if c.target != 999999999 {
			before = hash(c.target)
		}
		require.ErrorIs(t, s.svc.ResetOrganizationUserPassword(c.ctx, c.target, "new-password-3"), service.ErrDepartmentScopeDenied, name)
		if before != "" {
			require.Equal(t, before, hash(c.target), name)
		}
	}

	// Deletion: CAS, API keys and audit commit together.
	key, err := integrationEntClient.APIKey.Create().SetUserID(victim).SetKey(f.prefix + "victim-key").SetName("k").Save(f.ctx)
	require.NoError(t, err)
	expect := s.expect(victim)
	stale := *expect.ExpectedDepartmentVersion + 1
	deleteInput := func(version *int64) service.OrganizationUserDeleteInput {
		return service.OrganizationUserDeleteInput{ExpectedDepartmentID: expect.ExpectedDepartmentID, ExpectedUnassigned: expect.ExpectedUnassigned, ExpectedDepartmentVersion: version}
	}
	require.ErrorIs(t, s.svc.DeleteOrganizationUser(leaderCtx, victim, service.OrganizationUserDeleteInput{ExpectedDepartmentVersion: expect.ExpectedDepartmentVersion}), service.ErrDepartmentInvalid, "an omitted department is not a confirmation")
	require.ErrorIs(t, s.svc.DeleteOrganizationUser(leaderCtx, victim, deleteInput(&stale)), service.ErrDepartmentConflict)
	require.ErrorIs(t, s.svc.DeleteOrganizationUser(leaderCtx, admin, deleteInput(expect.ExpectedDepartmentVersion)), service.ErrDepartmentScopeDenied)
	require.ErrorIs(t, s.svc.DeleteOrganizationUser(leaderCtx, managerID, s.deleteInput(managerID)), service.ErrDepartmentScopeDenied, "leaders are revoked before deletion")
	require.ErrorIs(t, s.svc.DeleteOrganizationUser(managerCtx, member, s.deleteInput(member)), service.ErrDepartmentScopeDenied)
	require.ErrorIs(t, s.svc.DeleteOrganizationUser(s.adminCtx, member, s.deleteInput(member)), service.ErrDepartmentScopeDenied, "administrators use /admin/users")

	// A failure in the last step (the audit) rolls back the keys and the user.
	function := "fail_org_delete_" + f.prefix[len("dept_member_"):len(f.prefix)-1]
	f.mustExec(`CREATE FUNCTION ` + function + `() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.action='organization.user_delete' THEN RAISE EXCEPTION 'injected audit failure'; END IF; RETURN NEW; END $$`)
	f.mustExec(`CREATE TRIGGER ` + function + ` BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION ` + function + `()`)
	dropped := false
	drop := func() {
		if !dropped {
			f.mustExec(`DROP TRIGGER ` + function + ` ON audit_logs`)
			f.mustExec(`DROP FUNCTION ` + function + `()`)
			dropped = true
		}
	}
	t.Cleanup(drop)
	require.Error(t, s.svc.DeleteOrganizationUser(leaderCtx, victim, deleteInput(expect.ExpectedDepartmentVersion)))
	drop()
	var userDeleted, keyDeleted bool
	require.NoError(t, integrationDB.QueryRowContext(f.ctx, `SELECT u.deleted_at IS NOT NULL,k.deleted_at IS NOT NULL FROM users u JOIN api_keys k ON k.user_id=u.id WHERE u.id=$1 AND k.id=$2`, victim, key.ID).Scan(&userDeleted, &keyDeleted))
	require.False(t, userDeleted)
	require.False(t, keyDeleted)

	require.NoError(t, s.svc.DeleteOrganizationUser(leaderCtx, victim, deleteInput(expect.ExpectedDepartmentVersion)))
	var keyValue string
	require.NoError(t, integrationDB.QueryRowContext(f.ctx, `SELECT u.deleted_at IS NOT NULL,k.deleted_at IS NOT NULL,k.key FROM users u JOIN api_keys k ON k.user_id=u.id WHERE u.id=$1 AND k.id=$2`, victim, key.ID).Scan(&userDeleted, &keyDeleted, &keyValue))
	require.True(t, userDeleted)
	require.True(t, keyDeleted)
	require.NotEqual(t, key.Key, keyValue, "the key value is tombstoned")
	require.Equal(t, 1, s.audits("organization.user_delete", leaderID))
	require.ErrorIs(t, s.svc.DeleteOrganizationUser(leaderCtx, victim, deleteInput(expect.ExpectedDepartmentVersion)), service.ErrDepartmentScopeDenied, "a deleted user is not found")

	// §7: a promotion that commits first makes the deletion fail with 403.
	f.mustExec(`UPDATE users SET role='sub_admin' WHERE id=$1`, member)
	require.ErrorIs(t, s.svc.DeleteOrganizationUser(leaderCtx, member, s.deleteInput(member)), service.ErrDepartmentScopeDenied)
}

func (s *organizationLeaderSetup) deleteInput(userID int64) service.OrganizationUserDeleteInput {
	in := s.expect(userID)
	return service.OrganizationUserDeleteInput{ExpectedDepartmentID: in.ExpectedDepartmentID, ExpectedUnassigned: in.ExpectedUnassigned, ExpectedDepartmentVersion: in.ExpectedDepartmentVersion}
}

// changeRole commits a role change through the real administrator write path,
// which locks the user row like every grant and membership writer.
func (s *organizationLeaderSetup) changeRole(t *testing.T, userID int64, role string, permissions []string) {
	t.Helper()
	users := NewUserRepository(integrationEntClient, integrationDB)
	editor, err := users.GetByIDWithAdminAccess(s.adminCtx, userID)
	require.NoError(t, err)
	editor.Role = role
	editor.AdminPermissions = permissions
	require.NoError(t, users.Update(s.adminCtx, editor, service.UserUpdateFields{Role: true, AdminPermissions: true, ExpectedAdminAccessVersion: editor.AdminAccessVersion}))
}

// tryChangeRole is changeRole for goroutines, which must not call FailNow.
func (s *organizationLeaderSetup) tryChangeRole(userID int64, role string) error {
	users := NewUserRepository(integrationEntClient, integrationDB)
	editor, err := users.GetByIDWithAdminAccess(s.adminCtx, userID)
	if err != nil {
		return err
	}
	editor.Role = role
	editor.AdminPermissions = nil
	return users.Update(s.adminCtx, editor, service.UserUpdateFields{Role: true, AdminPermissions: true, ExpectedAdminAccessVersion: editor.AdminAccessVersion})
}

// K19: a request admitted as a sub-admin never becomes an unrestricted write
// when the actor is demoted or disabled after the middleware check.
func TestOrganizationLeaderIntegration_DemotedActorIsNeverUnrestricted(t *testing.T) {
	s := setupOrganizationLeaders(t)
	leaderID, leaderCtx := s.organizationLeader("org", service.OrganizationXunyou)
	requestCtx := subAdminRequest(leaderCtx)
	member := s.f.user("member", "xunyou.com", service.RoleUser, nil)
	outsider := s.f.user("outsider", "wsdashi.com", service.RoleUser, nil)
	group := s.subscriptionGroup(t)
	subs := s.subscriptionService(t)
	memberSub := s.seedSubscription(t, member, group)

	s.changeRole(t, leaderID, service.RoleUser, nil)
	_, err := subs.AssignSubscription(requestCtx, &service.AssignSubscriptionInput{UserID: outsider, GroupID: group, ValidityDays: 30, AssignedBy: leaderID})
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)
	require.Zero(t, s.subscriptionCount(t, outsider, group), "a denied write leaves nothing behind")
	_, err = subs.BulkAssignSubscription(requestCtx, &service.BulkAssignSubscriptionInput{UserIDs: []int64{outsider}, GroupID: group, ValidityDays: 30})
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)
	require.Zero(t, s.subscriptionCount(t, outsider, group))
	require.ErrorIs(t, subs.PrecheckSubscriptionWrite(requestCtx, memberSub), service.ErrDepartmentScopeDenied, "the replay precheck follows the demotion")
	require.ErrorIs(t, subs.RevokeSubscription(requestCtx, memberSub), service.ErrDepartmentScopeDenied)
	_, _, err = subs.ManageableSubscriptionUsers(requestCtx, []int64{member})
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)

	disabledID, disabledCtx := s.organizationLeader("disabled", service.OrganizationXunyou)
	s.f.mustExec(`UPDATE users SET status='disabled',disabled_source='other' WHERE id=$1`, disabledID)
	_, err = subs.AssignSubscription(subAdminRequest(disabledCtx), &service.AssignSubscriptionInput{UserID: outsider, GroupID: group, ValidityDays: 30})
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)
	require.Zero(t, s.subscriptionCount(t, outsider, group))
}

// lockObservingRepository runs a callback right after the batch has locked and
// authorized its targets, while the batch transaction is still open.
type lockObservingRepository struct {
	service.UserSubscriptionRepository
	service.SubscriptionWriteScopeRepository
	afterLock func()
}

func (r *lockObservingRepository) AuthorizeSubscriptionWriteUsers(ctx context.Context, ids []int64, lock bool) (context.Context, error) {
	next, err := r.SubscriptionWriteScopeRepository.AuthorizeSubscriptionWriteUsers(ctx, ids, lock)
	if err == nil && lock && len(ids) > 1 && r.afterLock != nil {
		callback := r.afterLock
		r.afterLock = nil
		callback()
	}
	return next, err
}

// K20 / G13: a bulk assignment authorizes every target under locks held until
// commit; an out-of-scope target rejects the batch with no partial write.
func TestOrganizationLeaderIntegration_BulkAssignLocksWholeBatch(t *testing.T) {
	s := setupOrganizationLeaders(t)
	f := s.f
	leaderID, leaderCtx := s.organizationLeader("org", service.OrganizationXunyou)
	writeCtx := subAdminRequest(leaderCtx)
	first := f.user("first", "xunyou.com", service.RoleUser, nil)
	second := f.user("second", "xunyou.com", service.RoleUser, nil)
	third := f.user("third", "xunyou.com", service.RoleUser, nil)
	group := s.subscriptionGroup(t)
	repo := NewUserSubscriptionRepository(integrationEntClient)
	observer := &lockObservingRepository{UserSubscriptionRepository: repo, SubscriptionWriteScopeRepository: repo.(service.SubscriptionWriteScopeRepository)}
	subs := service.NewSubscriptionService(NewGroupRepository(integrationEntClient, integrationDB), observer, nil, integrationEntClient, nil)
	t.Cleanup(subs.Stop)
	bulk := func(ids ...int64) (*service.BulkAssignResult, error) {
		return subs.BulkAssignSubscription(writeCtx, &service.BulkAssignSubscriptionInput{UserIDs: ids, GroupID: group, ValidityDays: 30})
	}

	// A promotion committed before the batch rejects all of it.
	s.changeRole(t, second, service.RoleAdmin, nil)
	_, err := bulk(first, second)
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)
	require.Zero(t, s.subscriptionCount(t, first, group))
	s.changeRole(t, second, service.RoleUser, nil)

	// A promotion attempted after the batch locked its targets waits for the
	// batch to commit, so the batch never writes for a target that left scope.
	promoted := make(chan struct{})
	var promoteErr error
	observer.afterLock = func() {
		go func() {
			defer close(promoted)
			promoteErr = s.tryChangeRole(second, service.RoleAdmin)
		}()
		select {
		case <-promoted:
			t.Error("a role change must wait for the batch transaction")
		case <-time.After(300 * time.Millisecond):
		}
	}
	result, err := bulk(first, second)
	require.NoError(t, err)
	require.Equal(t, 2, result.SuccessCount)
	select {
	case <-promoted:
	case <-time.After(10 * time.Second):
		t.Fatal("the role change did not resume after the batch committed")
	}
	require.NoError(t, promoteErr)
	s.changeRole(t, second, service.RoleUser, nil)

	// A business failure of one target is undone by its savepoint and reported
	// for that target only; the other targets commit with their audits.
	function := "fail_org_assign_" + f.prefix[len("dept_member_"):len(f.prefix)-1]
	f.mustExec(fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.action='organization.subscription_assign' AND NEW.extra->>'user_id'='%d' THEN RAISE EXCEPTION 'injected audit failure'; END IF; RETURN NEW; END $$`, function, third))
	f.mustExec(`CREATE TRIGGER ` + function + ` BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION ` + function + `()`)
	t.Cleanup(func() {
		f.mustExec(`DROP TRIGGER IF EXISTS ` + function + ` ON audit_logs`)
		f.mustExec(`DROP FUNCTION IF EXISTS ` + function + `()`)
	})
	fresh := f.user("fresh", "xunyou.com", service.RoleUser, nil)
	before := s.audits("organization.subscription_assign", leaderID)
	result, err = bulk(third, fresh)
	require.NoError(t, err)
	require.Equal(t, 1, result.SuccessCount)
	require.Equal(t, 1, result.FailedCount)
	require.Equal(t, "failed", result.Statuses[third])
	require.Equal(t, "created", result.Statuses[fresh])
	require.Zero(t, s.subscriptionCount(t, third, group), "the failed target's subscription is rolled back with its audit")
	require.Equal(t, 1, s.subscriptionCount(t, fresh, group))
	require.Equal(t, before+1, s.audits("organization.subscription_assign", leaderID))
}
