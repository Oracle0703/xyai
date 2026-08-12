//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/dgraph-io/ristretto"
	"github.com/stretchr/testify/require"
)

// resetQuotaUserSubRepoStub 支持 GetByID、ResetUsageWindows，
// 其余方法继承 userSubRepoNoop（panic）。
type resetQuotaUserSubRepoStub struct {
	userSubRepoNoop

	sub *UserSubscription

	resetDailyCalled   bool
	resetWeeklyCalled  bool
	resetMonthlyCalled bool
	resetDailyErr      error
	resetWeeklyErr     error
	resetMonthlyErr    error
	dailyStart         time.Time
	periodicStart      time.Time
	filteredFilter     SubscriptionAdminFilter
	filteredNow        time.Time
	filteredDailyStart time.Time
	filteredKeys       []SubscriptionCacheKey
	filteredErr        error
}

func (r *resetQuotaUserSubRepoStub) GetByID(_ context.Context, id int64) (*UserSubscription, error) {
	if r.sub == nil || r.sub.ID != id {
		return nil, ErrSubscriptionNotFound
	}
	cp := *r.sub
	return &cp, nil
}

func (r *resetQuotaUserSubRepoStub) ResetUsageWindows(_ context.Context, _ int64, resetDaily, resetWeekly, resetMonthly bool, dailyStart, periodicStart time.Time) error {
	r.resetDailyCalled = resetDaily
	r.resetWeeklyCalled = resetWeekly
	r.resetMonthlyCalled = resetMonthly
	r.dailyStart = dailyStart
	r.periodicStart = periodicStart
	if resetDaily && r.resetDailyErr != nil {
		return r.resetDailyErr
	}
	if resetWeekly && r.resetWeeklyErr != nil {
		return r.resetWeeklyErr
	}
	if resetMonthly && r.resetMonthlyErr != nil {
		return r.resetMonthlyErr
	}
	if r.sub == nil {
		return nil
	}
	if resetDaily {
		r.sub.DailyUsageUSD = 0
		r.sub.DailyWindowStart = &dailyStart
	}
	if resetWeekly {
		r.sub.WeeklyUsageUSD = 0
		r.sub.WeeklyWindowStart = &periodicStart
	}
	if resetMonthly {
		r.sub.MonthlyUsageUSD = 0
		r.sub.MonthlyWindowStart = &periodicStart
	}
	return nil
}

func (r *resetQuotaUserSubRepoStub) ResetDailyUsage(_ context.Context, _ int64, _ *time.Time, windowStart time.Time) error {
	r.resetDailyCalled = true
	if r.resetDailyErr == nil && r.sub != nil {
		r.sub.DailyUsageUSD = 0
		r.sub.DailyWindowStart = &windowStart
	}
	return r.resetDailyErr
}

func (r *resetQuotaUserSubRepoStub) ResetWeeklyUsage(_ context.Context, _ int64, _ *time.Time, _ time.Time) error {
	r.resetWeeklyCalled = true
	return r.resetWeeklyErr
}

func (r *resetQuotaUserSubRepoStub) ResetMonthlyUsage(_ context.Context, _ int64, _ *time.Time, _ time.Time) error {
	r.resetMonthlyCalled = true
	return r.resetMonthlyErr
}

func (r *resetQuotaUserSubRepoStub) ResetDailyFiltered(_ context.Context, filter SubscriptionAdminFilter, now, dailyStart time.Time) ([]SubscriptionCacheKey, error) {
	r.filteredFilter = filter
	r.filteredNow = now
	r.filteredDailyStart = dailyStart
	return append([]SubscriptionCacheKey(nil), r.filteredKeys...), r.filteredErr
}

type filteredResetBillingCache struct {
	*billingCacheStub
	mu            sync.Mutex
	invalidated   []SubscriptionCacheKey
	published     []string
	invalidateErr error
	publishErr    error
	invalidateFn  func(context.Context, int64, int64) error
	publishFn     func(context.Context, string) error
}

func (c *filteredResetBillingCache) snapshotCalls() ([]SubscriptionCacheKey, []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]SubscriptionCacheKey(nil), c.invalidated...), append([]string(nil), c.published...)
}

func newFilteredResetBillingCache() *filteredResetBillingCache {
	return &filteredResetBillingCache{billingCacheStub: newBillingCacheStub(8)}
}

func (c *filteredResetBillingCache) InvalidateSubscriptionCache(ctx context.Context, userID, groupID int64) error {
	if c.invalidateFn != nil {
		return c.invalidateFn(ctx, userID, groupID)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.invalidated = append(c.invalidated, SubscriptionCacheKey{UserID: userID, GroupID: groupID})
	return c.invalidateErr
}

func (c *filteredResetBillingCache) PublishSubscriptionCacheInvalidation(ctx context.Context, cacheKey string) error {
	if c.publishFn != nil {
		return c.publishFn(ctx, cacheKey)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.published = append(c.published, cacheKey)
	return c.publishErr
}

func (c *filteredResetBillingCache) SubscribeSubscriptionCacheInvalidation(context.Context, func(string)) error {
	return nil
}

func newFilteredResetService(t *testing.T, repo *resetQuotaUserSubRepoStub, cache *filteredResetBillingCache) *SubscriptionService {
	t.Helper()
	svc := NewSubscriptionService(groupRepoNoop{}, repo, &BillingCacheService{cache: cache}, nil, nil)
	l1, err := ristretto.NewCache(&ristretto.Config{NumCounters: 100, MaxCost: 100, BufferItems: 64})
	require.NoError(t, err)
	t.Cleanup(l1.Close)
	svc.subCacheL1 = l1
	return svc
}

func newResetQuotaSvc(stub *resetQuotaUserSubRepoStub) *SubscriptionService {
	return NewSubscriptionService(groupRepoNoop{}, stub, nil, nil, nil)
}

func TestAdminResetQuota_ResetBoth(t *testing.T) {
	stub := &resetQuotaUserSubRepoStub{
		sub: &UserSubscription{ID: 1, UserID: 10, GroupID: 20},
	}
	svc := newResetQuotaSvc(stub)
	resetAt := time.Date(2026, 7, 1, 10, 37, 42, 123, time.UTC)
	svc.now = func() time.Time { return resetAt }

	result, err := svc.AdminResetQuota(context.Background(), 1, true, true, false)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, stub.resetDailyCalled, "应调用 ResetDailyUsage")
	require.True(t, stub.resetWeeklyCalled, "应调用 ResetWeeklyUsage")
	require.False(t, stub.resetMonthlyCalled, "不应调用 ResetMonthlyUsage")
	// 手动重置后日窗口锚定当天 0 点（保持 0 点刷新节奏），周窗口锚定重置时刻。
	require.Equal(t, timezone.StartOfDay(resetAt), stub.dailyStart)
	require.Equal(t, resetAt, stub.periodicStart)
	require.Equal(t, timezone.StartOfDay(resetAt), *result.DailyWindowStart)
	require.Equal(t, resetAt, *result.WeeklyWindowStart)
}

func TestAdminResetQuota_ResetDailyOnly(t *testing.T) {
	stub := &resetQuotaUserSubRepoStub{
		sub: &UserSubscription{ID: 2, UserID: 10, GroupID: 20},
	}
	svc := newResetQuotaSvc(stub)

	result, err := svc.AdminResetQuota(context.Background(), 2, true, false, false)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, stub.resetDailyCalled, "应调用 ResetDailyUsage")
	require.False(t, stub.resetWeeklyCalled, "不应调用 ResetWeeklyUsage")
	require.False(t, stub.resetMonthlyCalled, "不应调用 ResetMonthlyUsage")
}

func TestAdminResetQuota_ResetWeeklyOnly(t *testing.T) {
	stub := &resetQuotaUserSubRepoStub{
		sub: &UserSubscription{ID: 3, UserID: 10, GroupID: 20},
	}
	svc := newResetQuotaSvc(stub)

	result, err := svc.AdminResetQuota(context.Background(), 3, false, true, false)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, stub.resetDailyCalled, "不应调用 ResetDailyUsage")
	require.True(t, stub.resetWeeklyCalled, "应调用 ResetWeeklyUsage")
	require.False(t, stub.resetMonthlyCalled, "不应调用 ResetMonthlyUsage")
}

func TestAdminResetQuota_BothFalseReturnsError(t *testing.T) {
	stub := &resetQuotaUserSubRepoStub{
		sub: &UserSubscription{ID: 7, UserID: 10, GroupID: 20},
	}
	svc := newResetQuotaSvc(stub)

	_, err := svc.AdminResetQuota(context.Background(), 7, false, false, false)

	require.ErrorIs(t, err, ErrInvalidInput)
	require.False(t, stub.resetDailyCalled)
	require.False(t, stub.resetWeeklyCalled)
	require.False(t, stub.resetMonthlyCalled)
}

func TestAdminResetQuota_SubscriptionNotFound(t *testing.T) {
	stub := &resetQuotaUserSubRepoStub{sub: nil}
	svc := newResetQuotaSvc(stub)

	_, err := svc.AdminResetQuota(context.Background(), 999, true, true, true)

	require.ErrorIs(t, err, ErrSubscriptionNotFound)
	require.False(t, stub.resetDailyCalled)
	require.False(t, stub.resetWeeklyCalled)
	require.False(t, stub.resetMonthlyCalled)
}

func TestAdminResetQuota_ResetDailyUsageError(t *testing.T) {
	dbErr := errors.New("db error")
	stub := &resetQuotaUserSubRepoStub{
		sub:           &UserSubscription{ID: 4, UserID: 10, GroupID: 20},
		resetDailyErr: dbErr,
	}
	svc := newResetQuotaSvc(stub)

	_, err := svc.AdminResetQuota(context.Background(), 4, true, true, false)

	require.ErrorIs(t, err, dbErr)
	require.True(t, stub.resetDailyCalled)
	require.True(t, stub.resetWeeklyCalled, "原子重置应在一次调用中提交所选窗口")
}

func TestAdminResetQuota_ResetWeeklyUsageError(t *testing.T) {
	dbErr := errors.New("db error")
	stub := &resetQuotaUserSubRepoStub{
		sub:            &UserSubscription{ID: 5, UserID: 10, GroupID: 20},
		resetWeeklyErr: dbErr,
	}
	svc := newResetQuotaSvc(stub)

	_, err := svc.AdminResetQuota(context.Background(), 5, false, true, false)

	require.ErrorIs(t, err, dbErr)
	require.True(t, stub.resetWeeklyCalled)
}

func TestAdminResetQuota_ResetMonthlyOnly(t *testing.T) {
	stub := &resetQuotaUserSubRepoStub{
		sub: &UserSubscription{ID: 8, UserID: 10, GroupID: 20},
	}
	svc := newResetQuotaSvc(stub)

	result, err := svc.AdminResetQuota(context.Background(), 8, false, false, true)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, stub.resetDailyCalled, "不应调用 ResetDailyUsage")
	require.False(t, stub.resetWeeklyCalled, "不应调用 ResetWeeklyUsage")
	require.True(t, stub.resetMonthlyCalled, "应调用 ResetMonthlyUsage")
}

func TestAdminResetQuota_BeforeStartsAtSameDayPreservesAutomaticBoundary(t *testing.T) {
	startsAt := time.Date(2026, 7, 1, 15, 0, 0, 0, time.UTC)
	resetAt := time.Date(2026, 7, 1, 10, 37, 42, 123, time.UTC)
	stub := &resetQuotaUserSubRepoStub{
		sub: &UserSubscription{
			ID:        10,
			UserID:    10,
			GroupID:   20,
			StartsAt:  startsAt,
			ExpiresAt: startsAt.Add(45 * 24 * time.Hour),
		},
	}
	svc := newResetQuotaSvc(stub)
	svc.now = func() time.Time { return resetAt }

	result, err := svc.AdminResetQuota(context.Background(), 10, false, false, true)

	require.NoError(t, err)
	require.Equal(t, resetAt, *result.MonthlyWindowStart)
	boundary, ok := result.automaticWindowStartAt(result.MonthlyWindowStart, 30*24*time.Hour, resetAt.Add(30*24*time.Hour))
	require.True(t, ok)
	require.Equal(t, resetAt.Add(30*24*time.Hour), boundary)
}

func TestAdminResetQuota_ResetMonthlyUsageError(t *testing.T) {
	dbErr := errors.New("db error")
	stub := &resetQuotaUserSubRepoStub{
		sub:             &UserSubscription{ID: 9, UserID: 10, GroupID: 20},
		resetMonthlyErr: dbErr,
	}
	svc := newResetQuotaSvc(stub)

	_, err := svc.AdminResetQuota(context.Background(), 9, false, false, true)

	require.ErrorIs(t, err, dbErr)
	require.True(t, stub.resetMonthlyCalled)
}

func TestAdminResetQuota_ReturnsRefreshedSub(t *testing.T) {
	stub := &resetQuotaUserSubRepoStub{
		sub: &UserSubscription{
			ID:            6,
			UserID:        10,
			GroupID:       20,
			DailyUsageUSD: 99.9,
		},
	}

	svc := newResetQuotaSvc(stub)
	result, err := svc.AdminResetQuota(context.Background(), 6, true, false, false)

	require.NoError(t, err)
	// ResetUsageWindows stub 会将 sub.DailyUsageUSD 归零，
	// 服务应返回第二次 GetByID 的刷新值而非初始的 99.9
	require.Equal(t, float64(0), result.DailyUsageUSD, "返回的订阅应反映已归零的用量")
	require.True(t, stub.resetDailyCalled)
}

func TestAdminResetDailyFiltered_NormalizesFilterAndInvalidatesEveryCache(t *testing.T) {
	resetAt := time.Date(2026, 8, 12, 15, 47, 0, 0, time.UTC)
	keys := []SubscriptionCacheKey{{UserID: 10, GroupID: 20}, {UserID: 11, GroupID: 21}}
	repo := &resetQuotaUserSubRepoStub{filteredKeys: keys}
	cache := newFilteredResetBillingCache()
	svc := newFilteredResetService(t, repo, cache)
	svc.now = func() time.Time { return resetAt }

	for _, key := range keys {
		require.True(t, svc.subCacheL1.Set(subCacheKey(key.UserID, key.GroupID), "cached", 1))
	}
	svc.subCacheL1.Wait()

	count, err := svc.AdminResetDailyFiltered(context.Background(), SubscriptionAdminFilter{
		Status:       " ACTIVE ",
		Platform:     " OPENAI ",
		Organization: " XUNYOU ",
	})

	require.NoError(t, err)
	require.Equal(t, 2, count)
	require.Equal(t, SubscriptionStatusActive, repo.filteredFilter.Status)
	require.Equal(t, PlatformOpenAI, repo.filteredFilter.Platform)
	require.Equal(t, SubscriptionOrganizationXunyou, repo.filteredFilter.Organization)
	require.Equal(t, resetAt, repo.filteredNow)
	require.Equal(t, timezone.StartOfDay(resetAt), repo.filteredDailyStart)
	invalidated, published := cache.snapshotCalls()
	require.ElementsMatch(t, keys, invalidated)
	require.ElementsMatch(t, []string{subCacheKey(10, 20), subCacheKey(11, 21)}, published)
	for _, key := range keys {
		_, found := svc.subCacheL1.Get(subCacheKey(key.UserID, key.GroupID))
		require.False(t, found)
	}
}

func TestAdminResetDailyFiltered_ZeroMatchSkipsCacheInvalidation(t *testing.T) {
	repo := &resetQuotaUserSubRepoStub{}
	cache := newFilteredResetBillingCache()
	svc := newFilteredResetService(t, repo, cache)

	count, err := svc.AdminResetDailyFiltered(context.Background(), SubscriptionAdminFilter{})

	require.NoError(t, err)
	require.Zero(t, count)
	invalidated, published := cache.snapshotCalls()
	require.Empty(t, invalidated)
	require.Empty(t, published)
}

func TestAdminResetDailyFiltered_RepositoryErrorSkipsCacheInvalidation(t *testing.T) {
	repoErr := errors.New("reset failed")
	repo := &resetQuotaUserSubRepoStub{filteredErr: repoErr}
	cache := newFilteredResetBillingCache()
	svc := newFilteredResetService(t, repo, cache)

	count, err := svc.AdminResetDailyFiltered(context.Background(), SubscriptionAdminFilter{})

	require.ErrorIs(t, err, repoErr)
	require.Zero(t, count)
	invalidated, published := cache.snapshotCalls()
	require.Empty(t, invalidated)
	require.Empty(t, published)
}

func TestAdminResetDailyFiltered_CacheErrorsDoNotHideCommittedReset(t *testing.T) {
	repo := &resetQuotaUserSubRepoStub{filteredKeys: []SubscriptionCacheKey{{UserID: 10, GroupID: 20}}}
	cache := newFilteredResetBillingCache()
	cache.invalidateErr = errors.New("cache unavailable")
	cache.publishErr = errors.New("publish unavailable")
	svc := newFilteredResetService(t, repo, cache)

	count, err := svc.AdminResetDailyFiltered(context.Background(), SubscriptionAdminFilter{})

	require.NoError(t, err)
	require.Equal(t, 1, count)
	invalidated, published := cache.snapshotCalls()
	require.Equal(t, []SubscriptionCacheKey{{UserID: 10, GroupID: 20}}, invalidated)
	require.Equal(t, []string{subCacheKey(10, 20)}, published)
}

func TestFilteredDailyResetCacheInvalidationPublishesAfterDeleteTimeoutWithFreshContext(t *testing.T) {
	cache := newFilteredResetBillingCache()
	invalidateDone := make(chan struct{})
	publishStarted := make(chan error, 1)
	cache.invalidateFn = func(ctx context.Context, _, _ int64) error {
		<-ctx.Done()
		close(invalidateDone)
		return ctx.Err()
	}
	cache.publishFn = func(ctx context.Context, _ string) error {
		select {
		case <-invalidateDone:
		default:
			publishStarted <- errors.New("publish started before invalidation phase finished")
			return nil
		}
		publishStarted <- ctx.Err()
		return nil
	}
	svc := newFilteredResetService(t, &resetQuotaUserSubRepoStub{}, cache)
	svc.invalidateFilteredDailyResetCachesWithTimeout(
		[]SubscriptionCacheKey{{UserID: 10, GroupID: 20}},
		100*time.Millisecond,
	)

	require.NoError(t, <-publishStarted)
}

func TestFilteredDailyResetCacheInvalidationUsesSingleTotalBudget(t *testing.T) {
	cache := newFilteredResetBillingCache()
	cache.invalidateFn = func(ctx context.Context, _, _ int64) error {
		<-ctx.Done()
		return ctx.Err()
	}
	cache.publishFn = func(ctx context.Context, _ string) error {
		<-ctx.Done()
		return ctx.Err()
	}
	svc := newFilteredResetService(t, &resetQuotaUserSubRepoStub{}, cache)
	keys := make([]SubscriptionCacheKey, 32)
	for i := range keys {
		keys[i] = SubscriptionCacheKey{UserID: int64(i + 1), GroupID: 20}
	}

	started := time.Now()
	svc.invalidateFilteredDailyResetCachesWithTimeout(keys, 50*time.Millisecond)

	require.Less(t, time.Since(started), 500*time.Millisecond)
}

func TestFilteredDailyResetCacheInvalidationWarnsWhenBudgetSkipsTasks(t *testing.T) {
	cache := newFilteredResetBillingCache()
	cache.invalidateFn = func(ctx context.Context, _, _ int64) error {
		<-ctx.Done()
		return ctx.Err()
	}
	svc := newFilteredResetService(t, &resetQuotaUserSubRepoStub{}, cache)
	keys := make([]SubscriptionCacheKey, 32)
	for i := range keys {
		keys[i] = SubscriptionCacheKey{UserID: int64(i + 1), GroupID: 20}
	}

	var logs bytes.Buffer
	previousOutput := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(previousOutput) })

	svc.invalidateFilteredDailyResetCachesWithTimeout(keys, 50*time.Millisecond)

	require.Contains(t, logs.String(), "skipped cache invalidation tasks after filtered daily reset")
	require.Contains(t, logs.String(), "phase=invalidate subscription cache")
}
