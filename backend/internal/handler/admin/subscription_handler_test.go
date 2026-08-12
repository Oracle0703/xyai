package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type subscriptionHandlerServiceStub struct {
	subscriptionHandlerService
	listCalls   int
	listPage    int
	listSize    int
	listFilter  service.SubscriptionAdminFilter
	resetCalls  int
	resetFilter service.SubscriptionAdminFilter
	resetCount  int
	resetErr    error
}

func (s *subscriptionHandlerServiceStub) ListAdmin(_ context.Context, page, pageSize int, filter service.SubscriptionAdminFilter) ([]service.UserSubscription, *pagination.PaginationResult, error) {
	normalized, err := service.NormalizeSubscriptionAdminFilter(filter)
	if err != nil {
		return nil, nil, err
	}
	s.listCalls++
	s.listPage = page
	s.listSize = pageSize
	s.listFilter = normalized
	return []service.UserSubscription{}, &pagination.PaginationResult{Page: page, PageSize: pageSize, Pages: 1}, nil
}

func (s *subscriptionHandlerServiceStub) AdminResetDailyFiltered(_ context.Context, filter service.SubscriptionAdminFilter) (int, error) {
	normalized, err := service.NormalizeSubscriptionAdminFilter(filter)
	if err != nil {
		return 0, err
	}
	s.resetCalls++
	s.resetFilter = normalized
	return s.resetCount, s.resetErr
}

func setupSubscriptionHandlerRouter(t *testing.T, svc subscriptionHandlerService) *gin.Engine {
	t.Helper()
	previousCoordinator := service.DefaultIdempotencyCoordinator()
	service.SetDefaultIdempotencyCoordinator(nil)
	t.Cleanup(func() { service.SetDefaultIdempotencyCoordinator(previousCoordinator) })

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 77})
		c.Next()
	})
	handler := newSubscriptionHandler(svc)
	router.GET("/api/v1/admin/subscriptions", handler.List)
	router.POST("/api/v1/admin/subscriptions/reset-daily-filtered", handler.ResetDailyFiltered)
	return router
}

func TestSubscriptionHandler_ListPassesNormalizedOrganizationFilter(t *testing.T) {
	svc := &subscriptionHandlerServiceStub{}
	router := setupSubscriptionHandlerRouter(t, svc)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/subscriptions?page=2&page_size=30&user_id=11&group_id=21&status=ACTIVE&platform=OPENAI&organization=XUNYOU&sort_by=expires_at&sort_order=asc", nil)

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, 1, svc.listCalls)
	require.Equal(t, 2, svc.listPage)
	require.Equal(t, 30, svc.listSize)
	require.Equal(t, int64(11), *svc.listFilter.UserID)
	require.Equal(t, int64(21), *svc.listFilter.GroupID)
	require.Equal(t, service.SubscriptionStatusActive, svc.listFilter.Status)
	require.Equal(t, service.PlatformOpenAI, svc.listFilter.Platform)
	require.Equal(t, service.SubscriptionOrganizationXunyou, svc.listFilter.Organization)
	require.Equal(t, "expires_at", svc.listFilter.SortBy)
	require.Equal(t, "asc", svc.listFilter.SortOrder)
}

func TestSubscriptionHandler_ListRejectsInvalidFilters(t *testing.T) {
	tests := []string{
		"user_id=abc",
		"user_id=0",
		"group_id=-1",
		"organization=unknown",
		"status=unknown",
		"platform=unknown",
		"sort_by=unknown",
		"sort_order=sideways",
	}
	for _, query := range tests {
		t.Run(query, func(t *testing.T) {
			svc := &subscriptionHandlerServiceStub{}
			router := setupSubscriptionHandlerRouter(t, svc)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/subscriptions?"+query, nil)

			router.ServeHTTP(recorder, request)

			require.Equal(t, http.StatusBadRequest, recorder.Code)
		})
	}
}

func TestSubscriptionHandler_ResetDailyFilteredReturnsCount(t *testing.T) {
	svc := &subscriptionHandlerServiceStub{resetCount: 12}
	router := setupSubscriptionHandlerRouter(t, svc)
	body := []byte(`{"status":"active","organization":"xunyou","group_id":21}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/subscriptions/reset-daily-filtered", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "subscriptions-reset-20260812-1")

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, 1, svc.resetCalls)
	require.Equal(t, service.SubscriptionStatusActive, svc.resetFilter.Status)
	require.Equal(t, service.SubscriptionOrganizationXunyou, svc.resetFilter.Organization)
	require.Equal(t, int64(21), *svc.resetFilter.GroupID)
	var responseBody struct {
		Data struct {
			ResetCount int `json:"reset_count"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &responseBody))
	require.Equal(t, 12, responseBody.Data.ResetCount)
}

func TestSubscriptionHandler_ResetDailyFilteredRejectsInvalidFilter(t *testing.T) {
	svc := &subscriptionHandlerServiceStub{}
	router := setupSubscriptionHandlerRouter(t, svc)
	for _, body := range []string{
		`{"user_id":0}`,
		`{"group_id":-1}`,
		`{"organization":"unknown"}`,
		`{"platform":"unknown"}`,
		`{"status":"unknown"}`,
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/subscriptions/reset-daily-filtered", bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")

		router.ServeHTTP(recorder, request)

		require.Equal(t, http.StatusBadRequest, recorder.Code, body)
	}
}

func TestSubscriptionHandler_ResetDailyFilteredReplaysSameIdempotencyKey(t *testing.T) {
	svc := &subscriptionHandlerServiceStub{resetCount: 2}
	router := setupSubscriptionHandlerRouter(t, svc)
	service.SetDefaultIdempotencyCoordinator(service.NewIdempotencyCoordinator(newMemoryIdempotencyRepoStub(), service.DefaultIdempotencyConfig()))
	body := []byte(`{"organization":"xunyou"}`)

	call := func() *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/subscriptions/reset-daily-filtered", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", "subscriptions-reset-replay")
		router.ServeHTTP(recorder, request)
		return recorder
	}

	first := call()
	second := call()

	require.Equal(t, http.StatusOK, first.Code)
	require.Equal(t, http.StatusOK, second.Code)
	require.Equal(t, 1, svc.resetCalls)
	require.Equal(t, "true", second.Header().Get("X-Idempotency-Replayed"))
}

func TestSubscriptionHandler_ResetDailyFilteredRejectsMalformedJSON(t *testing.T) {
	svc := &subscriptionHandlerServiceStub{}
	router := setupSubscriptionHandlerRouter(t, svc)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/subscriptions/reset-daily-filtered", bytes.NewBufferString(`{"group_id":"wrong"}`))
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Zero(t, svc.resetCalls)
}
