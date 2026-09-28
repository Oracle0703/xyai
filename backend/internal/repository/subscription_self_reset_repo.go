package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/setting"
	"github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/Wei-Shaw/sub2api/internal/service"
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
	// A soft-deleted group loads as nil and is rejected as GROUP_DISABLED, matching Status.
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
