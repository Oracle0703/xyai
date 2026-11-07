package service

import (
	"context"
	"errors"
	"fmt"

	dbent "github.com/Wei-Shaw/sub2api/ent"
)

// SubscriptionWriteScopeRepository limits organization leaders' subscription
// writes to their write targets (design §4.8, D11). It is implemented by the
// subscription repository so that authorization, the write and its audit share
// one transaction.
type SubscriptionWriteScopeRepository interface {
	CurrentSubscriptionWriter(ctx context.Context) (SubscriptionWriterKind, error)
	// SubscriptionWriteAccess reads the actor once and reports which of userIDs
	// are write targets and which of groupIDs are scoped to the actor's
	// organizations (K14, D13); both are empty for actors that are not
	// organization leaders.
	SubscriptionWriteAccess(ctx context.Context, userIDs, groupIDs []int64) (*SubscriptionWriteAccess, error)
	// ScopedAssignableSubscriptionGroups returns the active subscription groups
	// scoped to the actor's organizations (D13), ordered like ListActive and
	// with picker fields only; empty for actors that are not organization leaders.
	ScopedAssignableSubscriptionGroups(ctx context.Context) ([]Group, error)
	AuthorizeSubscriptionWriteUsers(ctx context.Context, userIDs []int64, lock bool) (context.Context, error)
	// AuthorizeSubscriptionWriteGroup checks the group against the actor
	// authorized by AuthorizeSubscriptionWriteUsers in the same context; with
	// lock it holds the group row until commit (D13).
	AuthorizeSubscriptionWriteGroup(ctx context.Context, groupID int64, lock bool) error
	// AuthorizeSubscriptionWriteByID authorizes both the owner and the group of
	// the subscription.
	AuthorizeSubscriptionWriteByID(ctx context.Context, subscriptionID int64, includeDeleted, lock bool) (context.Context, int64, error)
	RunSubscriptionWriteStep(ctx context.Context, step func(context.Context) error) (stepErr error, err error)
	RecordSubscriptionWrite(ctx context.Context, action string, details map[string]any) error
}

// subscriptionWriteAbort marks a batch failure that must roll back the whole
// batch rather than count as one failed target.
type subscriptionWriteAbort struct{ err error }

func (e *subscriptionWriteAbort) Error() string { return e.err.Error() }
func (e *subscriptionWriteAbort) Unwrap() error { return e.err }

// abortsScopedBatch reports errors that may not be reported per target:
// authorization failures (design G13, K20) and broken savepoints.
func abortsScopedBatch(err error) bool {
	var abort *subscriptionWriteAbort
	return errors.As(err, &abort) || errors.Is(err, ErrDepartmentScopeDenied) || errors.Is(err, ErrDepartmentScopeChanged) || errors.Is(err, ErrSubscriptionGroupScopeDenied)
}

// SubscriptionWriterKind is what a sub-admin request's actor currently is. The
// service re-reads it because grants may change after the admin middleware
// admitted the request (design K19, K21).
type SubscriptionWriterKind int

const (
	// SubscriptionWriterScoped is any other sub-admin; only organization
	// leaders then pass the target authorization. It is the zero value, so an
	// unset kind is the most restricted one.
	SubscriptionWriterScoped SubscriptionWriterKind = iota
	// SubscriptionWriterFullSite is a sub-admin holding admin.subscriptions:
	// site-wide assignment, but never extend, revoke or restore.
	SubscriptionWriterFullSite
	// SubscriptionWriterAdministrator writes without restriction.
	SubscriptionWriterAdministrator
)

// subscriptionWriteAction is the kind of write being authorized. The full-site
// subscription permission grants assignment only (adminPermissionRouteRules).
type subscriptionWriteAction int

const (
	subscriptionWriteAssign subscriptionWriteAction = iota // assign, bulk-assign
	subscriptionWriteManage                                // extend, revoke, restore
)

// subscriptionWriteScope returns nil for actors whose writes are not scoped:
// system and user contexts, full administrators, and full-site subscription
// sub-admins assigning. Their paths stay unchanged. Actions are checked against
// the current identity, never combining an action the request was admitted for
// with a site-wide scope gained since (design K21).
func (s *SubscriptionService) subscriptionWriteScope(ctx context.Context, action subscriptionWriteAction) (SubscriptionWriteScopeRepository, error) {
	if DepartmentActorID(ctx) <= 0 || !HasSubAdminAccountRestrictions(ctx) {
		return nil, nil
	}
	scope, ok := s.userSubRepo.(SubscriptionWriteScopeRepository)
	if !ok {
		return nil, ErrDepartmentScopeDenied
	}
	writer, err := scope.CurrentSubscriptionWriter(ctx)
	if err != nil {
		return nil, err
	}
	switch writer {
	case SubscriptionWriterAdministrator:
		return nil, nil
	case SubscriptionWriterFullSite:
		if action == subscriptionWriteAssign {
			return nil, nil
		}
		return nil, ErrDepartmentScopeDenied
	default:
		return scope, nil
	}
}

// scopedSubscriptionWrite runs fn in its own transaction; fn authorizes with
// lock before writing. Cache invalidation inside fn may run before commit, so
// callers invalidate again after this returns.
func (s *SubscriptionService) scopedSubscriptionWrite(ctx context.Context, fn func(context.Context) error) error {
	if dbent.TxFromContext(ctx) != nil {
		return fn(ctx)
	}
	if s.entClient == nil {
		return errors.New("scoped subscription write requires a transaction")
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err = fn(dbent.NewTxContext(ctx, tx)); err != nil {
		return err
	}
	return tx.Commit()
}

func subscriptionWriteDetails(ctx context.Context, userID int64, details map[string]any) map[string]any {
	details["user_id"] = userID
	if userID == DepartmentActorID(ctx) {
		details["self"] = true
	}
	return details
}

// PrecheckSubscriptionWrite authorizes a scoped write against the current
// grants before an idempotent replay, which never reaches the service (design
// G17). Only extend replays, so the check is that of a manage action (K21).
func (s *SubscriptionService) PrecheckSubscriptionWrite(ctx context.Context, subscriptionID int64) error {
	scope, err := s.subscriptionWriteScope(ctx, subscriptionWriteManage)
	if err != nil || scope == nil {
		return err
	}
	_, _, err = scope.AuthorizeSubscriptionWriteByID(ctx, subscriptionID, false, false)
	return err
}

func (s *SubscriptionService) assignScopedSubscription(ctx context.Context, scope SubscriptionWriteScopeRepository, input *AssignSubscriptionInput) (*UserSubscription, bool, error) {
	var sub *UserSubscription
	var reused bool
	err := s.scopedSubscriptionWrite(ctx, func(txCtx context.Context) error {
		txCtx, err := scope.AuthorizeSubscriptionWriteUsers(txCtx, []int64{input.UserID}, true)
		if err != nil {
			return err
		}
		if err = scope.AuthorizeSubscriptionWriteGroup(txCtx, input.GroupID, true); err != nil {
			return err
		}
		sub, reused, err = s.assignAuthorizedSubscription(txCtx, scope, input)
		return err
	})
	if err != nil {
		return nil, false, err
	}
	s.maybeInvalidateAssignmentCaches(input.UserID, input.GroupID, false)
	return sub, reused, nil
}

// assignAuthorizedSubscription writes one assignment and its audit; the
// caller has locked and authorized the target in the same transaction.
func (s *SubscriptionService) assignAuthorizedSubscription(txCtx context.Context, scope SubscriptionWriteScopeRepository, input *AssignSubscriptionInput) (*UserSubscription, bool, error) {
	sub, reused, err := s.assignSubscriptionWithReuse(txCtx, input)
	if err != nil {
		return nil, false, err
	}
	return sub, reused, scope.RecordSubscriptionWrite(txCtx, "organization.subscription_assign", subscriptionWriteDetails(txCtx, input.UserID, map[string]any{
		"subscription_id": sub.ID, "group_id": input.GroupID, "validity_days": input.ValidityDays, "reused": reused,
	}))
}

func (s *SubscriptionService) bulkAssignScopedSubscription(ctx context.Context, scope SubscriptionWriteScopeRepository, input *BulkAssignSubscriptionInput) (*BulkAssignResult, error) {
	var result *BulkAssignResult
	err := s.scopedSubscriptionWrite(ctx, func(txCtx context.Context) error {
		// One transaction locks the actor and every target before the first
		// write and keeps the locks until commit, so scope cannot change
		// mid-batch; one target out of scope rejects the batch (design G13, K20).
		txCtx, err := scope.AuthorizeSubscriptionWriteUsers(txCtx, input.UserIDs, true)
		if err != nil {
			return err
		}
		if err = scope.AuthorizeSubscriptionWriteGroup(txCtx, input.GroupID, true); err != nil {
			return err
		}
		result, err = s.bulkAssign(input, func(assign *AssignSubscriptionInput) (*UserSubscription, bool, error) {
			var sub *UserSubscription
			var reused bool
			// A business failure of one target (e.g. a conflicting assignment)
			// is undone by its savepoint and reported for that target only.
			stepErr, err := scope.RunSubscriptionWriteStep(txCtx, func(stepCtx context.Context) error {
				var e error
				sub, reused, e = s.assignAuthorizedSubscription(stepCtx, scope, assign)
				return e
			})
			if err != nil {
				return nil, false, &subscriptionWriteAbort{err: err}
			}
			return sub, reused, stepErr
		}, abortsScopedBatch)
		return err
	})
	if err != nil {
		return nil, err
	}
	for i := range result.Subscriptions {
		s.maybeInvalidateAssignmentCaches(result.Subscriptions[i].UserID, result.Subscriptions[i].GroupID, false)
	}
	return result, nil
}

func (s *SubscriptionService) extendScopedSubscription(ctx context.Context, scope SubscriptionWriteScopeRepository, subscriptionID int64, days int) (*UserSubscription, error) {
	var sub *UserSubscription
	err := s.scopedSubscriptionWrite(ctx, func(txCtx context.Context) error {
		txCtx, owner, err := scope.AuthorizeSubscriptionWriteByID(txCtx, subscriptionID, false, true)
		if err != nil {
			return err
		}
		if sub, err = s.extendSubscription(txCtx, subscriptionID, days); err != nil {
			return err
		}
		return scope.RecordSubscriptionWrite(txCtx, "organization.subscription_extend", subscriptionWriteDetails(ctx, owner, map[string]any{
			"subscription_id": subscriptionID, "group_id": sub.GroupID, "days": days, "expires_at": sub.ExpiresAt,
		}))
	})
	if err != nil {
		return nil, err
	}
	s.maybeInvalidateAssignmentCaches(sub.UserID, sub.GroupID, false)
	return sub, nil
}

func (s *SubscriptionService) revokeScopedSubscription(ctx context.Context, scope SubscriptionWriteScopeRepository, subscriptionID int64) error {
	var sub *UserSubscription
	err := s.scopedSubscriptionWrite(ctx, func(txCtx context.Context) error {
		txCtx, owner, err := scope.AuthorizeSubscriptionWriteByID(txCtx, subscriptionID, false, true)
		if err != nil {
			return err
		}
		if sub, err = s.revokeSubscription(txCtx, subscriptionID); err != nil {
			return err
		}
		return scope.RecordSubscriptionWrite(txCtx, "organization.subscription_revoke", subscriptionWriteDetails(ctx, owner, map[string]any{
			"subscription_id": subscriptionID, "group_id": sub.GroupID,
		}))
	})
	if err != nil {
		return err
	}
	return s.invalidateSubscriptionCaches(sub.UserID, sub.GroupID)
}

func (s *SubscriptionService) restoreScopedSubscription(ctx context.Context, scope SubscriptionWriteScopeRepository, subscriptionID int64) (*UserSubscription, error) {
	var sub *UserSubscription
	err := s.scopedSubscriptionWrite(ctx, func(txCtx context.Context) error {
		txCtx, owner, err := scope.AuthorizeSubscriptionWriteByID(txCtx, subscriptionID, true, true)
		if err != nil {
			return err
		}
		if sub, err = s.restoreSubscription(txCtx, subscriptionID); err != nil {
			return err
		}
		return scope.RecordSubscriptionWrite(txCtx, "organization.subscription_restore", subscriptionWriteDetails(ctx, owner, map[string]any{
			"subscription_id": subscriptionID, "group_id": sub.GroupID, "status": sub.Status,
		}))
	})
	if err != nil {
		return nil, err
	}
	if err = s.invalidateSubscriptionCaches(sub.UserID, sub.GroupID); err != nil {
		return nil, err
	}
	return sub, nil
}
