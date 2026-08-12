package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

func TestListAdminOrganizationFilterExplicitlyExcludesSoftDeletedUsers(t *testing.T) {
	var capturedSQL string
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(captureEntQueryMatcher{actual: &capturedSQL}))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	driver := entsql.OpenDB(dialect.Postgres, db)
	client := dbent.NewClient(dbent.Driver(driver))
	t.Cleanup(func() { _ = client.Close() })
	repo := &userSubscriptionRepository{client: client}

	wantErr := errors.New("stop after capturing count query")
	mock.ExpectQuery("admin subscription organization count").WillReturnError(wantErr)

	_, _, err = repo.ListAdmin(
		context.Background(),
		pagination.PaginationParams{Page: 1, PageSize: 20},
		service.SubscriptionAdminFilter{Organization: service.SubscriptionOrganizationXunyou},
		time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC),
	)

	require.ErrorIs(t, err, wantErr)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Regexp(t, `(?i)FROM "users".*"deleted_at" IS NULL`, normalizeSQLWhitespace(capturedSQL))
}

func TestUserSubscriptionAdminOrder_BindsStartOfDayAfterFilterArguments(t *testing.T) {
	startOfDay := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	table := entsql.Table("user_subscriptions")
	selector := entsql.Dialect(dialect.Postgres).
		Select(table.C("id")).
		From(table).
		Where(entsql.EQ(table.C("status"), service.SubscriptionStatusActive))

	userSubscriptionAdminOrder(
		service.SubscriptionAdminFilter{SortBy: "created_at", SortOrder: "desc"},
		startOfDay,
	)(selector)

	query, args := selector.Query()
	require.Contains(t, normalizeSQLWhitespace(query), `"daily_window_start" < $2`)
	require.Equal(t, []any{service.SubscriptionStatusActive, startOfDay}, args)
}

func TestListAdminOrganizationFilterUsesPostgresPlaceholders(t *testing.T) {
	var capturedSQL string
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(captureEntQueryMatcher{actual: &capturedSQL}))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	driver := entsql.OpenDB(dialect.Postgres, db)
	client := dbent.NewClient(dbent.Driver(driver))
	t.Cleanup(func() { _ = client.Close() })
	repo := &userSubscriptionRepository{client: client}

	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	wantErr := errors.New("stop after capturing list query")
	mock.ExpectQuery("admin subscription organization count").
		WithArgs("xunyou.com", service.SubscriptionStatusActive, now).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("admin subscription organization list").
		WithArgs("xunyou.com", service.SubscriptionStatusActive, now, sqlmock.AnyArg()).
		WillReturnError(wantErr)

	_, _, err = repo.ListAdmin(
		context.Background(),
		pagination.PaginationParams{Page: 1, PageSize: 20},
		service.SubscriptionAdminFilter{
			Status:       service.SubscriptionStatusActive,
			Organization: service.SubscriptionOrganizationXunyou,
			SortBy:       "created_at",
			SortOrder:    "desc",
		},
		now,
	)

	require.ErrorIs(t, err, wantErr)
	require.NoError(t, mock.ExpectationsWereMet())
	normalized := normalizeSQLWhitespace(capturedSQL)
	require.Contains(t, normalized, `LOWER(SPLIT_PART("users"."email", '@', 2)) = $1`)
	require.Contains(t, normalized, `"status" = $2`)
	require.Contains(t, normalized, `"expires_at" > $3`)
	require.Contains(t, normalized, `"daily_window_start" < $4`)
}
