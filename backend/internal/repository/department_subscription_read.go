package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// Scope, count, rows and nested users are read from the same snapshot. This
// prevents a transfer between the scope query and eager-loading a user's data.
func departmentSubscriptionRead[T any](ctx context.Context, client *dbent.Client, read func(context.Context) (T, error)) (T, error) {
	if service.DepartmentActorID(ctx) == 0 || dbent.TxFromContext(ctx) != nil {
		return read(ctx)
	}
	tx, err := client.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if errors.Is(err, dbent.ErrTxStarted) {
		return read(ctx)
	}
	if err != nil {
		var zero T
		return zero, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := read(dbent.NewTxContext(ctx, tx))
	if err != nil {
		return result, err
	}
	return result, tx.Commit()
}

type departmentSubscriptionList struct {
	items []service.UserSubscription
	page  *pagination.PaginationResult
}

func (r *userSubscriptionRepository) GetByID(ctx context.Context, id int64) (*service.UserSubscription, error) {
	return departmentSubscriptionRead(ctx, r.client, func(ctx context.Context) (*service.UserSubscription, error) { return r.getByID(ctx, id) })
}

func (r *userSubscriptionRepository) ListByUserID(ctx context.Context, userID int64) ([]service.UserSubscription, error) {
	return departmentSubscriptionRead(ctx, r.client, func(ctx context.Context) ([]service.UserSubscription, error) { return r.listByUserID(ctx, userID) })
}

func (r *userSubscriptionRepository) ListByGroupID(ctx context.Context, groupID int64, p pagination.PaginationParams) ([]service.UserSubscription, *pagination.PaginationResult, error) {
	result, err := departmentSubscriptionRead(ctx, r.client, func(ctx context.Context) (departmentSubscriptionList, error) {
		items, page, err := r.listByGroupID(ctx, groupID, p)
		return departmentSubscriptionList{items, page}, err
	})
	return result.items, result.page, err
}

func (r *userSubscriptionRepository) ListAdmin(ctx context.Context, p pagination.PaginationParams, filter service.SubscriptionAdminFilter, now time.Time) ([]service.UserSubscription, *pagination.PaginationResult, error) {
	result, err := departmentSubscriptionRead(ctx, r.client, func(ctx context.Context) (departmentSubscriptionList, error) {
		items, page, err := r.listAdmin(ctx, p, filter, now)
		return departmentSubscriptionList{items, page}, err
	})
	return result.items, result.page, err
}
