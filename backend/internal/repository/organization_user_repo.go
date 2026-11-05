package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// Organization leaders reset passwords of and delete normal users of their
// organizations (design §4.9, D12). Authorization, the write and the audit
// share one Ent transaction; the full-administrator deletion path keeps its
// own guard (prepareUserAdminAccessWrite) and is not reused or relaxed (K18).

type organizationUserWrite func(ctx context.Context, exec *dbent.Client, actor *service.User, member *service.ManagedDepartmentMember) error

func (r *departmentRepository) organizationUserWrite(ctx context.Context, userID int64, write organizationUserWrite) error {
	if r.client == nil {
		return errors.New("organization user writes require an ent client")
	}
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	ctx = dbent.NewTxContext(ctx, tx)
	exec := tx.Client()
	if err = lockDepartmentUsers(ctx, exec, []int64{service.DepartmentActorID(ctx), userID}); err != nil {
		return err
	}
	actor, err := loadDepartmentActor(ctx, exec, service.AdminPermissionOrganizationManage)
	if err != nil {
		return err
	}
	if !departmentActorIsOrganizationLeader(actor) {
		return service.ErrDepartmentScopeDenied
	}
	// Same scope and manageable rule as the 「我的部门」 rows: organization
	// members including unassigned ones, and only role=user is manageable.
	p := departmentMemberPredicates(actor, false, service.DepartmentListFilter{UserIDs: []int64{userID}})
	rows, err := exec.QueryContext(ctx, `SELECT `+managedDepartmentMemberColumns+departmentMemberFrom()+` WHERE `+p.where(), p.args...)
	if err != nil {
		return err
	}
	if !rows.Next() {
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return err
		}
		return service.ErrDepartmentScopeDenied
	}
	member, err := scanManagedDepartmentMember(rows, actor)
	_ = rows.Close()
	if err != nil {
		return err
	}
	if !member.Manageable || member.ID == actor.ID {
		return service.ErrDepartmentScopeDenied
	}
	if err = write(ctx, exec, actor, member); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *departmentRepository) ResetOrganizationUserPassword(ctx context.Context, userID int64, passwordHash string) error {
	return r.organizationUserWrite(ctx, userID, func(ctx context.Context, exec *dbent.Client, actor *service.User, member *service.ManagedDepartmentMember) error {
		// Existing sessions fail their next check: the token version is derived
		// from email and password_hash (resolvedTokenVersion).
		if _, err := exec.ExecContext(ctx, `UPDATE users SET password_hash=$1, updated_at=NOW() WHERE id=$2 AND deleted_at IS NULL`, passwordHash, userID); err != nil {
			return err
		}
		return departmentAudit(ctx, exec, actor, "organization.user_password_reset", withActorScope(actor, map[string]any{
			"user_id": userID, "email": member.Email,
		}))
	})
}

func (r *departmentRepository) DeleteOrganizationUser(ctx context.Context, userID int64, in service.OrganizationUserDeleteInput) ([]string, error) {
	var keys []string
	err := r.organizationUserWrite(ctx, userID, func(ctx context.Context, exec *dbent.Client, actor *service.User, member *service.ManagedDepartmentMember) error {
		if (in.ExpectedDepartmentID == nil && !in.ExpectedUnassigned) || in.ExpectedDepartmentVersion == nil {
			return service.ErrDepartmentInvalid
		}
		if !sameDepartment(member.DepartmentID, in.ExpectedDepartmentID) || member.DepartmentVersion != *in.ExpectedDepartmentVersion {
			return service.ErrDepartmentConflict
		}
		// API keys are listed after the user row is locked, inside the transaction.
		rows, err := exec.QueryContext(ctx, `SELECT id,key FROM api_keys WHERE user_id=$1 AND deleted_at IS NULL ORDER BY id FOR UPDATE`, userID)
		if err != nil {
			return err
		}
		ids := []int64{}
		for rows.Next() {
			var id int64
			var key string
			if err = rows.Scan(&id, &key); err != nil {
				_ = rows.Close()
				return err
			}
			ids = append(ids, id)
			keys = append(keys, key)
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return err
		}
		for _, id := range ids {
			if _, err = softDeleteAPIKey(ctx, exec, id, fmt.Sprintf("__deleted__%d__%d", id, time.Now().UnixNano())); err != nil {
				return err
			}
		}
		access, err := loadAdminAccess(ctx, exec, userID)
		if err != nil {
			return err
		}
		if err = deleteUserRecords(ctx, exec, actor, access, userID); err != nil {
			return err
		}
		return departmentAudit(ctx, exec, actor, "organization.user_delete", withActorScope(actor, map[string]any{
			"user_id": userID, "email": member.Email, "department_id": member.DepartmentID, "api_key_count": len(ids),
		}))
	})
	if err != nil {
		return nil, err
	}
	return keys, nil
}
