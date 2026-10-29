//go:build integration

package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// Only Claim is narrowed to this fixture's random key hash. Lease expiry,
// SKIP LOCKED, ordering and ownership are the production Claim semantics;
// scheduling, retries, acknowledgements and stats use the real repository.
// A shared integration database must never have its unrelated outbox consumed.
type departmentMemberScopedOutbox struct {
	service.AuthCacheInvalidationOutboxRepository
	db       *sql.DB
	cacheKey string
}

func (r *departmentMemberScopedOutbox) Claim(ctx context.Context, workerID string, limit int, lease time.Duration) ([]service.AuthCacheInvalidationEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	leaseSeconds := int64(lease / time.Second)
	if leaseSeconds < 1 {
		leaseSeconds = 30
	}
	rows, err := r.db.QueryContext(ctx, `
WITH candidates AS (
    SELECT id
    FROM auth_cache_invalidation_outbox
    WHERE available_at <= NOW()
      AND (claimed_at IS NULL OR claimed_at < NOW() - ($3 * INTERVAL '1 second'))
      AND cache_key = $4
    ORDER BY id ASC
    LIMIT $2
    FOR UPDATE SKIP LOCKED
)
UPDATE auth_cache_invalidation_outbox AS o
SET claimed_at = NOW(), claimed_by = $1
FROM candidates AS c
WHERE o.id = c.id
RETURNING o.id, o.cache_key, o.attempts, o.delivery_stage, o.created_at`, workerID, limit, leaseSeconds, r.cacheKey)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	events := make([]service.AuthCacheInvalidationEvent, 0, limit)
	for rows.Next() {
		var event service.AuthCacheInvalidationEvent
		if err := rows.Scan(&event.ID, &event.CacheKey, &event.Attempts, &event.Stage, &event.CreatedAt); err != nil {
			return nil, err
		}
		event.CacheKey = strings.TrimSpace(event.CacheKey)
		events = append(events, event)
	}
	return events, rows.Err()
}

type departmentMemberOutboxCacheReadCounter struct {
	service.APIKeyCache
	gets atomic.Int64
}

func (c *departmentMemberOutboxCacheReadCounter) GetAuthCache(ctx context.Context, key string) (*service.APIKeyAuthCacheEntry, error) {
	c.gets.Add(1)
	return c.APIKeyCache.GetAuthCache(ctx, key)
}

func TestDepartmentMemberAuthIntegration_OutboxConvergesRemoteAPIKeyCaches(t *testing.T) {
	s := setupDepartmentMembers(t)
	f := s.f
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	t.Cleanup(cancel)
	member := f.user("outbox_remote", "xunyou.com", service.RoleUser, nil)
	s.place(member, s.granted)
	keys, hashes := createDepartmentMemberAuthKeys(t, f, member, 1)
	credential, cacheKey := keys[0].Key, hashes[0]
	_, err := integrationDB.ExecContext(ctx, `DELETE FROM auth_cache_invalidation_outbox WHERE cache_key=$1`, cacheKey)
	require.NoError(t, err, "remove only this fixture's setup invalidations before the tested status transitions")

	// This due event precedes both tested changes. An accidentally unscoped
	// worker would claim it; it must remain untouched throughout both passes.
	sentinelSum := sha256.Sum256([]byte(uuid.NewString()))
	sentinelHash := hex.EncodeToString(sentinelSum[:])
	var sentinelID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`INSERT INTO auth_cache_invalidation_outbox(cache_key) VALUES($1) RETURNING id`, sentinelHash).Scan(&sentinelID))
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, deleteErr := integrationDB.ExecContext(cleanupCtx, `DELETE FROM auth_cache_invalidation_outbox WHERE id=$1 AND cache_key=$2`, sentinelID, sentinelHash)
		require.NoError(t, deleteErr)
	})

	// miniredis exercises real go-redis TCP commands and Pub/Sub. It is not a
	// real Redis deployment, outage/failover test or production latency proof.
	server := miniredis.RunT(t)
	publisherClient := redis.NewClient(&redis.Options{Addr: server.Addr()})
	remoteClient := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { require.NoError(t, publisherClient.Close()) })
	t.Cleanup(func() { require.NoError(t, remoteClient.Close()) })
	publisherCache := NewAPIKeyCache(publisherClient)
	remoteCache := &departmentMemberOutboxCacheReadCounter{APIKeyCache: NewAPIKeyCache(remoteClient)}
	countingRepo := &departmentMemberAuthCountingRepository{APIKeyRepository: NewAPIKeyRepository(integrationEntClient, integrationDB)}
	cfg := &config.Config{
		RunMode: config.RunModeSimple,
		APIKeyAuth: config.APIKeyAuthCacheConfig{
			L1Size: 128, L1TTLSeconds: 600, L2TTLSeconds: 600,
		},
	}
	remote := service.NewAPIKeyService(countingRepo, NewUserRepository(integrationEntClient, integrationDB), nil, nil, nil, remoteCache, cfg)
	remote.StartAuthCacheInvalidationSubscriber(ctx)
	t.Cleanup(remote.StopAuthCacheInvalidationSubscriber)
	require.Eventually(t, func() bool { return remote.AuthCacheInvalidationSubscriberHealth().Connected }, 5*time.Second, 10*time.Millisecond)

	router := gin.New()
	router.Use(gin.HandlerFunc(middleware.NewAPIKeyAuthMiddleware(remote, nil, cfg)))
	router.GET("/protected", func(c *gin.Context) { c.Status(http.StatusOK) })
	request := func() *httptest.ResponseRecorder {
		requestCtx, requestCancel := context.WithTimeout(ctx, 2*time.Second)
		defer requestCancel()
		req := httptest.NewRequest(http.MethodGet, "/protected", nil).WithContext(requestCtx)
		req.Header.Set("Authorization", "Bearer "+credential)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	awaitL1 := func(status int, reason string) {
		t.Helper()
		// No PG lookup and no Redis GET proves a hit in this distinct service's
		// L1, rather than merely a successful response from shared L2.
		require.Eventually(t, func() bool {
			beforePG, beforeRedis := countingRepo.lookups.Load(), remoteCache.gets.Load()
			response := request()
			return response.Code == status && strings.Contains(response.Body.String(), reason) &&
				countingRepo.lookups.Load() == beforePG && remoteCache.gets.Load() == beforeRedis
		}, 5*time.Second, 10*time.Millisecond)
	}
	assertL2 := func(status string) {
		t.Helper()
		entry, cacheErr := remoteCache.APIKeyCache.GetAuthCache(ctx, cacheKey)
		require.NoError(t, cacheErr)
		require.NotNil(t, entry)
		require.NotNil(t, entry.Snapshot)
		require.Equal(t, status, entry.Snapshot.User.Status)
	}
	newWorker := func() *service.AuthCacheInvalidationWorker {
		repo := &departmentMemberScopedOutbox{
			AuthCacheInvalidationOutboxRepository: NewAuthCacheInvalidationOutboxRepository(integrationDB),
			db:                                    integrationDB, cacheKey: cacheKey,
		}
		// Deliberately no local APIKeyService argument: the remote L1 can only
		// be invalidated by the production subscriber receiving the publication.
		worker := service.NewAuthCacheInvalidationWorker(repo, publisherCache)
		t.Cleanup(worker.Stop)
		return worker
	}
	awaitHTTP := func(status int, reason string, beforeLookup int64) {
		t.Helper()
		require.Eventually(t, func() bool {
			response := request()
			return response.Code == status && strings.Contains(response.Body.String(), reason) && countingRepo.lookups.Load() > beforeLookup
		}, 5*time.Second, 10*time.Millisecond, "outbox publication must evict both L2 and the remote L1 so authentication reloads the committed PG state")
	}

	awaitL1(http.StatusOK, "")
	assertL2(service.StatusActive)
	beforeDisable := countingRepo.lookups.Load()
	// The fixture's DepartmentService has a nil active invalidator. No direct
	// InvalidateAuthCacheByUserID or manual Redis deletion is executed here.
	var membershipVersion int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT department_version FROM users WHERE id=$1`, member).Scan(&membershipVersion))
	in := service.ManagedMemberUpdateInput{ExpectedDepartmentID: &s.granted, ExpectedDepartmentVersion: &membershipVersion}
	leaderCtx := service.WithDepartmentActor(ctx, s.leaderID)
	in.Status = departmentMemberString(service.StatusDisabled)
	_, err = s.svc.UpdateManagedMember(leaderCtx, member, in)
	require.NoError(t, err)
	f.requireStatus(member, service.StatusDisabled, "department_manager")
	response := request()
	require.Equal(t, http.StatusOK, response.Code, "before the worker starts, the warmed remote cache must still contain the old active state")
	require.Equal(t, beforeDisable, countingRepo.lookups.Load())
	assertL2(service.StatusActive)

	disableWorker := newWorker()
	disableWorker.Start()
	awaitHTTP(http.StatusUnauthorized, "USER_INACTIVE", beforeDisable)
	require.Eventually(t, func() bool {
		var deliveryStage int
		queryErr := integrationDB.QueryRowContext(ctx,
			`SELECT delivery_stage FROM auth_cache_invalidation_outbox WHERE cache_key=$1`, cacheKey).Scan(&deliveryStage)
		return queryErr == nil && deliveryStage == 1
	}, 5*time.Second, 10*time.Millisecond, "let the first publication finish before stopping its worker")
	disableWorker.Stop()
	awaitL1(http.StatusUnauthorized, "USER_INACTIVE")
	assertL2(service.StatusDisabled)
	var stage int
	var availableAt, createdAt time.Time
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT delivery_stage,available_at,created_at FROM auth_cache_invalidation_outbox WHERE cache_key=$1`, cacheKey).
		Scan(&stage, &availableAt, &createdAt))
	require.Equal(t, 1, stage, "the real worker must schedule its safety pass, not immediately acknowledge the first delivery")
	require.GreaterOrEqual(t, availableAt.Sub(createdAt), 30*time.Second)

	beforeEnable := countingRepo.lookups.Load()
	in.Status = departmentMemberString(service.StatusActive)
	_, err = s.svc.UpdateManagedMember(leaderCtx, member, in)
	require.NoError(t, err)
	response = request()
	require.Equal(t, http.StatusUnauthorized, response.Code, "without active invalidation the remote disabled snapshot remains until outbox processing")
	require.Contains(t, response.Body.String(), "USER_INACTIVE")
	require.Equal(t, beforeEnable, countingRepo.lookups.Load())
	assertL2(service.StatusDisabled)

	enableWorker := newWorker()
	enableWorker.Start()
	awaitHTTP(http.StatusOK, "", beforeEnable)
	assertL2(service.StatusActive)
	// Do not advance Redis time, change available_at, or shorten production's
	// 30-second delay. Observe both durable events complete their safety pass.
	require.Eventually(t, func() bool {
		var remaining int
		queryErr := integrationDB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM auth_cache_invalidation_outbox WHERE cache_key=$1`, cacheKey).Scan(&remaining)
		return queryErr == nil && remaining == 0
	}, 40*time.Second, 100*time.Millisecond, "both status changes must complete production first and safety deliveries")
	enableWorker.Stop()
	disableHealth, enableHealth := disableWorker.Health(ctx), enableWorker.Health(ctx)
	require.EqualValues(t, 4, disableHealth.Processed+enableHealth.Processed)
	require.Zero(t, disableHealth.Failures+enableHealth.Failures)
	require.Empty(t, disableHealth.StatsError)
	require.Empty(t, enableHealth.StatsError)
	require.Zero(t, countingRepo.userKeyLists.Load(), "this test must never invoke the active user-key invalidation path")
	// Durable acknowledgement does not imply that the remote subscriber has
	// handled the final publication yet. Let it evict L1 before requiring the
	// following request to refill L2 with the still-active committed state.
	require.Eventually(t, func() bool {
		response := request()
		entry, cacheErr := remoteCache.APIKeyCache.GetAuthCache(ctx, cacheKey)
		return response.Code == http.StatusOK && cacheErr == nil && entry != nil &&
			entry.Snapshot != nil && entry.Snapshot.User.Status == service.StatusActive
	}, 5*time.Second, 10*time.Millisecond)

	var attempts int
	var claimedAt sql.NullTime
	var claimedBy sql.NullString
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT delivery_stage,attempts,claimed_at,claimed_by FROM auth_cache_invalidation_outbox WHERE id=$1 AND cache_key=$2`, sentinelID, sentinelHash).
		Scan(&stage, &attempts, &claimedAt, &claimedBy))
	require.Zero(t, stage)
	require.Zero(t, attempts)
	require.False(t, claimedAt.Valid)
	require.False(t, claimedBy.Valid, "the fixture-scoped consumer must not claim another hash")
	t.Log("real PG department writes and outbox acknowledgements + production worker/APIKeyService + miniredis TCP/PubSub: remote warmed L1/L2 HTTP 200 -> stale 200 -> 401 USER_INACTIVE -> stale 401 -> 200; both actual 30-second safety passes completed; no active invalidation, real Redis outage or multi-process deployment exercised")
}
