//go:build integration

package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type selfResetAdmissionQueryCounter struct {
	*sql.DB
	reads int
}

func (c *selfResetAdmissionQueryCounter) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	c.reads++
	return c.DB.QueryContext(ctx, query, args...)
}

func TestSubscriptionSelfResetAdmissionSnapshot(t *testing.T) {
	f := newSelfResetGroupFixture(t)
	ctx := f.ctx
	groupID := f.group("snapshot", service.OrganizationXunyou)
	user, err := integrationEntClient.User.Create().SetEmail(f.prefix + "@xunyou.com").SetPasswordHash("unused").SetStatus(service.StatusActive).Save(ctx)
	require.NoError(t, err)
	now := time.Now()
	today := timezone.StartOfDay(now).Format(time.DateOnly)
	sub, err := integrationEntClient.UserSubscription.Create().SetUserID(user.ID).SetGroupID(groupID).
		SetStatus(service.SubscriptionStatusActive).SetStartsAt(now.AddDate(-1, 0, 0)).SetExpiresAt(now.AddDate(1, 0, 0)).
		SetDailyWindowStart(timezone.StartOfDay(now)).SetDailyUsageUsd(80).Save(ctx)
	require.NoError(t, err)
	previous, err := f.repo.ReadPolicy(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, writeSelfResetFixturePolicy(context.Background(), previous)) })
	cfg, err := f.svc.Config(ctx)
	require.NoError(t, err)
	revision := cfg.Revision
	// Ordinary users are denied in both configurations. Distinct limits let
	// us also detect a mixed status response that happens to deny the reset.
	all := json.RawMessage(`{"rollout":"all","daily_limit_by_organization":{"xunyou":2,"wsdashi":1,"other":1}}`)
	admin := json.RawMessage(`{"rollout":"admin","daily_limit_by_organization":{"xunyou":3,"wsdashi":1,"other":1}}`)
	on := service.SubscriptionSelfResetGroupChanges{Enable: []int64{groupID}}
	off := service.SubscriptionSelfResetGroupChanges{Disable: []int64{groupID}}
	save := func(policy json.RawMessage, change service.SubscriptionSelfResetGroupChanges) error {
		var e error
		revision, e = f.svc.SaveConfig(ctx, service.SubscriptionSelfResetConfigChange{
			ExpectedRevision: revision, Policy: policy, Groups: selfResetGroupChanges{service.OrganizationXunyou: change},
		})
		return e
	}
	coordinator := service.NewIdempotencyCoordinator(NewIdempotencyRepository(integrationEntClient, integrationDB), service.DefaultIdempotencyConfig())
	runReset := func(callCtx context.Context, key string, started chan<- int) error {
		_, e := coordinator.Execute(callCtx, service.IdempotencyExecuteOptions{
			Scope: service.SubscriptionSelfResetIdempotencyScope(user.ID), ActorScope: fmt.Sprint(user.ID), Method: "POST",
			Route: "/:id/reset-daily", IdempotencyKey: key, Payload: []any{sub.ID, today}, RequireKey: true, AtomicSuccess: true, TTL: time.Hour,
		}, func(txCtx context.Context) (any, error) {
			if started != nil {
				var pid int
				if e := scanSingleRow(txCtx, dbent.TxFromContext(txCtx).Client(), "SELECT pg_backend_pid()", nil, &pid); e != nil {
					return nil, e
				}
				started <- pid
			}
			return f.svc.Reset(txCtx, user.ID, sub.ID, service.RoleUser, today)
		})
		return e
	}
	reset := func(key string) error { return runReset(ctx, key, nil) }
	requireState := func(count int, dailyUsage float64) {
		t.Helper()
		usage, e := f.repo.GetUsage(ctx, sub.ID)
		require.NoError(t, e)
		require.Equal(t, count, usage.UsedCount)
		var events int
		require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM subscription_self_reset_events WHERE subscription_id=$1`, sub.ID).Scan(&events))
		require.Equal(t, count, events)
		updated, e := integrationEntClient.UserSubscription.Get(ctx, sub.ID)
		require.NoError(t, e)
		require.Equal(t, dailyUsage, updated.DailyUsageUsd)
	}
	require.NoError(t, save(all, off))
	require.Equal(t, "SELF_RESET_GROUP_NOT_ALLOWED", errors.Reason(reset("before")))

	// Hold the subscription row (not a configuration lock). Observe the real
	// backend waiting before we commit the new configuration on another connection.
	holder, holderPID := departmentMemberConcurrencyTx(t, ctx)
	_, err = holder.ExecContext(ctx, `SELECT id FROM user_subscriptions WHERE id=$1 FOR UPDATE`, sub.ID)
	require.NoError(t, err)
	workerCtx, cancelWorker := context.WithCancel(ctx)
	started := make(chan int, 1)
	done := departmentMemberStartConcurrent(t, cancelWorker, func() { _ = holder.Rollback() }, func() error {
		return runReset(workerCtx, "waiting", started)
	})
	var workerPID int
	select {
	case workerPID = <-started:
	case e := <-done:
		t.Fatalf("reset ended before entering its business transaction: %v", e)
	case <-ctx.Done():
		t.Fatalf("waiting for reset to enter its business transaction: %v", ctx.Err())
	}
	// Query the lock manager on the already connected holder. Its only
	// conflicting lock is the subscription row above; no observer connection
	// or matching the text in pg_stat_activity is required.
	poll := time.NewTicker(20 * time.Millisecond)
	defer poll.Stop()
	for {
		var blocked bool
		require.NoError(t, holder.QueryRowContext(ctx, "SELECT pg_backend_pid() = ANY(pg_blocking_pids($1))", workerPID).Scan(&blocked))
		if blocked {
			t.Logf("observed reset PID %d waiting on subscription %d held by PID %d", workerPID, sub.ID, holderPID)
			break
		}
		select {
		case e := <-done:
			t.Fatalf("reset PID %d ended before waiting on holder %d: %v", workerPID, holderPID, e)
		case <-ctx.Done():
			t.Fatalf("waiting for reset PID %d to block on holder %d: %v", workerPID, holderPID, ctx.Err())
		case <-poll.C:
		}
	}
	require.NoError(t, save(admin, on))
	require.NoError(t, holder.Commit())
	require.Equal(t, "SELF_RESET_ROLLOUT_DISABLED", errors.Reason(<-done))
	requireState(0, 80)

	// Count the SQL statements issued by one repository ReadAdmission call.
	counter := &selfResetAdmissionQueryCounter{DB: integrationDB}
	driver := entsql.NewDriver(dialect.Postgres, entsql.Conn{ExecQuerier: counter})
	counted := NewSubscriptionSelfResetRepository(dbent.NewClient(dbent.Driver(driver)))
	raw, admitted, err := counted.ReadAdmission(ctx, []int64{groupID}, service.OrganizationXunyou)
	require.NoError(t, err)
	require.JSONEq(t, string(admin), raw)
	require.True(t, admitted[groupID])
	require.Equal(t, 1, counter.reads)

	// Race real configuration writes against both public operations. For
	// admins, rollout permits both versions, so the group's tick must pair with
	// the matching limit. For ordinary users, neither version permits a reset.
	for i := range 24 {
		policy, change := all, off
		if i%2 == 1 {
			policy, change = admin, on
		}
		committed := make(chan error, 1)
		go func() { committed <- save(policy, change) }()
		status, statusErr := f.svc.Status(ctx, user.ID, service.RoleAdmin)
		resetErr := reset(fmt.Sprintf("racing-%d", i))
		require.NoError(t, <-committed)
		require.NoError(t, statusErr)
		require.Len(t, status.Subscriptions, 1)
		item := status.Subscriptions[0]
		switch status.DailyLimit {
		case 2:
			require.False(t, item.CanReset)
			require.Equal(t, "GROUP_NOT_ALLOWED", *item.DisabledReason)
		case 3:
			require.True(t, item.CanReset)
			require.Nil(t, item.DisabledReason)
		default:
			t.Fatalf("unexpected admission snapshot: %+v", status)
		}
		require.Contains(t, []string{"SELF_RESET_GROUP_NOT_ALLOWED", "SELF_RESET_ROLLOUT_DISABLED"}, errors.Reason(resetErr))
	}
	requireState(0, 80)
	// Positive control: the very same ordinary user and subscription can reset
	// when one committed configuration genuinely permits it.
	require.NoError(t, save(all, on))
	require.NoError(t, reset("permitted"))
	requireState(1, 0)
}
