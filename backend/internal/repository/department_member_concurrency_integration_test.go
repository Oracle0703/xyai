//go:build integration

package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func departmentMemberConcurrencyContext(t *testing.T, s *departmentMemberSetup) (context.Context, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	s.adminCtx = service.WithDepartmentActor(ctx, s.adminID)
	s.leaderCtx = service.WithDepartmentActor(ctx, s.leaderID)
	return ctx, cancel
}

func departmentMemberConcurrencyTx(t *testing.T, ctx context.Context) (*sql.Tx, int) {
	t.Helper()
	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	var pid int
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&pid))
	return tx, pid
}

func departmentMemberAuditBarrier(t *testing.T, ctx context.Context) (*sql.Tx, int) {
	t.Helper()
	tx, pid := departmentMemberConcurrencyTx(t, ctx)
	_, err := tx.ExecContext(ctx, `LOCK TABLE audit_logs IN SHARE MODE`)
	require.NoError(t, err)
	return tx, pid
}

func departmentMemberWaitBlocked(t *testing.T, ctx context.Context, holderPID int, queryPattern string) int {
	t.Helper()
	var blockedPID int
	require.Eventually(t, func() bool {
		err := integrationDB.QueryRowContext(ctx, `
SELECT COALESCE(MAX(pid),0)
FROM pg_stat_activity
WHERE datname=current_database() AND state='active' AND wait_event_type='Lock'
  AND $1=ANY(pg_blocking_pids(pid)) AND query LIKE $2`, holderPID, queryPattern).Scan(&blockedPID)
		return err == nil && blockedPID > 0
	}, 5*time.Second, 10*time.Millisecond, "expected a writer blocked by PID %d matching %q", holderPID, queryPattern)
	return blockedPID
}

func departmentMemberStartConcurrent(t *testing.T, cancel context.CancelFunc, release func(), operation func() error) <-chan error {
	t.Helper()
	result := make(chan error, 1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		result <- operation()
	}()
	t.Cleanup(func() {
		// Release the deliberate barrier before cancellation and before fixture cleanup.
		release()
		cancel()
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			t.Error("department member concurrent writer did not terminate after lock release and cancellation")
		}
	})
	return result
}

func departmentMemberAwaitConcurrent(t *testing.T, ctx context.Context, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		t.Fatalf("department member concurrent writer did not finish: %v", ctx.Err())
		return ctx.Err()
	}
}

func TestDepartmentMemberConcurrencyIntegration_AuthorizationChangedBeforeQueuedWrite(t *testing.T) {
	for _, action := range []string{"revoke_grants", "demote_actor"} {
		t.Run(action, func(t *testing.T) {
			s := setupDepartmentMembers(t)
			ctx, cancel := departmentMemberConcurrencyContext(t, s)
			member := s.f.user("authorization", "xunyou.com", service.RoleUser, nil)
			s.place(member, s.granted)
			input := s.expect(member)
			input.Username = departmentMemberString("must not be written")
			var holderPID int
			var commit func() error
			var release func()
			if action == "revoke_grants" {
				tx, pid := departmentMemberConcurrencyTx(t, ctx)
				holderPID, commit = pid, tx.Commit
				release = func() { _ = tx.Rollback() }
				require.NoError(t, lockDepartmentUsers(ctx, tx, []int64{s.adminID, s.leaderID}))
				// Construct a committed revocation while holding the same actor lock
				// used by SetAccess. The opposite ordering below calls real SetAccess.
				_, err := tx.ExecContext(ctx, `DELETE FROM department_access_grants WHERE user_id=$1`, s.leaderID)
				require.NoError(t, err)
			} else {
				repo := NewUserRepository(integrationEntClient, integrationDB).(*userRepository)
				leader, err := repo.GetByIDWithAdminAccess(s.adminCtx, s.leaderID)
				require.NoError(t, err)
				tx, err := integrationEntClient.Tx(s.adminCtx)
				require.NoError(t, err)
				t.Cleanup(func() { _ = tx.Rollback() })
				commit, release = tx.Commit, func() { _ = tx.Rollback() }
				txCtx := dbent.NewTxContext(s.adminCtx, tx)
				require.NoError(t, scanSingleRow(txCtx, tx.Client(), `SELECT pg_backend_pid()`, nil, &holderPID))
				bound := *repo
				bound.client = tx.Client()
				leader.Role, leader.AdminPermissions = service.RoleUser, []string{}
				require.NoError(t, bound.Update(txCtx, leader, service.UserUpdateFields{
					Role: true, AdminPermissions: true, ExpectedAdminAccessVersion: leader.AdminAccessVersion,
				}))
			}
			done := departmentMemberStartConcurrent(t, cancel, release, func() error {
				_, err := s.svc.UpdateManagedMember(s.leaderCtx, member, input)
				return err
			})
			departmentMemberWaitBlocked(t, ctx, holderPID, "%FROM users WHERE id=ANY%")
			require.NoError(t, commit())
			require.ErrorIs(t, departmentMemberAwaitConcurrent(t, ctx, done), service.ErrDepartmentScopeDenied)
			var username string
			var grants int
			require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT username FROM users WHERE id=$1`, member).Scan(&username))
			require.Empty(t, username)
			require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM department_access_grants WHERE user_id=$1`, s.leaderID).Scan(&grants))
			require.Zero(t, grants)
			require.Zero(t, s.audits("department.manager_update_member", s.leaderID))
			if action == "demote_actor" {
				var role string
				require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT role FROM users WHERE id=$1`, s.leaderID).Scan(&role))
				require.Equal(t, service.RoleUser, role)
				require.Equal(t, 1, s.audits("department.user_access_changed", s.adminID))
			}
		})
	}
}

func TestDepartmentMemberConcurrencyIntegration_ManagerWriteSerializesGrantRevocation(t *testing.T) {
	s := setupDepartmentMembers(t)
	ctx, cancel := departmentMemberConcurrencyContext(t, s)
	member := s.f.user("before_revocation", "xunyou.com", service.RoleUser, nil)
	s.place(member, s.granted)
	input := s.expect(member)
	input.DepartmentNote = departmentMemberString("written before revocation")
	access, err := s.svc.GetAccess(s.adminCtx, s.leaderID)
	require.NoError(t, err)
	barrier, holderPID := departmentMemberAuditBarrier(t, ctx)
	release := func() { _ = barrier.Rollback() }
	writeDone := departmentMemberStartConcurrent(t, cancel, release, func() error {
		_, err := s.svc.UpdateManagedMember(s.leaderCtx, member, input)
		return err
	})
	writerPID := departmentMemberWaitBlocked(t, ctx, holderPID, "%INSERT INTO audit_logs%")
	revokeDone := departmentMemberStartConcurrent(t, cancel, release, func() error {
		_, err := s.svc.SetAccess(s.adminCtx, s.leaderID, service.DepartmentAccessInput{
			DepartmentIDs: []int64{}, ExpectedVersion: access.Version,
		})
		return err
	})
	departmentMemberWaitBlocked(t, ctx, writerPID, "%FROM users WHERE id=ANY%")
	require.NoError(t, barrier.Commit())
	require.NoError(t, departmentMemberAwaitConcurrent(t, ctx, writeDone))
	require.NoError(t, departmentMemberAwaitConcurrent(t, ctx, revokeDone))
	require.Equal(t, "written before revocation", s.f.note(member))
	require.Equal(t, 1, s.audits("department.manager_update_member", s.leaderID))
	_, err = s.svc.UpdateManagedMember(s.leaderCtx, member, input)
	require.ErrorIs(t, err, service.ErrDepartmentScopeDenied)
}

func TestDepartmentMemberConcurrencyIntegration_MembershipChangedBeforeQueuedUpdate(t *testing.T) {
	for _, change := range []string{"unassigned", "ungranted_department", "still_granted_department", "round_trip"} {
		t.Run(change, func(t *testing.T) {
			s := setupDepartmentMembers(t)
			ctx, cancel := departmentMemberConcurrencyContext(t, s)
			if change == "still_granted_department" {
				s.grant(true, s.granted, s.other)
			}
			member := s.f.user("membership", "xunyou.com", service.RoleUser, nil)
			s.place(member, s.granted)
			s.f.mustExec(`UPDATE users SET department_note='old department note' WHERE id=$1`, member)
			input := s.expect(member)
			input.Status = departmentMemberString(service.StatusDisabled)
			tx, holderPID := departmentMemberConcurrencyTx(t, ctx)
			require.NoError(t, lockDepartmentUsers(ctx, tx, []int64{member}))
			var expectedDepartment *int64
			expectedVersion := *input.ExpectedDepartmentVersion + 1
			if change == "ungranted_department" || change == "still_granted_department" {
				expectedDepartment = &s.other
			}
			_, err := tx.ExecContext(ctx, `UPDATE users SET department_id=$1 WHERE id=$2`, expectedDepartment, member)
			require.NoError(t, err)
			if change == "round_trip" {
				_, err = tx.ExecContext(ctx, `UPDATE users SET department_id=$1 WHERE id=$2`, s.granted, member)
				require.NoError(t, err)
				expectedDepartment, expectedVersion = &s.granted, expectedVersion+1
			}
			done := departmentMemberStartConcurrent(t, cancel, func() { _ = tx.Rollback() }, func() error {
				_, err := s.svc.UpdateManagedMember(s.leaderCtx, member, input)
				return err
			})
			departmentMemberWaitBlocked(t, ctx, holderPID, "%FROM users WHERE id=ANY%")
			require.NoError(t, tx.Commit())
			wantErr := service.ErrDepartmentScopeDenied
			if change == "still_granted_department" || change == "round_trip" {
				wantErr = service.ErrDepartmentConflict
			}
			require.ErrorIs(t, departmentMemberAwaitConcurrent(t, ctx, done), wantErr)
			department, version := s.current(member)
			require.Equal(t, expectedDepartment, department)
			require.Equal(t, expectedVersion, version)
			s.f.requireStatus(member, service.StatusActive, "")
			require.Empty(t, s.f.note(member))
			require.Len(t, s.f.noteClearedAudits(member), 1)
			require.Zero(t, s.audits("department.manager_update_member", s.leaderID))
		})
	}
}

func TestDepartmentMemberConcurrencyIntegration_ManagerWriteSerializesTransfer(t *testing.T) {
	s := setupDepartmentMembers(t)
	ctx, cancel := departmentMemberConcurrencyContext(t, s)
	member := s.f.user("before_transfer", "xunyou.com", service.RoleUser, nil)
	s.place(member, s.granted)
	s.f.mustExec(`UPDATE users SET department_note='older note' WHERE id=$1`, member)
	input := s.expect(member)
	input.DepartmentNote = departmentMemberString("latest manager note")
	transfer := service.DepartmentAssignInput{DepartmentID: &s.other, Members: []service.DepartmentMemberChange{s.change(member)}}
	barrier, holderPID := departmentMemberAuditBarrier(t, ctx)
	release := func() { _ = barrier.Rollback() }
	writeDone := departmentMemberStartConcurrent(t, cancel, release, func() error {
		_, err := s.svc.UpdateManagedMember(s.leaderCtx, member, input)
		return err
	})
	writerPID := departmentMemberWaitBlocked(t, ctx, holderPID, "%INSERT INTO audit_logs%")
	transferDone := departmentMemberStartConcurrent(t, cancel, release, func() error {
		_, err := s.svc.Assign(s.adminCtx, transfer)
		return err
	})
	departmentMemberWaitBlocked(t, ctx, writerPID, "%FROM users WHERE id=ANY%")
	require.NoError(t, barrier.Commit())
	require.NoError(t, departmentMemberAwaitConcurrent(t, ctx, writeDone))
	require.NoError(t, departmentMemberAwaitConcurrent(t, ctx, transferDone))
	department, version := s.current(member)
	require.Equal(t, &s.other, department)
	require.Equal(t, *input.ExpectedDepartmentVersion+1, version)
	require.Empty(t, s.f.note(member))
	audits := s.f.noteClearedAudits(member)
	require.Len(t, audits, 1)
	require.Equal(t, "latest manager note", audits[0].note, "the later transfer must clear the committed latest note")
	require.Equal(t, 1, s.audits("department.manager_update_member", s.leaderID))
	require.Equal(t, 1, s.audits("department.assign_members", s.adminID))
}

func TestDepartmentMemberConcurrencyIntegration_SystemBanWinsOverManagerEnable(t *testing.T) {
	for _, order := range []string{"ban_first", "enable_first"} {
		t.Run(order, func(t *testing.T) {
			s := setupDepartmentMembers(t)
			ctx, cancel := departmentMemberConcurrencyContext(t, s)
			member := s.f.user("system_ban", "xunyou.com", service.RoleUser, nil)
			s.place(member, s.granted)
			s.f.mustExec(`UPDATE users SET status='disabled',disabled_source='department_manager' WHERE id=$1`, member)
			input := s.expect(member)
			input.Status = departmentMemberString(service.StatusActive)
			repo := NewUserRepository(integrationEntClient, integrationDB).(*userRepository)
			snapshot, err := repo.GetByID(ctx, member)
			require.NoError(t, err)
			snapshot.Status = service.StatusDisabled
			if order == "ban_first" {
				tx, err := integrationEntClient.Tx(ctx)
				require.NoError(t, err)
				t.Cleanup(func() { _ = tx.Rollback() })
				txCtx := dbent.NewTxContext(ctx, tx)
				var holderPID int
				require.NoError(t, scanSingleRow(txCtx, tx.Client(), `SELECT pg_backend_pid()`, nil, &holderPID))
				bound := *repo
				bound.client = tx.Client()
				require.NoError(t, bound.Update(txCtx, snapshot, service.UserUpdateFields{Status: true}))
				enableDone := departmentMemberStartConcurrent(t, cancel, func() { _ = tx.Rollback() }, func() error {
					_, err := s.svc.UpdateManagedMember(s.leaderCtx, member, input)
					return err
				})
				departmentMemberWaitBlocked(t, ctx, holderPID, "%FROM users WHERE id=ANY%")
				require.NoError(t, tx.Commit())
				require.ErrorIs(t, departmentMemberAwaitConcurrent(t, ctx, enableDone), service.ErrDepartmentMemberEnableDenied)
				require.Zero(t, s.audits("department.manager_update_member", s.leaderID))
			} else {
				barrier, holderPID := departmentMemberAuditBarrier(t, ctx)
				release := func() { _ = barrier.Rollback() }
				enableDone := departmentMemberStartConcurrent(t, cancel, release, func() error {
					_, err := s.svc.UpdateManagedMember(s.leaderCtx, member, input)
					return err
				})
				writerPID := departmentMemberWaitBlocked(t, ctx, holderPID, "%INSERT INTO audit_logs%")
				banDone := departmentMemberStartConcurrent(t, cancel, release, func() error {
					return repo.Update(ctx, snapshot, service.UserUpdateFields{Status: true})
				})
				departmentMemberWaitBlocked(t, ctx, writerPID, "%UPDATE%users%")
				require.NoError(t, barrier.Commit())
				require.NoError(t, departmentMemberAwaitConcurrent(t, ctx, enableDone))
				require.NoError(t, departmentMemberAwaitConcurrent(t, ctx, banDone))
				require.Equal(t, 1, s.audits("department.manager_update_member", s.leaderID))
			}
			s.f.requireStatus(member, service.StatusDisabled, "other", "a system ban must not leave the member active or manager-enable-able")
		})
	}
}

func TestDepartmentMemberConcurrencyIntegration_BillingAndMemberWritePreserveBothChanges(t *testing.T) {
	for _, scenario := range []string{"billing_first_profile", "billing_first_disable", "manager_first"} {
		t.Run(scenario, func(t *testing.T) {
			s := setupDepartmentMembers(t)
			ctx, cancel := departmentMemberConcurrencyContext(t, s)
			member := s.f.user("billing", "xunyou.com", service.RoleUser, nil)
			s.place(member, s.granted)
			s.f.mustExec(`UPDATE users SET balance=100 WHERE id=$1`, member)
			input := s.expect(member)
			input.Username = departmentMemberString("updated alongside billing")
			input.DepartmentNote = departmentMemberString("note alongside billing")
			if scenario != "billing_first_profile" {
				input.Status = departmentMemberString(service.StatusDisabled)
			}
			if scenario != "manager_first" {
				tx, holderPID := departmentMemberConcurrencyTx(t, ctx)
				balance, sufficient, err := deductUsageBillingBalance(ctx, tx, member, 2)
				require.NoError(t, err)
				require.True(t, sufficient)
				require.Equal(t, 98.0, balance)
				writeDone := departmentMemberStartConcurrent(t, cancel, func() { _ = tx.Rollback() }, func() error {
					_, err := s.svc.UpdateManagedMember(s.leaderCtx, member, input)
					return err
				})
				departmentMemberWaitBlocked(t, ctx, holderPID, "%FROM users WHERE id=ANY%")
				require.NoError(t, tx.Commit())
				require.NoError(t, departmentMemberAwaitConcurrent(t, ctx, writeDone))
			} else {
				barrier, holderPID := departmentMemberAuditBarrier(t, ctx)
				release := func() { _ = barrier.Rollback() }
				writeDone := departmentMemberStartConcurrent(t, cancel, release, func() error {
					_, err := s.svc.UpdateManagedMember(s.leaderCtx, member, input)
					return err
				})
				writerPID := departmentMemberWaitBlocked(t, ctx, holderPID, "%INSERT INTO audit_logs%")
				billingDone := departmentMemberStartConcurrent(t, cancel, release, func() error {
					tx, err := integrationDB.BeginTx(ctx, nil)
					if err != nil {
						return err
					}
					defer func() { _ = tx.Rollback() }()
					if _, _, err = deductUsageBillingBalance(ctx, tx, member, 2); err != nil {
						return err
					}
					return tx.Commit()
				})
				departmentMemberWaitBlocked(t, ctx, writerPID, "%SET balance = balance -%")
				require.NoError(t, barrier.Commit())
				require.NoError(t, departmentMemberAwaitConcurrent(t, ctx, writeDone))
				require.NoError(t, departmentMemberAwaitConcurrent(t, ctx, billingDone))
			}
			var balance float64
			var username string
			require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT balance,username FROM users WHERE id=$1`, member).Scan(&balance, &username))
			require.Equal(t, 98.0, balance)
			require.Equal(t, "updated alongside billing", username)
			require.Equal(t, "note alongside billing", s.f.note(member))
			if scenario == "billing_first_profile" {
				s.f.requireStatus(member, service.StatusActive, "")
			} else {
				s.f.requireStatus(member, service.StatusDisabled, "department_manager")
			}
			require.Equal(t, 1, s.audits("department.manager_update_member", s.leaderID))
		})
	}
}
