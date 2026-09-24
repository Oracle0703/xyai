//go:build integration

package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	errors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type selfResetCommitErrorRepo struct{ *idempotencyRepository }

func (r *selfResetCommitErrorRepo) WithTransaction(ctx context.Context, fn func(context.Context) error) error {
	if err := r.idempotencyRepository.WithTransaction(ctx, fn); err != nil {
		return err
	}
	return fmt.Errorf("injected lost commit acknowledgement")
}

func TestSubscriptionSelfResetIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	require.NoError(t, timezone.Init("Asia/Shanghai"))
	t.Cleanup(func() { require.NoError(t, timezone.Init("UTC")) })
	client := integrationEntClient
	prefix := uuid.NewString()
	owner, err := client.User.Create().SetEmail(prefix + "@xunyou.com").SetPasswordHash("unused").SetStatus(service.StatusActive).Save(ctx)
	require.NoError(t, err)
	group, err := client.Group.Create().SetName(prefix).SetStatus(service.StatusActive).SetSubscriptionType(service.SubscriptionTypeSubscription).SetDailyLimitUsd(100).Save(ctx)
	require.NoError(t, err)
	now := time.Now()
	today := timezone.StartOfDay(now).Format(time.DateOnly)
	createSub := func(userID, groupID int64, starts, expires time.Time) *dbent.UserSubscription {
		sub, e := client.UserSubscription.Create().SetUserID(userID).SetGroupID(groupID).SetStatus(service.SubscriptionStatusActive).SetStartsAt(starts).SetExpiresAt(expires).SetDailyWindowStart(timezone.StartOfDay(now)).SetDailyUsageUsd(80).SetWeeklyUsageUsd(90).SetMonthlyUsageUsd(95).Save(ctx)
		require.NoError(t, e)
		return sub
	}
	sub := createSub(owner.ID, group.ID, now.AddDate(-1, 0, 0), now.AddDate(10, 0, 0))
	repo := NewSubscriptionSelfResetRepository(client)
	subRepo := NewUserSubscriptionRepository(client)
	subs := service.NewSubscriptionService(nil, subRepo, nil, client, nil)
	defer subs.Stop()
	svc := service.NewSubscriptionSelfResetService(repo, subs)
	policy := `{"daily_limit_by_organization":{"xunyou":1,"wsdashi":1,"other":1}}`
	require.NoError(t, repo.WritePolicy(ctx, policy))
	defer func() { require.NoError(t, repo.WritePolicy(context.Background(), policy)) }()
	config := service.DefaultIdempotencyConfig()
	config.DefaultTTL = time.Second
	coordinator := service.NewIdempotencyCoordinator(NewIdempotencyRepository(client, integrationDB), config)
	resetAs := func(userID, id int64, date, key string) (*service.IdempotencyExecuteResult, error) {
		return coordinator.Execute(ctx, service.IdempotencyExecuteOptions{Scope: service.SubscriptionSelfResetIdempotencyScope(userID), ActorScope: fmt.Sprint(userID), Method: "POST", Route: "/:id/reset-daily", IdempotencyKey: key, Payload: []any{id, date}, RequireKey: true, AtomicSuccess: true, TTL: 48 * time.Hour}, func(txCtx context.Context) (any, error) { return svc.Reset(txCtx, userID, id, service.RoleAdmin, date) })
	}
	reset := func(id int64, date, key string) (*service.IdempotencyExecuteResult, error) {
		return resetAs(owner.ID, id, date, key)
	}
	// First-row creation and distinct keys compete on the subscription lock.
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) { defer wg.Done(); _, err := reset(sub.ID, today, fmt.Sprintf("key-%d", i)); errs <- err }(i)
	}
	wg.Wait()
	close(errs)
	success := 0
	for err := range errs {
		if err == nil {
			success++
		} else {
			require.Equal(t, "SELF_RESET_LIMIT_REACHED", errors.Reason(err))
		}
	}
	require.Equal(t, 1, success)
	usage, err := repo.GetUsage(ctx, sub.ID)
	require.NoError(t, err)
	require.Equal(t, today, usage.QuotaDate)
	require.Equal(t, 1, usage.UsedCount)
	updated, err := client.UserSubscription.Get(ctx, sub.ID)
	require.NoError(t, err)
	require.Zero(t, updated.DailyUsageUsd)
	require.Equal(t, 90.0, updated.WeeklyUsageUsd)
	require.Equal(t, 95.0, updated.MonthlyUsageUsd)
	require.WithinDuration(t, sub.ExpiresAt, updated.ExpiresAt, time.Microsecond)
	// Keys are scoped per user: another user's identical key is a separate operation.
	other, err := client.User.Create().SetEmail(prefix + "@example.com").SetPasswordHash("unused").SetStatus(service.StatusActive).Save(ctx)
	require.NoError(t, err)
	otherSub := createSub(other.ID, group.ID, now.AddDate(-1, 0, 0), now.AddDate(10, 0, 0))
	otherResult, err := resetAs(other.ID, otherSub.ID, today, "key-0")
	require.NoError(t, err)
	require.False(t, otherResult.Replayed)
	usage, err = repo.GetUsage(ctx, otherSub.ID)
	require.NoError(t, err)
	require.Equal(t, 1, usage.UsedCount)
	// A same-day policy increase preserves consumption, and a one-connection pool must work.
	require.NoError(t, repo.WritePolicy(ctx, `{"daily_limit_by_organization":{"xunyou":3,"wsdashi":1,"other":1}}`))
	require.NoError(t, subRepo.IncrementUsage(ctx, sub.ID, 5))
	integrationDB.SetMaxOpenConns(1)
	_, err = reset(sub.ID, today, "one-connection")
	require.NoError(t, err)
	integrationDB.SetMaxOpenConns(10)
	replayed, err := reset(sub.ID, today, "one-connection")
	require.NoError(t, err)
	require.True(t, replayed.Replayed)
	usage, err = repo.GetUsage(ctx, sub.ID)
	require.NoError(t, err)
	require.Equal(t, 2, usage.UsedCount)
	var ttlHours float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT EXTRACT(EPOCH FROM (expires_at-created_at))/3600 FROM idempotency_records WHERE scope=$1 AND idempotency_key_hash=$2`, service.SubscriptionSelfResetIdempotencyScope(owner.ID), service.HashIdempotencyKey("one-connection")).Scan(&ttlHours))
	require.InDelta(t, 48, ttlHours, 0.01)
	_, err = reset(sub.ID, today, "one-connection")
	require.NoError(t, err)
	// No usage / stale day / wrong owner must not consume.
	_, err = reset(sub.ID, today, "no-usage")
	require.Equal(t, "SELF_RESET_NO_USAGE", errors.Reason(err))
	_, err = reset(sub.ID, "2000-01-01", "old-date")
	require.Equal(t, "SELF_RESET_DAY_CHANGED", errors.Reason(err))
	tx, err := client.Tx(ctx)
	require.NoError(t, err)
	_, err = repo.GetOwnedByIDForUpdate(dbent.NewTxContext(ctx, tx), owner.ID+100000, sub.ID)
	require.ErrorIs(t, err, service.ErrSubscriptionNotFound)
	require.NoError(t, tx.Rollback())
	// Force failure after both writes; the coordinator must roll back both.
	require.NoError(t, subRepo.IncrementUsage(ctx, sub.ID, 7))
	_, err = coordinator.Execute(ctx, service.IdempotencyExecuteOptions{Scope: prefix, ActorScope: fmt.Sprint(owner.ID), Method: "POST", Route: "/:id/reset-daily", IdempotencyKey: "rollback", Payload: sub.ID, RequireKey: true, AtomicSuccess: true, TTL: 48 * time.Hour}, func(txCtx context.Context) (any, error) {
		_, e := svc.Reset(txCtx, owner.ID, sub.ID, service.RoleAdmin, today)
		if e != nil {
			return nil, e
		}
		return nil, fmt.Errorf("injected failure after reset")
	})
	require.ErrorContains(t, err, "injected failure")
	usage, err = repo.GetUsage(ctx, sub.ID)
	require.NoError(t, err)
	require.Equal(t, 2, usage.UsedCount)
	updated, err = client.UserSubscription.Get(ctx, sub.ID)
	require.NoError(t, err)
	require.Equal(t, 7.0, updated.DailyUsageUsd)
	// Failing the final idempotency write must roll back usage and count too.
	failingRepo := &failAtomicMarkSucceededRepo{idempotencyRepository: &idempotencyRepository{client: client, sql: integrationDB}}
	failingCoordinator := service.NewIdempotencyCoordinator(failingRepo, config)
	_, err = failingCoordinator.Execute(ctx, service.IdempotencyExecuteOptions{Scope: prefix, ActorScope: fmt.Sprint(owner.ID), Method: "POST", Route: "/:id/reset-daily", IdempotencyKey: "mark-failure", Payload: sub.ID, RequireKey: true, AtomicSuccess: true, TTL: 48 * time.Hour}, func(txCtx context.Context) (any, error) {
		return svc.Reset(txCtx, owner.ID, sub.ID, service.RoleAdmin, today)
	})
	require.ErrorIs(t, err, service.ErrIdempotencyStoreUnavail)
	require.True(t, failingRepo.sawTransaction)
	usage, err = repo.GetUsage(ctx, sub.ID)
	require.NoError(t, err)
	require.Equal(t, 2, usage.UsedCount)
	updated, err = client.UserSubscription.Get(ctx, sub.ID)
	require.NoError(t, err)
	require.Equal(t, 7.0, updated.DailyUsageUsd)
	// Natural day rollover is lazy and independent of the DB session timezone (UTC).
	// A lost acknowledgement after a real commit must recover, not perform the reset twice.
	commitErrorCoordinator := service.NewIdempotencyCoordinator(&selfResetCommitErrorRepo{&idempotencyRepository{client: client, sql: integrationDB}}, config)
	recovered, err := commitErrorCoordinator.Execute(ctx, service.IdempotencyExecuteOptions{Scope: prefix, ActorScope: fmt.Sprint(owner.ID), Method: "POST", Route: "/:id/reset-daily", IdempotencyKey: "commit-ack-lost", Payload: sub.ID, RequireKey: true, AtomicSuccess: true, TTL: 48 * time.Hour}, func(txCtx context.Context) (any, error) {
		return svc.Reset(txCtx, owner.ID, sub.ID, service.RoleAdmin, today)
	})
	require.NoError(t, err)
	require.NotNil(t, recovered)
	usage, err = repo.GetUsage(ctx, sub.ID)
	require.NoError(t, err)
	require.Equal(t, 3, usage.UsedCount)
	require.NoError(t, subRepo.IncrementUsage(ctx, sub.ID, 7))
	_, err = integrationDB.ExecContext(ctx, `UPDATE subscription_self_daily_reset_usage SET quota_date=$2::date,used_count=99 WHERE subscription_id=$1`, sub.ID, timezone.StartOfDay(now).AddDate(0, 0, -1).Format(time.DateOnly))
	require.NoError(t, err)
	status, err := svc.Status(ctx, owner.ID, service.RoleAdmin)
	require.NoError(t, err)
	require.Equal(t, today, status.QuotaDate)
	require.Equal(t, 3, status.Subscriptions[0].RemainingCount)
	_, err = reset(sub.ID, today, "new-day")
	require.NoError(t, err)
	usage, err = repo.GetUsage(ctx, sub.ID)
	require.NoError(t, err)
	require.Equal(t, 1, usage.UsedCount)
	// Administrator resets never touch the self-service ledger.
	for i := 0; i < 2; i++ {
		_, err = subs.AdminResetQuota(ctx, sub.ID, true, false, false)
		require.NoError(t, err)
	}
	usage, err = repo.GetUsage(ctx, sub.ID)
	require.NoError(t, err)
	require.Equal(t, 1, usage.UsedCount)
	// A second subscription gets its own count, even for the same owner.
	group2, err := client.Group.Create().SetName(prefix + "-2").SetStatus(service.StatusActive).SetSubscriptionType(service.SubscriptionTypeSubscription).SetDailyLimitUsd(100).Save(ctx)
	require.NoError(t, err)
	sub2 := createSub(owner.ID, group2.ID, now.Add(-time.Hour), now.AddDate(10, 0, 0))
	_, err = reset(sub2.ID, today, "other-subscription")
	require.NoError(t, err)
	usage, err = repo.GetUsage(ctx, sub2.ID)
	require.NoError(t, err)
	require.Equal(t, 1, usage.UsedCount)
	_, err = client.UserSubscription.UpdateOneID(sub2.ID).SetStartsAt(now.Add(-time.Hour)).SetExpiresAt(now.Add(time.Hour)).SetDailyUsageUsd(50).Save(ctx)
	require.NoError(t, err)
	_, err = reset(sub2.ID, today, "one-time")
	require.Equal(t, "SELF_RESET_ONE_TIME_QUOTA", errors.Reason(err))
	usage, err = repo.GetUsage(ctx, sub2.ID)
	require.NoError(t, err)
	require.Equal(t, 1, usage.UsedCount)
	// A soft-deleted group reports the same reason from status and reset.
	_, err = integrationDB.ExecContext(ctx, `UPDATE groups SET deleted_at=NOW() WHERE id=$1`, group2.ID)
	require.NoError(t, err)
	status, err = svc.Status(ctx, owner.ID, service.RoleAdmin)
	require.NoError(t, err)
	for _, item := range status.Subscriptions {
		if item.SubscriptionID == sub2.ID {
			require.Equal(t, "GROUP_DISABLED", *item.DisabledReason)
		}
	}
	_, err = reset(sub2.ID, today, "deleted-group")
	require.Equal(t, "SELF_RESET_GROUP_DISABLED", errors.Reason(err))
}

func TestSubscriptionSelfResetCrossInstanceCaches(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client := integrationEntClient
	prefix := uuid.NewString()
	owner, err := client.User.Create().SetEmail(prefix + "@xunyou.com").SetPasswordHash("unused").SetStatus(service.StatusActive).Save(ctx)
	require.NoError(t, err)
	group, err := client.Group.Create().SetName(prefix).SetStatus(service.StatusActive).SetSubscriptionType(service.SubscriptionTypeSubscription).SetDailyLimitUsd(100).Save(ctx)
	require.NoError(t, err)
	now := time.Now()
	sub, err := client.UserSubscription.Create().SetUserID(owner.ID).SetGroupID(group.ID).SetStatus(service.SubscriptionStatusActive).SetStartsAt(now.Add(-time.Hour)).SetExpiresAt(now.AddDate(10, 0, 0)).SetDailyWindowStart(timezone.StartOfDay(now)).SetDailyUsageUsd(80).Save(ctx)
	require.NoError(t, err)
	redisServer := miniredis.RunT(t)
	rdb1 := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer rdb1.Close()
	rdb2 := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer rdb2.Close()
	cache1, cache2 := NewBillingCache(rdb1), NewBillingCache(rdb2)
	cfg := &config.Config{SubscriptionCache: config.SubscriptionCacheConfig{L1Size: 100, L1TTLSeconds: 300}}
	subRepo := NewUserSubscriptionRepository(client)
	billing1 := service.NewBillingCacheService(cache1, nil, subRepo, nil, nil, nil, cfg, nil)
	defer billing1.Stop()
	billing2 := service.NewBillingCacheService(cache2, nil, subRepo, nil, nil, nil, cfg, nil)
	defer billing2.Stop()
	node1 := service.NewSubscriptionService(nil, subRepo, billing1, client, cfg)
	defer node1.Stop()
	node2 := service.NewSubscriptionService(nil, subRepo, billing2, client, cfg)
	defer node2.Stop()
	for _, node := range []*service.SubscriptionService{node1, node2} {
		warm, e := node.GetActiveSubscription(ctx, owner.ID, group.ID)
		require.NoError(t, e)
		require.Equal(t, 80.0, warm.DailyUsageUSD)
	}
	// Ristretto admission is asynchronous. Confirm stale L1s against an independently updated DB row.
	time.Sleep(30 * time.Millisecond)
	_, err = client.UserSubscription.UpdateOneID(sub.ID).SetDailyUsageUsd(81).Save(ctx)
	require.NoError(t, err)
	for _, node := range []*service.SubscriptionService{node1, node2} {
		warm, e := node.GetActiveSubscription(ctx, owner.ID, group.ID)
		require.NoError(t, e)
		require.Equal(t, 80.0, warm.DailyUsageUSD, "L1 must actually be warm")
	}
	require.NoError(t, cache1.SetSubscriptionCache(ctx, owner.ID, group.ID, &service.SubscriptionCacheData{Status: service.SubscriptionStatusActive, ExpiresAt: sub.ExpiresAt, DailyUsage: 80}))
	read, e := cache2.GetSubscriptionCache(ctx, owner.ID, group.ID)
	require.NoError(t, e)
	require.Equal(t, 80.0, read.DailyUsage)
	repo := NewSubscriptionSelfResetRepository(client)
	svc := service.NewSubscriptionSelfResetService(repo, node1)
	coordinator := service.NewIdempotencyCoordinator(NewIdempotencyRepository(client, integrationDB), service.DefaultIdempotencyConfig())
	_, err = coordinator.Execute(ctx, service.IdempotencyExecuteOptions{Scope: prefix, ActorScope: fmt.Sprint(owner.ID), Method: "POST", Route: "/:id/reset-daily", IdempotencyKey: "reset", Payload: sub.ID, RequireKey: true, AtomicSuccess: true, TTL: 48 * time.Hour}, func(txCtx context.Context) (any, error) {
		return svc.Reset(txCtx, owner.ID, sub.ID, service.RoleAdmin, timezone.StartOfDay(now).Format(time.DateOnly))
	})
	require.NoError(t, err)
	_, err = cache2.GetSubscriptionCache(ctx, owner.ID, group.ID)
	require.Error(t, err, "shared billing cache must be deleted")
	for _, node := range []*service.SubscriptionService{node1, node2} {
		require.Eventually(t, func() bool {
			fresh, e := node.GetActiveSubscription(ctx, owner.ID, group.ID)
			return e == nil && fresh.DailyUsageUSD == 0
		}, 3*time.Second, 10*time.Millisecond, "both service instances must reload committed usage")
	}
}

func TestSubscriptionSelfResetOrganizationParityAndDates(t *testing.T) {
	ctx := context.Background()
	for _, email := range []string{"a@XUNYOU.COM", "a@wsdashi.com", "a@team.xunyou.com", "a@xunyou.com@other.com", "a@other.com@xunyou.com", "", "a@", "a"} {
		var expected string
		require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT CASE WHEN LOWER(SPLIT_PART($1,'@',2))='xunyou.com' THEN 'xunyou' WHEN LOWER(SPLIT_PART($1,'@',2))='wsdashi.com' THEN 'wsdashi' ELSE 'other' END`, email).Scan(&expected))
		require.Equal(t, expected, service.ResolveSubscriptionOrganization(email), email)
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	for _, stamp := range []string{"2026-09-22T23:59:00+08:00", "2026-09-23T00:30:00+08:00", "2026-09-23T08:30:00+08:00"} {
		now, err := time.Parse(time.RFC3339, stamp)
		require.NoError(t, err)
		date := now.In(loc).Format(time.DateOnly)
		var actual string
		require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT to_char($1::date, 'YYYY-MM-DD')`, date).Scan(&actual))
		require.Equal(t, date, actual)
	}
}
