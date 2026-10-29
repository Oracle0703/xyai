//go:build integration

package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func departmentMemberAuthRequest(router http.Handler, credential string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("Authorization", "Bearer "+credential)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestDepartmentMemberAuthIntegration_JWTReflectsMemberStatus(t *testing.T) {
	s := setupDepartmentMembers(t)
	member := s.f.user("jwt", "xunyou.com", service.RoleUser, nil)
	s.place(member, s.granted)
	users := NewUserRepository(integrationEntClient, integrationDB)
	cfg := &config.Config{}
	cfg.JWT.Secret = uuid.NewString() + uuid.NewString()
	cfg.JWT.AccessTokenExpireMinutes = 60
	auth := service.NewAuthService(integrationEntClient, users, nil, nil, cfg, nil, nil, nil, nil, nil, nil, nil, nil)
	userService := service.NewUserService(users, nil, nil, nil)
	router := gin.New()
	router.Use(gin.HandlerFunc(middleware.NewJWTAuthMiddleware(auth, userService, nil, nil)))
	router.GET("/protected", func(c *gin.Context) {
		subject, ok := middleware.GetAuthSubjectFromContext(c)
		if !ok {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.JSON(http.StatusOK, gin.H{"user_id": subject.UserID})
	})
	user, err := users.GetByID(s.f.ctx, member)
	require.NoError(t, err)
	token, err := auth.GenerateToken(s.f.ctx, user)
	require.NoError(t, err)

	response := departmentMemberAuthRequest(router, token)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var body struct {
		UserID int64 `json:"user_id"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.Equal(t, member, body.UserID)

	in := s.expect(member)
	in.Status = departmentMemberString(service.StatusDisabled)
	_, err = s.svc.UpdateManagedMember(s.leaderCtx, member, in)
	require.NoError(t, err)
	response = departmentMemberAuthRequest(router, token)
	require.Equal(t, http.StatusUnauthorized, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), "USER_INACTIVE")

	in.Status = departmentMemberString(service.StatusActive)
	_, err = s.svc.UpdateManagedMember(s.leaderCtx, member, in)
	require.NoError(t, err)
	response = departmentMemberAuthRequest(router, token)
	require.Equal(t, http.StatusOK, response.Code, "the same unexpired token works after re-enabling; disabling is not permanent token revocation")
	t.Log("same signed JWT: active HTTP 200 -> manager-disabled HTTP 401 USER_INACTIVE -> enabled HTTP 200; session binding and refresh-token flow are outside this test")
}

// This wrapper counts calls but delegates every result to the real PG repository.
type departmentMemberAuthCountingRepository struct {
	service.APIKeyRepository
	lookups      atomic.Int64
	userKeyLists atomic.Int64
}

func (r *departmentMemberAuthCountingRepository) GetByKeyForAuth(ctx context.Context, key string) (*service.APIKey, error) {
	r.lookups.Add(1)
	return r.APIKeyRepository.GetByKeyForAuth(ctx, key)
}

func (r *departmentMemberAuthCountingRepository) ListKeysByUserID(ctx context.Context, userID int64) ([]string, error) {
	r.userKeyLists.Add(1)
	return r.APIKeyRepository.ListKeysByUserID(ctx, userID)
}

func createDepartmentMemberAuthKeys(t *testing.T, f *departmentMemberFixture, userID int64, count int) ([]*service.APIKey, []string) {
	t.Helper()
	group := mustCreateGroup(t, integrationEntClient, &service.Group{Name: f.prefix + "auth", RateMultiplier: 1})
	keyIDs := []int64{}
	hashes := []string{}
	t.Cleanup(func() {
		ctx := context.Background()
		_, err := integrationDB.ExecContext(ctx, `DELETE FROM api_keys WHERE id=ANY($1)`, pq.Array(keyIDs))
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(ctx, `DELETE FROM groups WHERE id=$1`, group.ID)
		require.NoError(t, err)
		// Key deletion enqueues another event; clean only these hashes after deletion.
		_, err = integrationDB.ExecContext(ctx, `DELETE FROM auth_cache_invalidation_outbox WHERE cache_key=ANY($1)`, pq.Array(hashes))
		require.NoError(t, err)
	})
	keys := make([]*service.APIKey, 0, count)
	repo := NewAPIKeyRepository(integrationEntClient, integrationDB)
	for i := 0; i < count; i++ {
		key := &service.APIKey{
			UserID: userID, GroupID: &group.ID, Key: "sk-dept-member-" + uuid.NewString(),
			Name: f.prefix + strconv.Itoa(i), Status: service.StatusActive,
		}
		require.NoError(t, repo.Create(f.ctx, key))
		keyIDs = append(keyIDs, key.ID)
		keys = append(keys, key)
		sum := sha256.Sum256([]byte(key.Key))
		hashes = append(hashes, hex.EncodeToString(sum[:]))
	}
	return keys, hashes
}

func TestDepartmentMemberAuthIntegration_WarmAPIKeyL1ReflectsMemberStatus(t *testing.T) {
	s := setupDepartmentMembers(t)
	member := s.f.user("api_key", "xunyou.com", service.RoleUser, nil)
	s.place(member, s.granted)
	keys, _ := createDepartmentMemberAuthKeys(t, s.f, member, 1)
	countingRepo := &departmentMemberAuthCountingRepository{APIKeyRepository: NewAPIKeyRepository(integrationEntClient, integrationDB)}
	cfg := &config.Config{
		RunMode: config.RunModeSimple,
		APIKeyAuth: config.APIKeyAuthCacheConfig{
			L1Size: 128, L1TTLSeconds: 600, L2TTLSeconds: 0,
		},
	}
	apiKeys := service.NewAPIKeyService(countingRepo, NewUserRepository(integrationEntClient, integrationDB), nil, nil, nil, nil, cfg)
	s.svc = service.NewDepartmentService(NewDepartmentRepository(integrationDB), apiKeys)
	router := gin.New()
	router.Use(gin.HandlerFunc(middleware.NewAPIKeyAuthMiddleware(apiKeys, nil, cfg)))
	router.GET("/protected", func(c *gin.Context) { c.Status(http.StatusOK) })
	credential := keys[0].Key
	response := departmentMemberAuthRequest(router, credential)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Positive(t, countingRepo.lookups.Load())

	awaitCachedResponse := func(status int, reason string) {
		t.Helper()
		// Ristretto admission is asynchronous. Observe a real hit instead of assuming
		// that a successful initial request has already warmed the cache.
		require.Eventually(t, func() bool {
			before := countingRepo.lookups.Load()
			response := departmentMemberAuthRequest(router, credential)
			return response.Code == status && strings.Contains(response.Body.String(), reason) && countingRepo.lookups.Load() == before
		}, 5*time.Second, 10*time.Millisecond, "expected an HTTP result from the warmed L1 without another PG auth lookup")
	}
	awaitCachedResponse(http.StatusOK, "")
	beforeDisable := countingRepo.lookups.Load()
	in := s.expect(member)
	in.Status = departmentMemberString(service.StatusDisabled)
	_, err := s.svc.UpdateManagedMember(s.leaderCtx, member, in)
	require.NoError(t, err)
	require.EqualValues(t, 1, countingRepo.userKeyLists.Load(), "DepartmentService must call the real user-key invalidation path after commit")
	response = departmentMemberAuthRequest(router, credential)
	require.Equal(t, http.StatusUnauthorized, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), "USER_INACTIVE")
	require.Greater(t, countingRepo.lookups.Load(), beforeDisable, "the previously cached active snapshot must have been evicted")

	awaitCachedResponse(http.StatusUnauthorized, "USER_INACTIVE")
	beforeEnable := countingRepo.lookups.Load()
	in.Status = departmentMemberString(service.StatusActive)
	_, err = s.svc.UpdateManagedMember(s.leaderCtx, member, in)
	require.NoError(t, err)
	require.EqualValues(t, 2, countingRepo.userKeyLists.Load())
	response = departmentMemberAuthRequest(router, credential)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Greater(t, countingRepo.lookups.Load(), beforeEnable, "re-enabling must evict the cached disabled snapshot too")
	t.Log("real APIKey middleware and PG repository with warmed process-local L1: HTTP 200 -> 401 USER_INACTIVE -> 200; Redis L2, Pub/Sub and cross-instance convergence are not covered")
}

func TestDepartmentMemberAuthIntegration_OutboxEnqueuesOnlyCommittedStatusChanges(t *testing.T) {
	s := setupDepartmentMembers(t)
	f := s.f
	member := f.user("outbox", "xunyou.com", service.RoleUser, nil)
	s.place(member, s.granted)
	keys, hashes := createDepartmentMemberAuthKeys(t, f, member, 2)
	clearEvents := func() {
		t.Helper()
		f.mustExec(`DELETE FROM auth_cache_invalidation_outbox WHERE cache_key=ANY($1)`, pq.Array(hashes))
	}
	countEvents := func() int {
		t.Helper()
		var count int
		require.NoError(t, integrationDB.QueryRowContext(f.ctx, `SELECT COUNT(*) FROM auth_cache_invalidation_outbox WHERE cache_key=ANY($1)`, pq.Array(hashes)).Scan(&count))
		return count
	}
	assertInitialEvents := func() {
		t.Helper()
		rows, err := integrationDB.QueryContext(f.ctx, `SELECT cache_key,delivery_stage,attempts,claimed_at FROM auth_cache_invalidation_outbox WHERE cache_key=ANY($1) ORDER BY id`, pq.Array(hashes))
		require.NoError(t, err)
		defer func() { _ = rows.Close() }()
		seen := map[string]int{}
		for rows.Next() {
			var hash string
			var stage, attempts int
			var claimedAt sql.NullTime
			require.NoError(t, rows.Scan(&hash, &stage, &attempts, &claimedAt))
			require.Contains(t, hashes, hash)
			require.Zero(t, stage)
			require.Zero(t, attempts)
			require.False(t, claimedAt.Valid)
			for _, key := range keys {
				require.NotContains(t, hash, key.Key)
			}
			seen[hash]++
		}
		require.NoError(t, rows.Err())
		require.Len(t, seen, len(hashes))
		for _, hash := range hashes {
			require.Equal(t, 1, seen[hash], "each key receives one unclaimed initial invalidation")
		}
	}
	clearEvents()
	in := s.expect(member)
	in.Status = departmentMemberString(service.StatusDisabled)
	_, err := s.svc.UpdateManagedMember(s.leaderCtx, member, in)
	require.NoError(t, err)
	assertInitialEvents()
	clearEvents()
	_, err = s.svc.UpdateManagedMember(s.leaderCtx, member, in)
	require.NoError(t, err)
	require.Zero(t, countEvents(), "a repeated disable is a no-op")
	in.Status = nil
	in.Username = departmentMemberString("outbox-profile")
	in.DepartmentNote = departmentMemberString("profile note")
	_, err = s.svc.UpdateManagedMember(s.leaderCtx, member, in)
	require.NoError(t, err)
	require.Zero(t, countEvents(), "profile-only edits do not enqueue security invalidations")
	in = s.expect(member)
	in.Status = departmentMemberString(service.StatusActive)
	_, err = s.svc.UpdateManagedMember(s.leaderCtx, member, in)
	require.NoError(t, err)
	assertInitialEvents()
	clearEvents()

	functionName := pq.QuoteIdentifier(f.prefix + "reject_auth_audit")
	triggerName := pq.QuoteIdentifier(f.prefix + "reject_auth_audit_trigger")
	f.mustExec(`CREATE FUNCTION ` + functionName + `() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.actor_user_id=` + strconv.FormatInt(s.leaderID, 10) + ` AND NEW.action='department.manager_update_member' THEN
        RAISE EXCEPTION 'department member auth audit failure';
    END IF;
    RETURN NEW;
END;
$$`)
	t.Cleanup(func() {
		_, dropErr := integrationDB.ExecContext(context.Background(), `DROP TRIGGER IF EXISTS `+triggerName+` ON audit_logs`)
		require.NoError(t, dropErr)
		_, dropErr = integrationDB.ExecContext(context.Background(), `DROP FUNCTION IF EXISTS `+functionName+`() `)
		require.NoError(t, dropErr)
	})
	f.mustExec(`CREATE TRIGGER ` + triggerName + ` BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION ` + functionName + `()`)
	beforeAudits := s.audits("department.manager_update_member", s.leaderID)
	in.Status = departmentMemberString(service.StatusDisabled)
	_, err = s.svc.UpdateManagedMember(s.leaderCtx, member, in)
	require.Error(t, err)
	f.requireStatus(member, service.StatusActive, "")
	require.Equal(t, "profile note", f.note(member))
	require.Zero(t, countEvents(), "a rolled-back disable must not leave outbox events")
	require.Equal(t, beforeAudits, s.audits("department.manager_update_member", s.leaderID))
	t.Log("outbox assertions and cleanup are restricted to this test's two random-key hashes; no outbox consumer, Redis connection or upstream request was started")
}
