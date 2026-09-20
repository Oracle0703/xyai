//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestUserAdminAccessIntegration_StaleEditorAndLifecycle(t *testing.T) {
	ctx := context.Background()
	prefix := organizationUsageIntegrationPrefix("access_cas")
	cleanupOrganizationUsageIntegrationData(t, prefix)
	admin, _ := organizationUsageIntegrationUser(t, prefix+"admin@example.com", service.StatusActive)
	leader, _ := organizationUsageIntegrationUser(t, prefix+"leader@xunyou.com", service.StatusActive)
	_, err := integrationDB.Exec(`UPDATE users SET role='admin' WHERE id=$1`, admin.ID)
	require.NoError(t, err)
	_, err = integrationDB.Exec(`UPDATE users SET role='sub_admin',admin_permissions='["admin.subscriptions"]' WHERE id=$1`, leader.ID)
	require.NoError(t, err)
	adminCtx := service.WithDepartmentActor(ctx, admin.ID)
	departments := service.NewDepartmentService(NewDepartmentRepository(integrationDB))
	dept, err := departments.Save(adminCtx, 0, service.DepartmentSaveInput{Organization: service.OrganizationXunyou, Name: prefix})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, e := integrationDB.Exec(`DELETE FROM department_access_grants WHERE department_id=$1`, dept.ID)
		require.NoError(t, e)
		_, e = integrationDB.Exec(`DELETE FROM departments WHERE id=$1`, dept.ID)
		require.NoError(t, e)
		_, e = integrationDB.Exec(`DELETE FROM audit_logs WHERE actor_user_id=ANY($1)`, pq.Array([]int64{admin.ID, leader.ID}))
		require.NoError(t, e)
	})
	repo, ok := NewUserRepository(integrationEntClient, integrationDB).(*userRepository)
	require.True(t, ok)
	// A queued grant write must re-read the target after lifecycle cleanup commits.
	withQueuedGrant := func(version string, expectedError error, mutate func(context.Context, *userRepository) error) {
		t.Helper()
		tx, e := integrationEntClient.Tx(adminCtx)
		require.NoError(t, e)
		defer func() { _ = tx.Rollback() }()
		txRepo := *repo
		txRepo.client = tx.Client()
		require.NoError(t, mutate(dbent.NewTxContext(adminCtx, tx), &txRepo))
		queued := make(chan error, 1)
		go func() {
			_, grantErr := departments.SetAccess(adminCtx, leader.ID, service.DepartmentAccessInput{Report: true, DepartmentIDs: []int64{dept.ID}, ExpectedVersion: version})
			queued <- grantErr
		}()
		require.Eventually(t, func() bool {
			var blocked int
			e := integrationDB.QueryRow(`SELECT COUNT(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%FROM users WHERE id=ANY%'`).Scan(&blocked)
			return e == nil && blocked > 0
		}, 5*time.Second, 10*time.Millisecond)
		require.NoError(t, tx.Commit())
		select {
		case e = <-queued:
			require.ErrorIs(t, e, expectedError)
		case <-time.After(5 * time.Second):
			t.Fatal("queued grant did not finish after lifecycle transaction committed")
		}
	}
	oldEditor, err := repo.GetByIDWithAdminAccess(adminCtx, leader.ID)
	require.NoError(t, err)
	access, err := departments.GetAccess(adminCtx, leader.ID)
	require.NoError(t, err)
	require.Equal(t, access.Version, oldEditor.AdminAccessVersion)
	_, err = departments.SetAccess(adminCtx, leader.ID, service.DepartmentAccessInput{DepartmentIDs: []int64{dept.ID}, Report: true, ResetQuota: true, ExpectedVersion: access.Version})
	require.ErrorIs(t, err, service.ErrDepartmentGlobalConfirmation)
	unchanged, err := departments.GetAccess(adminCtx, leader.ID)
	require.NoError(t, err)
	require.Equal(t, access.Version, unchanged.Version, "an unconfirmed permission switch must be atomic")
	_, err = departments.SetAccess(adminCtx, leader.ID, service.DepartmentAccessInput{DepartmentIDs: []int64{dept.ID}, Report: true, ResetQuota: true, ReplaceGlobalSubscriptions: true, ExpectedVersion: access.Version})
	require.NoError(t, err)
	err = repo.Update(adminCtx, oldEditor, service.UserUpdateFields{AdminPermissions: true, ExpectedAdminAccessVersion: oldEditor.AdminAccessVersion})
	require.ErrorIs(t, err, service.ErrAdminAccessChanged)
	err = repo.Update(adminCtx, oldEditor, service.UserUpdateFields{Role: true})
	require.ErrorIs(t, err, service.ErrAdminAccessVersionRequired)
	oldEditor.Notes = "notes only"
	require.NoError(t, repo.Update(adminCtx, oldEditor, service.UserUpdateFields{Notes: true}))
	fresh, err := repo.GetByIDWithAdminAccess(adminCtx, leader.ID)
	require.NoError(t, err)
	require.NotContains(t, fresh.AdminPermissions, service.AdminPermissionSubscriptions)
	require.Contains(t, fresh.AdminPermissions, service.AdminPermissionDepartmentSubscriptions)
	fresh.Role, fresh.AdminPermissions = service.RoleUser, []string{}
	withQueuedGrant(fresh.AdminAccessVersion, service.ErrAdminAccessChanged, func(txCtx context.Context, txRepo *userRepository) error {
		return txRepo.Update(txCtx, fresh, service.UserUpdateFields{Role: true, AdminPermissions: true, ExpectedAdminAccessVersion: fresh.AdminAccessVersion})
	})
	var grants int
	require.NoError(t, integrationDB.QueryRow(`SELECT count(*) FROM department_access_grants WHERE user_id=$1`, leader.ID).Scan(&grants))
	require.Zero(t, grants)
	var roleAudits int
	require.NoError(t, integrationDB.QueryRow(`SELECT count(*) FROM audit_logs WHERE actor_user_id=$1 AND action='department.user_access_changed' AND extra->>'before_role'='sub_admin' AND extra->>'after_role'='user'`, admin.ID).Scan(&roleAudits))
	require.Equal(t, 1, roleAudits)
	fresh.Role, fresh.AdminPermissions = service.RoleSubAdmin, []string{service.AdminPermissionOrganizationUsage, service.AdminPermissionSubscriptions}
	require.NoError(t, repo.Update(adminCtx, fresh, service.UserUpdateFields{Role: true, AdminPermissions: true, ExpectedAdminAccessVersion: fresh.AdminAccessVersion}))
	access, err = departments.GetAccess(adminCtx, leader.ID)
	require.NoError(t, err)
	require.Empty(t, access.DepartmentIDs, "promotion must not resurrect grants")
	access, err = departments.SetAccess(adminCtx, leader.ID, service.DepartmentAccessInput{Report: true, DepartmentIDs: []int64{dept.ID}, ExpectedVersion: access.Version})
	require.NoError(t, err, "report-only grants may coexist with explicitly granted global subscriptions")
	withQueuedGrant(access.Version, service.ErrDepartmentScopeDenied, func(txCtx context.Context, txRepo *userRepository) error {
		return txRepo.Delete(txCtx, leader.ID)
	})
	require.NoError(t, integrationDB.QueryRow(`SELECT count(*) FROM department_access_grants WHERE user_id=$1`, leader.ID).Scan(&grants))
	require.Zero(t, grants)
	var deleted bool
	require.NoError(t, integrationDB.QueryRow(`SELECT deleted_at IS NOT NULL FROM users WHERE id=$1`, leader.ID).Scan(&deleted))
	require.True(t, deleted, "deletion remains soft")
}
