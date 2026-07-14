package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type dashboardUsageRepoCacheProbe struct {
	service.UsageLogRepository
	trendCalls          atomic.Int32
	usersTrendCalls     atomic.Int32
	lastUsersTrendLimit atomic.Int32
	lastUsersTrendIDsMu sync.Mutex
	lastUsersTrendIDs   []int64
}

func (r *dashboardUsageRepoCacheProbe) GetUsageTrendWithFilters(
	ctx context.Context,
	startTime, endTime time.Time,
	granularity string,
	userID, apiKeyID, accountID, groupID int64,
	model string,
	requestType *int16,
	stream *bool,
	billingType *int8,
) ([]usagestats.TrendDataPoint, error) {
	r.trendCalls.Add(1)
	return []usagestats.TrendDataPoint{{
		Date:        "2026-03-11",
		Requests:    1,
		TotalTokens: 2,
		Cost:        3,
		ActualCost:  4,
	}}, nil
}

func (r *dashboardUsageRepoCacheProbe) GetUserUsageTrend(
	ctx context.Context,
	startTime, endTime time.Time,
	granularity string,
	userIDs []int64,
	limit int,
) ([]usagestats.UserUsageTrendPoint, error) {
	r.usersTrendCalls.Add(1)
	r.lastUsersTrendLimit.Store(int32(limit))
	r.lastUsersTrendIDsMu.Lock()
	r.lastUsersTrendIDs = append([]int64(nil), userIDs...)
	r.lastUsersTrendIDsMu.Unlock()
	return []usagestats.UserUsageTrendPoint{{
		Date:       "2026-03-11",
		UserID:     1,
		Email:      "cache@test.dev",
		Requests:   2,
		Tokens:     20,
		Cost:       2,
		ActualCost: 1,
	}}, nil
}

func (r *dashboardUsageRepoCacheProbe) selectedUserIDs() []int64 {
	r.lastUsersTrendIDsMu.Lock()
	defer r.lastUsersTrendIDsMu.Unlock()
	return append([]int64(nil), r.lastUsersTrendIDs...)
}

func resetDashboardReadCachesForTest() {
	dashboardTrendCache = newSnapshotCache(30 * time.Second)
	dashboardUsersTrendCache = newSnapshotCache(30 * time.Second)
	dashboardAPIKeysTrendCache = newSnapshotCache(30 * time.Second)
	dashboardModelStatsCache = newSnapshotCache(30 * time.Second)
	dashboardGroupStatsCache = newSnapshotCache(30 * time.Second)
	dashboardSnapshotV2Cache = newSnapshotCache(30 * time.Second)
}

func TestDashboardHandler_GetUsageTrend_UsesCache(t *testing.T) {
	t.Cleanup(resetDashboardReadCachesForTest)
	resetDashboardReadCachesForTest()

	gin.SetMode(gin.TestMode)
	repo := &dashboardUsageRepoCacheProbe{}
	dashboardSvc := service.NewDashboardService(repo, nil, nil, nil)
	handler := NewDashboardHandler(dashboardSvc, nil)
	router := gin.New()
	router.GET("/admin/dashboard/trend", handler.GetUsageTrend)

	req1 := httptest.NewRequest(http.MethodGet, "/admin/dashboard/trend?start_date=2026-03-01&end_date=2026-03-07&granularity=day", nil)
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)
	require.Equal(t, http.StatusOK, rec1.Code)
	require.Equal(t, "miss", rec1.Header().Get("X-Snapshot-Cache"))

	req2 := httptest.NewRequest(http.MethodGet, "/admin/dashboard/trend?start_date=2026-03-01&end_date=2026-03-07&granularity=day", nil)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)
	require.Equal(t, http.StatusOK, rec2.Code)
	require.Equal(t, "hit", rec2.Header().Get("X-Snapshot-Cache"))
	require.Equal(t, int32(1), repo.trendCalls.Load())
}

func TestDashboardHandler_GetUserUsageTrend_UsesCache(t *testing.T) {
	t.Cleanup(resetDashboardReadCachesForTest)
	resetDashboardReadCachesForTest()

	gin.SetMode(gin.TestMode)
	repo := &dashboardUsageRepoCacheProbe{}
	dashboardSvc := service.NewDashboardService(repo, nil, nil, nil)
	handler := NewDashboardHandler(dashboardSvc, nil)
	router := gin.New()
	router.GET("/admin/dashboard/users-trend", handler.GetUserUsageTrend)

	req1 := httptest.NewRequest(http.MethodGet, "/admin/dashboard/users-trend?start_date=2026-03-01&end_date=2026-03-07&granularity=day&limit=8", nil)
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)
	require.Equal(t, http.StatusOK, rec1.Code)
	require.Equal(t, "miss", rec1.Header().Get("X-Snapshot-Cache"))

	req2 := httptest.NewRequest(http.MethodGet, "/admin/dashboard/users-trend?start_date=2026-03-01&end_date=2026-03-07&granularity=day&limit=8", nil)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)
	require.Equal(t, http.StatusOK, rec2.Code)
	require.Equal(t, "hit", rec2.Header().Get("X-Snapshot-Cache"))
	require.Equal(t, int32(1), repo.usersTrendCalls.Load())
	require.Empty(t, repo.selectedUserIDs())
	require.Equal(t, int32(8), repo.lastUsersTrendLimit.Load())
}

func TestDashboardHandler_GetUserUsageTrend_SelectedUsersValidation(t *testing.T) {
	tests := []struct {
		name string
		url  string
		code int
	}{
		{"valid day", "/admin/dashboard/users-trend?user_ids=8,7,8&start_date=2026-07-01&end_date=2026-07-30&granularity=day", http.StatusOK},
		{"valid hour", "/admin/dashboard/users-trend?user_ids=7&start_date=2026-07-01&end_date=2026-07-01&granularity=hour", http.StatusOK},
		{"five unique after duplicate", "/admin/dashboard/users-trend?user_ids=1,2,3,4,5,5&start_date=2026-07-01&end_date=2026-07-01&granularity=day", http.StatusOK},
		{"empty value", "/admin/dashboard/users-trend?user_ids=&start_date=2026-07-01&end_date=2026-07-01", http.StatusBadRequest},
		{"invalid id", "/admin/dashboard/users-trend?user_ids=7,nope&start_date=2026-07-01&end_date=2026-07-01", http.StatusBadRequest},
		{"empty segment", "/admin/dashboard/users-trend?user_ids=7,,8&start_date=2026-07-01&end_date=2026-07-01", http.StatusBadRequest},
		{"non positive", "/admin/dashboard/users-trend?user_ids=0,-1&start_date=2026-07-01&end_date=2026-07-01", http.StatusBadRequest},
		{"too many", "/admin/dashboard/users-trend?user_ids=1,2,3,4,5,6&start_date=2026-07-01&end_date=2026-07-01", http.StatusBadRequest},
		{"missing start", "/admin/dashboard/users-trend?user_ids=7&end_date=2026-07-01", http.StatusBadRequest},
		{"missing end", "/admin/dashboard/users-trend?user_ids=7&start_date=2026-07-01", http.StatusBadRequest},
		{"invalid start", "/admin/dashboard/users-trend?user_ids=7&start_date=2026-02-30&end_date=2026-07-01", http.StatusBadRequest},
		{"end before start", "/admin/dashboard/users-trend?user_ids=7&start_date=2026-07-02&end_date=2026-07-01", http.StatusBadRequest},
		{"invalid granularity", "/admin/dashboard/users-trend?user_ids=7&start_date=2026-07-01&end_date=2026-07-01&granularity=week", http.StatusBadRequest},
		{"over 90 days", "/admin/dashboard/users-trend?user_ids=7&start_date=2026-01-01&end_date=2026-04-01&granularity=day", http.StatusBadRequest},
		{"hour spans days", "/admin/dashboard/users-trend?user_ids=7&start_date=2026-07-01&end_date=2026-07-02&granularity=hour", http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetDashboardReadCachesForTest()
			repo := &dashboardUsageRepoCacheProbe{}
			dashboardSvc := service.NewDashboardService(repo, nil, nil, nil)
			handler := NewDashboardHandler(dashboardSvc, nil)
			router := gin.New()
			router.GET("/admin/dashboard/users-trend", handler.GetUserUsageTrend)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tt.url, nil))
			require.Equal(t, tt.code, recorder.Code)
			if tt.code == http.StatusBadRequest {
				require.Equal(t, int32(0), repo.usersTrendCalls.Load())
			}
		})
	}
}

func TestDashboardHandler_GetUserUsageTrend_SelectedUsersCacheCanonicalization(t *testing.T) {
	t.Cleanup(resetDashboardReadCachesForTest)
	resetDashboardReadCachesForTest()

	gin.SetMode(gin.TestMode)
	repo := &dashboardUsageRepoCacheProbe{}
	dashboardSvc := service.NewDashboardService(repo, nil, nil, nil)
	handler := NewDashboardHandler(dashboardSvc, nil)
	router := gin.New()
	router.GET("/admin/dashboard/users-trend", handler.GetUserUsageTrend)

	request := func(url string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, url, nil))
		return recorder
	}
	base := "/admin/dashboard/users-trend?start_date=2026-07-01&end_date=2026-07-30&granularity=day"

	first := request(base + "&user_ids=8,7,8&limit=99")
	require.Equal(t, http.StatusOK, first.Code)
	require.Equal(t, "miss", first.Header().Get("X-Snapshot-Cache"))
	require.Equal(t, []int64{7, 8}, repo.selectedUserIDs())
	require.Equal(t, int32(0), repo.lastUsersTrendLimit.Load())

	second := request(base + "&user_ids=7,8&limit=1")
	require.Equal(t, http.StatusOK, second.Code)
	require.Equal(t, "hit", second.Header().Get("X-Snapshot-Cache"))
	require.Equal(t, int32(1), repo.usersTrendCalls.Load())

	third := request(base + "&user_ids=7,9")
	require.Equal(t, http.StatusOK, third.Code)
	require.Equal(t, "miss", third.Header().Get("X-Snapshot-Cache"))
	require.Equal(t, int32(2), repo.usersTrendCalls.Load())
	require.Equal(t, []int64{7, 9}, repo.selectedUserIDs())
}
