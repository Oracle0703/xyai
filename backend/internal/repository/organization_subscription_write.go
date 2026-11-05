package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

// Organization leaders write subscriptions only for subscriptionManageTargets
// (design §4.8, D11). Full administrators and full-site subscription
// sub-admins keep their existing unrestricted writes.

var _ service.SubscriptionWriteScopeRepository = (*userSubscriptionRepository)(nil)

type organizationSubscriptionWriteActorKey struct{}

// SubscriptionWriteScoped is called for requests the admin middleware admitted
// as a sub-admin and re-reads the actor. It reports whether writes are limited
// to organization targets: true for sub-admins without the full-site
// subscription permission, which must then pass AuthorizeSubscriptionWriteUsers
// (only organization leaders do). An actor demoted to user, deleted or
// disabled since the middleware check is rejected, never treated as
// unrestricted (design K19).
func (r *userSubscriptionRepository) SubscriptionWriteScoped(ctx context.Context) (bool, error) {
	actorID := service.DepartmentActorID(ctx)
	if actorID <= 0 || service.DepartmentActorIsAdminAPIKey(ctx) {
		return false, nil
	}
	var role, status string
	var raw []byte
	err := scanSingleRow(ctx, clientFromContext(ctx, r.client), `SELECT role,status,admin_permissions FROM users WHERE id=$1 AND deleted_at IS NULL`, []any{actorID}, &role, &status, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, service.ErrDepartmentScopeDenied
	}
	if err != nil {
		return false, err
	}
	if status != service.StatusActive {
		return false, service.ErrDepartmentScopeDenied
	}
	switch role {
	case service.RoleAdmin:
		return false, nil
	case service.RoleSubAdmin:
	default:
		return false, service.ErrDepartmentScopeDenied
	}
	var permissions []string
	if err = json.Unmarshal(raw, &permissions); err != nil {
		return false, err
	}
	return !slices.Contains(permissions, service.AdminPermissionSubscriptions), nil
}

// ManageableSubscriptionUsers returns which of userIDs the actor may write
// subscriptions for. Department leaders never write, so they get none.
func (r *userSubscriptionRepository) ManageableSubscriptionUsers(ctx context.Context, userIDs []int64) (map[int64]bool, error) {
	client := clientFromContext(ctx, r.client)
	actor, err := loadDepartmentActor(ctx, client, service.AdminPermissionDepartmentSubscriptions)
	if err != nil {
		return nil, err
	}
	result := map[int64]bool{}
	if !departmentActorIsOrganizationLeader(actor) || len(userIDs) == 0 {
		return result, nil
	}
	return result, queryManageableSubscriptionUsers(ctx, client, actor, userIDs, result)
}

func queryManageableSubscriptionUsers(ctx context.Context, q sqlExecutor, actor *service.User, userIDs []int64, result map[int64]bool) error {
	args := []any{pq.Array(userIDs)}
	rows, err := q.QueryContext(ctx, `SELECT u.id FROM users u WHERE u.id=ANY($1::bigint[]) AND `+subscriptionManageTargets(actor, &args), args...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return err
		}
		result[id] = true
	}
	return rows.Err()
}

// AuthorizeSubscriptionWriteUsers checks that every target is a write target
// of the current organization leader. With lock it must run inside the write
// transaction: the actor and targets are locked first, then re-read, so a
// concurrent revocation or role change cannot slip between check and write.
// Missing targets and targets outside the scope are both rejected with 403.
func (r *userSubscriptionRepository) AuthorizeSubscriptionWriteUsers(ctx context.Context, userIDs []int64, lock bool) (context.Context, error) {
	client := clientFromContext(ctx, r.client)
	unique := map[int64]bool{}
	ids := []int64{}
	for _, id := range userIDs {
		if id <= 0 {
			return ctx, service.ErrDepartmentScopeDenied
		}
		if !unique[id] {
			unique[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return ctx, service.ErrDepartmentScopeDenied
	}
	if lock {
		if err := lockDepartmentUsers(ctx, client, append([]int64{service.DepartmentActorID(ctx)}, ids...)); err != nil {
			return ctx, err
		}
	}
	actor, err := loadDepartmentActor(ctx, client, service.AdminPermissionOrganizationManage)
	if err != nil {
		return ctx, err
	}
	if !departmentActorIsOrganizationLeader(actor) {
		return ctx, service.ErrDepartmentScopeDenied
	}
	allowed := map[int64]bool{}
	if err = queryManageableSubscriptionUsers(ctx, client, actor, ids, allowed); err != nil {
		return ctx, err
	}
	if len(allowed) != len(ids) {
		return ctx, service.ErrDepartmentScopeDenied
	}
	return context.WithValue(ctx, organizationSubscriptionWriteActorKey{}, actor), nil
}

// AuthorizeSubscriptionWriteByID authorizes the owner of a subscription. A
// missing subscription is reported as 403 so that leaders cannot probe IDs of
// other organizations (design K16).
func (r *userSubscriptionRepository) AuthorizeSubscriptionWriteByID(ctx context.Context, subscriptionID int64, includeDeleted, lock bool) (context.Context, int64, error) {
	client := clientFromContext(ctx, r.client)
	query := `SELECT user_id FROM user_subscriptions WHERE id=$1`
	if !includeDeleted {
		query += ` AND deleted_at IS NULL`
	}
	var owner int64
	err := scanSingleRow(ctx, client, query, []any{subscriptionID}, &owner)
	if errors.Is(err, sql.ErrNoRows) {
		return ctx, 0, service.ErrDepartmentScopeDenied
	}
	if err != nil {
		return ctx, 0, err
	}
	ctx, err = r.AuthorizeSubscriptionWriteUsers(ctx, []int64{owner}, lock)
	if err != nil || !lock {
		return ctx, owner, err
	}
	// Lock order is users before subscriptions, as in department resets.
	var locked int64
	err = scanSingleRow(ctx, client, query+` FOR UPDATE`, []any{subscriptionID}, &locked)
	if errors.Is(err, sql.ErrNoRows) {
		return ctx, 0, service.ErrDepartmentScopeDenied
	}
	if err != nil {
		return ctx, 0, err
	}
	if locked != owner {
		return ctx, 0, service.ErrDepartmentScopeChanged
	}
	return ctx, owner, nil
}

// RunSubscriptionWriteStep runs one write of a batch inside a savepoint of the
// batch transaction, so a business failure of that write is undone without
// aborting the batch. stepErr is the write's own error; err reports that the
// savepoint itself failed and the batch must be aborted.
func (r *userSubscriptionRepository) RunSubscriptionWriteStep(ctx context.Context, step func(context.Context) error) (stepErr error, err error) {
	if dbent.TxFromContext(ctx) == nil {
		return nil, errors.New("subscription write steps require a transaction")
	}
	client := clientFromContext(ctx, r.client)
	if _, err = client.ExecContext(ctx, `SAVEPOINT organization_subscription_write`); err != nil {
		return nil, err
	}
	if stepErr = step(ctx); stepErr != nil {
		_, err = client.ExecContext(ctx, `ROLLBACK TO SAVEPOINT organization_subscription_write`)
		return stepErr, err
	}
	_, err = client.ExecContext(ctx, `RELEASE SAVEPOINT organization_subscription_write`)
	return nil, err
}

// RecordSubscriptionWrite writes the audit entry in the write transaction.
func (r *userSubscriptionRepository) RecordSubscriptionWrite(ctx context.Context, action string, details map[string]any) error {
	actor, ok := ctx.Value(organizationSubscriptionWriteActorKey{}).(*service.User)
	if !ok {
		return service.ErrDepartmentScopeDenied
	}
	return departmentAudit(ctx, clientFromContext(ctx, r.client), actor, action, withActorScope(actor, details))
}
