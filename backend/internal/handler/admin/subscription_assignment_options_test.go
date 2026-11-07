package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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

type subscriptionAssignmentGroupsStub struct {
	subscriptionHandlerService
}

// The service filters by type and organization scope (D13); the handler only
// projects picker fields.
func (subscriptionAssignmentGroupsStub) AssignableSubscriptionGroups(context.Context) ([]service.Group, error) {
	return []service.Group{
		{ID: 21, Name: "Subscription", Description: "For subscribers", Platform: "openai", RateMultiplier: 2, SubscriptionType: "subscription", Status: "active", IsExclusive: true},
	}, nil
}

func TestSubscriptionAssignmentOptions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &subscriptionAssignmentOptionsStub{t: t}
	users := &UserHandler{adminService: stub}
	groups := newSubscriptionHandler(subscriptionAssignmentGroupsStub{})
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

type groupOrganizationScopeStub struct {
	subscriptionHandlerService
	inputs []service.GroupOrganizationScopeInput
}

func (s *groupOrganizationScopeStub) ListGroupOrganizationScopes(context.Context) ([]service.GroupOrganizationScope, error) {
	return []service.GroupOrganizationScope{{GroupID: 3, OrganizationKeys: []string{"wsdashi", "xunyou"}}}, nil
}

func (s *groupOrganizationScopeStub) SetGroupOrganizationScopes(_ context.Context, input service.GroupOrganizationScopeInput) (int, error) {
	s.inputs = append(s.inputs, input)
	if input.OrganizationKeys == nil {
		return 0, service.ErrGroupOrganizationScopeInvalid
	}
	return len(input.GroupIDs), nil
}

// D13: the handler passes organization_keys through unchanged so that an
// omitted field stays distinguishable from [] (clear).
func TestGroupOrganizationScopeHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &groupOrganizationScopeStub{}
	h := newSubscriptionHandler(stub)
	router := gin.New()
	router.GET("/scopes", h.ListGroupOrganizationScopes)
	router.PUT("/scopes", h.SetGroupOrganizationScopes)
	serve := func(method, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		request := httptest.NewRequest(method, "/scopes", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, request)
		return w
	}

	w := serve(http.MethodGet, "")
	require.Equal(t, http.StatusOK, w.Code)
	require.JSONEq(t, `{"code":0,"message":"success","data":[{"group_id":3,"organization_keys":["wsdashi","xunyou"]}]}`, w.Body.String())

	w = serve(http.MethodPut, `{"group_ids":[1,2],"organization_keys":[]}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.JSONEq(t, `{"code":0,"message":"success","data":{"updated":2}}`, w.Body.String())
	require.NotNil(t, stub.inputs[0].OrganizationKeys)
	require.Empty(t, *stub.inputs[0].OrganizationKeys)

	w = serve(http.MethodPut, `{"group_ids":[1]}`)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	require.Nil(t, stub.inputs[1].OrganizationKeys)
	require.Contains(t, w.Body.String(), "GROUP_ORGANIZATION_SCOPE_INVALID")

	w = serve(http.MethodPut, `{"group_ids":"x"}`)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Len(t, stub.inputs, 2, "a malformed body never reaches the service")
}
