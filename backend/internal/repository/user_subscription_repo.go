package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/ent/predicate"
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type userSubscriptionRepository struct {
	client *dbent.Client
}

func NewUserSubscriptionRepository(client *dbent.Client) service.UserSubscriptionRepository {
	return &userSubscriptionRepository{client: client}
}

func (r *userSubscriptionRepository) Create(ctx context.Context, sub *service.UserSubscription) error {
	if sub == nil {
		return service.ErrSubscriptionNilInput
	}

	client := clientFromContext(ctx, r.client)
	builder := client.UserSubscription.Create().
		SetUserID(sub.UserID).
		SetGroupID(sub.GroupID).
		SetExpiresAt(sub.ExpiresAt).
		SetNillableDailyWindowStart(sub.DailyWindowStart).
		SetNillableWeeklyWindowStart(sub.WeeklyWindowStart).
		SetNillableMonthlyWindowStart(sub.MonthlyWindowStart).
		SetDailyUsageUsd(sub.DailyUsageUSD).
		SetWeeklyUsageUsd(sub.WeeklyUsageUSD).
		SetMonthlyUsageUsd(sub.MonthlyUsageUSD).
		SetNillableAssignedBy(sub.AssignedBy)

	if sub.StartsAt.IsZero() {
		builder.SetStartsAt(time.Now())
	} else {
		builder.SetStartsAt(sub.StartsAt)
	}
	if sub.Status != "" {
		builder.SetStatus(sub.Status)
	}
	if !sub.AssignedAt.IsZero() {
		builder.SetAssignedAt(sub.AssignedAt)
	}
	// Keep compatibility with historical behavior: always store notes as a string value.
	builder.SetNotes(sub.Notes)

	created, err := builder.Save(ctx)
	if err == nil {
		applyUserSubscriptionEntityToService(sub, created)
	}
	return translatePersistenceError(err, nil, service.ErrSubscriptionAlreadyExists)
}

func (r *userSubscriptionRepository) GetByID(ctx context.Context, id int64) (*service.UserSubscription, error) {
	client := clientFromContext(ctx, r.client)
	m, err := client.UserSubscription.Query().
		Where(usersubscription.IDEQ(id)).
		WithUser().
		WithGroup().
		WithAssignedByUser().
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
	}
	return userSubscriptionEntityToService(m), nil
}

func (r *userSubscriptionRepository) GetByIDForUpdate(ctx context.Context, id int64) (*service.UserSubscription, error) {
	client := clientFromContext(ctx, r.client)
	m, err := client.UserSubscription.Query().
		Where(usersubscription.IDEQ(id)).
		ForUpdate().
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
	}
	return userSubscriptionEntityToService(m), nil
}

func (r *userSubscriptionRepository) GetByIDIncludeDeleted(ctx context.Context, id int64) (*service.UserSubscription, error) {
	client := clientFromContext(ctx, r.client)
	queryCtx := mixins.SkipSoftDelete(ctx)
	m, err := client.UserSubscription.Query().
		Where(usersubscription.IDEQ(id)).
		WithUser().
		WithGroup().
		WithAssignedByUser().
		Only(queryCtx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
	}
	return userSubscriptionEntityToServicePreserveStatus(m), nil
}

func (r *userSubscriptionRepository) GetByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error) {
	client := clientFromContext(ctx, r.client)
	m, err := client.UserSubscription.Query().
		Where(usersubscription.UserIDEQ(userID), usersubscription.GroupIDEQ(groupID)).
		WithGroup().
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
	}
	return userSubscriptionEntityToService(m), nil
}

func (r *userSubscriptionRepository) GetActiveByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error) {
	client := clientFromContext(ctx, r.client)
	m, err := client.UserSubscription.Query().
		Where(
			usersubscription.UserIDEQ(userID),
			usersubscription.GroupIDEQ(groupID),
			usersubscription.StatusEQ(service.SubscriptionStatusActive),
			usersubscription.ExpiresAtGT(time.Now()),
		).
		WithGroup().
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
	}
	return userSubscriptionEntityToService(m), nil
}

func (r *userSubscriptionRepository) Update(ctx context.Context, sub *service.UserSubscription) error {
	if sub == nil {
		return service.ErrSubscriptionNilInput
	}

	client := clientFromContext(ctx, r.client)
	builder := client.UserSubscription.UpdateOneID(sub.ID).
		SetUserID(sub.UserID).
		SetGroupID(sub.GroupID).
		SetStartsAt(sub.StartsAt).
		SetExpiresAt(sub.ExpiresAt).
		SetStatus(sub.Status).
		SetNillableDailyWindowStart(sub.DailyWindowStart).
		SetNillableWeeklyWindowStart(sub.WeeklyWindowStart).
		SetNillableMonthlyWindowStart(sub.MonthlyWindowStart).
		SetDailyUsageUsd(sub.DailyUsageUSD).
		SetWeeklyUsageUsd(sub.WeeklyUsageUSD).
		SetMonthlyUsageUsd(sub.MonthlyUsageUSD).
		SetNillableAssignedBy(sub.AssignedBy).
		SetAssignedAt(sub.AssignedAt).
		SetNotes(sub.Notes)

	updated, err := builder.Save(ctx)
	if err == nil {
		applyUserSubscriptionEntityToService(sub, updated)
		return nil
	}
	return translatePersistenceError(err, service.ErrSubscriptionNotFound, service.ErrSubscriptionAlreadyExists)
}

func (r *userSubscriptionRepository) Delete(ctx context.Context, id int64) error {
	// Match GORM semantics: deleting a missing row is not an error.
	client := clientFromContext(ctx, r.client)
	_, err := client.UserSubscription.Delete().Where(usersubscription.IDEQ(id)).Exec(ctx)
	return err
}

func (r *userSubscriptionRepository) Restore(ctx context.Context, subscriptionID int64, restoredStatus string) (*service.UserSubscription, error) {
	client := clientFromContext(ctx, r.client)
	queryCtx := mixins.SkipSoftDelete(ctx)
	_, err := client.UserSubscription.UpdateOneID(subscriptionID).
		SetStatus(restoredStatus).
		ClearDeletedAt().
		SetUpdatedAt(time.Now()).
		Save(queryCtx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSubscriptionNotFound, service.ErrSubscriptionRestoreConflict)
	}
	return r.GetByID(ctx, subscriptionID)
}

func (r *userSubscriptionRepository) ListByUserID(ctx context.Context, userID int64) ([]service.UserSubscription, error) {
	client := clientFromContext(ctx, r.client)
	subs, err := client.UserSubscription.Query().
		Where(usersubscription.UserIDEQ(userID)).
		WithGroup().
		Order(dbent.Desc(usersubscription.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return userSubscriptionEntitiesToService(subs), nil
}

func (r *userSubscriptionRepository) ListActiveByUserID(ctx context.Context, userID int64) ([]service.UserSubscription, error) {
	client := clientFromContext(ctx, r.client)
	subs, err := client.UserSubscription.Query().
		Where(
			usersubscription.UserIDEQ(userID),
			usersubscription.StatusEQ(service.SubscriptionStatusActive),
			usersubscription.ExpiresAtGT(time.Now()),
		).
		WithGroup().
		Order(dbent.Desc(usersubscription.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return userSubscriptionEntitiesToService(subs), nil
}

func (r *userSubscriptionRepository) ListByGroupID(ctx context.Context, groupID int64, params pagination.PaginationParams) ([]service.UserSubscription, *pagination.PaginationResult, error) {
	client := clientFromContext(ctx, r.client)
	q := client.UserSubscription.Query().Where(usersubscription.GroupIDEQ(groupID))

	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	subs, err := q.
		WithUser().
		WithGroup().
		Order(dbent.Desc(usersubscription.FieldCreatedAt)).
		Offset(params.Offset()).
		Limit(params.Limit()).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}

	return userSubscriptionEntitiesToService(subs), paginationResultFromTotal(int64(total), params), nil
}

func (r *userSubscriptionRepository) List(ctx context.Context, params pagination.PaginationParams, userID, groupID *int64, status, platform, sortBy, sortOrder string) ([]service.UserSubscription, *pagination.PaginationResult, error) {
	client := clientFromContext(ctx, r.client)
	q := client.UserSubscription.Query()
	includeSoftDeleted := status == "" || status == service.SubscriptionStatusRevoked
	if userID != nil {
		q = q.Where(usersubscription.UserIDEQ(*userID))
	}
	if groupID != nil {
		q = q.Where(usersubscription.GroupIDEQ(*groupID))
	}
	if platform != "" {
		groupPredicates := []predicate.Group{group.PlatformEQ(platform)}
		if includeSoftDeleted {
			groupPredicates = append(groupPredicates, group.DeletedAtIsNil())
		}
		q = q.Where(usersubscription.HasGroupWith(groupPredicates...))
	}

	// Status filtering with real-time expiration check
	now := time.Now()
	switch status {
	case service.SubscriptionStatusActive:
		// Active: status is active AND not yet expired
		q = q.Where(
			usersubscription.StatusEQ(service.SubscriptionStatusActive),
			usersubscription.ExpiresAtGT(now),
		)
	case service.SubscriptionStatusExpired:
		// Expired: status is expired OR (status is active but already expired)
		q = q.Where(
			usersubscription.Or(
				usersubscription.StatusEQ(service.SubscriptionStatusExpired),
				usersubscription.And(
					usersubscription.StatusEQ(service.SubscriptionStatusActive),
					usersubscription.ExpiresAtLTE(now),
				),
			),
		)
	case service.SubscriptionStatusRevoked:
		// Revoked is a DTO/API display state backed by user_subscriptions.deleted_at.
		q = q.Where(usersubscription.DeletedAtNotNil())
	case "":
		// No filter. Use SkipSoftDelete below so admin "all status" includes revoked history.
	default:
		// Other persisted status.
		q = q.Where(usersubscription.StatusEQ(status))
	}

	queryCtx := ctx
	if includeSoftDeleted {
		queryCtx = mixins.SkipSoftDelete(ctx)
	}

	total, err := q.Clone().Count(queryCtx)
	if err != nil {
		return nil, nil, err
	}

	if !includeSoftDeleted {
		q = q.WithUser().WithGroup().WithAssignedByUser()
	}

	// Determine sort field
	var field string
	switch sortBy {
	case "expires_at":
		field = usersubscription.FieldExpiresAt
	case "status":
		field = usersubscription.FieldStatus
	default:
		field = usersubscription.FieldCreatedAt
	}

	// Determine sort order (default: desc)
	if sortOrder == "asc" && sortBy != "" {
		q = q.Order(dbent.Asc(field))
	} else {
		q = q.Order(dbent.Desc(field))
	}

	subs, err := q.
		Offset(params.Offset()).
		Limit(params.Limit()).
		All(queryCtx)
	if err != nil {
		return nil, nil, err
	}

	result := userSubscriptionEntitiesToService(subs)
	if includeSoftDeleted {
		if err := r.attachUserSubscriptionRelations(ctx, result); err != nil {
			return nil, nil, err
		}
	}

	return result, paginationResultFromTotal(int64(total), params), nil
}

func (r *userSubscriptionRepository) ListAdmin(
	ctx context.Context,
	params pagination.PaginationParams,
	filter service.SubscriptionAdminFilter,
	now time.Time,
) ([]service.UserSubscription, *pagination.PaginationResult, error) {
	client := clientFromContext(ctx, r.client)
	q, queryCtx, includeSoftDeleted := applyUserSubscriptionAdminFilter(
		client.UserSubscription.Query(),
		ctx,
		filter,
		now,
	)

	total, err := q.Clone().Count(queryCtx)
	if err != nil {
		return nil, nil, err
	}

	if !includeSoftDeleted {
		q = q.WithUser().WithGroup().WithAssignedByUser()
	}

	subs, err := q.
		Order(userSubscriptionAdminOrder(filter, timezone.StartOfDay(now))).
		Offset(params.Offset()).
		Limit(params.Limit()).
		All(queryCtx)
	if err != nil {
		return nil, nil, err
	}

	result := userSubscriptionEntitiesToService(subs)
	if includeSoftDeleted {
		if err := r.attachUserSubscriptionRelations(ctx, result); err != nil {
			return nil, nil, err
		}
	}

	return result, paginationResultFromTotal(int64(total), params), nil
}

func applyUserSubscriptionAdminFilter(
	q *dbent.UserSubscriptionQuery,
	ctx context.Context,
	filter service.SubscriptionAdminFilter,
	now time.Time,
) (*dbent.UserSubscriptionQuery, context.Context, bool) {
	includeSoftDeleted := filter.Status == "" || filter.Status == service.SubscriptionStatusRevoked
	if filter.UserID != nil {
		q = q.Where(usersubscription.UserIDEQ(*filter.UserID))
	}
	if filter.GroupID != nil {
		q = q.Where(usersubscription.GroupIDEQ(*filter.GroupID))
	}
	if filter.Platform != "" {
		groupPredicates := []predicate.Group{group.PlatformEQ(filter.Platform)}
		if includeSoftDeleted {
			groupPredicates = append(groupPredicates, group.DeletedAtIsNil())
		}
		q = q.Where(usersubscription.HasGroupWith(groupPredicates...))
	}
	if filter.Organization != "" {
		domain := subscriptionOrganizationDomain(filter.Organization)
		q = q.Where(usersubscription.HasUserWith(
			user.DeletedAtIsNil(),
			predicate.User(func(selector *entsql.Selector) {
				selector.Where(entsql.P(func(builder *entsql.Builder) {
					builder.WriteString("LOWER(SPLIT_PART(").
						WriteString(selector.C(user.FieldEmail)).
						WriteString(", '@', 2)) = ").
						Arg(domain)
				}))
			}),
		))
	}

	switch filter.Status {
	case service.SubscriptionStatusActive:
		q = q.Where(
			usersubscription.StatusEQ(service.SubscriptionStatusActive),
			usersubscription.ExpiresAtGT(now),
		)
	case service.SubscriptionStatusExpired:
		q = q.Where(usersubscription.Or(
			usersubscription.StatusEQ(service.SubscriptionStatusExpired),
			usersubscription.And(
				usersubscription.StatusEQ(service.SubscriptionStatusActive),
				usersubscription.ExpiresAtLTE(now),
			),
		))
	case service.SubscriptionStatusRevoked:
		q = q.Where(usersubscription.DeletedAtNotNil())
	case "":
	default:
		q = q.Where(usersubscription.StatusEQ(filter.Status))
	}

	queryCtx := ctx
	if includeSoftDeleted {
		queryCtx = mixins.SkipSoftDelete(ctx)
	}
	return q, queryCtx, includeSoftDeleted
}

func subscriptionOrganizationDomain(organization string) string {
	switch organization {
	case service.SubscriptionOrganizationXunyou:
		return "xunyou.com"
	case service.SubscriptionOrganizationWsdashi:
		return "wsdashi.com"
	default:
		return ""
	}
}

func userSubscriptionAdminOrder(filter service.SubscriptionAdminFilter, startOfDay time.Time) func(*entsql.Selector) {
	return func(selector *entsql.Selector) {
		groupTable := entsql.Table(group.Table).As("admin_sort_group")
		selector.LeftJoin(groupTable).
			On(selector.C(usersubscription.FieldGroupID), groupTable.C(group.FieldID)).
			OnP(entsql.IsNull(groupTable.C(group.FieldDeletedAt)))

		startsAt := selector.C(usersubscription.FieldStartsAt)
		expiresAt := selector.C(usersubscription.FieldExpiresAt)
		dailyWindowStart := selector.C(usersubscription.FieldDailyWindowStart)
		dailyUsage := selector.C(usersubscription.FieldDailyUsageUsd)
		dailyLimit := groupTable.C(group.FieldDailyLimitUsd)

		selector.OrderExpr(entsql.ExprFunc(func(builder *entsql.Builder) {
			builder.WriteString("CASE WHEN ").WriteString(dailyLimit).
				WriteString(" IS NULL OR ").WriteString(dailyLimit).
				WriteString(" <= 0 THEN 1 ELSE 0 END ASC, CASE WHEN ").
				WriteString(dailyLimit).WriteString(" > 0 THEN GREATEST(").
				WriteString(dailyLimit).WriteString(" - CASE WHEN ").
				WriteString(expiresAt).WriteString(" <= ").WriteString(startsAt).
				WriteString(" + INTERVAL '1 day' THEN ").WriteString(dailyUsage).
				WriteString(" WHEN ").WriteString(dailyWindowStart).WriteString(" IS NOT NULL AND ").
				WriteString(dailyWindowStart).WriteString(" < ").Arg(startOfDay).
				WriteString(" THEN 0 ELSE ").WriteString(dailyUsage).WriteString(" END").
				WriteString(", 0) / ").WriteString(dailyLimit).
				WriteString(" END ASC")
		}))

		secondaryField := usersubscription.FieldCreatedAt
		switch filter.SortBy {
		case "expires_at":
			secondaryField = usersubscription.FieldExpiresAt
		case "status":
			secondaryField = usersubscription.FieldStatus
		}
		if filter.SortOrder == "asc" {
			selector.OrderBy(entsql.Asc(selector.C(secondaryField)))
		} else {
			selector.OrderBy(entsql.Desc(selector.C(secondaryField)))
		}
		selector.OrderBy(entsql.Asc(selector.C(usersubscription.FieldID)))
	}
}

func (r *userSubscriptionRepository) ExistsByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (bool, error) {
	client := clientFromContext(ctx, r.client)
	return client.UserSubscription.Query().
		Where(usersubscription.UserIDEQ(userID), usersubscription.GroupIDEQ(groupID)).
		Exist(ctx)
}

func (r *userSubscriptionRepository) ExistsActiveByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (bool, error) {
	return r.ExistsByUserIDAndGroupID(ctx, userID, groupID)
}

func (r *userSubscriptionRepository) ExtendExpiry(ctx context.Context, subscriptionID int64, newExpiresAt time.Time) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.UserSubscription.UpdateOneID(subscriptionID).
		SetExpiresAt(newExpiresAt).
		Save(ctx)
	return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
}

func (r *userSubscriptionRepository) UpdateStatus(ctx context.Context, subscriptionID int64, status string) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.UserSubscription.UpdateOneID(subscriptionID).
		SetStatus(status).
		Save(ctx)
	return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
}

func (r *userSubscriptionRepository) UpdateNotes(ctx context.Context, subscriptionID int64, notes string) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.UserSubscription.UpdateOneID(subscriptionID).
		SetNotes(notes).
		Save(ctx)
	return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
}

func (r *userSubscriptionRepository) ActivateWindows(ctx context.Context, id int64, dailyStart, periodicStart time.Time) error {
	client := clientFromContext(ctx, r.client)
	n, err := client.UserSubscription.Update().
		Where(
			usersubscription.IDEQ(id),
			usersubscription.DailyWindowStartIsNil(),
			usersubscription.WeeklyWindowStartIsNil(),
			usersubscription.MonthlyWindowStartIsNil(),
		).
		SetDailyWindowStart(dailyStart).
		SetWeeklyWindowStart(periodicStart).
		SetMonthlyWindowStart(periodicStart).
		Save(ctx)
	return r.translateConditionalWindowReset(ctx, client, id, n, err)
}

func (r *userSubscriptionRepository) ResetUsageWindows(ctx context.Context, id int64, resetDaily, resetWeekly, resetMonthly bool, dailyStart, periodicStart time.Time) error {
	client := clientFromContext(ctx, r.client)
	update := client.UserSubscription.UpdateOneID(id)
	if resetDaily {
		update.SetDailyUsageUsd(0).SetDailyWindowStart(dailyStart)
	}
	if resetWeekly {
		update.SetWeeklyUsageUsd(0).SetWeeklyWindowStart(periodicStart)
	}
	if resetMonthly {
		update.SetMonthlyUsageUsd(0).SetMonthlyWindowStart(periodicStart)
	}
	_, err := update.Save(ctx)
	return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
}

func (r *userSubscriptionRepository) ResetDailyUsage(ctx context.Context, id int64, expectedWindowStart *time.Time, newWindowStart time.Time) error {
	client := clientFromContext(ctx, r.client)
	query := client.UserSubscription.Update().Where(usersubscription.IDEQ(id))
	if expectedWindowStart == nil {
		query = query.Where(usersubscription.DailyWindowStartIsNil())
	} else {
		query = query.Where(usersubscription.DailyWindowStartEQ(*expectedWindowStart))
	}
	n, err := query.
		SetDailyUsageUsd(0).
		SetDailyWindowStart(newWindowStart).
		Save(ctx)
	return r.translateConditionalWindowReset(ctx, client, id, n, err)
}

func (r *userSubscriptionRepository) ResetWeeklyUsage(ctx context.Context, id int64, expectedWindowStart *time.Time, newWindowStart time.Time) error {
	client := clientFromContext(ctx, r.client)
	query := client.UserSubscription.Update().Where(usersubscription.IDEQ(id))
	if expectedWindowStart == nil {
		query = query.Where(usersubscription.WeeklyWindowStartIsNil())
	} else {
		query = query.Where(usersubscription.WeeklyWindowStartEQ(*expectedWindowStart))
	}
	n, err := query.
		SetWeeklyUsageUsd(0).
		SetWeeklyWindowStart(newWindowStart).
		Save(ctx)
	return r.translateConditionalWindowReset(ctx, client, id, n, err)
}

func (r *userSubscriptionRepository) ResetMonthlyUsage(ctx context.Context, id int64, expectedWindowStart *time.Time, newWindowStart time.Time) error {
	client := clientFromContext(ctx, r.client)
	query := client.UserSubscription.Update().Where(usersubscription.IDEQ(id))
	if expectedWindowStart == nil {
		query = query.Where(usersubscription.MonthlyWindowStartIsNil())
	} else {
		query = query.Where(usersubscription.MonthlyWindowStartEQ(*expectedWindowStart))
	}
	n, err := query.
		SetMonthlyUsageUsd(0).
		SetMonthlyWindowStart(newWindowStart).
		Save(ctx)
	return r.translateConditionalWindowReset(ctx, client, id, n, err)
}

func (r *userSubscriptionRepository) ResetDailyFiltered(
	ctx context.Context,
	filter service.SubscriptionAdminFilter,
	now time.Time,
	newWindowStart time.Time,
) ([]service.SubscriptionCacheKey, error) {
	var predicates []string
	args := make([]any, 0, 8)
	addPredicate := func(format string, value any) {
		args = append(args, value)
		predicates = append(predicates, fmt.Sprintf(format, len(args)))
	}

	predicates = append(predicates,
		"us.deleted_at IS NULL",
		"u.deleted_at IS NULL",
		"g.deleted_at IS NULL",
		"us.status = 'active'",
	)
	addPredicate("us.expires_at > $%d", now)
	if filter.UserID != nil {
		addPredicate("us.user_id = $%d", *filter.UserID)
	}
	if filter.GroupID != nil {
		addPredicate("us.group_id = $%d", *filter.GroupID)
	}
	if filter.Platform != "" {
		addPredicate("g.platform = $%d", filter.Platform)
	}
	if filter.Organization != "" {
		addPredicate("LOWER(SPLIT_PART(u.email, '@', 2)) = $%d", subscriptionOrganizationDomain(filter.Organization))
	}
	if filter.Status != "" {
		addPredicate("us.status = $%d", filter.Status)
	}

	args = append(args, newWindowStart)
	windowArg := len(args)
	args = append(args, now)
	updatedAtArg := len(args)
	query := fmt.Sprintf(`
		WITH candidates AS (
			SELECT us.id
			FROM user_subscriptions us
			JOIN users u ON u.id = us.user_id
			JOIN groups g ON g.id = us.group_id
			WHERE %s
		)
		UPDATE user_subscriptions us
		SET daily_usage_usd = 0,
			daily_window_start = $%d,
			updated_at = $%d
		FROM candidates c
		WHERE us.id = c.id
		RETURNING us.user_id, us.group_id
	`, strings.Join(predicates, " AND "), windowArg, updatedAtArg)

	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	keys := make([]service.SubscriptionCacheKey, 0)
	seen := make(map[service.SubscriptionCacheKey]struct{})
	for rows.Next() {
		var key service.SubscriptionCacheKey
		if err := rows.Scan(&key.UserID, &key.GroupID); err != nil {
			return nil, err
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return keys, nil
}

func (r *userSubscriptionRepository) translateConditionalWindowReset(ctx context.Context, client *dbent.Client, id int64, affected int, err error) error {
	if err != nil {
		return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
	}
	if affected > 0 {
		return nil
	}

	// A stale reset is an expected no-op: another request already advanced the
	// window. Preserve not-found semantics for callers that target a missing row.
	exists, err := client.UserSubscription.Query().Where(usersubscription.IDEQ(id)).Exist(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
	}
	if !exists {
		return service.ErrSubscriptionNotFound
	}
	return nil
}

// IncrementUsage 原子性地累加订阅用量。
// 限额检查已在请求前由 BillingCacheService.CheckBillingEligibility 完成，
// 此处仅负责记录实际消费，确保消费数据的完整性。
func (r *userSubscriptionRepository) IncrementUsage(ctx context.Context, id int64, costUSD float64) error {
	const updateSQL = `
		UPDATE user_subscriptions us
		SET
			daily_usage_usd = us.daily_usage_usd + $1,
			weekly_usage_usd = us.weekly_usage_usd + $1,
			monthly_usage_usd = us.monthly_usage_usd + $1,
			updated_at = NOW()
		FROM groups g
		WHERE us.id = $2
			AND us.deleted_at IS NULL
			AND us.group_id = g.id
			AND g.deleted_at IS NULL
	`

	client := clientFromContext(ctx, r.client)
	result, err := client.ExecContext(ctx, updateSQL, costUSD, id)
	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if affected > 0 {
		return nil
	}

	// affected == 0：订阅不存在或已删除
	return service.ErrSubscriptionNotFound
}

func (r *userSubscriptionRepository) BatchUpdateExpiredStatus(ctx context.Context) (int64, error) {
	client := clientFromContext(ctx, r.client)
	n, err := client.UserSubscription.Update().
		Where(
			usersubscription.StatusEQ(service.SubscriptionStatusActive),
			usersubscription.ExpiresAtLTE(time.Now()),
		).
		SetStatus(service.SubscriptionStatusExpired).
		Save(ctx)
	return int64(n), err
}

// Extra repository helpers (currently used only by integration tests).

func (r *userSubscriptionRepository) ListExpired(ctx context.Context) ([]service.UserSubscription, error) {
	client := clientFromContext(ctx, r.client)
	subs, err := client.UserSubscription.Query().
		Where(
			usersubscription.StatusEQ(service.SubscriptionStatusActive),
			usersubscription.ExpiresAtLTE(time.Now()),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return userSubscriptionEntitiesToService(subs), nil
}

func (r *userSubscriptionRepository) CountByGroupID(ctx context.Context, groupID int64) (int64, error) {
	client := clientFromContext(ctx, r.client)
	count, err := client.UserSubscription.Query().Where(usersubscription.GroupIDEQ(groupID)).Count(ctx)
	return int64(count), err
}

func (r *userSubscriptionRepository) CountActiveByGroupID(ctx context.Context, groupID int64) (int64, error) {
	client := clientFromContext(ctx, r.client)
	count, err := client.UserSubscription.Query().
		Where(
			usersubscription.GroupIDEQ(groupID),
			usersubscription.StatusEQ(service.SubscriptionStatusActive),
			usersubscription.ExpiresAtGT(time.Now()),
		).
		Count(ctx)
	return int64(count), err
}

func (r *userSubscriptionRepository) DeleteByGroupID(ctx context.Context, groupID int64) (int64, error) {
	client := clientFromContext(ctx, r.client)
	n, err := client.UserSubscription.Delete().Where(usersubscription.GroupIDEQ(groupID)).Exec(ctx)
	return int64(n), err
}

func (r *userSubscriptionRepository) attachUserSubscriptionRelations(ctx context.Context, subs []service.UserSubscription) error {
	if len(subs) == 0 {
		return nil
	}

	userIDs := make([]int64, 0, len(subs))
	groupIDs := make([]int64, 0, len(subs))
	assignedByIDs := make([]int64, 0, len(subs))
	for i := range subs {
		userIDs = append(userIDs, subs[i].UserID)
		groupIDs = append(groupIDs, subs[i].GroupID)
		if subs[i].AssignedBy != nil {
			assignedByIDs = append(assignedByIDs, *subs[i].AssignedBy)
		}
	}

	client := clientFromContext(ctx, r.client)
	users, err := client.User.Query().Where(user.IDIn(uniqueInt64s(userIDs)...)).All(ctx)
	if err != nil {
		return err
	}
	userByID := make(map[int64]*service.User, len(users))
	for _, u := range users {
		userByID[u.ID] = userEntityToService(u)
	}

	groups, err := client.Group.Query().Where(group.IDIn(uniqueInt64s(groupIDs)...)).All(ctx)
	if err != nil {
		return err
	}
	groupByID := make(map[int64]*service.Group, len(groups))
	for _, g := range groups {
		groupByID[g.ID] = groupEntityToService(g)
	}

	assignedByID := map[int64]*service.User{}
	if len(assignedByIDs) > 0 {
		assignedUsers, err := client.User.Query().Where(user.IDIn(uniqueInt64s(assignedByIDs)...)).All(ctx)
		if err != nil {
			return err
		}
		assignedByID = make(map[int64]*service.User, len(assignedUsers))
		for _, u := range assignedUsers {
			assignedByID[u.ID] = userEntityToService(u)
		}
	}

	for i := range subs {
		subs[i].User = userByID[subs[i].UserID]
		subs[i].Group = groupByID[subs[i].GroupID]
		if subs[i].AssignedBy != nil {
			subs[i].AssignedByUser = assignedByID[*subs[i].AssignedBy]
		}
	}
	return nil
}

func uniqueInt64s(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	out := make([]int64, 0, len(values))
	for _, v := range values {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func userSubscriptionEntityToService(m *dbent.UserSubscription) *service.UserSubscription {
	return userSubscriptionEntityToServiceWithStatusMapping(m, true)
}

func userSubscriptionEntityToServicePreserveStatus(m *dbent.UserSubscription) *service.UserSubscription {
	return userSubscriptionEntityToServiceWithStatusMapping(m, false)
}

func userSubscriptionEntityToServiceWithStatusMapping(m *dbent.UserSubscription, mapDeletedToRevoked bool) *service.UserSubscription {
	if m == nil {
		return nil
	}
	status := m.Status
	if mapDeletedToRevoked && m.DeletedAt != nil {
		status = service.SubscriptionStatusRevoked
	}
	out := &service.UserSubscription{
		ID:                 m.ID,
		UserID:             m.UserID,
		GroupID:            m.GroupID,
		StartsAt:           m.StartsAt,
		ExpiresAt:          m.ExpiresAt,
		Status:             status,
		DailyWindowStart:   m.DailyWindowStart,
		WeeklyWindowStart:  m.WeeklyWindowStart,
		MonthlyWindowStart: m.MonthlyWindowStart,
		DailyUsageUSD:      m.DailyUsageUsd,
		WeeklyUsageUSD:     m.WeeklyUsageUsd,
		MonthlyUsageUSD:    m.MonthlyUsageUsd,
		AssignedBy:         m.AssignedBy,
		AssignedAt:         m.AssignedAt,
		Notes:              derefString(m.Notes),
		CreatedAt:          m.CreatedAt,
		UpdatedAt:          m.UpdatedAt,
		DeletedAt:          m.DeletedAt,
	}
	if m.Edges.User != nil {
		out.User = userEntityToService(m.Edges.User)
	}
	if m.Edges.Group != nil {
		out.Group = groupEntityToService(m.Edges.Group)
	}
	if m.Edges.AssignedByUser != nil {
		out.AssignedByUser = userEntityToService(m.Edges.AssignedByUser)
	}
	return out
}

func userSubscriptionEntitiesToService(models []*dbent.UserSubscription) []service.UserSubscription {
	out := make([]service.UserSubscription, 0, len(models))
	for i := range models {
		if s := userSubscriptionEntityToService(models[i]); s != nil {
			out = append(out, *s)
		}
	}
	return out
}

func applyUserSubscriptionEntityToService(dst *service.UserSubscription, src *dbent.UserSubscription) {
	if dst == nil || src == nil {
		return
	}
	dst.ID = src.ID
	dst.CreatedAt = src.CreatedAt
	dst.UpdatedAt = src.UpdatedAt
}
