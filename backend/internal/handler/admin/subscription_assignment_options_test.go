package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type subscriptionAssignmentOptionsStub struct {
	service.AdminService
	t         *testing.T
	userCalls int
}

func (s *subscriptionAssignmentOptionsStub) ListUsers(_ context.Context, page, pageSize int, filters service.UserListFilters, sortBy, sortOrder string) ([]service.User, int64, error) {
	s.userCalls++
	require.Equal(s.t, 1, page)
	require.Equal(s.t, 30, pageSize)
	require.Equal(s.t, service.UserListFilters{Search: "reader"}, filters)
	require.Equal(s.t, "email", sortBy)
	require.Equal(s.t, "asc", sortOrder)
	return []service.User{{ID: 11, Email: "reader@example.com", Balance: 123, Notes: "private", AdminPermissions: []string{service.AdminPermissionUsage}}}, 1, nil
}

func (s *subscriptionAssignmentOptionsStub) GetAllGroups(_ context.Context) ([]service.Group, error) {
	return []service.Group{
		{ID: 21, Name: "Subscription", Description: "For subscribers", Platform: "openai", RateMultiplier: 2, SubscriptionType: "subscription", Status: "active", IsExclusive: true},
		{ID: 22, Name: "Standard", SubscriptionType: "standard", Status: "active"},
	}, nil
}

func TestSubscriptionAssignmentOptions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &subscriptionAssignmentOptionsStub{t: t}
	users := &UserHandler{adminService: stub}
	groups := &GroupHandler{adminService: stub}
	router := gin.New()
	router.GET("/users", users.SearchSubscriptionAssignmentUsers)
	router.GET("/groups", groups.SubscriptionAssignmentGroups)

	for _, tc := range []struct{ path, want string }{
		{"/users?q=%20%20", `{"code":0,"message":"success","data":[]}`},
		{"/users?q=%20reader%20", `{"code":0,"message":"success","data":[{"id":11,"email":"reader@example.com"}]}`},
		{"/groups", `{"code":0,"message":"success","data":[{"id":21,"name":"Subscription","description":"For subscribers","platform":"openai","rate_multiplier":2,"subscription_type":"subscription","status":"active"}]}`},
	} {
		t.Run(tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
			require.Equal(t, http.StatusOK, w.Code)
			require.JSONEq(t, tc.want, w.Body.String())
		})
	}
	require.Equal(t, 1, stub.userCalls)
}
