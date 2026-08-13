package main

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/service/promptmetrics"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type cleanupEntDriverSpy struct {
	closeCalls atomic.Int64
}

func (d *cleanupEntDriverSpy) Exec(context.Context, string, any, any) error  { return nil }
func (d *cleanupEntDriverSpy) Query(context.Context, string, any, any) error { return nil }
func (d *cleanupEntDriverSpy) Tx(context.Context) (dialect.Tx, error) {
	return dialect.NopTx(d), nil
}
func (d *cleanupEntDriverSpy) Close() error {
	d.closeCalls.Add(1)
	return nil
}
func (d *cleanupEntDriverSpy) Dialect() string { return dialect.Postgres }

type cleanupQuotaCacheSpy struct {
	service.BillingCache
	mu     sync.Mutex
	popped bool
}

func (s *cleanupQuotaCacheSpy) PopDirtyUserPlatformQuotaKeys(context.Context, int) ([]service.UserPlatformQuotaKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.popped {
		return nil, nil
	}
	s.popped = true
	return []service.UserPlatformQuotaKey{{UserID: 1, Platform: service.PlatformOpenAI}}, nil
}
func (s *cleanupQuotaCacheSpy) ReaddDirtyUserPlatformQuotaKeys(context.Context, []service.UserPlatformQuotaKey) error {
	return nil
}
func (s *cleanupQuotaCacheSpy) BatchGetUserPlatformQuotaCache(context.Context, []service.UserPlatformQuotaKey) ([]*service.UserPlatformQuotaCacheEntry, error) {
	now := time.Now().UTC()
	return []*service.UserPlatformQuotaCacheEntry{{
		DailyWindowStart: &now, WeeklyWindowStart: &now, MonthlyWindowStart: &now,
	}}, nil
}

type cleanupQuotaWriterSpy struct {
	service.UserPlatformQuotaRepository
	rdb       *redis.Client
	entDriver *cleanupEntDriverSpy
	calls     atomic.Int64
	orderOK   atomic.Bool
}

func (s *cleanupQuotaWriterSpy) BatchSnapshotUsage(ctx context.Context, _ []service.UserPlatformQuotaSnapshot, _ time.Time) error {
	s.calls.Add(1)
	s.orderOK.Store(s.rdb.Ping(ctx).Err() == nil && s.entDriver.closeCalls.Load() == 0)
	return nil
}

func TestProvideServiceBuildInfo(t *testing.T) {
	in := handler.BuildInfo{
		Version:   "v-test",
		BuildType: "release",
	}
	out := provideServiceBuildInfo(in)
	require.Equal(t, in.Version, out.Version)
	require.Equal(t, in.BuildType, out.BuildType)
}

func TestProvideCleanup_WithMinimalDependencies_NoPanic(t *testing.T) {
	cleanup := newMinimalCleanupForTest(nil, nil, nil, nil, nil)

	require.NotPanics(t, func() {
		cleanup()
	})
}

func TestProvideCleanup_LocalTasksFinishBeforeInfrastructureAndCleanupIsIdempotent(t *testing.T) {
	redisServer := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	require.NoError(t, rdb.Ping(context.Background()).Err())

	entDriver := &cleanupEntDriverSpy{}
	entClient := dbent.NewClient(dbent.Driver(entDriver))
	cfg := &config.Config{
		Database: config.DatabaseConfig{UserPlatformQuotaFlusherEnabled: true},
		PromptMetrics: config.PromptMetricsConfig{
			Enabled: true, WorkerCount: 1, QueueSize: 1, WriteTimeoutSeconds: 1,
		},
	}
	quotaWriter := &cleanupQuotaWriterSpy{rdb: rdb, entDriver: entDriver}
	quotaFlusher := service.NewUserPlatformQuotaUsageFlusher(cfg, &cleanupQuotaCacheSpy{}, quotaWriter, nil)
	presetRunner := service.NewUserConcurrencyPresetRunner(service.NewUserConcurrencyPresetService(nil, nil, nil, cfg))
	presetRunner.Start()
	promptMetrics := promptmetrics.NewExtension(cfg, nil)

	cleanup := newMinimalCleanupForTest(
		entClient,
		rdb,
		presetRunner,
		promptMetrics,
		quotaFlusher,
	)

	cleanup()
	cleanup()

	require.Equal(t, int64(1), quotaWriter.calls.Load(), "quota final flush must run once")
	require.True(t, quotaWriter.orderOK.Load(), "quota final flush must complete while Redis and Ent are open")
	require.Error(t, rdb.Ping(context.Background()).Err(), "Redis must be closed after local cleanup")
	require.Equal(t, int64(1), entDriver.closeCalls.Load(), "Ent close must run once")
}

func newMinimalCleanupForTest(
	entClient *dbent.Client,
	rdb *redis.Client,
	presetRunner *service.UserConcurrencyPresetRunner,
	promptMetrics *promptmetrics.Extension,
	quotaFlusher *service.UserPlatformQuotaUsageFlusher,
) func() {
	cfg := &config.Config{}
	oauthSvc := service.NewOAuthService(nil, nil)
	openAIOAuthSvc := service.NewOpenAIOAuthService(nil, nil)
	geminiOAuthSvc := service.NewGeminiOAuthService(nil, nil, nil, nil, cfg)
	antigravityOAuthSvc := service.NewAntigravityOAuthService(nil)
	tokenRefreshSvc := service.NewTokenRefreshService(nil, oauthSvc, openAIOAuthSvc, geminiOAuthSvc, antigravityOAuthSvc, nil, nil, cfg, nil)

	return provideCleanup(
		entClient,
		rdb,
		&service.OpsMetricsCollector{},
		&service.OpsAggregationService{},
		&service.OpsAlertEvaluatorService{},
		&service.OpsCleanupService{},
		&service.OpsScheduledReportService{},
		service.NewOpsSystemLogSink(nil),
		nil,
		nil,
		nil,
		nil,
		service.NewSchedulerSnapshotService(nil, nil, nil, nil, cfg),
		tokenRefreshSvc,
		service.NewAccountExpiryService(nil, time.Second),
		service.NewTokenAnalysisService(nil, cfg, nil),
		service.NewOpenAICodexVersionSyncService(nil, nil, nil, time.Second),
		service.NewProxyExpiryService(nil, time.Second),
		service.NewSubscriptionExpiryService(nil, time.Second),
		&service.UsageCleanupService{},
		service.NewIdempotencyCleanupService(nil, cfg),
		&service.BatchImageCleanupService{},
		nil,
		service.NewPricingService(cfg, nil),
		service.NewEmailQueueService(nil, 1),
		service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil),
		&service.UsageRecordWorkerPool{},
		&service.SubscriptionService{},
		oauthSvc,
		openAIOAuthSvc,
		geminiOAuthSvc,
		antigravityOAuthSvc,
		nil,
		nil,
		nil,
		presetRunner,
		nil,
		nil,
		nil,
		nil,
		promptMetrics,
		quotaFlusher,
		nil,
		nil,
		nil,
		nil,
	)
}
