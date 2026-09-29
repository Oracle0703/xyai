package repository

import (
	"context"
	"database/sql"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// Admin details and their concurrency token must describe the same snapshot.
func (r *userRepository) GetByIDWithAdminAccess(ctx context.Context, id int64) (*service.User, error) {
	read := func(client *dbent.Client) (*service.User, error) {
		copyRepo := *r
		copyRepo.client = client
		u, err := copyRepo.GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		access, err := loadAdminAccess(ctx, client, id)
		if err != nil {
			return nil, err
		}
		u.AdminAccessVersion = access.Version
		return u, nil
	}
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return read(tx.Client())
	}
	tx, err := r.client.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	u, err := read(tx.Client())
	if err != nil {
		return nil, err
	}
	return u, tx.Commit()
}

func prepareUserAdminAccessWrite(ctx context.Context, q sqlExecutor, userID int64) (*service.User, *service.DepartmentAccess, error) {
	ids := []int64{userID}
	actorID := service.DepartmentActorID(ctx)
	if actorID > 0 {
		ids = append(ids, actorID)
	}
	if err := lockDepartmentUsers(ctx, q, ids); err != nil {
		return nil, nil, err
	}
	var actor *service.User
	var err error
	if actorID > 0 {
		actor, err = loadDepartmentActor(ctx, q, "")
		if err != nil {
			return nil, nil, err
		}
	}
	access, err := loadAdminAccess(ctx, q, userID)
	return actor, access, err
}

func clearUserDepartmentGrants(ctx context.Context, q sqlExecutor, actor *service.User, access *service.DepartmentAccess, reason string) error {
	if len(access.DepartmentIDs) == 0 {
		return nil
	}
	if _, err := q.ExecContext(ctx, `DELETE FROM department_access_grants WHERE user_id=$1`, access.UserID); err != nil {
		return err
	}
	return departmentAudit(ctx, q, actor, "department.clear_user_grants", map[string]any{
		"user_id": access.UserID, "department_ids": access.DepartmentIDs, "reason": reason,
	})
}

func hasDepartmentScopedPermission(permissions []string) bool {
	for _, permission := range permissions {
		if permission == service.AdminPermissionOrganizationUsage || permission == service.AdminPermissionDepartmentSubscriptions {
			return true
		}
	}
	return false
}
