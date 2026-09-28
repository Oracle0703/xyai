//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestDepartmentSubscriptionsIntegration_ReadAndResetIsolation(t *testing.T) {
	ctx := context.Background()
	prefix := organizationUsageIntegrationPrefix("dept_quota")
	cleanupOrganizationUsageIntegrationData(t, prefix)
	admin, _ := organizationUsageIntegrationUser(t, prefix+"admin@example.com", service.StatusActive)
	leader, _ := organizationUsageIntegrationUser(t, prefix+"leader@xunyou.com", service.StatusActive)
	member, _ := organizationUsageIntegrationUser(t, prefix+"member@xunyou.com", service.StatusActive)
	outsider, _ := organizationUsageIntegrationUser(t, prefix+"outsider@wsdashi.com", service.StatusActive)
	_, err := integrationDB.ExecContext(ctx, `UPDATE users SET role='admin' WHERE id=$1`, admin.ID)
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
	groups := []int64{}
	for _, platform := range []string{"openai", "anthropic", "grok"} {
		var id int64
		err = integrationDB.QueryRowContext(ctx, `INSERT INTO groups(name,platform,subscription_type,daily_limit_usd) VALUES($1,$2,'subscription',20) RETURNING id`, prefix+platform, platform).Scan(&id)
		require.NoError(t, err)
		groups = append(groups, id)
	}
	t.Cleanup(func() {
		_, e := integrationDB.ExecContext(ctx, `DELETE FROM user_subscriptions WHERE group_id=ANY($1)`, pq.Array(groups))
		require.NoError(t, e)
		_, e = integrationDB.ExecContext(ctx, `DELETE FROM groups WHERE id=ANY($1)`, pq.Array(groups))
		require.NoError(t, e)
		_, e = integrationDB.ExecContext(ctx, `DELETE FROM department_access_grants WHERE user_id=$1`, leader.ID)
		require.NoError(t, e)
		_, e = integrationDB.ExecContext(ctx, `UPDATE users SET department_id=NULL WHERE department_id=ANY($1)`, pq.Array([]int64{dept.ID, other.ID}))
		require.NoError(t, e)
		_, e = integrationDB.ExecContext(ctx, `DELETE FROM departments WHERE id=ANY($1)`, pq.Array([]int64{dept.ID, other.ID}))
		require.NoError(t, e)
		_, e = integrationDB.ExecContext(ctx, `DELETE FROM audit_logs WHERE actor_user_id=ANY($1)`, pq.Array([]int64{admin.ID, leader.ID}))
		require.NoError(t, e)
	})
	_, err = departments.Assign(adminCtx, service.DepartmentAssignInput{DepartmentID: &dept.ID, Members: []service.DepartmentMemberChange{{UserID: member.ID}}})
	require.NoError(t, err)
	_, err = departments.Assign(adminCtx, service.DepartmentAssignInput{DepartmentID: &other.ID, Members: []service.DepartmentMemberChange{{UserID: outsider.ID}}})
	require.NoError(t, err)
	access, err := departments.GetAccess(adminCtx, leader.ID)
	require.NoError(t, err)
	access, err = departments.SetAccess(adminCtx, leader.ID, service.DepartmentAccessInput{DepartmentIDs: []int64{dept.ID}, Report: true, ResetQuota: true, ExpectedVersion: access.Version})
	require.NoError(t, err)
	createSub := func(userID, groupID int64) int64 {
		var id int64
		e := integrationDB.QueryRowContext(ctx, `INSERT INTO user_subscriptions(user_id,group_id,starts_at,expires_at,status,daily_usage_usd,weekly_usage_usd,monthly_usage_usd) VALUES($1,$2,NOW()-INTERVAL '1 day',NOW()+INTERVAL '30 days','active',3,4,5) RETURNING id`, userID, groupID).Scan(&id)
		require.NoError(t, e)
		return id
	}
	mine := createSub(member.ID, groups[0])
	second := createSub(member.ID, groups[1])
	foreign := createSub(outsider.ID, groups[0])
	createSub(outsider.ID, groups[2])
	options, err := departments.SubscriptionGroups(leaderCtx, "")
	require.NoError(t, err)
	optionIDs := make([]int64, 0, len(options))
	for _, option := range options {
		optionIDs = append(optionIDs, option.ID)
	}
	require.ElementsMatch(t, groups[:2], optionIDs, "compact options must not disclose a group used only by another department")
	options, err = departments.SubscriptionGroups(leaderCtx, prefix+"grok")
	require.NoError(t, err)
	require.Empty(t, options, "search must not broaden the granted scope")
	repo := NewUserSubscriptionRepository(integrationEntClient)
	svc := service.NewSubscriptionService(NewGroupRepository(integrationEntClient, integrationDB), repo, nil, integrationEntClient, nil)
	t.Cleanup(svc.Stop)
	subs, pag, err := repo.ListAdmin(leaderCtx, pagination.PaginationParams{Page: 1, PageSize: 20}, service.SubscriptionAdminFilter{SortBy: "created_at", SortOrder: "desc"}, time.Now())
	require.NoError(t, err)
	require.Len(t, subs, 2)
	require.Equal(t, int64(2), pag.Total)
	_, err = svc.GetByID(leaderCtx, foreign)
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)
	_, err = svc.ListUserSubscriptions(leaderCtx, outsider.ID)
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)
	_, err = svc.AdminResetQuota(leaderCtx, foreign, true, true, true)
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)
	_, err = svc.AdminResetQuota(leaderCtx, mine, true, true, true)
	require.NoError(t, err)
	var daily, weekly, monthly float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT daily_usage_usd,weekly_usage_usd,monthly_usage_usd FROM user_subscriptions WHERE id=$1`, mine).Scan(&daily, &weekly, &monthly))
	require.Zero(t, daily)
	require.Zero(t, weekly)
	require.Zero(t, monthly)
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT daily_usage_usd FROM user_subscriptions WHERE id=$1`, second).Scan(&daily))
	require.Equal(t, 3.0, daily)
	scope, err := departments.QueryScope(leaderCtx, service.AdminPermissionDepartmentSubscriptions, service.DepartmentScopeQuery{GroupID: &groups[1]})
	require.NoError(t, err)
	_, err = svc.AdminResetDailyFiltered(leaderCtx, service.SubscriptionAdminFilter{GroupID: &groups[1]})
	require.ErrorIs(t, err, service.ErrDepartmentScopeChanged)
	count, err := svc.AdminResetDailyFiltered(leaderCtx, service.SubscriptionAdminFilter{GroupID: &groups[1], ScopeVersion: scope.Version})
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT daily_usage_usd,weekly_usage_usd FROM user_subscriptions WHERE id=$1`, second).Scan(&daily, &weekly))
	require.Zero(t, daily)
	require.Equal(t, 4.0, weekly)
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT daily_usage_usd FROM user_subscriptions WHERE id=$1`, foreign).Scan(&daily))
	require.Equal(t, 3.0, daily)
	access, err = departments.SetAccess(adminCtx, leader.ID, service.DepartmentAccessInput{DepartmentIDs: []int64{dept.ID}, ResetQuota: true, ExpectedVersion: access.Version})
	require.NoError(t, err)
	_, err = svc.AdminResetQuota(leaderCtx, mine, true, false, false)
	require.NoError(t, err, "revoking report permission must not remove separately granted reset capability")
	// Revocation must wait for an already authorized reset's user locks.
	concrete := repo.(*userSubscriptionRepository)
	// Exercise the shared row-lock protocol against real concurrent writers.
	runDuringReset := func(action func() error, queryPattern string) {
		t.Helper()
		lockedCtx, _, lockedTx, lockErr := concrete.beginDepartmentSubscriptionWrite(leaderCtx, service.SubscriptionAdminFilter{}, mine, time.Now())
		require.NoError(t, lockErr)
		defer func() { _ = lockedTx.Rollback() }()
		done := make(chan error, 1)
		go func() { done <- action() }()
		require.Eventually(t, func() bool {
			var blocked int
			e := integrationDB.QueryRowContext(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE $1`, queryPattern).Scan(&blocked)
			return e == nil && blocked > 0
		}, 5*time.Second, 10*time.Millisecond)
		require.NoError(t, repo.ResetUsageWindows(lockedCtx, mine, true, false, false, time.Now(), time.Now()))
		require.NoError(t, lockedTx.Commit())
		select {
		case e := <-done:
			require.NoError(t, e)
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent writer did not finish after reset released locks")
		}
	}
	var balanceBefore, balanceAfter float64
	require.NoError(t, integrationDB.QueryRow(`SELECT balance FROM users WHERE id=$1`, member.ID).Scan(&balanceBefore))
	runDuringReset(func() error {
		tx, e := integrationDB.BeginTx(ctx, nil)
		if e != nil {
			return e
		}
		defer func() { _ = tx.Rollback() }()
		_, _, e = deductUsageBillingBalance(ctx, tx, member.ID, 2)
		if e != nil {
			return e
		}
		return tx.Commit()
	}, "%SET balance = balance -%")
	require.NoError(t, integrationDB.QueryRow(`SELECT balance FROM users WHERE id=$1`, member.ID).Scan(&balanceAfter))
	require.Equal(t, balanceBefore-2, balanceAfter, "quota reset must not lose concurrent balance billing")
	runDuringReset(func() error {
		_, e := departments.Assign(adminCtx, service.DepartmentAssignInput{Members: []service.DepartmentMemberChange{{UserID: member.ID, ExpectedDepartmentID: &dept.ID, ExpectedVersion: 1}}})
		return e
	}, "%FROM users WHERE id=ANY%")
	_, err = svc.AdminResetQuota(leaderCtx, mine, true, false, false)
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)
	_, err = departments.Assign(adminCtx, service.DepartmentAssignInput{DepartmentID: &dept.ID, Members: []service.DepartmentMemberChange{{UserID: member.ID, ExpectedVersion: 2}}})
	require.NoError(t, err)
	runDuringReset(func() error {
		_, e := integrationDB.Exec(`UPDATE users SET email=$1 WHERE id=$2`, prefix+"moved@wsdashi.com", member.ID)
		return e
	}, "%UPDATE users SET email=%")
	_, err = svc.AdminResetQuota(leaderCtx, mine, true, false, false)
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)
	_, err = integrationDB.Exec(`UPDATE users SET email=$1 WHERE id=$2`, member.Email, member.ID)
	require.NoError(t, err)
	_, err = departments.Assign(adminCtx, service.DepartmentAssignInput{DepartmentID: &dept.ID, Members: []service.DepartmentMemberChange{{UserID: member.ID, ExpectedVersion: 4}}})
	require.NoError(t, err)
	runDuringReset(func() error {
		_, e := integrationDB.Exec(`UPDATE users SET status='disabled' WHERE id=$1`, leader.ID)
		return e
	}, "%UPDATE users SET status=%")
	_, err = svc.AdminResetQuota(leaderCtx, mine, true, false, false)
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)
	_, err = integrationDB.Exec(`UPDATE users SET status='active' WHERE id=$1`, leader.ID)
	require.NoError(t, err)
	users, ok := NewUserRepository(integrationEntClient, integrationDB).(*userRepository)
	require.True(t, ok)
	edit, err := users.GetByIDWithAdminAccess(adminCtx, leader.ID)
	require.NoError(t, err)
	edit.Role, edit.AdminPermissions = service.RoleUser, []string{}
	runDuringReset(func() error {
		return users.Update(adminCtx, edit, service.UserUpdateFields{Role: true, AdminPermissions: true, ExpectedAdminAccessVersion: edit.AdminAccessVersion})
	}, "%FROM users WHERE id=ANY%")
	_, err = svc.AdminResetQuota(leaderCtx, mine, true, false, false)
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)
	edit.Role, edit.AdminPermissions = service.RoleSubAdmin, []string{service.AdminPermissionDepartmentSubscriptions}
	require.NoError(t, users.Update(adminCtx, edit, service.UserUpdateFields{Role: true, AdminPermissions: true, ExpectedAdminAccessVersion: edit.AdminAccessVersion}))
	access, err = departments.GetAccess(adminCtx, leader.ID)
	require.NoError(t, err)
	require.Empty(t, access.DepartmentIDs)
	access, err = departments.SetAccess(adminCtx, leader.ID, service.DepartmentAccessInput{DepartmentIDs: []int64{dept.ID}, ResetQuota: true, ExpectedVersion: access.Version})
	require.NoError(t, err)
	writeCtx, _, owned, err := concrete.beginDepartmentSubscriptionWrite(leaderCtx, service.SubscriptionAdminFilter{}, mine, time.Now())
	require.NoError(t, err)
	require.NotNil(t, owned)
	t.Cleanup(func() { _ = owned.Rollback() })
	revoked := make(chan error, 1)
	go func() {
		_, revokeErr := departments.SetAccess(adminCtx, leader.ID, service.DepartmentAccessInput{DepartmentIDs: []int64{}, ResetQuota: true, ExpectedVersion: access.Version})
		revoked <- revokeErr
	}()
	require.Eventually(t, func() bool {
		var blocked int
		e := integrationDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%FROM users WHERE id=ANY%'`).Scan(&blocked)
		return e == nil && blocked > 0
	}, 5*time.Second, 10*time.Millisecond)
	require.NoError(t, repo.ResetUsageWindows(writeCtx, mine, true, false, false, time.Now(), time.Now()))
	require.NoError(t, owned.Commit())
	select {
	case err = <-revoked:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("revocation did not complete after reset transaction committed")
	}

	_, err = svc.AdminResetQuota(leaderCtx, mine, true, false, false)
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)
	_, _, err = repo.ListAdmin(leaderCtx, pagination.PaginationParams{Page: 1, PageSize: 20}, service.SubscriptionAdminFilter{}, time.Now())
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)
}
