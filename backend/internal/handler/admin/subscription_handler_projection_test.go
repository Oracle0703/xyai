package admin

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type subscriptionProjectionStub struct {
	subscriptionHandlerService
	assignCalls int
	bulkCalls   int
}

func projectionSubscription() *service.UserSubscription {
	daily := 5.0
	return &service.UserSubscription{
		ID: 1, UserID: 10, GroupID: 20, Status: service.SubscriptionStatusActive, DailyUsageUSD: 1.5,
		User:           &service.User{ID: 10, Email: "member@xunyou.com", Username: "member", Status: service.StatusActive, Balance: 42, TotalRecharged: 99},
		AssignedByUser: &service.User{ID: 1, Email: "root@admin.com", Balance: 1000},
		Group:          &service.Group{ID: 20, Name: "GPT", Platform: service.PlatformOpenAI, Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeSubscription, DailyLimitUSD: &daily},
	}
}

func (s *subscriptionProjectionStub) GetByID(context.Context, int64) (*service.UserSubscription, error) {
	return projectionSubscription(), nil
}

func (s *subscriptionProjectionStub) AssignSubscription(context.Context, *service.AssignSubscriptionInput) (*service.UserSubscription, error) {
	s.assignCalls++
	return projectionSubscription(), nil
}

func (s *subscriptionProjectionStub) BulkAssignSubscription(context.Context, *service.BulkAssignSubscriptionInput) (*service.BulkAssignResult, error) {
	s.bulkCalls++
	return &service.BulkAssignResult{SuccessCount: 1, Subscriptions: []service.UserSubscription{*projectionSubscription()}}, nil
}

func setupSubscriptionProjectionRouter(role string, svc subscriptionHandlerService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 77})
		c.Set(string(middleware2.ContextKeyUserRole), role)
		c.Next()
	})
	handler := newSubscriptionHandler(svc)
	router.GET("/api/v1/admin/subscriptions/:id", handler.GetByID)
	router.POST("/api/v1/admin/subscriptions/assign", handler.Assign)
	router.POST("/api/v1/admin/subscriptions/bulk-assign", handler.BulkAssign)
	return router
}

func TestSubscriptionHandler_SubAdminGetsLeastPrivilegeProjection(t *testing.T) {
	recorder := httptest.NewRecorder()
	setupSubscriptionProjectionRouter(service.RoleSubAdmin, &subscriptionProjectionStub{}).
		ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/admin/subscriptions/1", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	for _, hidden := range []string{"balance", "total_recharged", "assigned_by_user", "root@admin.com", "notes"} {
		require.NotContains(t, body, hidden)
	}
	// The subscription page still needs group quota fields to draw progress.
	require.Contains(t, body, `"daily_limit_usd":5`)
	require.Contains(t, body, `"subscription_type":"subscription"`)
	require.Contains(t, body, "member@xunyou.com")
}

func TestSubscriptionHandler_AdminKeepsFullProjection(t *testing.T) {
	recorder := httptest.NewRecorder()
	setupSubscriptionProjectionRouter(service.RoleAdmin, &subscriptionProjectionStub{}).
		ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/admin/subscriptions/1", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "assigned_by_user")
	require.Contains(t, recorder.Body.String(), `"balance":42`)
}

func TestSubscriptionHandler_SubAdminCannotAssignToSelf(t *testing.T) {
	for _, tc := range []struct{ path, body string }{
		{"/api/v1/admin/subscriptions/assign", `{"user_id":77,"group_id":20}`},
		{"/api/v1/admin/subscriptions/bulk-assign", `{"user_ids":[5,77],"group_id":20}`},
	} {
		svc := &subscriptionProjectionStub{}
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, tc.path, bytes.NewBufferString(tc.body))
		request.Header.Set("Content-Type", "application/json")
		setupSubscriptionProjectionRouter(service.RoleSubAdmin, svc).ServeHTTP(recorder, request)

		require.Equal(t, http.StatusForbidden, recorder.Code, tc.path)
		require.Contains(t, recorder.Body.String(), "SUBSCRIPTION_SELF_ASSIGN_DENIED")
		require.Zero(t, svc.assignCalls+svc.bulkCalls)
	}

	svc := &subscriptionProjectionStub{}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/subscriptions/assign", bytes.NewBufferString(`{"user_id":77,"group_id":20}`))
	request.Header.Set("Content-Type", "application/json")
	setupSubscriptionProjectionRouter(service.RoleAdmin, svc).ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, 1, svc.assignCalls)
}

func TestSubscriptionHandler_SubAdminBulkAssignOmitsSubscriptionDetails(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/subscriptions/bulk-assign", bytes.NewBufferString(`{"user_ids":[10],"group_id":20}`))
	request.Header.Set("Content-Type", "application/json")
	setupSubscriptionProjectionRouter(service.RoleSubAdmin, &subscriptionProjectionStub{}).ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"subscriptions":[]`)
	require.NotContains(t, recorder.Body.String(), "balance")
}
