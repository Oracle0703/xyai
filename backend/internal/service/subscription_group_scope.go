package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// Organization leaders write subscriptions only in groups scoped to one of
// their organizations (docs/features/organization-leader-design-cn.md D13).
// Full administrators maintain the scopes; groups without a scope are
// assignable only by administrators and full-site subscription sub-admins.

// MaxGroupOrganizationScopeBatch bounds one scope replacement request.
const MaxGroupOrganizationScopeBatch = 500

var (
	ErrSubscriptionGroupScopeDenied = infraerrors.Forbidden("SUBSCRIPTION_GROUP_SCOPE_DENIED", "subscription group is not assigned to your organization")
	// ErrSubscriptionGroupDeleted is returned to organization leaders writing an
	// existing subscription (restore in practice) whose group was deleted. It is
	// only reported after the owner was authorized, so it discloses nothing.
	ErrSubscriptionGroupDeleted      = infraerrors.Conflict("SUBSCRIPTION_GROUP_DELETED", "the group of this subscription has been deleted")
	ErrGroupOrganizationScopeInvalid = infraerrors.BadRequest("GROUP_ORGANIZATION_SCOPE_INVALID", "invalid group organization scope request")
)

// GroupOrganizationScope lists the organizations a group is scoped to.
type GroupOrganizationScope struct {
	GroupID          int64    `json:"group_id"`
	OrganizationKeys []string `json:"organization_keys"`
}

// GroupOrganizationScopeInput replaces the scopes of every listed group with
// OrganizationKeys. OrganizationKeys must be an explicit array: nil (omitted or
// null) is rejected and [] clears the scopes.
type GroupOrganizationScopeInput struct {
	GroupIDs         []int64   `json:"group_ids"`
	OrganizationKeys *[]string `json:"organization_keys"`
}

// GroupOrganizationScopeRepository is implemented by the group repository.
type GroupOrganizationScopeRepository interface {
	ListGroupOrganizationScopes(ctx context.Context) ([]GroupOrganizationScope, error)
	// ReplaceGroupOrganizationScopes runs inside a transaction: it locks the
	// groups, rejects missing or deleted ones with ErrGroupNotFound and then
	// replaces their scopes.
	ReplaceGroupOrganizationScopes(ctx context.Context, groupIDs []int64, organizationKeys []string) error
}

// SubscriptionWriteAccess tells which subscriptions a scoped actor may write:
// the owner must be a write target and the group must be in the actor's
// organizations (design K14, D13).
type SubscriptionWriteAccess struct {
	Users  map[int64]bool
	Groups map[int64]bool
}

// Allows reports whether a subscription of userID in groupID is writable.
func (a *SubscriptionWriteAccess) Allows(userID, groupID int64) bool {
	return a != nil && a.Users[userID] && a.Groups[groupID]
}

func normalizeGroupOrganizationScopeInput(input GroupOrganizationScopeInput) ([]int64, []string, error) {
	if input.OrganizationKeys == nil || len(input.GroupIDs) == 0 || len(input.GroupIDs) > MaxGroupOrganizationScopeBatch {
		return nil, nil, ErrGroupOrganizationScopeInvalid
	}
	ids := make([]int64, 0, len(input.GroupIDs))
	for _, id := range input.GroupIDs {
		if id <= 0 {
			return nil, nil, ErrGroupOrganizationScopeInvalid
		}
		ids = append(ids, id)
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	keys := make([]string, 0, len(*input.OrganizationKeys))
	for _, raw := range *input.OrganizationKeys {
		key := strings.TrimSpace(raw)
		if !slices.Contains(OrganizationLeaderOrganizations, key) {
			return nil, nil, ErrGroupOrganizationScopeInvalid
		}
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return ids, slices.Compact(keys), nil
}

func (s *SubscriptionService) groupOrganizationScopeRepository(ctx context.Context) (GroupOrganizationScopeRepository, error) {
	// Scopes decide what organization leaders may give away, so only full
	// administrators maintain them; the route whitelist already excludes
	// every sub-admin and this check keeps the service closed on its own.
	if HasSubAdminAccountRestrictions(ctx) {
		return nil, ErrDepartmentScopeDenied
	}
	repo, ok := s.groupRepo.(GroupOrganizationScopeRepository)
	if !ok {
		return nil, errors.New("group organization scopes are not supported by the group repository")
	}
	return repo, nil
}

// ListGroupOrganizationScopes returns every scoped group; unscoped groups are
// omitted.
func (s *SubscriptionService) ListGroupOrganizationScopes(ctx context.Context) ([]GroupOrganizationScope, error) {
	repo, err := s.groupOrganizationScopeRepository(ctx)
	if err != nil {
		return nil, err
	}
	return repo.ListGroupOrganizationScopes(ctx)
}

// SetGroupOrganizationScopes replaces the scopes of the listed groups in one
// transaction and returns how many groups were updated.
func (s *SubscriptionService) SetGroupOrganizationScopes(ctx context.Context, input GroupOrganizationScopeInput) (int, error) {
	repo, err := s.groupOrganizationScopeRepository(ctx)
	if err != nil {
		return 0, err
	}
	ids, keys, err := normalizeGroupOrganizationScopeInput(input)
	if err != nil {
		return 0, err
	}
	if dbent.TxFromContext(ctx) != nil {
		return len(ids), repo.ReplaceGroupOrganizationScopes(ctx, ids, keys)
	}
	if s.entClient == nil {
		return 0, errors.New("group organization scopes require a transaction")
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err = repo.ReplaceGroupOrganizationScopes(dbent.NewTxContext(ctx, tx), ids, keys); err != nil {
		return 0, err
	}
	return len(ids), tx.Commit()
}

// AssignableSubscriptionGroups returns the active subscription groups the
// actor may assign. Organization leaders only get groups scoped to their
// organizations, from the same predicate as the write check (D13, K14).
func (s *SubscriptionService) AssignableSubscriptionGroups(ctx context.Context) ([]Group, error) {
	scope, err := s.subscriptionWriteScope(ctx, subscriptionWriteAssign)
	if err != nil {
		return nil, err
	}
	if scope != nil {
		return scope.ScopedAssignableSubscriptionGroups(ctx)
	}
	groups, err := s.groupRepo.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]Group, 0, len(groups))
	for _, group := range groups {
		if group.IsSubscriptionType() {
			result = append(result, group)
		}
	}
	return result, nil
}

// ManageableSubscriptions reports, for scoped sub-admins, which of the listed
// subscriptions (owner userIDs, groupIDs) the actor may write. It returns nil
// for actors whose writes are not limited; their responses carry no can_manage
// flag. Full-site subscription sub-admins are among them: the page offers them
// assignment only.
func (s *SubscriptionService) ManageableSubscriptions(ctx context.Context, userIDs, groupIDs []int64) (*SubscriptionWriteAccess, error) {
	scope, err := s.subscriptionWriteScope(ctx, subscriptionWriteAssign)
	if err != nil || scope == nil {
		return nil, err
	}
	return scope.SubscriptionWriteAccess(ctx, userIDs, groupIDs)
}
