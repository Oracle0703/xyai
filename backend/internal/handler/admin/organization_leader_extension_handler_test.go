package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Request contracts of the 2026-11-04 extension (design §4.8, §4.9).

type organizationUserRecorder struct {
	service.DepartmentRepository
	deletes   int
	deleteIn  service.OrganizationUserDeleteInput
	passwords int
	hash      string
	query     service.DepartmentScopeQuery
}

func (r *organizationUserRecorder) DeleteOrganizationUser(_ context.Context, _ int64, in service.OrganizationUserDeleteInput) ([]string, error) {
	r.deletes++
	r.deleteIn = in
	return nil, nil
}

func (r *organizationUserRecorder) ResetOrganizationUserPassword(_ context.Context, _ int64, hash string) error {
	r.passwords++
	r.hash = hash
	return nil
}

func (r *organizationUserRecorder) QueryScope(_ context.Context, _ string, query service.DepartmentScopeQuery) (*service.DepartmentScope, error) {
	r.query = query
	return &service.DepartmentScope{}, nil
}

func serveOrganizationUserRequest(t *testing.T, repo *organizationUserRecorder, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := NewDepartmentHandler(service.NewDepartmentService(repo, nil))
	router.POST("/organization/users/:id/password", h.ResetOrganizationUserPassword)
	router.DELETE("/organization/users/:id", h.DeleteOrganizationUser)
	router.GET("/subscriptions/search-users", h.SubscriptionUsers)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(method, path, strings.NewReader(body)))
	return response
}

func TestOrganizationUserDeleteRequiresExplicitDepartment(t *testing.T) {
	for _, body := range []string{
		`{}`,
		`{"expected_department_version":3}`,
		`{"expected_department_id":5}`,
		`{"expected_department_id":5,"expected_department_version":3,"force":true}`,
		`null`,
	} {
		repo := &organizationUserRecorder{}
		response := serveOrganizationUserRequest(t, repo, http.MethodDelete, "/organization/users/7", body)
		require.Equal(t, http.StatusBadRequest, response.Code, body)
		require.Zero(t, repo.deletes, body)
	}
	repo := &organizationUserRecorder{}
	response := serveOrganizationUserRequest(t, repo, http.MethodDelete, "/organization/users/7", `{"expected_department_id":null,"expected_department_version":3}`)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.True(t, repo.deleteIn.ExpectedUnassigned, "an explicit null confirms an unassigned user")
	repo = &organizationUserRecorder{}
	response = serveOrganizationUserRequest(t, repo, http.MethodDelete, "/organization/users/7", `{"expected_department_id":5,"expected_department_version":3}`)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.False(t, repo.deleteIn.ExpectedUnassigned)
	require.Equal(t, int64(5), *repo.deleteIn.ExpectedDepartmentID)
}

func TestOrganizationUserPasswordContract(t *testing.T) {
	for _, body := range []string{`{}`, `{"password":"12345"}`, `{"password":"good-password","role":"admin"}`, `null`} {
		repo := &organizationUserRecorder{}
		response := serveOrganizationUserRequest(t, repo, http.MethodPost, "/organization/users/7/password", body)
		require.Equal(t, http.StatusBadRequest, response.Code, body)
		require.Zero(t, repo.passwords, body)
	}
	repo := &organizationUserRecorder{}
	response := serveOrganizationUserRequest(t, repo, http.MethodPost, "/organization/users/7/password", `{"password":"good-password"}`)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.NotContains(t, response.Body.String(), "good-password")
	require.True(t, (&service.User{PasswordHash: repo.hash}).CheckPassword("good-password"), "only the hash reaches the repository")
}

func TestSubscriptionUsersPurpose(t *testing.T) {
	repo := &organizationUserRecorder{}
	response := serveOrganizationUserRequest(t, repo, http.MethodGet, "/subscriptions/search-users?q=ab&purpose=write", "")
	require.Equal(t, http.StatusBadRequest, response.Code)
	response = serveOrganizationUserRequest(t, repo, http.MethodGet, "/subscriptions/search-users?q=ab&purpose=assign", "")
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.True(t, repo.query.AssignTargets)
	response = serveOrganizationUserRequest(t, repo, http.MethodGet, "/subscriptions/search-users?q=ab", "")
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.False(t, repo.query.AssignTargets)
}

type scopedSubscriptionWriteStub struct {
	subscriptionHandlerService
	precheckErr error
	prechecks   int
	extends     int
	manageable  map[int64]bool
	scoped      bool
}

func (s *scopedSubscriptionWriteStub) PrecheckSubscriptionWrite(context.Context, int64) error {
	s.prechecks++
	return s.precheckErr
}

func (s *scopedSubscriptionWriteStub) ExtendSubscription(_ context.Context, id int64, _ int) (*service.UserSubscription, error) {
	s.extends++
	return &service.UserSubscription{ID: id, UserID: 9}, nil
}

func (s *scopedSubscriptionWriteStub) ListAdmin(context.Context, int, int, service.SubscriptionAdminFilter) ([]service.UserSubscription, *pagination.PaginationResult, error) {
	return []service.UserSubscription{{ID: 1, UserID: 9}, {ID: 2, UserID: 10}}, &pagination.PaginationResult{Page: 1, PageSize: 20, Pages: 1, Total: 2}, nil
}

func (s *scopedSubscriptionWriteStub) ManageableSubscriptionUsers(context.Context, []int64) (map[int64]bool, bool, error) {
	return s.manageable, s.scoped, nil
}

func scopedSubscriptionRouter(t *testing.T, svc subscriptionHandlerService, role string) *gin.Engine {
	t.Helper()
	previous := service.DefaultIdempotencyCoordinator()
	service.SetDefaultIdempotencyCoordinator(service.NewIdempotencyCoordinator(newMemoryIdempotencyRepoStub(), service.DefaultIdempotencyConfig()))
	t.Cleanup(func() { service.SetDefaultIdempotencyCoordinator(previous) })
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 77})
		c.Set(string(middleware2.ContextKeyUserRole), role)
		c.Next()
	})
	h := newSubscriptionHandler(svc)
	router.POST("/api/v1/admin/subscriptions/:id/extend", h.Extend)
	router.GET("/api/v1/admin/subscriptions", h.List)
	return router
}

// G17: an idempotent replay never reaches the service, so the scope check runs first.
func TestSubscriptionExtendPrechecksBeforeReplay(t *testing.T) {
	svc := &scopedSubscriptionWriteStub{}
	router := scopedSubscriptionRouter(t, svc, service.RoleSubAdmin)
	extend := func() *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/subscriptions/5/extend", strings.NewReader(`{"days":3}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", "extend-once")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	require.Equal(t, http.StatusOK, extend().Code)
	require.Equal(t, 1, svc.extends)
	svc.precheckErr = service.ErrDepartmentScopeDenied
	response := extend()
	require.Equal(t, http.StatusForbidden, response.Code, response.Body.String())
	require.Empty(t, response.Header().Get("X-Idempotency-Replayed"))
	require.Equal(t, 1, svc.extends)
	require.Equal(t, 2, svc.prechecks)
}

func TestSubscriptionListMarksManageableRows(t *testing.T) {
	list := func(svc *scopedSubscriptionWriteStub, role string) []map[string]any {
		response := httptest.NewRecorder()
		scopedSubscriptionRouter(t, svc, role).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/admin/subscriptions", nil))
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		var body struct {
			Data struct {
				Items []map[string]any `json:"items"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
		return body.Data.Items
	}
	items := list(&scopedSubscriptionWriteStub{scoped: true, manageable: map[int64]bool{9: true}}, service.RoleSubAdmin)
	require.Len(t, items, 2)
	require.Equal(t, true, items[0]["can_manage"])
	require.Equal(t, false, items[1]["can_manage"])
	items = list(&scopedSubscriptionWriteStub{}, service.RoleSubAdmin)
	require.NotContains(t, items[0], "can_manage", "unscoped sub-admins keep the previous response")
	items = list(&scopedSubscriptionWriteStub{scoped: true}, service.RoleAdmin)
	require.NotContains(t, items[0], "can_manage")
}
