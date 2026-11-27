package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/setting"
	"github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type subscriptionSelfResetRepository struct{ client *dbent.Client }

func NewSubscriptionSelfResetRepository(client *dbent.Client) service.SubscriptionSelfResetRepository {
	return &subscriptionSelfResetRepository{client: client}
}

func (r *subscriptionSelfResetRepository) ReadPolicy(ctx context.Context) (string, error) {
	row, err := clientFromContext(ctx, r.client).Setting.Query().Where(setting.KeyEQ(service.SubscriptionSelfResetPolicyKey)).Only(ctx)
	if err != nil {
		return "", err
	}
	return row.Value, nil
}

func (r *subscriptionSelfResetRepository) WritePolicy(ctx context.Context, value string) error {
	if dbent.TxFromContext(ctx) == nil {
		return errors.New("self reset policy writes require a configuration transaction")
	}
	return clientFromContext(ctx, r.client).Setting.Create().SetKey(service.SubscriptionSelfResetPolicyKey).SetValue(value).SetUpdatedAt(time.Now()).OnConflictColumns(setting.FieldKey).UpdateNewValues().Exec(ctx)
}

func (r *subscriptionSelfResetRepository) ListOwned(ctx context.Context, userID int64) ([]service.UserSubscription, map[int64]service.SubscriptionSelfResetUsage, string, error) {
	client := clientFromContext(ctx, r.client)
	owner, err := client.User.Query().Where(user.IDEQ(userID), user.StatusEQ(service.StatusActive)).Only(ctx)
	if err != nil {
		return nil, nil, "", translatePersistenceError(err, service.ErrUserNotFound, nil)
	}
	rows, err := client.UserSubscription.Query().Where(usersubscription.UserIDEQ(userID)).WithGroup().All(ctx)
	if err != nil {
		return nil, nil, "", err
	}
	counts := make(map[int64]service.SubscriptionSelfResetUsage, len(rows))
	result, err := client.QueryContext(ctx, `SELECT c.subscription_id, to_char(c.quota_date, 'YYYY-MM-DD'), c.used_count FROM subscription_self_daily_reset_usage c JOIN user_subscriptions s ON s.id=c.subscription_id WHERE s.user_id=$1 AND s.deleted_at IS NULL`, userID)
	if err != nil {
		return nil, nil, "", err
	}
	defer func() { _ = result.Close() }()
	for result.Next() {
		var id int64
		var count service.SubscriptionSelfResetUsage
		if err := result.Scan(&id, &count.QuotaDate, &count.UsedCount); err != nil {
			return nil, nil, "", err
		}
		counts[id] = count
	}
	if err := result.Err(); err != nil {
		return nil, nil, "", err
	}
	return userSubscriptionEntitiesToService(rows), counts, owner.Email, nil
}

// GetOwnedByIDForUpdate and Consume rely on SubscriptionSelfResetService.Reset having asserted the transaction.
func (r *subscriptionSelfResetRepository) GetOwnedByIDForUpdate(ctx context.Context, userID, subscriptionID int64) (*service.UserSubscription, error) {
	client := clientFromContext(ctx, r.client)
	// Ent loads edges in separate queries after locking only the owned subscription.
	// A soft-deleted group loads as nil and is rejected as GROUP_NOT_ALLOWED, matching Status.
	row, err := client.UserSubscription.Query().Where(usersubscription.IDEQ(subscriptionID), usersubscription.UserIDEQ(userID), usersubscription.DeletedAtIsNil()).ForUpdate().WithUser().WithGroup().Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
	}
	return userSubscriptionEntityToService(row), nil
}

func (r *subscriptionSelfResetRepository) GetUsage(ctx context.Context, subscriptionID int64) (service.SubscriptionSelfResetUsage, error) {
	var usage service.SubscriptionSelfResetUsage
	rows, err := clientFromContext(ctx, r.client).QueryContext(ctx, `SELECT to_char(quota_date, 'YYYY-MM-DD'), used_count FROM subscription_self_daily_reset_usage WHERE subscription_id=$1`, subscriptionID)
	if err != nil {
		return usage, err
	}
	defer func() { _ = rows.Close() }()
	if rows.Next() {
		if err := rows.Scan(&usage.QuotaDate, &usage.UsedCount); err != nil {
			return usage, err
		}
	}
	return usage, rows.Err()
}

func (r *subscriptionSelfResetRepository) Consume(ctx context.Context, subscriptionID int64, date string) error {
	_, err := clientFromContext(ctx, r.client).ExecContext(ctx, `INSERT INTO subscription_self_daily_reset_usage (subscription_id, quota_date, used_count) VALUES ($1, $2::date, 1) ON CONFLICT (subscription_id) DO UPDATE SET quota_date=EXCLUDED.quota_date, used_count=CASE WHEN subscription_self_daily_reset_usage.quota_date=EXCLUDED.quota_date THEN subscription_self_daily_reset_usage.used_count+1 ELSE 1 END, updated_at=NOW()`, subscriptionID, date)
	return err
}

func (r *subscriptionSelfResetRepository) RecordEvent(ctx context.Context, event *service.SubscriptionSelfResetEvent) error {
	_, err := clientFromContext(ctx, r.client).ExecContext(ctx, `INSERT INTO subscription_self_reset_events (subscription_id, user_id, group_id, organization, quota_date, used_count, daily_limit, daily_usage_usd_before) VALUES ($1, $2, $3, $4, $5::date, $6, $7, $8)`,
		event.SubscriptionID, event.UserID, event.GroupID, event.Organization, event.QuotaDate, event.UsedCount, event.DailyLimit, event.DailyUsageUSDBefore)
	return err
}

func (r *subscriptionSelfResetRepository) ListEvents(ctx context.Context, filter service.SubscriptionSelfResetEventFilter) ([]service.SubscriptionSelfResetEvent, int64, error) {
	client := clientFromContext(ctx, r.client)
	var conds []string
	var args []any
	add := func(cond string, arg any) {
		args = append(args, arg)
		conds = append(conds, fmt.Sprintf(cond, len(args)))
	}
	if filter.Email != "" {
		add(`u.email ILIKE '%%' || $%d || '%%'`, escapeLikeWildcards(filter.Email))
	}
	if filter.Organization != "" {
		add(`e.organization = $%d`, filter.Organization)
	}
	if filter.GroupID > 0 {
		add(`e.group_id = $%d`, filter.GroupID)
	}
	if filter.StartDate != "" {
		add(`e.quota_date >= $%d::date`, filter.StartDate)
	}
	if filter.EndDate != "" {
		add(`e.quota_date <= $%d::date`, filter.EndDate)
	}
	from := ` FROM subscription_self_reset_events e LEFT JOIN users u ON u.id = e.user_id LEFT JOIN groups g ON g.id = e.group_id`
	if len(conds) > 0 {
		from += " WHERE " + strings.Join(conds, " AND ")
	}
	var total int64
	countRows, err := client.QueryContext(ctx, `SELECT COUNT(*)`+from, args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = countRows.Close() }()
	if countRows.Next() {
		if err := countRows.Scan(&total); err != nil {
			return nil, 0, err
		}
	}
	if err := countRows.Err(); err != nil {
		return nil, 0, err
	}
	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)
	rows, err := client.QueryContext(ctx, `SELECT e.id, e.subscription_id, e.user_id, COALESCE(u.email, ''), e.group_id, COALESCE(g.name, ''), e.organization, to_char(e.quota_date, 'YYYY-MM-DD'), e.used_count, e.daily_limit, e.daily_usage_usd_before::float8, e.created_at`+from+fmt.Sprintf(` ORDER BY e.id DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	events := []service.SubscriptionSelfResetEvent{}
	for rows.Next() {
		var e service.SubscriptionSelfResetEvent
		if err := rows.Scan(&e.ID, &e.SubscriptionID, &e.UserID, &e.UserEmail, &e.GroupID, &e.GroupName, &e.Organization, &e.QuotaDate, &e.UsedCount, &e.DailyLimit, &e.DailyUsageUSDBefore, &e.CreatedAt); err != nil {
			return nil, 0, err
		}
		events = append(events, e)
	}
	return events, total, rows.Err()
}

func (r *subscriptionSelfResetRepository) ListGroups(ctx context.Context) (map[string][]service.SubscriptionSelfResetGroup, error) {
	client := clientFromContext(ctx, r.client)
	result := make(map[string][]service.SubscriptionSelfResetGroup, 3)
	for _, organization := range []string{service.OrganizationXunyou, service.OrganizationWsdashi, service.OrganizationOther} {
		query := `SELECT g.id, g.name, g.platform, g.status, g.subscription_type, g.daily_limit_usd,
EXISTS (SELECT 1 FROM subscription_self_reset_groups r WHERE r.group_id=g.id AND r.organization_key=$1) AS enabled
FROM groups g
WHERE g.deleted_at IS NULL AND g.subscription_type='subscription'
  AND CASE WHEN $1='other'
           THEN NOT EXISTS (SELECT 1 FROM group_organization_scopes s WHERE s.group_id=g.id)
           ELSE EXISTS (SELECT 1 FROM group_organization_scopes s WHERE s.group_id=g.id AND s.organization_key=$1)
      END
ORDER BY g.sort_order ASC, g.id ASC`
		rows, err := client.QueryContext(ctx, query, organization)
		if err != nil {
			return nil, err
		}
		items := make([]service.SubscriptionSelfResetGroup, 0)
		for rows.Next() {
			var item service.SubscriptionSelfResetGroup
			var dailyLimit sql.NullFloat64
			if err := rows.Scan(&item.ID, &item.Name, &item.Platform, &item.Status, &item.SubscriptionType, &dailyLimit, &item.Enabled); err != nil {
				_ = rows.Close()
				return nil, err
			}
			if dailyLimit.Valid {
				value := dailyLimit.Float64
				item.DailyLimitUSD = &value
			}
			items = append(items, item)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
		result[organization] = items
	}
	return result, nil
}

func (r *subscriptionSelfResetRepository) ListEventGroups(ctx context.Context) ([]service.SubscriptionSelfResetEventGroup, error) {
	rows, err := clientFromContext(ctx, r.client).QueryContext(ctx, `
SELECT ids.group_id, COALESCE(g.name, '#' || ids.group_id::text)
  FROM (SELECT DISTINCT group_id FROM subscription_self_reset_events) ids
  LEFT JOIN groups g ON g.id=ids.group_id
 ORDER BY ids.group_id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]service.SubscriptionSelfResetEventGroup, 0)
	for rows.Next() {
		var item service.SubscriptionSelfResetEventGroup
		if err := rows.Scan(&item.ID, &item.Name); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// A single SELECT makes the rollout, daily limits, ticks and current scopes
// describe one committed configuration even at READ COMMITTED.
func (r *subscriptionSelfResetRepository) ReadAdmission(ctx context.Context, groupIDs []int64, organization string) (string, map[int64]bool, error) {
	var raw string
	var ids pq.Int64Array
	err := scanSingleRow(ctx, clientFromContext(ctx, r.client), `
SELECT p.value, ARRAY(
    SELECT r.group_id
    FROM subscription_self_reset_groups r
    JOIN groups g ON g.id=r.group_id
    WHERE r.group_id=ANY($1) AND r.organization_key=$2
      AND g.deleted_at IS NULL AND g.subscription_type='subscription'
      AND CASE WHEN $2='other'
               THEN NOT EXISTS (SELECT 1 FROM group_organization_scopes s WHERE s.group_id=g.id)
               ELSE EXISTS (SELECT 1 FROM group_organization_scopes s WHERE s.group_id=g.id AND s.organization_key=$2)
          END
)
FROM settings p WHERE p.key=$3`, []any{pq.Array(groupIDs), organization, service.SubscriptionSelfResetPolicyKey}, &raw, &ids)
	if err != nil {
		return "", nil, err
	}
	result := make(map[int64]bool, len(ids))
	for _, id := range ids {
		result[id] = true
	}
	return raw, result, nil
}

// ApplyGroupChanges locks the groups FOR UPDATE in ID order, the same mode as
// scope replacement and group deletion, then writes in separate statements so
// they read the scope committed by whoever held the lock before. Without a
// transaction the lock would be released at once.
func (r *subscriptionSelfResetRepository) ApplyGroupChanges(ctx context.Context, changes map[string]service.SubscriptionSelfResetGroupChanges) (map[string][]int64, error) {
	if dbent.TxFromContext(ctx) == nil {
		return nil, errors.New("self reset group changes require a transaction")
	}
	client := clientFromContext(ctx, r.client)
	idsSet := make(map[int64]struct{})
	for _, change := range changes {
		for _, id := range append(append([]int64{}, change.Enable...), change.Disable...) {
			if id > 0 {
				idsSet[id] = struct{}{}
			}
		}
	}
	ids := make([]int64, 0, len(idsSet))
	for id := range idsSet {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if len(ids) > 0 {
		rows, err := client.QueryContext(ctx, `SELECT id FROM groups WHERE id=ANY($1) AND deleted_at IS NULL ORDER BY id FOR UPDATE`, pq.Array(ids))
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				_ = rows.Close()
				return nil, err
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}

	ignored := make(map[string][]int64)
	for _, organization := range []string{service.OrganizationXunyou, service.OrganizationWsdashi, service.OrganizationOther} {
		change, ok := changes[organization]
		if !ok {
			continue
		}
		if len(change.Disable) > 0 {
			if _, err := client.ExecContext(ctx, `DELETE FROM subscription_self_reset_groups WHERE organization_key=$1 AND group_id=ANY($2)`, organization, pq.Array(change.Disable)); err != nil {
				return nil, err
			}
		}
		if len(change.Enable) == 0 {
			continue
		}
		// $1 needs the cast: the WHERE clause compares it as text while the
		// INSERT target would coerce a bare parameter to varchar, and PostgreSQL
		// rejects the statement with "inconsistent types deduced for parameter $1".
		if _, err := client.ExecContext(ctx, `
INSERT INTO subscription_self_reset_groups (group_id, organization_key)
SELECT g.id, $1::text
  FROM groups g
 WHERE g.id=ANY($2) AND g.deleted_at IS NULL AND g.subscription_type='subscription'
   AND CASE WHEN $1='other'
            THEN NOT EXISTS (SELECT 1 FROM group_organization_scopes s WHERE s.group_id=g.id)
            ELSE EXISTS (SELECT 1 FROM group_organization_scopes s WHERE s.group_id=g.id AND s.organization_key=$1)
       END
ON CONFLICT DO NOTHING`, organization, pq.Array(change.Enable)); err != nil {
			return nil, err
		}
		eligible, err := r.allowedIDsForOrganization(ctx, change.Enable, organization)
		if err != nil {
			return nil, err
		}
		for _, id := range change.Enable {
			if !eligible[id] {
				ignored[organization] = append(ignored[organization], id)
			}
		}
	}
	return ignored, nil
}

func (r *subscriptionSelfResetRepository) allowedIDsForOrganization(ctx context.Context, groupIDs []int64, organization string) (map[int64]bool, error) {
	result := make(map[int64]bool, len(groupIDs))
	if len(groupIDs) == 0 {
		return result, nil
	}
	rows, err := clientFromContext(ctx, r.client).QueryContext(ctx, `
SELECT g.id
  FROM groups g
 WHERE g.id=ANY($1) AND g.deleted_at IS NULL AND g.subscription_type='subscription'
   AND CASE WHEN $2='other'
            THEN NOT EXISTS (SELECT 1 FROM group_organization_scopes s WHERE s.group_id=g.id)
            ELSE EXISTS (SELECT 1 FROM group_organization_scopes s WHERE s.group_id=g.id AND s.organization_key=$2)
       END`, pq.Array(groupIDs), organization)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		result[id] = true
	}
	return result, rows.Err()
}
