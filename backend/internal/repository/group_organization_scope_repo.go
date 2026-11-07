package repository

import (
	"context"
	"errors"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

// Group organization scopes (docs/features/organization-leader-design-cn.md
// D13) live in the native table group_organization_scopes, outside the
// upstream Ent group schema.

var _ service.GroupOrganizationScopeRepository = (*groupRepository)(nil)

func (r *groupRepository) ListGroupOrganizationScopes(ctx context.Context) ([]service.GroupOrganizationScope, error) {
	rows, err := clientFromContext(ctx, r.client).QueryContext(ctx, `SELECT s.group_id, array_agg(s.organization_key ORDER BY s.organization_key)
FROM group_organization_scopes s JOIN groups g ON g.id=s.group_id AND g.deleted_at IS NULL
GROUP BY s.group_id ORDER BY s.group_id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := []service.GroupOrganizationScope{}
	for rows.Next() {
		var item service.GroupOrganizationScope
		var keys pq.StringArray
		if err = rows.Scan(&item.GroupID, &keys); err != nil {
			return nil, err
		}
		item.OrganizationKeys = []string(keys)
		result = append(result, item)
	}
	return result, rows.Err()
}

// ReplaceGroupOrganizationScopes locks the groups FOR UPDATE in ID order, the
// same mode as guarded group deletion. Organization leaders' writes hold the
// group FOR SHARE until commit, so a replacement waits for in-flight writes
// and writes started later see the new scope.
func (r *groupRepository) ReplaceGroupOrganizationScopes(ctx context.Context, groupIDs []int64, organizationKeys []string) error {
	if dbent.TxFromContext(ctx) == nil {
		return errors.New("group organization scope replacement requires a transaction")
	}
	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx, `SELECT subscription_type FROM groups WHERE id=ANY($1) AND deleted_at IS NULL ORDER BY id FOR UPDATE`, pq.Array(groupIDs))
	if err != nil {
		return err
	}
	locked, standard := 0, 0
	for rows.Next() {
		var subscriptionType string
		if err = rows.Scan(&subscriptionType); err != nil {
			_ = rows.Close()
			return err
		}
		locked++
		if subscriptionType != service.SubscriptionTypeSubscription {
			standard++
		}
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}
	if locked != len(groupIDs) {
		return service.ErrGroupNotFound
	}
	// Scopes only apply to subscription groups; migration 264 clears them when a
	// group stops being one. Clearing is allowed for any group.
	if standard > 0 && len(organizationKeys) > 0 {
		return service.ErrGroupOrganizationScopeInvalid
	}
	if _, err = client.ExecContext(ctx, `DELETE FROM group_organization_scopes WHERE group_id=ANY($1)`, pq.Array(groupIDs)); err != nil {
		return err
	}
	if len(organizationKeys) == 0 {
		return nil
	}
	_, err = client.ExecContext(ctx, `INSERT INTO group_organization_scopes (group_id, organization_key)
SELECT g.id, k.key FROM unnest($1::bigint[]) AS g(id) CROSS JOIN unnest($2::text[]) AS k(key)`, pq.Array(groupIDs), pq.Array(organizationKeys))
	return err
}
