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
