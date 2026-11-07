//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// Acceptance tests for group organization scopes,
// docs/features/organization-leader-design-cn.md D13.

func groupIDs(groups []service.Group) []int64 {
	ids := make([]int64, 0, len(groups))
	for _, g := range groups {
		ids = append(ids, g.ID)
	}
	return ids
}

func TestOrganizationLeaderIntegration_GroupOrganizationScopes(t *testing.T) {
	s := setupOrganizationLeaders(t)
	f := s.f
	leaderID, leaderCtx := s.organizationLeader("org", service.OrganizationXunyou)
	writeCtx := subAdminRequest(leaderCtx)
	member := f.user("member", "xunyou.com", service.RoleUser, nil)
	fresh := f.user("fresh", "xunyou.com", service.RoleUser, nil)
	outsider := f.user("outsider", "wsdashi.com", service.RoleUser, nil)
	scoped := s.subscriptionGroup(t)
	foreign := s.scopedSubscriptionGroup(t, "foreign")
	unscoped := s.scopedSubscriptionGroup(t, "unscoped")
	subs := s.subscriptionService(t)
	keys := func(values ...string) *[]string { return &values }
	setScopes := func(ctx context.Context, ids []int64, organizations *[]string) error {
		_, err := subs.SetGroupOrganizationScopes(ctx, service.GroupOrganizationScopeInput{GroupIDs: ids, OrganizationKeys: organizations})
		return err
	}
	assign := func(userID, group int64) error {
		_, err := subs.AssignSubscription(writeCtx, &service.AssignSubscriptionInput{UserID: userID, GroupID: group, ValidityDays: 30, AssignedBy: leaderID})
		return err
	}

	// Administrators replace scopes; the input must be explicit and valid.
	updated, err := subs.SetGroupOrganizationScopes(s.adminCtx, service.GroupOrganizationScopeInput{GroupIDs: []int64{foreign, foreign}, OrganizationKeys: keys("wsdashi", "wsdashi")})
	require.NoError(t, err)
	require.Equal(t, 1, updated, "duplicate IDs are counted once")
	for name, input := range map[string]service.GroupOrganizationScopeInput{
		"omitted organizations": {GroupIDs: []int64{foreign}},
		"other is not a scope":  {GroupIDs: []int64{foreign}, OrganizationKeys: keys(service.OrganizationOther)},
		"no groups":             {OrganizationKeys: keys("xunyou")},
		"invalid group id":      {GroupIDs: []int64{0}, OrganizationKeys: keys("xunyou")},
	} {
		_, err = subs.SetGroupOrganizationScopes(s.adminCtx, input)
		require.ErrorIs(t, err, service.ErrGroupOrganizationScopeInvalid, name)
	}
	require.ErrorIs(t, setScopes(s.adminCtx, []int64{foreign, 999999999}, keys("xunyou")), service.ErrGroupNotFound)
	require.ErrorIs(t, setScopes(writeCtx, []int64{scoped}, keys("xunyou", "wsdashi")), service.ErrDepartmentScopeDenied, "organization leaders never edit scopes")
	_, err = subs.ListGroupOrganizationScopes(writeCtx)
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)
	scopes, err := subs.ListGroupOrganizationScopes(s.adminCtx)
	require.NoError(t, err)
	byGroup := map[int64][]string{}
	for _, item := range scopes {
		byGroup[item.GroupID] = item.OrganizationKeys
	}
	require.Equal(t, []string{"xunyou"}, byGroup[scoped])
	require.Equal(t, []string{"wsdashi"}, byGroup[foreign], "a failed replacement leaves the previous scope")
	require.NotContains(t, byGroup, unscoped)

	// The picker and the write check use the same predicate (K14).
	groups, err := subs.AssignableSubscriptionGroups(writeCtx)
	require.NoError(t, err)
	require.Contains(t, groupIDs(groups), scoped)
	require.NotContains(t, groupIDs(groups), foreign)
	require.NotContains(t, groupIDs(groups), unscoped)
	groups, err = subs.AssignableSubscriptionGroups(s.adminCtx)
	require.NoError(t, err)
	require.Subset(t, groupIDs(groups), []int64{scoped, foreign, unscoped}, "administrators assign every group")

	// Assignment outside the leader's organizations is rejected with no write.
	for name, group := range map[string]int64{"other organization": foreign, "unscoped": unscoped, "missing": 999999999} {
		require.ErrorIs(t, assign(member, group), service.ErrSubscriptionGroupScopeDenied, name)
		require.Zero(t, s.subscriptionCount(t, member, group), name)
	}
	require.ErrorIs(t, assign(outsider, foreign), service.ErrDepartmentScopeDenied, "the owner is checked before the group")
	_, err = subs.BulkAssignSubscription(writeCtx, &service.BulkAssignSubscriptionInput{UserIDs: []int64{member, fresh}, GroupID: foreign, ValidityDays: 30})
	require.ErrorIs(t, err, service.ErrSubscriptionGroupScopeDenied)
	require.Zero(t, s.subscriptionCount(t, member, foreign)+s.subscriptionCount(t, fresh, foreign))
	require.NoError(t, assign(member, scoped))

	// Subscriptions an administrator gave in other groups stay read-only.
	foreignSub := s.seedSubscription(t, fresh, foreign)
	revokedSub := s.seedSubscription(t, member, foreign)
	f.mustExec(`UPDATE user_subscriptions SET deleted_at=NOW() WHERE id=$1`, revokedSub)
	_, err = subs.ExtendSubscription(writeCtx, foreignSub, 5)
	require.ErrorIs(t, err, service.ErrSubscriptionGroupScopeDenied)
	require.ErrorIs(t, subs.PrecheckSubscriptionWrite(writeCtx, foreignSub), service.ErrSubscriptionGroupScopeDenied)
	require.ErrorIs(t, subs.RevokeSubscription(writeCtx, foreignSub), service.ErrSubscriptionGroupScopeDenied)
	require.Equal(t, foreignSub, s.subscriptionOf(t, fresh, foreign, false))
	_, err = subs.RestoreSubscription(writeCtx, revokedSub)
	require.ErrorIs(t, err, service.ErrSubscriptionGroupScopeDenied)
	require.Zero(t, s.subscriptionOf(t, member, foreign, false))
	access, err := subs.ManageableSubscriptions(writeCtx, []int64{member, fresh}, []int64{scoped, foreign, unscoped})
	require.NoError(t, err)
	require.True(t, access.Allows(member, scoped))
	require.False(t, access.Allows(fresh, foreign))
	require.False(t, access.Allows(member, unscoped))
	for _, action := range []string{"organization.subscription_extend", "organization.subscription_revoke", "organization.subscription_restore"} {
		require.Zero(t, s.audits(action, leaderID), action)
	}

	// Scope changes apply to the next write; [] clears a scope.
	require.NoError(t, setScopes(s.adminCtx, []int64{foreign, unscoped}, keys("wsdashi", "xunyou")))
	_, err = subs.ExtendSubscription(writeCtx, foreignSub, 5)
	require.NoError(t, err)
	require.NoError(t, assign(member, unscoped))
	require.NoError(t, setScopes(s.adminCtx, []int64{foreign, unscoped}, keys()))
	require.ErrorIs(t, assign(fresh, unscoped), service.ErrSubscriptionGroupScopeDenied)
	require.ErrorIs(t, subs.RevokeSubscription(writeCtx, foreignSub), service.ErrSubscriptionGroupScopeDenied)

	// A scope replacement holds the group FOR UPDATE; an assignment started
	// meanwhile waits and then reads the committed scope, never the old one.
	tx, err := integrationDB.BeginTx(f.ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(f.ctx, `SELECT id FROM groups WHERE id=$1 FOR UPDATE`, scoped)
	require.NoError(t, err)
	_, err = tx.ExecContext(f.ctx, `DELETE FROM group_organization_scopes WHERE group_id=$1`, scoped)
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() { done <- assign(fresh, scoped) }()
	select {
	case err = <-done:
		t.Fatalf("the assignment must wait for the scope replacement, got %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	require.NoError(t, tx.Commit())
	select {
	case err = <-done:
		require.ErrorIs(t, err, service.ErrSubscriptionGroupScopeDenied)
	case <-time.After(10 * time.Second):
		t.Fatal("the assignment did not resume after the scope replacement committed")
	}
	require.Zero(t, s.subscriptionCount(t, fresh, scoped))
}

// Scopes only apply to subscription groups: a group that becomes standard loses
// its scope (migration 264), so switching it back does not silently re-grant
// organization leaders. A deleted group of a revoked subscription is reported
// as deleted, not as a scope denial. The picker query returns only the scoped
// active subscription groups, in group order, with the picker fields.
func TestOrganizationLeaderIntegration_GroupOrganizationScopeLifecycle(t *testing.T) {
	s := setupOrganizationLeaders(t)
	f := s.f
	leaderID, leaderCtx := s.organizationLeader("life", service.OrganizationXunyou)
	writeCtx := subAdminRequest(leaderCtx)
	member := f.user("member", "xunyou.com", service.RoleUser, nil)
	subs := s.subscriptionService(t)
	keys := func(values ...string) *[]string { return &values }
	setScopes := func(ids []int64, organizations *[]string) error {
		_, err := subs.SetGroupOrganizationScopes(s.adminCtx, service.GroupOrganizationScopeInput{GroupIDs: ids, OrganizationKeys: organizations})
		return err
	}
	assign := func(group int64) error {
		_, err := subs.AssignSubscription(writeCtx, &service.AssignSubscriptionInput{UserID: member, GroupID: group, ValidityDays: 30, AssignedBy: leaderID})
		return err
	}
	scopeCount := func(group int64) int {
		var n int
		require.NoError(t, integrationDB.QueryRowContext(f.ctx, `SELECT COUNT(*) FROM group_organization_scopes WHERE group_id=$1`, group).Scan(&n))
		return n
	}

	first := s.scopedSubscriptionGroup(t, "first", service.OrganizationXunyou, service.OrganizationWsdashi)
	second := s.scopedSubscriptionGroup(t, "second", service.OrganizationXunyou)
	paused := s.scopedSubscriptionGroup(t, "paused", service.OrganizationXunyou)
	f.mustExec(`UPDATE groups SET sort_order=2, description='first plan' WHERE id=$1`, first)
	f.mustExec(`UPDATE groups SET sort_order=1 WHERE id=$1`, second)
	f.mustExec(`UPDATE groups SET status='disabled' WHERE id=$1`, paused)
	groups, err := subs.AssignableSubscriptionGroups(writeCtx)
	require.NoError(t, err)
	ours := []service.Group{}
	for _, g := range groups {
		if g.ID == first || g.ID == second || g.ID == paused {
			ours = append(ours, g)
		}
	}
	require.Equal(t, []int64{second, first}, groupIDs(ours), "group order, inactive groups excluded")
	require.Equal(t, f.prefix+"first", ours[1].Name)
	require.Equal(t, "first plan", ours[1].Description)
	require.Equal(t, "openai", ours[1].Platform)
	require.Equal(t, service.SubscriptionTypeSubscription, ours[1].SubscriptionType)
	require.Equal(t, service.StatusActive, ours[1].Status)
	require.Equal(t, 1.0, ours[1].RateMultiplier)

	// Becoming standard clears the scope; turning back does not restore it.
	f.mustExec(`UPDATE groups SET subscription_type='standard' WHERE id=$1`, first)
	require.Zero(t, scopeCount(first))
	f.mustExec(`UPDATE groups SET subscription_type='subscription' WHERE id=$1`, first)
	require.Zero(t, scopeCount(first))
	require.ErrorIs(t, assign(first), service.ErrSubscriptionGroupScopeDenied)
	// Other updates keep the scope.
	f.mustExec(`UPDATE groups SET description='renamed', subscription_type='subscription' WHERE id=$1`, second)
	require.Equal(t, 1, scopeCount(second))

	// Standard groups cannot be scoped, only cleared.
	f.mustExec(`UPDATE groups SET subscription_type='standard' WHERE id=$1`, first)
	require.ErrorIs(t, setScopes([]int64{first, second}, keys("xunyou")), service.ErrGroupOrganizationScopeInvalid)
	require.Equal(t, 1, scopeCount(second), "a rejected batch writes nothing")
	require.NoError(t, setScopes([]int64{first}, keys()))

	// Restoring a revoked subscription whose group was deleted says so.
	sub := s.seedSubscription(t, member, second)
	require.NoError(t, subs.RevokeSubscription(writeCtx, sub))
	f.mustExec(`UPDATE groups SET deleted_at=NOW() WHERE id=$1`, second)
	_, err = subs.RestoreSubscription(writeCtx, sub)
	require.ErrorIs(t, err, service.ErrSubscriptionGroupDeleted)
	require.Zero(t, s.subscriptionOf(t, member, second, false))
	require.ErrorIs(t, subs.PrecheckSubscriptionWrite(writeCtx, sub), service.ErrDepartmentScopeDenied, "a revoked subscription is not extendable")
}
