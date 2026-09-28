package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestDepartmentLookupsPreserveDatabaseErrors(t *testing.T) {
	failure := errors.New("database read interrupted")
	for _, access := range []bool{false, true} {
		for _, atRow := range []bool{false, true} {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			query := mock.ExpectQuery("SELECT .* FROM users")
			if atRow {
				query.WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("sub_admin").RowError(0, failure))
			} else {
				query.WillReturnError(failure)
			}
			if access {
				_, err = loadAdminAccess(context.Background(), db, 1)
			} else {
				_, err = loadDepartmentActor(service.WithDepartmentActor(context.Background(), 1), db)
			}
			require.ErrorIs(t, err, failure, "access=%v row=%v", access, atRow)
			require.NoError(t, mock.ExpectationsWereMet())
		}
	}
}

func TestDepartmentResetDistinguishesMissingSubscriptionFromDatabaseFailure(t *testing.T) {
	failure := errors.New("subscription query interrupted")
	for _, missing := range []bool{false, true} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		t.Cleanup(func() { _ = db.Close() })
		client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
		repo := &userSubscriptionRepository{client: client}
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT id,email,role,admin_permissions,status FROM users").WillReturnRows(
			sqlmock.NewRows([]string{"id", "email", "role", "admin_permissions", "status"}).AddRow(1, "admin@example.com", "admin", "[]", "active"),
		)
		query := mock.ExpectQuery(`SELECT .* FROM "user_subscriptions"`)
		want := failure
		if missing {
			query.WillReturnRows(sqlmock.NewRows([]string{"id"}))
			want = service.ErrDepartmentScopeDenied
		} else {
			query.WillReturnError(failure)
		}
		mock.ExpectRollback()
		_, _, owned, err := repo.beginDepartmentSubscriptionWrite(service.WithDepartmentActor(context.Background(), 1), service.SubscriptionAdminFilter{}, 7, time.Now())
		require.ErrorIs(t, err, want)
		require.Nil(t, owned)
		require.NoError(t, mock.ExpectationsWereMet())
	}
}
