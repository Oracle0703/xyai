//go:build integration

package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type failAtomicMarkSucceededRepo struct {
	*idempotencyRepository
	sawTransaction bool
}

func (r *failAtomicMarkSucceededRepo) MarkSucceeded(ctx context.Context, _ int64, _ int, _ string, _ time.Time) error {
	r.sawTransaction = dbent.TxFromContext(ctx) != nil
	return errors.New("mark succeeded injected failure")
}

func TestIdempotencyRepo_WithTransactionRollsBackCallbackWrites(t *testing.T) {
	client := testEntClient(t)
	repo := &idempotencyRepository{client: client, sql: integrationDB}
	scope := uniqueTestValue(t, "idem-atomic-rollback")

	err := repo.WithTransaction(context.Background(), func(ctx context.Context) error {
		tx := dbent.TxFromContext(ctx)
		require.NotNil(t, tx)
		_, execErr := tx.Client().ExecContext(ctx, `
			INSERT INTO idempotency_records (
				scope, idempotency_key_hash, request_fingerprint, status, expires_at
			) VALUES ($1, $2, $3, $4, $5)
		`, scope, hashedTestValue(t, "idem-atomic-hash"), hashedTestValue(t, "idem-atomic-fp"), service.IdempotencyStatusProcessing, time.Now().Add(time.Hour))
		require.NoError(t, execErr)
		return errors.New("rollback injected")
	})
	require.ErrorContains(t, err, "rollback injected")

	var count int
	require.NoError(t, integrationDB.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM idempotency_records WHERE scope = $1`, scope).Scan(&count))
	require.Zero(t, count)
}

func TestIdempotencyCoordinator_AtomicSuccessRollsBackFilteredDailyResetWhenMarkSucceededFails(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	suffix := uniqueTestValue(t, "idem-subscription-atomic")
	userRow, err := client.User.Create().
		SetEmail(suffix + "@xunyou.com").
		SetPasswordHash("test-password-hash").
		SetStatus(service.StatusActive).
		SetRole(service.RoleUser).
		Save(ctx)
	require.NoError(t, err)
	groupRow, err := client.Group.Create().
		SetName(suffix).
		SetStatus(service.StatusActive).
		SetPlatform(service.PlatformOpenAI).
		SetDailyLimitUsd(10).
		Save(ctx)
	require.NoError(t, err)
	now := time.Now().UTC()
	subRow, err := client.UserSubscription.Create().
		SetUserID(userRow.ID).
		SetGroupID(groupRow.ID).
		SetStartsAt(now.Add(-time.Hour)).
		SetExpiresAt(now.Add(time.Hour)).
		SetStatus(service.SubscriptionStatusActive).
		SetDailyUsageUsd(5).
		SetDailyWindowStart(now.Add(-time.Hour)).
		SetAssignedAt(now).
		SetNotes("").
		Save(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM idempotency_records WHERE scope = $1`, suffix)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM user_subscriptions WHERE id = $1`, subRow.ID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM users WHERE id = $1`, userRow.ID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM groups WHERE id = $1`, groupRow.ID)
	})

	idempotencyRepo := &failAtomicMarkSucceededRepo{idempotencyRepository: &idempotencyRepository{client: client, sql: integrationDB}}
	coordinator := service.NewIdempotencyCoordinator(idempotencyRepo, service.DefaultIdempotencyConfig())
	subscriptionRepo := NewUserSubscriptionRepository(client).(*userSubscriptionRepository)

	_, err = coordinator.Execute(ctx, service.IdempotencyExecuteOptions{
		Scope:          suffix,
		IdempotencyKey: suffix,
		Method:         "POST",
		Route:          "/api/v1/admin/subscriptions/reset-daily-filtered",
		ActorScope:     "admin:1",
		Payload:        service.SubscriptionAdminFilter{Organization: service.SubscriptionOrganizationXunyou},
		RequireKey:     true,
		AtomicSuccess:  true,
	}, func(txCtx context.Context) (any, error) {
		keys, resetErr := subscriptionRepo.ResetDailyFiltered(
			txCtx,
			service.SubscriptionAdminFilter{Organization: service.SubscriptionOrganizationXunyou},
			now,
			now.Truncate(24*time.Hour),
		)
		return map[string]any{"reset_count": len(keys)}, resetErr
	})

	require.Error(t, err)
	require.Equal(t, infraerrors.Code(service.ErrIdempotencyStoreUnavail), infraerrors.Code(err))
	require.True(t, idempotencyRepo.sawTransaction)
	reloaded, err := client.UserSubscription.Get(ctx, subRow.ID)
	require.NoError(t, err)
	require.InDelta(t, 5, reloaded.DailyUsageUsd, 1e-9)
}

// hashedTestValue returns a unique SHA-256 hex string (64 chars) that fits VARCHAR(64) columns.
func hashedTestValue(t *testing.T, prefix string) string {
	t.Helper()
	sum := sha256.Sum256([]byte(uniqueTestValue(t, prefix)))
	return hex.EncodeToString(sum[:])
}

func TestIdempotencyRepo_CreateProcessing_CompeteSameKey(t *testing.T) {
	tx := testTx(t)
	repo := &idempotencyRepository{sql: tx}
	ctx := context.Background()

	now := time.Now().UTC()
	record := &service.IdempotencyRecord{
		Scope:              uniqueTestValue(t, "idem-scope-create"),
		IdempotencyKeyHash: hashedTestValue(t, "idem-hash"),
		RequestFingerprint: hashedTestValue(t, "idem-fp"),
		Status:             service.IdempotencyStatusProcessing,
		LockedUntil:        ptrTime(now.Add(30 * time.Second)),
		ExpiresAt:          now.Add(24 * time.Hour),
	}
	owner, err := repo.CreateProcessing(ctx, record)
	require.NoError(t, err)
	require.True(t, owner)
	require.NotZero(t, record.ID)

	duplicate := &service.IdempotencyRecord{
		Scope:              record.Scope,
		IdempotencyKeyHash: record.IdempotencyKeyHash,
		RequestFingerprint: hashedTestValue(t, "idem-fp-other"),
		Status:             service.IdempotencyStatusProcessing,
		LockedUntil:        ptrTime(now.Add(30 * time.Second)),
		ExpiresAt:          now.Add(24 * time.Hour),
	}
	owner, err = repo.CreateProcessing(ctx, duplicate)
	require.NoError(t, err)
	require.False(t, owner, "same scope+key hash should be de-duplicated")
}

func TestIdempotencyRepo_TryReclaim_StatusAndLockWindow(t *testing.T) {
	tx := testTx(t)
	repo := &idempotencyRepository{sql: tx}
	ctx := context.Background()

	now := time.Now().UTC()
	record := &service.IdempotencyRecord{
		Scope:              uniqueTestValue(t, "idem-scope-reclaim"),
		IdempotencyKeyHash: hashedTestValue(t, "idem-hash-reclaim"),
		RequestFingerprint: hashedTestValue(t, "idem-fp-reclaim"),
		Status:             service.IdempotencyStatusProcessing,
		LockedUntil:        ptrTime(now.Add(10 * time.Second)),
		ExpiresAt:          now.Add(24 * time.Hour),
	}
	owner, err := repo.CreateProcessing(ctx, record)
	require.NoError(t, err)
	require.True(t, owner)

	require.NoError(t, repo.MarkFailedRetryable(
		ctx,
		record.ID,
		"RETRYABLE_FAILURE",
		now.Add(-2*time.Second),
		now.Add(24*time.Hour),
	))

	newLockedUntil := now.Add(20 * time.Second)
	reclaimed, err := repo.TryReclaim(
		ctx,
		record.ID,
		service.IdempotencyStatusFailedRetryable,
		now,
		newLockedUntil,
		now.Add(24*time.Hour),
	)
	require.NoError(t, err)
	require.True(t, reclaimed, "failed_retryable + expired lock should allow reclaim")

	got, err := repo.GetByScopeAndKeyHash(ctx, record.Scope, record.IdempotencyKeyHash)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, service.IdempotencyStatusProcessing, got.Status)
	require.NotNil(t, got.LockedUntil)
	require.True(t, got.LockedUntil.After(now))

	require.NoError(t, repo.MarkFailedRetryable(
		ctx,
		record.ID,
		"RETRYABLE_FAILURE",
		now.Add(20*time.Second),
		now.Add(24*time.Hour),
	))

	reclaimed, err = repo.TryReclaim(
		ctx,
		record.ID,
		service.IdempotencyStatusFailedRetryable,
		now,
		now.Add(40*time.Second),
		now.Add(24*time.Hour),
	)
	require.NoError(t, err)
	require.False(t, reclaimed, "within lock window should not reclaim")
}

func TestIdempotencyRepo_StatusTransition_ToSucceeded(t *testing.T) {
	tx := testTx(t)
	repo := &idempotencyRepository{sql: tx}
	ctx := context.Background()

	now := time.Now().UTC()
	record := &service.IdempotencyRecord{
		Scope:              uniqueTestValue(t, "idem-scope-success"),
		IdempotencyKeyHash: hashedTestValue(t, "idem-hash-success"),
		RequestFingerprint: hashedTestValue(t, "idem-fp-success"),
		Status:             service.IdempotencyStatusProcessing,
		LockedUntil:        ptrTime(now.Add(10 * time.Second)),
		ExpiresAt:          now.Add(24 * time.Hour),
	}
	owner, err := repo.CreateProcessing(ctx, record)
	require.NoError(t, err)
	require.True(t, owner)

	require.NoError(t, repo.MarkSucceeded(ctx, record.ID, 200, `{"ok":true}`, now.Add(24*time.Hour)))

	got, err := repo.GetByScopeAndKeyHash(ctx, record.Scope, record.IdempotencyKeyHash)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, service.IdempotencyStatusSucceeded, got.Status)
	require.NotNil(t, got.ResponseStatus)
	require.Equal(t, 200, *got.ResponseStatus)
	require.NotNil(t, got.ResponseBody)
	require.Equal(t, `{"ok":true}`, *got.ResponseBody)
	require.Nil(t, got.LockedUntil)
}
