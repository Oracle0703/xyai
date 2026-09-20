package repository

import (
	"context"
	"errors"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// nil IDs mean an explicitly unrestricted actor; a non-nil empty slice means
// an authorized but empty department and must never be treated as unrestricted.
func (r *userSubscriptionRepository) departmentSubscriptionIDs(ctx context.Context, filter service.SubscriptionAdminFilter) ([]int64, error) {
	if service.DepartmentActorID(ctx) == 0 {
		return nil, nil
	}
	client := clientFromContext(ctx, r.client)
	scope, err := resolveDepartmentScope(ctx, client, service.AdminPermissionDepartmentSubscriptions)
	if err != nil {
		return nil, err
	}
	organization, department, err := service.NormalizeDepartmentFilter(filter.Organization, filter.DepartmentID)
	if err != nil {
		return nil, err
	}
	if err = scope.ValidateSelection(organization, department, filter.ScopeVersion); err != nil {
		return nil, err
	}
	if scope.Unrestricted && department == "all" {
		return nil, nil
	}
	ids := []int64{}
	for _, m := range scope.SelectedMembers(organization, department, "") {
		ids = append(ids, m.ID)
	}
	if filter.UserID != nil {
		allowed := false
		for _, id := range ids {
			if id == *filter.UserID {
				allowed = true
			}
		}
		if !allowed {
			return nil, service.ErrDepartmentScopeDenied
		}
	}
	return ids, nil
}

func (r *userSubscriptionRepository) applyDepartmentSubscriptionScope(ctx context.Context, q *dbent.UserSubscriptionQuery, filter service.SubscriptionAdminFilter) (*dbent.UserSubscriptionQuery, error) {
	ids, err := r.departmentSubscriptionIDs(ctx, filter)
	if err != nil {
		return nil, err
	}
	if ids != nil {
		q = q.Where(usersubscription.UserIDIn(ids...))
	}
	return q, nil
}

func (r *userSubscriptionRepository) checkDepartmentSubscription(ctx context.Context, id int64) error {
	ids, scopeErr := r.departmentSubscriptionIDs(ctx, service.SubscriptionAdminFilter{})
	if scopeErr != nil {
		return scopeErr
	}
	if ids == nil {
		return nil
	}
	q, err := r.applyDepartmentSubscriptionScope(ctx, clientFromContext(ctx, r.client).UserSubscription.Query().Where(usersubscription.IDEQ(id)), service.SubscriptionAdminFilter{})
	if err != nil {
		return err
	}
	if service.DepartmentActorID(ctx) == 0 {
		return nil
	}
	exists, err := q.Exist(ctx)
	if err != nil {
		return err
	}
	if !exists {
		return service.ErrDepartmentScopeDenied
	}
	return nil
}

func (r *userSubscriptionRepository) beginDepartmentSubscriptionWrite(ctx context.Context, filter service.SubscriptionAdminFilter, subscriptionID int64) (context.Context, []int64, *dbent.Tx, error) {
	if service.DepartmentActorID(ctx) == 0 {
		return ctx, nil, nil, nil
	}
	var owned *dbent.Tx
	if dbent.TxFromContext(ctx) == nil {
		tx, err := r.client.Tx(ctx)
		if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
			return ctx, nil, nil, err
		}
		if err == nil {
			owned = tx
			ctx = dbent.NewTxContext(ctx, tx)
		}
	}
	fail := func(err error) (context.Context, []int64, *dbent.Tx, error) {
		if owned != nil {
			_ = owned.Rollback()
		}
		return ctx, nil, nil, err
	}
	ids, err := r.departmentSubscriptionIDs(ctx, filter)
	if err != nil {
		return fail(err)
	}
	if subscriptionID == 0 && filter.ScopeVersion == "" {
		actor, err := loadDepartmentActor(ctx, clientFromContext(ctx, r.client), service.AdminPermissionSubscriptions, service.AdminPermissionDepartmentSubscriptions)
		if err != nil {
			return fail(err)
		}
		if actor.Role != service.RoleAdmin && service.HasAdminPermission(actor, service.AdminPermissionDepartmentSubscriptions) {
			return fail(service.ErrDepartmentScopeChanged)
		}
	}
	lockIDs := []int64{service.DepartmentActorID(ctx)}
	client := clientFromContext(ctx, r.client)
	if subscriptionID > 0 {
		sub, err := client.UserSubscription.Get(ctx, subscriptionID)
		if err != nil {
			return fail(service.ErrDepartmentScopeDenied)
		}
		lockIDs = append(lockIDs, sub.UserID)
	} else {
		lockIDs = append(lockIDs, ids...)
	}
	if err = lockDepartmentUsers(ctx, client, lockIDs); err != nil {
		return fail(err)
	}
	current, err := r.departmentSubscriptionIDs(ctx, filter)
	if err != nil {
		return fail(err)
	}
	if ids != nil {
		// A batch cannot silently include new members that were not locked.
		locked := map[int64]bool{}
		for _, id := range lockIDs {
			locked[id] = true
		}
		if subscriptionID == 0 {
			for _, id := range current {
				if !locked[id] {
					return fail(service.ErrDepartmentScopeChanged)
				}
			}
		}
	}
	if subscriptionID > 0 {
		if err = r.checkDepartmentSubscription(ctx, subscriptionID); err != nil {
			return fail(err)
		}
	}
	return ctx, current, owned, nil
}
