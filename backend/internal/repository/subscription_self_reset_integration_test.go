//go:build integration

package repository

import (
	"context"
	"fmt"
	"slices"
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
	"github.com/lib/pq"
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

// writeSelfResetFixturePolicy seeds or restores raw policy values in the isolated test DB.
func writeSelfResetFixturePolicy(ctx context.Context, policy string) error {
	_, err := integrationDB.ExecContext(ctx, `INSERT INTO settings (key, value, updated_at) VALUES ($1,$2,NOW())
ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value, updated_at=NOW()`, service.SubscriptionSelfResetPolicyKey, policy)
	return err
}

// selfResetAllow admits the group for the organization: the group gets that
// organization's scope ("other" means no scope at all) and is ticked for it.
func selfResetAllow(t *testing.T, ctx context.Context, groupID int64, organization string) {
	t.Helper()
	if organization != service.OrganizationOther {
		_, err := integrationDB.ExecContext(ctx, `INSERT INTO group_organization_scopes(group_id,organization_key) VALUES($1,$2) ON CONFLICT DO NOTHING`, groupID, organization)
		require.NoError(t, err)
	}
	_, err := integrationDB.ExecContext(ctx, `INSERT INTO subscription_self_reset_groups(group_id,organization_key) VALUES($1,$2)`, groupID, organization)
	require.NoError(t, err)
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
	require.NoError(t, writeSelfResetFixturePolicy(ctx, policy))
	defer func() { require.NoError(t, writeSelfResetFixturePolicy(context.Background(), policy)) }()
	config := service.DefaultIdempotencyConfig()
	config.DefaultTTL = time.Second
	coordinator := service.NewIdempotencyCoordinator(NewIdempotencyRepository(client, integrationDB), config)
	resetAs := func(userID, id int64, date, key string) (*service.IdempotencyExecuteResult, error) {
		return coordinator.Execute(ctx, service.IdempotencyExecuteOptions{Scope: service.SubscriptionSelfResetIdempotencyScope(userID), ActorScope: fmt.Sprint(userID), Method: "POST", Route: "/:id/reset-daily", IdempotencyKey: key, Payload: []any{id, date}, RequireKey: true, AtomicSuccess: true, TTL: 48 * time.Hour}, func(txCtx context.Context) (any, error) { return svc.Reset(txCtx, userID, id, service.RoleAdmin, date) })
	}
	reset := func(id int64, date, key string) (*service.IdempotencyExecuteResult, error) {
		return resetAs(owner.ID, id, date, key)
	}
	// Fail-closed: a group nobody ticked is not admitted, and admission is
	// checked before the day. A scope without a tick changes nothing.
	requireNotAllowed := func(step string) {
		status, e := svc.Status(ctx, owner.ID, service.RoleAdmin)
		require.NoError(t, e, step)
		require.Equal(t, "GROUP_NOT_ALLOWED", *status.Subscriptions[0].DisabledReason, step)
		for _, date := range []string{today, "2000-01-01"} {
			_, e = reset(sub.ID, date, step+date)
			require.Equal(t, "SELF_RESET_GROUP_NOT_ALLOWED", errors.Reason(e), step)
		}
		usage, e := repo.GetUsage(ctx, sub.ID)
		require.NoError(t, e, step)
		require.Zero(t, usage.UsedCount, step)
		unchanged, e := client.UserSubscription.Get(ctx, sub.ID)
		require.NoError(t, e, step)
		require.Equal(t, 80.0, unchanged.DailyUsageUsd, step)
	}
	requireNotAllowed("no-tick-")
	_, err = integrationDB.ExecContext(ctx, `INSERT INTO group_organization_scopes(group_id,organization_key) VALUES($1,$2)`, group.ID, service.OrganizationXunyou)
	require.NoError(t, err)
	requireNotAllowed("scope-only-")
	selfResetAllow(t, ctx, group.ID, service.OrganizationXunyou)
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
	// The other user's organization only matches a group without a scope.
	other, err := client.User.Create().SetEmail(prefix + "@example.com").SetPasswordHash("unused").SetStatus(service.StatusActive).Save(ctx)
	require.NoError(t, err)
	otherGroup, err := client.Group.Create().SetName(prefix + "-other").SetStatus(service.StatusActive).SetSubscriptionType(service.SubscriptionTypeSubscription).SetDailyLimitUsd(100).Save(ctx)
	require.NoError(t, err)
	selfResetAllow(t, ctx, otherGroup.ID, service.OrganizationOther)
	otherSub := createSub(other.ID, otherGroup.ID, now.AddDate(-1, 0, 0), now.AddDate(10, 0, 0))
	otherResult, err := resetAs(other.ID, otherSub.ID, today, "key-0")
	require.NoError(t, err)
	require.False(t, otherResult.Replayed)
	usage, err = repo.GetUsage(ctx, otherSub.ID)
	require.NoError(t, err)
	require.Equal(t, 1, usage.UsedCount)
	// A same-day policy increase preserves consumption, and a one-connection pool must work.
	require.NoError(t, writeSelfResetFixturePolicy(ctx, `{"daily_limit_by_organization":{"xunyou":3,"wsdashi":1,"other":1}}`))
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
	selfResetAllow(t, ctx, group2.ID, service.OrganizationXunyou)
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
	// A soft-deleted group leaves admission: status and reset report the same reason.
	_, err = integrationDB.ExecContext(ctx, `UPDATE groups SET deleted_at=NOW() WHERE id=$1`, group2.ID)
	require.NoError(t, err)
	status, err = svc.Status(ctx, owner.ID, service.RoleAdmin)
	require.NoError(t, err)
	found := false
	for _, item := range status.Subscriptions {
		if item.SubscriptionID == sub2.ID {
			found = true
			require.Equal(t, "GROUP_NOT_ALLOWED", *item.DisabledReason)
		}
	}
	require.True(t, found, "the subscription of a soft-deleted group stays in the status list")
	_, err = reset(sub2.ID, today, "deleted-group")
	require.Equal(t, "SELF_RESET_GROUP_NOT_ALLOWED", errors.Reason(err))
	// History has exactly one row per committed success: replays, rollbacks and rejections add none.
	events, total, err := repo.ListEvents(ctx, service.SubscriptionSelfResetEventFilter{Email: prefix, Page: 1, PageSize: 50})
	require.NoError(t, err)
	require.EqualValues(t, 6, total)
	perSub := map[int64]int{}
	for _, e := range events {
		perSub[e.SubscriptionID]++
	}
	require.Equal(t, map[int64]int{sub.ID: 4, sub2.ID: 1, otherSub.ID: 1}, perSub)
	first := events[len(events)-1]
	require.Equal(t, service.SubscriptionSelfResetEvent{ID: first.ID, SubscriptionID: sub.ID, UserID: owner.ID, UserEmail: owner.Email, GroupID: group.ID, GroupName: group.Name, Organization: service.OrganizationXunyou, QuotaDate: today, UsedCount: 1, DailyLimit: 1, DailyUsageUSDBefore: 80, CreatedAt: first.CreatedAt}, first)
	events, total, err = repo.ListEvents(ctx, service.SubscriptionSelfResetEventFilter{Email: prefix, Organization: service.OrganizationOther, StartDate: today, EndDate: today, Page: 1, PageSize: 50})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, otherSub.ID, events[0].SubscriptionID)
	_, total, err = repo.ListEvents(ctx, service.SubscriptionSelfResetEventFilter{Email: prefix, StartDate: timezone.StartOfDay(now).AddDate(0, 0, 1).Format(time.DateOnly), Page: 1, PageSize: 50})
	require.NoError(t, err)
	require.Zero(t, total)
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
	selfResetAllow(t, ctx, group.ID, service.OrganizationXunyou)
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

type selfResetGroupChanges = map[string]service.SubscriptionSelfResetGroupChanges

// selfResetGroupFixture drives the per-organization group allowlist
// (docs/features/subscription-self-daily-reset-design-cn.md §5, §6.3, §8.1)
// through the real services. The database is shared, so it only looks at the
// groups it created.
type selfResetGroupFixture struct {
	t      *testing.T
	ctx    context.Context
	prefix string
	repo   service.SubscriptionSelfResetRepository
	subs   *service.SubscriptionService
	svc    *service.SubscriptionSelfResetService
	mine   map[int64]bool
}

func newSelfResetGroupFixture(t *testing.T) *selfResetGroupFixture {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	f := &selfResetGroupFixture{t: t, ctx: ctx, prefix: uuid.NewString(), repo: NewSubscriptionSelfResetRepository(integrationEntClient), mine: map[int64]bool{}}
	f.subs = service.NewSubscriptionService(NewGroupRepository(integrationEntClient, integrationDB), NewUserSubscriptionRepository(integrationEntClient), nil, integrationEntClient, nil)
	t.Cleanup(f.subs.Stop)
	f.svc = service.NewSubscriptionSelfResetService(f.repo, f.subs)
	return f
}

func (f *selfResetGroupFixture) exec(query string, args ...any) {
	f.t.Helper()
	_, err := integrationDB.ExecContext(f.ctx, query, args...)
	require.NoError(f.t, err)
}

// group creates an active subscription group with a daily limit and the given scopes.
func (f *selfResetGroupFixture) group(name string, scopes ...string) int64 {
	f.t.Helper()
	var id int64
	require.NoError(f.t, integrationDB.QueryRowContext(f.ctx, `INSERT INTO groups(name,platform,subscription_type,daily_limit_usd) VALUES($1,'openai','subscription',100) RETURNING id`, f.prefix+"-"+name).Scan(&id))
	for _, organization := range scopes {
		f.exec(`INSERT INTO group_organization_scopes(group_id,organization_key) VALUES($1,$2)`, id, organization)
	}
	f.mine[id] = true
	return id
}

// ticks lists the organizations the group is ticked for, e.g. "wsdashi,xunyou".
func (f *selfResetGroupFixture) ticks(groupID int64) string {
	f.t.Helper()
	var keys string
	require.NoError(f.t, integrationDB.QueryRowContext(f.ctx, `SELECT COALESCE(string_agg(organization_key, ',' ORDER BY organization_key), '') FROM subscription_self_reset_groups WHERE group_id=$1`, groupID).Scan(&keys))
	return keys
}

func (f *selfResetGroupFixture) revision() int64 {
	f.t.Helper()
	revision, err := f.repo.ConfigRevision(f.ctx, false)
	require.NoError(f.t, err)
	return revision
}

func (f *selfResetGroupFixture) saveGroups(revision int64, changes selfResetGroupChanges) error {
	_, err := f.svc.SaveConfig(f.ctx, service.SubscriptionSelfResetConfigChange{ExpectedRevision: revision, Groups: changes})
	return err
}

func (f *selfResetGroupFixture) setGroups(changes selfResetGroupChanges) map[string][]service.SubscriptionSelfResetGroup {
	f.t.Helper()
	require.NoError(f.t, f.saveGroups(f.revision(), changes))
	return f.listGroups()
}

func (f *selfResetGroupFixture) listGroups() map[string][]service.SubscriptionSelfResetGroup {
	f.t.Helper()
	groups, err := f.svc.Groups(f.ctx)
	require.NoError(f.t, err)
	return groups
}

// candidates maps this fixture's candidate groups of an organization to their tick.
func (f *selfResetGroupFixture) candidates(groups map[string][]service.SubscriptionSelfResetGroup, organization string) map[int64]bool {
	result := map[int64]bool{}
	for _, group := range groups[organization] {
		if f.mine[group.ID] {
			result[group.ID] = group.Enabled
		}
	}
	return result
}

// allowed returns, in the given order, the groups admitted for the organization.
func (f *selfResetGroupFixture) allowed(organization string, ids ...int64) []int64 {
	f.t.Helper()
	_, admitted, err := f.repo.ReadAdmission(f.ctx, ids, organization)
	require.NoError(f.t, err)
	result := []int64{}
	for _, id := range ids {
		if admitted[id] {
			result = append(result, id)
		}
	}
	return result
}

// setScopes replaces the scopes through the service behind /admin/groups; no keys clears them.
func (f *selfResetGroupFixture) setScopes(ids []int64, keys ...string) {
	f.t.Helper()
	if keys == nil {
		keys = []string{}
	}
	_, err := f.subs.SetGroupOrganizationScopes(f.ctx, service.GroupOrganizationScopeInput{GroupIDs: ids, OrganizationKeys: &keys})
	require.NoError(f.t, err)
}

// Candidates, differential saves, strict admission by the user's own
// organization, and the history filters.
func TestSubscriptionSelfResetGroupAdmission(t *testing.T) {
	f := newSelfResetGroupFixture(t)
	ctx, client := f.ctx, integrationEntClient
	xunyou, wsdashi, other := service.OrganizationXunyou, service.OrganizationWsdashi, service.OrganizationOther
	gX, gW, gXW, gO := f.group("x", xunyou), f.group("w", wsdashi), f.group("xw", xunyou, wsdashi), f.group("o")
	all := []int64{gX, gW, gXW, gO}

	// Each organization lists the groups scoped to it; "other" lists unscoped
	// groups. Nothing is ticked at first, so nobody is admitted.
	groups := f.listGroups()
	require.Equal(t, map[int64]bool{gX: false, gXW: false}, f.candidates(groups, xunyou))
	require.Equal(t, map[int64]bool{gW: false, gXW: false}, f.candidates(groups, wsdashi))
	require.Equal(t, map[int64]bool{gO: false}, f.candidates(groups, other))
	for _, organization := range []string{xunyou, wsdashi, other} {
		require.Empty(t, f.allowed(organization, all...), organization)
	}

	// Saving outside a transaction is refused: the group lock would not be held.
	_, err := f.repo.ApplyGroupChanges(ctx, selfResetGroupChanges{xunyou: {Enable: []int64{gX}}})
	require.ErrorContains(t, err, "require a transaction")
	require.Empty(t, f.ticks(gX))

	// One ineligible enable rolls back the entire batch for every organization.
	standard, deleted, missing := f.group("standard"), f.group("deleted", xunyou), int64(1)<<40
	f.exec(`UPDATE groups SET subscription_type='standard' WHERE id=$1`, standard)
	f.exec(`UPDATE groups SET deleted_at=NOW() WHERE id=$1`, deleted)
	err = f.saveGroups(f.revision(), selfResetGroupChanges{
		xunyou:  {Enable: []int64{gX, gW, gXW, gO, standard, deleted, missing}},
		wsdashi: {Enable: []int64{gW}},
		other:   {Enable: []int64{gO, gX}},
	})
	require.ErrorIs(t, err, service.ErrSelfResetGroupConflict)
	for _, id := range all {
		require.Empty(t, f.ticks(id))
	}
	groups = f.setGroups(selfResetGroupChanges{
		xunyou: {Enable: []int64{gX, gXW}}, wsdashi: {Enable: []int64{gW}}, other: {Enable: []int64{gO}},
	})
	require.Equal(t, map[int64]bool{gX: true, gXW: true}, f.candidates(groups, xunyou))
	require.Equal(t, map[int64]bool{gW: true, gXW: false}, f.candidates(groups, wsdashi), "a tick belongs to one organization")
	require.Equal(t, map[int64]bool{gO: true}, f.candidates(groups, other))
	require.Equal(t, []int64{gX, gXW}, f.allowed(xunyou, all...))
	require.Equal(t, []int64{gW}, f.allowed(wsdashi, all...))
	require.Equal(t, []int64{gO}, f.allowed(other, all...))
	for _, id := range []int64{standard, deleted} {
		require.Empty(t, f.ticks(id))
	}

	// The candidate rows carry the fields of the dialog, in group order.
	// Disabled groups and groups without a daily limit stay listed.
	gN := f.group("nolimit", xunyou)
	f.exec(`UPDATE groups SET daily_limit_usd=NULL, status='disabled', sort_order=-1 WHERE id=$1`, gN)
	groups = f.setGroups(selfResetGroupChanges{xunyou: {Enable: []int64{gN}}})
	listed := []service.SubscriptionSelfResetGroup{}
	for _, group := range groups[xunyou] {
		if f.mine[group.ID] {
			listed = append(listed, group)
		}
	}
	limit := 100.0
	require.Equal(t, []service.SubscriptionSelfResetGroup{
		{ID: gN, Name: f.prefix + "-nolimit", Platform: "openai", Status: "disabled", SubscriptionType: service.SubscriptionTypeSubscription, Enabled: true},
		{ID: gX, Name: f.prefix + "-x", Platform: "openai", Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeSubscription, DailyLimitUSD: &limit, Enabled: true},
		{ID: gXW, Name: f.prefix + "-xw", Platform: "openai", Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeSubscription, DailyLimitUSD: &limit, Enabled: true},
	}, listed)

	// Users are matched by their own organization only: a group ticked for
	// another organization, or an unscoped group ticked for "other", does not
	// admit them.
	previous, err := f.repo.ReadPolicy(ctx)
	require.NoError(t, err)
	require.NoError(t, writeSelfResetFixturePolicy(ctx, `{"rollout":"all","daily_limit_by_organization":{"xunyou":1,"wsdashi":1,"other":1}}`))
	t.Cleanup(func() { require.NoError(t, writeSelfResetFixturePolicy(context.Background(), previous)) })
	now := time.Now()
	today := timezone.StartOfDay(now).Format(time.DateOnly)
	user := func(domain string) int64 {
		created, e := client.User.Create().SetEmail(f.prefix + "@" + domain).SetPasswordHash("unused").SetStatus(service.StatusActive).Save(ctx)
		require.NoError(t, e)
		return created.ID
	}
	subscribe := func(userID, groupID int64) int64 {
		created, e := client.UserSubscription.Create().SetUserID(userID).SetGroupID(groupID).SetStatus(service.SubscriptionStatusActive).SetStartsAt(now.AddDate(-1, 0, 0)).SetExpiresAt(now.AddDate(10, 0, 0)).SetDailyWindowStart(timezone.StartOfDay(now)).SetDailyUsageUsd(80).Save(ctx)
		require.NoError(t, e)
		return created.ID
	}
	reasons := func(userID int64) map[int64]string {
		status, e := f.svc.Status(ctx, userID, service.RoleUser)
		require.NoError(t, e)
		result := map[int64]string{}
		for _, item := range status.Subscriptions {
			result[item.SubscriptionID] = ""
			if item.DisabledReason != nil {
				result[item.SubscriptionID] = *item.DisabledReason
			}
		}
		return result
	}
	coordinator := service.NewIdempotencyCoordinator(NewIdempotencyRepository(client, integrationDB), service.DefaultIdempotencyConfig())
	attempt := 0
	reset := func(userID, subscriptionID int64) string {
		attempt++
		_, e := coordinator.Execute(ctx, service.IdempotencyExecuteOptions{Scope: service.SubscriptionSelfResetIdempotencyScope(userID), ActorScope: fmt.Sprint(userID), Method: "POST", Route: "/:id/reset-daily", IdempotencyKey: fmt.Sprint("reset-", attempt), Payload: []any{subscriptionID, today}, RequireKey: true, AtomicSuccess: true, TTL: 48 * time.Hour}, func(txCtx context.Context) (any, error) {
			return f.svc.Reset(txCtx, userID, subscriptionID, service.RoleUser, today)
		})
		return errors.Reason(e)
	}
	ux, uw, uo := user("xunyou.com"), user("wsdashi.com"), user("example.com")
	xX, xO, xW, xN := subscribe(ux, gX), subscribe(ux, gO), subscribe(ux, gW), subscribe(ux, gN)
	wW, wXW, wO := subscribe(uw, gW), subscribe(uw, gXW), subscribe(uw, gO)
	oO, oX := subscribe(uo, gO), subscribe(uo, gX)
	require.Equal(t, map[int64]string{xX: "", xO: "GROUP_NOT_ALLOWED", xW: "GROUP_NOT_ALLOWED", xN: "GROUP_DISABLED"}, reasons(ux), "a ticked but disabled group keeps its own reason")
	require.Equal(t, map[int64]string{wW: "", wXW: "GROUP_NOT_ALLOWED", wO: "GROUP_NOT_ALLOWED"}, reasons(uw))
	require.Equal(t, map[int64]string{oO: "", oX: "GROUP_NOT_ALLOWED"}, reasons(uo))
	for userID, denied := range map[int64][]int64{ux: {xO, xW}, uw: {wXW, wO}, uo: {oX}} {
		for _, subscriptionID := range denied {
			require.Equal(t, "SELF_RESET_GROUP_NOT_ALLOWED", reset(userID, subscriptionID))
			usage, e := f.repo.GetUsage(ctx, subscriptionID)
			require.NoError(t, e)
			require.Zero(t, usage.UsedCount)
			unchanged, e := client.UserSubscription.Get(ctx, subscriptionID)
			require.NoError(t, e)
			require.Equal(t, 80.0, unchanged.DailyUsageUsd)
		}
	}
	require.Empty(t, reset(ux, xX))
	require.Empty(t, reset(uw, wW))
	require.Empty(t, reset(uo, oO))

	// A save is a diff: listed IDs change, everything else stays. Enabling a
	// ticked group and disabling an unticked one are no-ops, and a disable only
	// touches its own organization.
	groups = f.setGroups(selfResetGroupChanges{xunyou: {Enable: []int64{gX}, Disable: []int64{gXW, gW}}})
	require.Equal(t, map[int64]bool{gN: true, gX: true, gXW: false}, f.candidates(groups, xunyou))
	require.Equal(t, wsdashi, f.ticks(gW))
	require.Equal(t, other, f.ticks(gO))
	// Admission follows the saved ticks at once.
	f.setGroups(selfResetGroupChanges{xunyou: {Disable: []int64{gX}}})
	require.Equal(t, "GROUP_NOT_ALLOWED", reasons(ux)[xX])
	require.Equal(t, "SELF_RESET_GROUP_NOT_ALLOWED", reset(ux, xX))

	// History: the group filter, alone and combined with the other filters.
	events, total, err := f.repo.ListEvents(ctx, service.SubscriptionSelfResetEventFilter{GroupID: gW, Page: 1, PageSize: 50})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, wW, events[0].SubscriptionID)
	require.Equal(t, wsdashi, events[0].Organization)
	_, total, err = f.repo.ListEvents(ctx, service.SubscriptionSelfResetEventFilter{Email: f.prefix, Page: 1, PageSize: 50})
	require.NoError(t, err)
	require.EqualValues(t, 3, total)
	_, total, err = f.repo.ListEvents(ctx, service.SubscriptionSelfResetEventFilter{Email: f.prefix, GroupID: gX, Organization: xunyou, StartDate: today, EndDate: today, Page: 1, PageSize: 50})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	_, total, err = f.repo.ListEvents(ctx, service.SubscriptionSelfResetEventFilter{Email: f.prefix, GroupID: gX, Organization: wsdashi, Page: 1, PageSize: 50})
	require.NoError(t, err)
	require.Zero(t, total)
	_, total, err = f.repo.ListEvents(ctx, service.SubscriptionSelfResetEventFilter{GroupID: gXW, Page: 1, PageSize: 50})
	require.NoError(t, err)
	require.Zero(t, total)

	// The filter options come from the history itself: a soft-deleted group
	// keeps its name, and only a group whose row is gone shows "#id".
	ghost := f.group("ghost")
	require.NoError(t, f.repo.RecordEvent(ctx, &service.SubscriptionSelfResetEvent{SubscriptionID: oO, UserID: uo, GroupID: ghost, Organization: other, QuotaDate: today, UsedCount: 1, DailyLimit: 1, DailyUsageUSDBefore: 1}))
	f.exec(`DELETE FROM groups WHERE id=$1`, ghost)
	f.exec(`UPDATE groups SET deleted_at=NOW() WHERE id=$1`, gW)
	options, err := f.svc.EventGroups(ctx)
	require.NoError(t, err)
	require.True(t, slices.IsSortedFunc(options, func(a, b service.SubscriptionSelfResetEventGroup) int { return int(a.ID - b.ID) }))
	names := map[int64]string{}
	for _, option := range options {
		if f.mine[option.ID] {
			names[option.ID] = option.Name
		}
	}
	require.Equal(t, map[int64]string{gX: f.prefix + "-x", gW: f.prefix + "-w", gO: f.prefix + "-o", ghost: fmt.Sprintf("#%d", ghost)}, names, "groups without history are not options")
	events, total, err = f.repo.ListEvents(ctx, service.SubscriptionSelfResetEventFilter{GroupID: gW, Page: 1, PageSize: 50})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, f.prefix+"-w", events[0].GroupName)
	events, total, err = f.repo.ListEvents(ctx, service.SubscriptionSelfResetEventFilter{GroupID: ghost, Page: 1, PageSize: 50})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Empty(t, events[0].GroupName)
}

// Ticks follow the group's final scope, its type and its deletion.
func TestSubscriptionSelfResetGroupScopeAndTypeChanges(t *testing.T) {
	f := newSelfResetGroupFixture(t)
	xunyou, wsdashi, other := service.OrganizationXunyou, service.OrganizationWsdashi, service.OrganizationOther
	bystander, g := f.group("bystander", xunyou), f.group("scope", xunyou)
	f.setGroups(selfResetGroupChanges{xunyou: {Enable: []int64{bystander, g}}})

	// Saving the same scope again, or widening it, keeps the tick; the added
	// organization is not ticked by that.
	f.setScopes([]int64{g}, xunyou)
	require.Equal(t, xunyou, f.ticks(g))
	f.setScopes([]int64{g}, xunyou, wsdashi)
	require.Equal(t, xunyou, f.ticks(g))
	require.Empty(t, f.allowed(wsdashi, g))
	f.setGroups(selfResetGroupChanges{wsdashi: {Enable: []int64{g}}})
	require.Equal(t, wsdashi+","+xunyou, f.ticks(g))

	// Leaving an organization deletes that tick only; coming back does not restore it.
	f.setScopes([]int64{g}, wsdashi)
	require.Equal(t, wsdashi, f.ticks(g))
	require.Empty(t, f.allowed(xunyou, g))
	f.setScopes([]int64{g}, xunyou, wsdashi)
	require.Equal(t, wsdashi, f.ticks(g))
	require.Empty(t, f.allowed(xunyou, g))
	require.Equal(t, []int64{g}, f.allowed(wsdashi, g))
	require.Equal(t, map[int64]bool{bystander: true, g: false}, f.candidates(f.listGroups(), xunyou))

	// Clearing the scope deletes the remaining ticks. The group is now an
	// "other" candidate; giving it a scope again deletes the "other" tick.
	f.setScopes([]int64{g})
	require.Empty(t, f.ticks(g))
	require.Equal(t, map[int64]bool{g: false}, f.candidates(f.listGroups(), other))
	err := f.saveGroups(f.revision(), selfResetGroupChanges{other: {Enable: []int64{g}}, xunyou: {Enable: []int64{g}}})
	require.ErrorIs(t, err, service.ErrSelfResetGroupConflict)
	require.Empty(t, f.ticks(g))
	f.setGroups(selfResetGroupChanges{other: {Enable: []int64{g}}})
	require.Equal(t, other, f.ticks(g))
	require.Equal(t, []int64{g}, f.allowed(other, g))
	f.setScopes([]int64{g}, xunyou)
	require.Empty(t, f.ticks(g))
	require.Empty(t, f.allowed(other, g))
	require.Empty(t, f.allowed(xunyou, g))
	require.Equal(t, xunyou, f.ticks(bystander), "groups outside the replacement keep their ticks")

	// Admission checks the current scope itself: a tick left behind by a missed
	// cleanup admits nobody.
	f.exec(`INSERT INTO subscription_self_reset_groups(group_id,organization_key) VALUES($1,'other'),($1,'wsdashi')`, g)
	require.Empty(t, f.allowed(other, g))
	require.Empty(t, f.allowed(wsdashi, g))
	f.setScopes([]int64{g}, xunyou)
	require.Empty(t, f.ticks(g), "the next replacement removes stale ticks")

	// Leaving the subscription type deletes every tick (migration 266); other
	// updates do not. Turning back restores neither the scope nor the tick, so
	// both groups come back as unticked "other" candidates.
	scoped, unscoped := f.group("type", xunyou), f.group("type-other")
	f.setGroups(selfResetGroupChanges{xunyou: {Enable: []int64{scoped}}, other: {Enable: []int64{unscoped}}})
	f.exec(`UPDATE groups SET description='renamed', subscription_type='subscription' WHERE id=ANY($1)`, pq.Array([]int64{scoped, unscoped}))
	require.Equal(t, xunyou, f.ticks(scoped))
	require.Equal(t, other, f.ticks(unscoped))
	f.exec(`UPDATE groups SET subscription_type='standard' WHERE id=ANY($1)`, pq.Array([]int64{scoped, unscoped}))
	require.Empty(t, f.ticks(scoped))
	require.Empty(t, f.ticks(unscoped))
	f.exec(`UPDATE groups SET subscription_type='subscription' WHERE id=ANY($1)`, pq.Array([]int64{scoped, unscoped}))
	require.Empty(t, f.ticks(scoped))
	require.Empty(t, f.ticks(unscoped))
	require.Empty(t, f.allowed(xunyou, scoped))
	require.Empty(t, f.allowed(other, scoped, unscoped))
	require.Equal(t, map[int64]bool{scoped: false, unscoped: false}, f.candidates(f.listGroups(), other))

	// A soft-deleted group is no candidate and admits nobody. Its tick row
	// stays for the inspection query; ticking it again conflicts.
	gone := f.group("soft", xunyou)
	f.setGroups(selfResetGroupChanges{xunyou: {Enable: []int64{gone}}})
	require.Equal(t, []int64{gone}, f.allowed(xunyou, gone))
	f.exec(`UPDATE groups SET deleted_at=NOW() WHERE id=$1`, gone)
	require.Empty(t, f.allowed(xunyou, gone))
	require.NotContains(t, f.candidates(f.listGroups(), xunyou), gone)
	require.Equal(t, xunyou, f.ticks(gone))
	require.ErrorIs(t, f.saveGroups(f.revision(), selfResetGroupChanges{xunyou: {Enable: []int64{gone}}}), service.ErrSelfResetGroupConflict)
}

// Saves take groups FOR UPDATE in ID order before touching the ticks, the
// same lock as a scope replacement.
func TestSubscriptionSelfResetGroupConcurrentSaves(t *testing.T) {
	f := newSelfResetGroupFixture(t)
	xunyou := service.OrganizationXunyou
	a, b := f.group("a", xunyou), f.group("b", xunyou)
	f.setGroups(selfResetGroupChanges{xunyou: {Enable: []int64{a}}})

	// Two administrators save opposite diffs. Unserialized, they deadlock on
	// the tick rows or interleave into both or neither group being ticked.
	for round := 0; round < 20; round++ {
		revision := f.revision()
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		for _, change := range []service.SubscriptionSelfResetGroupChanges{{Enable: []int64{b}, Disable: []int64{a}}, {Enable: []int64{a}, Disable: []int64{b}}} {
			wg.Add(1)
			go func(change service.SubscriptionSelfResetGroupChanges) {
				defer wg.Done()
				errs <- f.saveGroups(revision, selfResetGroupChanges{xunyou: change})
			}(change)
		}
		wg.Wait()
		close(errs)
		conflicts := 0
		for err := range errs {
			if err != nil {
				require.ErrorIs(t, err, service.ErrSelfResetRevisionConflict)
				conflicts++
			}
		}
		require.Equal(t, 1, conflicts, "round %d", round)
		require.Len(t, f.allowed(xunyou, a, b), 1, "round %d: the saves must run one after the other", round)
	}

	// A scope replacement holds the lock until it commits. A save started
	// meanwhile waits, then rejects the batch against the committed scope.
	f.setGroups(selfResetGroupChanges{xunyou: {Enable: []int64{b}}})
	tx, err := integrationEntClient.Tx(f.ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	keys := []string{}
	_, err = f.subs.SetGroupOrganizationScopes(dbent.NewTxContext(f.ctx, tx), service.GroupOrganizationScopeInput{GroupIDs: []int64{b}, OrganizationKeys: &keys})
	require.NoError(t, err)
	revision := f.revision()
	done := make(chan error, 1)
	go func() {
		done <- f.saveGroups(revision, selfResetGroupChanges{xunyou: {Enable: []int64{b}}})
	}()
	select {
	case got := <-done:
		t.Fatalf("the save must wait for the scope replacement, got %+v", got)
	case <-time.After(300 * time.Millisecond):
	}
	require.NoError(t, tx.Commit())
	select {
	case got := <-done:
		require.ErrorIs(t, got, service.ErrSelfResetGroupConflict)
	case <-time.After(10 * time.Second):
		t.Fatal("the save did not resume after the scope replacement committed")
	}
	require.Empty(t, f.ticks(b))
	require.Empty(t, f.allowed(xunyou, b))
}
