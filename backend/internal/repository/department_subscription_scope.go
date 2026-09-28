package repository

import (
	"context"
	"errors"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

// nil means explicitly unrestricted; empty non-nil means an empty scoped result.
func (r *userSubscriptionRepository) departmentSubscriptionIDs(ctx context.Context, filter service.SubscriptionAdminFilter) ([]int64, error) {
	if service.DepartmentActorID(ctx) == 0 {
		return nil, nil
	}
	scope, err := resolveDepartmentQueryScope(ctx, clientFromContext(ctx, r.client), service.AdminPermissionDepartmentSubscriptions, filter.DepartmentQuery())
	if err != nil {
		return nil, err
	}
	if filter.ResolvedScopeVersion != nil {
		*filter.ResolvedScopeVersion = scope.Version
	}
	if scope.Unrestricted && (filter.DepartmentID == "" || filter.DepartmentID == "all") {
		return nil, nil
	}
	ids := make([]int64, 0, len(scope.Members))
	for _, m := range scope.Members {
		ids = append(ids, m.ID)
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
	if service.DepartmentActorID(ctx) == 0 {
		return nil
	}
	client := clientFromContext(ctx, r.client)
	actor, err := loadDepartmentActor(ctx, client, service.AdminPermissionSubscriptions, service.AdminPermissionDepartmentSubscriptions)
	if err != nil {
		return err
	}
	if departmentSubscriptionUnrestricted(actor) {
		return nil
	}
	rows, err := client.QueryContext(ctx, `SELECT 1 FROM user_subscriptions s JOIN users u ON u.id=s.user_id
 JOIN departments d ON d.id=u.department_id AND d.organization_key=`+organizationUsageOrganizationExpression("u")+`
 WHERE s.id=$1 AND u.deleted_at IS NULL AND u.status='active'
 AND EXISTS(SELECT 1 FROM department_access_grants dg WHERE dg.user_id=$2 AND dg.department_id=d.id)`, id, actor.ID)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	if rows.Next() {
		return nil
	}
	if err = rows.Err(); err != nil {
		return err
	}
	return service.ErrDepartmentScopeDenied
}

type departmentWriteActorKey struct{}

func departmentResetActor(ctx context.Context, q sqlExecutor) (*service.User, error) {
	if actor, ok := ctx.Value(departmentWriteActorKey{}).(*service.User); ok {
		return actor, nil
	}
	return loadDepartmentActor(ctx, q, service.AdminPermissionSubscriptions, service.AdminPermissionDepartmentSubscriptions)
}

// Use the same predicates for candidate discovery and the final atomic UPDATE.
func dailyResetPredicates(filter service.SubscriptionAdminFilter, now time.Time, memberIDs []int64) departmentPredicates {
	p := departmentPredicates{clauses: []string{"us.deleted_at IS NULL", "u.deleted_at IS NULL", "g.deleted_at IS NULL", "us.status='active'"}}
	p.add("us.expires_at > $%d", now)
	if memberIDs != nil {
		p.add("us.user_id=ANY($%d::bigint[])", pq.Array(memberIDs))
	}
	if filter.UserID != nil {
		p.add("us.user_id=$%d", *filter.UserID)
	}
	if filter.GroupID != nil {
		p.add("us.group_id=$%d", *filter.GroupID)
	}
	if filter.Platform != "" {
		p.add("g.platform=$%d", filter.Platform)
	}
	if filter.Organization != "" {
		p.add(organizationUsageOrganizationExpression("u")+"=$%d", filter.Organization)
	}
	if filter.Status != "" {
		p.add("us.status=$%d", filter.Status)
	}
	if filter.DepartmentID == "unassigned" {
		p.clauses = append(p.clauses, "u.department_id IS NULL")
	} else if filter.DepartmentID != "" && filter.DepartmentID != "all" {
		p.add("u.department_id=$%d::bigint", filter.DepartmentID)
	}
	return p
}

func (r *userSubscriptionRepository) beginDepartmentSubscriptionWrite(ctx context.Context, filter service.SubscriptionAdminFilter, subscriptionID int64, now time.Time) (context.Context, []int64, *dbent.Tx, error) {
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
	client := clientFromContext(ctx, r.client)
	actor, err := loadDepartmentActor(ctx, client, service.AdminPermissionSubscriptions, service.AdminPermissionDepartmentSubscriptions)
	if err != nil {
		return fail(err)
	}
	_, dept, err := service.NormalizeDepartmentFilter(filter.Organization, filter.DepartmentID)
	if err != nil {
		return fail(err)
	}
	constrained := !departmentSubscriptionUnrestricted(actor) || dept != "all"
	if subscriptionID == 0 && constrained && filter.ScopeVersion == "" {
		return fail(service.ErrDepartmentScopeChanged)
	}
	lockIDs := []int64{actor.ID}
	candidates := []int64{}
	var targetUser int64
	if subscriptionID > 0 {
		sub, err := client.UserSubscription.Get(ctx, subscriptionID)
		if dbent.IsNotFound(err) {
			return fail(service.ErrDepartmentScopeDenied)
		}
		if err != nil {
			return fail(err)
		}
		targetUser = sub.UserID
		if constrained {
			lockIDs = append(lockIDs, targetUser)
		}
	} else if constrained {
		p := dailyResetPredicates(filter, now, nil)
		if !departmentSubscriptionUnrestricted(actor) {
			p.add("u.status='active' AND EXISTS(SELECT 1 FROM department_access_grants dg JOIN departments d ON d.id=dg.department_id WHERE dg.user_id=$%d AND d.id=u.department_id AND d.organization_key="+organizationUsageOrganizationExpression("u")+")", actor.ID)
		}
		rows, err := client.QueryContext(ctx, `SELECT DISTINCT us.user_id FROM user_subscriptions us JOIN users u ON u.id=us.user_id JOIN groups g ON g.id=us.group_id WHERE `+p.where()+` ORDER BY us.user_id`, p.args...)
		if err != nil {
			return fail(err)
		}
		for rows.Next() {
			var id int64
			if err = rows.Scan(&id); err != nil {
				_ = rows.Close()
				return fail(err)
			}
			candidates = append(candidates, id)
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return fail(err)
		}
		lockIDs = append(lockIDs, candidates...)
	}
	if err = lockDepartmentUsers(ctx, client, lockIDs); err != nil {
		return fail(err)
	}
	currentActor, err := loadDepartmentActor(ctx, client, service.AdminPermissionSubscriptions, service.AdminPermissionDepartmentSubscriptions)
	if err != nil {
		return fail(err)
	}
	if departmentSubscriptionUnrestricted(actor) != departmentSubscriptionUnrestricted(currentActor) {
		return fail(service.ErrDepartmentScopeChanged)
	}
	var allowedIDs []int64
	if constrained || filter.ScopeVersion != "" {
		selection := filter.DepartmentQuery()
		if subscriptionID > 0 {
			selection.UserID = &targetUser
		}
		scope, err := departmentQueryScopeForActor(ctx, client, currentActor, service.AdminPermissionDepartmentSubscriptions, selection)
		if err != nil {
			return fail(err)
		}
		allowed := map[int64]bool{}
		for _, m := range scope.Members {
			allowed[m.ID] = true
		}
		if subscriptionID > 0 && !allowed[targetUser] {
			return fail(service.ErrDepartmentScopeDenied)
		}
		if constrained && subscriptionID == 0 {
			// Freeze the locked candidate population; later arrivals cannot expand the UPDATE.
			allowedIDs = []int64{}
			for _, id := range candidates {
				if allowed[id] {
					allowedIDs = append(allowedIDs, id)
				}
			}
		}
	}
	if subscriptionID > 0 {
		sub, err := client.UserSubscription.Query().Where(usersubscription.IDEQ(subscriptionID)).ForUpdate().Only(ctx)
		if dbent.IsNotFound(err) {
			return fail(service.ErrDepartmentScopeDenied)
		}
		if err != nil {
			return fail(err)
		}
		if sub.UserID != targetUser {
			return fail(service.ErrDepartmentScopeChanged)
		}
	}
	ctx = context.WithValue(ctx, departmentWriteActorKey{}, currentActor)
	return ctx, allowedIDs, owned, nil
}
