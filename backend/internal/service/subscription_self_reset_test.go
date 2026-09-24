package service

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionSelfResetPolicyValidation(t *testing.T) {
	for _, raw := range []string{`null`, `{}`, `{"daily_limit_by_organization":{"xunyou":1,"wsdashi":null,"other":1}}`, `{"daily_limit_by_organization":{"xunyou":1.5,"wsdashi":1,"other":1}}`, `{"daily_limit_by_organization":{"xunyou":-1,"wsdashi":1,"other":1}}`, `{"daily_limit_by_organization":{"xunyou":101,"wsdashi":1,"other":1}}`, `{"daily_limit_by_organization":{"xunyou":1,"wsdashi":1,"other":1,"unknown":1}}`, `{"daily_limit_by_organization":{"xunyou":1,"wsdashi":1,"other":1},"extra":true}`, `{"daily_limit_by_organization":{"xunyou":1,"wsdashi":1,"other":1}} {}`} {
		_, err := ParseSubscriptionSelfResetPolicy([]byte(raw))
		require.Error(t, err, raw)
	}
	policy, err := ParseSubscriptionSelfResetPolicy([]byte(`{"daily_limit_by_organization":{"xunyou":0,"wsdashi":100,"other":1}}`))
	require.NoError(t, err)
	require.Equal(t, 0, policy.DailyLimitByOrganization[OrganizationXunyou])
	require.Equal(t, SubscriptionSelfResetRolloutAdmin, policy.Rollout)
	for _, rollout := range []string{SubscriptionSelfResetRolloutOff, SubscriptionSelfResetRolloutAdmin, SubscriptionSelfResetRolloutAll} {
		policy, err := ParseSubscriptionSelfResetPolicy([]byte(`{"rollout":"` + rollout + `","daily_limit_by_organization":{"xunyou":1,"wsdashi":1,"other":1}}`))
		require.NoError(t, err)
		require.Equal(t, rollout, policy.Rollout)
	}
	_, err = ParseSubscriptionSelfResetPolicy([]byte(`{"rollout":"user","daily_limit_by_organization":{"xunyou":1,"wsdashi":1,"other":1}}`))
	require.Error(t, err)
}

func TestSubscriptionSelfResetRollout(t *testing.T) {
	admin := &SubscriptionSelfResetPolicy{Rollout: SubscriptionSelfResetRolloutAdmin}
	require.True(t, selfResetRolloutAllowed(admin, RoleAdmin))
	require.False(t, selfResetRolloutAllowed(admin, RoleUser))
	require.False(t, selfResetRolloutAllowed(&SubscriptionSelfResetPolicy{Rollout: SubscriptionSelfResetRolloutOff}, RoleAdmin))
	require.True(t, selfResetRolloutAllowed(&SubscriptionSelfResetPolicy{Rollout: SubscriptionSelfResetRolloutAll}, RoleUser))
}

func TestSubscriptionSelfResetEligibility(t *testing.T) {
	previous := timezone.Name()
	require.NoError(t, timezone.Init("Asia/Shanghai"))
	t.Cleanup(func() { require.NoError(t, timezone.Init(previous)) })
	now := time.Date(2026, 9, 23, 1, 0, 0, 0, timezone.Location())
	start := timezone.StartOfDay(now)
	limit := 100.0
	base := UserSubscription{ID: 31, UserID: 1, GroupID: 2, StartsAt: now.AddDate(-10, 0, 0), ExpiresAt: now.Add(time.Hour), Status: SubscriptionStatusActive, DailyUsageUSD: 80, DailyWindowStart: &start, Group: &Group{Status: StatusActive, DailyLimitUSD: &limit}}
	for _, tc := range []struct {
		name      string
		change    func(*UserSubscription)
		count     SubscriptionSelfResetUsage
		limit     int
		reason    string
		remaining int
	}{
		{"ten year subscription in final hour", nil, SubscriptionSelfResetUsage{}, 1, "", 1},
		{"used today", nil, SubscriptionSelfResetUsage{"2026-09-23", 1}, 1, "DAILY_LIMIT_REACHED", 0},
		{"yesterday count", nil, SubscriptionSelfResetUsage{"2026-09-22", 1}, 1, "", 1},
		{"policy raised", nil, SubscriptionSelfResetUsage{"2026-09-23", 1}, 3, "", 2},
		{"policy lowered", nil, SubscriptionSelfResetUsage{"2026-09-23", 2}, 1, "DAILY_LIMIT_REACHED", 0},
		{"disabled", nil, SubscriptionSelfResetUsage{}, 0, "POLICY_DISABLED", 0},
		{"no usage", func(s *UserSubscription) { s.DailyUsageUSD = 0 }, SubscriptionSelfResetUsage{}, 1, "NO_USAGE", 1},
		{"old window", func(s *UserSubscription) { old := start.AddDate(0, 0, -1); s.DailyWindowStart = &old }, SubscriptionSelfResetUsage{}, 1, "NO_USAGE", 1},
		{"one time", func(s *UserSubscription) { s.StartsAt = now.Add(-time.Hour) }, SubscriptionSelfResetUsage{}, 1, "ONE_TIME_QUOTA", 1},
		{"expired", func(s *UserSubscription) { s.ExpiresAt = now }, SubscriptionSelfResetUsage{}, 1, "SUBSCRIPTION_INACTIVE", 1},
		{"not started", func(s *UserSubscription) { s.StartsAt = now.Add(time.Minute) }, SubscriptionSelfResetUsage{}, 1, "SUBSCRIPTION_INACTIVE", 1},
		{"suspended", func(s *UserSubscription) { s.Status = SubscriptionStatusSuspended }, SubscriptionSelfResetUsage{}, 1, "SUBSCRIPTION_INACTIVE", 1},
		{"disabled group", func(s *UserSubscription) { s.Group = &Group{Status: "disabled", DailyLimitUSD: &limit} }, SubscriptionSelfResetUsage{}, 1, "GROUP_DISABLED", 1},
		{"no daily limit", func(s *UserSubscription) { s.Group = &Group{Status: StatusActive} }, SubscriptionSelfResetUsage{}, 1, "NO_DAILY_LIMIT", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sub := base
			if tc.change != nil {
				tc.change(&sub)
			}
			item := selfResetItem(&sub, tc.count, tc.limit, now)
			require.Equal(t, tc.remaining, item.RemainingCount)
			require.Equal(t, tc.reason == "", item.CanReset)
			if tc.reason != "" {
				require.Equal(t, tc.reason, *item.DisabledReason)
			} else {
				require.Nil(t, item.DisabledReason)
			}
		})
	}
}

type selfResetRolloutRepo struct {
	SubscriptionSelfResetRepository
	subs   []UserSubscription
	locked bool
}

func (r *selfResetRolloutRepo) ReadPolicy(context.Context) (string, error) {
	return `{"rollout":"admin","daily_limit_by_organization":{"xunyou":1,"wsdashi":1,"other":1}}`, nil
}

func (r *selfResetRolloutRepo) ListOwned(context.Context, int64) ([]UserSubscription, map[int64]SubscriptionSelfResetUsage, string, error) {
	return r.subs, nil, "user@xunyou.com", nil
}

func (r *selfResetRolloutRepo) GetOwnedByIDForUpdate(context.Context, int64, int64) (*UserSubscription, error) {
	r.locked = true
	return nil, ErrSubscriptionNotFound
}

func TestSubscriptionSelfResetRolloutGatesStatusAndReset(t *testing.T) {
	now := time.Now()
	start := timezone.StartOfDay(now)
	limit := 100.0
	sub := UserSubscription{ID: 31, StartsAt: now.AddDate(-1, 0, 0), ExpiresAt: now.AddDate(1, 0, 0), Status: SubscriptionStatusActive, DailyUsageUSD: 80, DailyWindowStart: &start, Group: &Group{Status: StatusActive, DailyLimitUSD: &limit}}
	repo := &selfResetRolloutRepo{subs: []UserSubscription{sub}}
	svc := NewSubscriptionSelfResetService(repo, nil)

	status, err := svc.Status(context.Background(), 1, RoleUser)
	require.NoError(t, err)
	require.False(t, status.Subscriptions[0].CanReset)
	require.Equal(t, "ROLLOUT_DISABLED", *status.Subscriptions[0].DisabledReason)
	status, err = svc.Status(context.Background(), 1, RoleAdmin)
	require.NoError(t, err)
	require.True(t, status.Subscriptions[0].CanReset)

	// A user outside the rollout is rejected before the subscription is locked or counted.
	_, err = svc.Reset(dbent.NewTxContext(context.Background(), &dbent.Tx{}), 1, 31, RoleUser, start.Format(time.DateOnly))
	require.Equal(t, "SELF_RESET_ROLLOUT_DISABLED", infraerrors.Reason(err))
	require.False(t, repo.locked)
}

func TestSubscriptionSelfResetRequiresTransaction(t *testing.T) {
	svc := NewSubscriptionSelfResetService(nil, nil)
	_, err := svc.Reset(context.Background(), 1, 2, RoleAdmin, "2026-09-23")
	require.ErrorIs(t, err, ErrIdempotencyStoreUnavail)
}

func TestSubscriptionSelfResetDayUsesConfiguredTimezone(t *testing.T) {
	previous := timezone.Name()
	t.Cleanup(func() { require.NoError(t, timezone.Init(previous)) })
	for _, tc := range []struct {
		zone, stamp, date string
		hours             float64
	}{
		{"UTC", "2026-09-22T16:30:00Z", "2026-09-22", 24},
		{"Asia/Shanghai", "2026-09-22T16:30:00Z", "2026-09-23", 24},
		{"America/New_York", "2026-03-08T07:30:00Z", "2026-03-08", 23},
		{"America/New_York", "2026-11-01T07:30:00Z", "2026-11-01", 25},
	} {
		require.NoError(t, timezone.Init(tc.zone))
		now, err := time.Parse(time.RFC3339, tc.stamp)
		require.NoError(t, err)
		start := timezone.StartOfDay(now)
		require.Equal(t, tc.date, start.Format(time.DateOnly))
		require.Equal(t, tc.hours, start.AddDate(0, 0, 1).Sub(start).Hours())
	}
}
