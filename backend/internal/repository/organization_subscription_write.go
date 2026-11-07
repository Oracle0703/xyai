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
// (design §4.8, D11) and only in groups scoped to their organizations (D13). Full administrators and full-site subscription
// sub-admins keep their existing unrestricted writes.

var _ service.SubscriptionWriteScopeRepository = (*userSubscriptionRepository)(nil)

type organizationSubscriptionWriteActorKey struct{}

// CurrentSubscriptionWriter is called for requests the admin middleware
// admitted as a sub-admin and re-reads the actor. Sub-admins without the
// full-site subscription permission are scoped and must then pass
// AuthorizeSubscriptionWriteUsers (only organization leaders do); those with it
// are reported as full-site so that the service checks the action against the
// current permissions (design K21). An actor demoted to user, deleted or
// disabled since the middleware check is rejected, never treated as
// unrestricted (design K19).
func (r *userSubscriptionRepository) CurrentSubscriptionWriter(ctx context.Context) (service.SubscriptionWriterKind, error) {
	actorID := service.DepartmentActorID(ctx)
	if actorID <= 0 || service.DepartmentActorIsAdminAPIKey(ctx) {
		return service.SubscriptionWriterAdministrator, nil
	}
	var role, status string
	var raw []byte
	err := scanSingleRow(ctx, clientFromContext(ctx, r.client), `SELECT role,status,admin_permissions FROM users WHERE id=$1 AND deleted_at IS NULL`, []any{actorID}, &role, &status, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return service.SubscriptionWriterScoped, service.ErrDepartmentScopeDenied
	}
	if err != nil {
		return service.SubscriptionWriterScoped, err
	}
	if status != service.StatusActive {
		return service.SubscriptionWriterScoped, service.ErrDepartmentScopeDenied
	}
	switch role {
	case service.RoleAdmin:
		return service.SubscriptionWriterAdministrator, nil
	case service.RoleSubAdmin:
	default:
		return service.SubscriptionWriterScoped, service.ErrDepartmentScopeDenied
	}
	var permissions []string
	if err = json.Unmarshal(raw, &permissions); err != nil {
		return service.SubscriptionWriterScoped, err
	}
	if slices.Contains(permissions, service.AdminPermissionSubscriptions) {
		return service.SubscriptionWriterFullSite, nil
	}
	return service.SubscriptionWriterScoped, nil
}

// SubscriptionWriteAccess returns which of userIDs are write targets and
// which of groupIDs are scoped to the actor's organizations, from one actor
// read. Department leaders never write, so they get none.
func (r *userSubscriptionRepository) SubscriptionWriteAccess(ctx context.Context, userIDs, groupIDs []int64) (*service.SubscriptionWriteAccess, error) {
	client := clientFromContext(ctx, r.client)
	actor, err := loadDepartmentActor(ctx, client, service.AdminPermissionDepartmentSubscriptions)
	if err != nil {
		return nil, err
	}
	access := &service.SubscriptionWriteAccess{Users: map[int64]bool{}, Groups: map[int64]bool{}}
	if !departmentActorIsOrganizationLeader(actor) {
		return access, nil
	}
	if len(userIDs) > 0 {
		if err = queryManageableSubscriptionUsers(ctx, client, actor, userIDs, access.Users); err != nil {
			return nil, err
		}
	}
	if len(groupIDs) == 0 {
		return access, nil
	}
	args := []any{pq.Array(groupIDs)}
	rows, err := client.QueryContext(ctx, `SELECT g.id FROM groups g WHERE g.id=ANY($1::bigint[]) AND g.deleted_at IS NULL AND `+subscriptionWriteGroupScope(actor, "g", &args), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		access.Groups[id] = true
	}
	return access, rows.Err()
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

// ScopedAssignableSubscriptionGroups returns the active, live subscription
// groups scoped to the current organization leader's organizations in
// ListActive order, filled with the fields the assignment picker shows. Other
// actors get none.
func (r *userSubscriptionRepository) ScopedAssignableSubscriptionGroups(ctx context.Context) ([]service.Group, error) {
	client := clientFromContext(ctx, r.client)
	actor, err := loadDepartmentActor(ctx, client, service.AdminPermissionDepartmentSubscriptions, service.AdminPermissionOrganizationManage)
	if err != nil {
		return nil, err
	}
	result := []service.Group{}
	if !departmentActorIsOrganizationLeader(actor) {
		return result, nil
	}
	args := []any{service.StatusActive, service.SubscriptionTypeSubscription}
	rows, err := client.QueryContext(ctx, `SELECT g.id,g.name,COALESCE(g.description,''),g.platform,g.rate_multiplier,g.subscription_type,g.status
FROM groups g
WHERE g.deleted_at IS NULL AND g.status=$1 AND g.subscription_type=$2 AND `+subscriptionWriteGroupScope(actor, "g", &args)+`
ORDER BY g.sort_order ASC, g.id ASC`, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var group service.Group
		if err = rows.Scan(&group.ID, &group.Name, &group.Description, &group.Platform, &group.RateMultiplier, &group.SubscriptionType, &group.Status); err != nil {
			return nil, err
		}
		result = append(result, group)
	}
	return result, rows.Err()
}

// AuthorizeSubscriptionWriteGroup checks that the group is scoped to the
// organizations of the actor that AuthorizeSubscriptionWriteUsers authorized.
// With lock the group row is share-locked after the users and before any
// subscription; scope replacement and group deletion take FOR UPDATE, so the
// scope cannot change until commit. The scope is read by a separate statement
// after the lock so that it sees what a concurrent replacement committed.
// Missing, deleted and unscoped groups are all rejected with 403.
func (r *userSubscriptionRepository) AuthorizeSubscriptionWriteGroup(ctx context.Context, groupID int64, lock bool) error {
	actor, ok := ctx.Value(organizationSubscriptionWriteActorKey{}).(*service.User)
	if !ok || !departmentActorIsOrganizationLeader(actor) {
		return service.ErrDepartmentScopeDenied
	}
	if groupID <= 0 {
		return service.ErrSubscriptionGroupScopeDenied
	}
	client := clientFromContext(ctx, r.client)
	var found int64
	if lock {
		err := scanSingleRow(ctx, client, `SELECT id FROM groups WHERE id=$1 AND deleted_at IS NULL FOR SHARE`, []any{groupID}, &found)
		if errors.Is(err, sql.ErrNoRows) {
			return service.ErrSubscriptionGroupScopeDenied
		}
		if err != nil {
			return err
		}
	}
	args := []any{groupID}
	err := scanSingleRow(ctx, client, `SELECT g.id FROM groups g WHERE g.id=$1 AND g.deleted_at IS NULL AND `+subscriptionWriteGroupScope(actor, "g", &args), args, &found)
	if errors.Is(err, sql.ErrNoRows) {
		return service.ErrSubscriptionGroupScopeDenied
	}
	return err
}

// AuthorizeSubscriptionWriteByID authorizes the owner and the group of a
// subscription. A missing subscription is reported as 403 so that leaders
// cannot probe IDs of other organizations (design K16). The group is checked
// only after the owner, so its distinct error never discloses a subscription
// outside the leader's write targets.
func (r *userSubscriptionRepository) AuthorizeSubscriptionWriteByID(ctx context.Context, subscriptionID int64, includeDeleted, lock bool) (context.Context, int64, error) {
	client := clientFromContext(ctx, r.client)
	query := `SELECT user_id,group_id FROM user_subscriptions WHERE id=$1`
	if !includeDeleted {
		query += ` AND deleted_at IS NULL`
	}
	var owner, group int64
	err := scanSingleRow(ctx, client, query, []any{subscriptionID}, &owner, &group)
	if errors.Is(err, sql.ErrNoRows) {
		return ctx, 0, service.ErrDepartmentScopeDenied
	}
	if err != nil {
		return ctx, 0, err
	}
	ctx, err = r.AuthorizeSubscriptionWriteUsers(ctx, []int64{owner}, lock)
	if err != nil {
		return ctx, owner, err
	}
	if err = r.AuthorizeSubscriptionWriteGroup(ctx, group, lock); err != nil || !lock {
		if errors.Is(err, service.ErrSubscriptionGroupScopeDenied) {
			err = subscriptionGroupDeletedOr(ctx, client, group, err)
		}
		return ctx, owner, err
	}
	// Lock order is users, groups, then subscriptions, as in department resets.
	var lockedOwner, lockedGroup int64
	err = scanSingleRow(ctx, client, query+` FOR UPDATE`, []any{subscriptionID}, &lockedOwner, &lockedGroup)
	if errors.Is(err, sql.ErrNoRows) {
		return ctx, 0, service.ErrDepartmentScopeDenied
	}
	if err != nil {
		return ctx, 0, err
	}
	if lockedOwner != owner || lockedGroup != group {
		return ctx, 0, service.ErrDepartmentScopeChanged
	}
	return ctx, owner, nil
}

// subscriptionGroupDeletedOr reports a soft-deleted group of an authorized
// subscription as ErrSubscriptionGroupDeleted rather than as a scope denial,
// which would send the leader looking for a scope problem that does not exist.
func subscriptionGroupDeletedOr(ctx context.Context, q sqlExecutor, groupID int64, denied error) error {
	var deleted bool
	err := scanSingleRow(ctx, q, `SELECT deleted_at IS NOT NULL FROM groups WHERE id=$1`, []any{groupID}, &deleted)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if deleted {
		return service.ErrSubscriptionGroupDeleted
	}
	return denied
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
