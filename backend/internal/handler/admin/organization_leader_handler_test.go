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

type organizationLeaderRecorder struct {
	service.DepartmentRepository
	calls        int
	access       service.OrganizationAccessInput
	leader       service.OrganizationLeaderInput
	memberUpdate service.ManagedMemberUpdateInput
}

func (r *organizationLeaderRecorder) SetOrganizationAccess(_ context.Context, id int64, in service.OrganizationAccessInput) (*service.DepartmentAccess, error) {
	r.calls++
	r.access = in
	return &service.DepartmentAccess{UserID: id, Organizations: *in.Organizations}, nil
}

func (r *organizationLeaderRecorder) UpdateOrganizationLeader(_ context.Context, id int64, in service.OrganizationLeaderInput) (*service.DepartmentAccess, bool, error) {
	r.calls++
	r.leader = in
	return &service.DepartmentAccess{UserID: id, DepartmentIDs: *in.DepartmentIDs}, false, nil
}

func (r *organizationLeaderRecorder) UpdateManagedMember(_ context.Context, id int64, in service.ManagedMemberUpdateInput) (*service.ManagedDepartmentMember, bool, error) {
	r.calls++
	r.memberUpdate = in
	return &service.ManagedDepartmentMember{ID: id}, false, nil
}

func serveOrganizationLeaderRequest(t *testing.T, repo *organizationLeaderRecorder, register func(*gin.Engine, *DepartmentHandler), method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	register(router, NewDepartmentHandler(service.NewDepartmentService(repo, nil)))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(method, path, strings.NewReader(body)))
	return response
}

func TestOrganizationScopeRequiresExplicitOrganizations(t *testing.T) {
	register := func(r *gin.Engine, h *DepartmentHandler) {
		r.PUT("/users/:id/organization-scope", h.SetOrganizationAccess)
	}
	for _, body := range []string{
		`{}`,
		`{"expected_version":"v"}`,
		`{"organizations":null,"expected_version":"v"}`,
		`{"organizations":["other"],"expected_version":"v"}`,
		`{"organizations":["xunyou","xunyou"],"expected_version":"v"}`,
		`{"organizations":["xunyou"],"expected_version":""}`,
		`{"organizations":["xunyou"],"expected_version":"v","replace_department_grants":"true"}`,
		`{"organizations":["xunyou"],"expected_version":"v","department_ids":[1]}`,
	} {
		t.Run(body, func(t *testing.T) {
			repo := &organizationLeaderRecorder{}
			response := serveOrganizationLeaderRequest(t, repo, register, http.MethodPut, "/users/7/organization-scope", body)
			require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
			require.Zero(t, repo.calls, "a malformed request must never be read as a revocation")
		})
	}
	repo := &organizationLeaderRecorder{}
	response := serveOrganizationLeaderRequest(t, repo, register, http.MethodPut, "/users/7/organization-scope", `{"organizations":[],"expected_version":"v"}`)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Empty(t, *repo.access.Organizations, "only an explicit [] revokes")
	require.False(t, repo.access.ReplaceDepartmentGrants)
	require.False(t, repo.access.ReplaceGlobalSubscriptions)
}

func TestOrganizationLeaderUpdateRequiresEveryField(t *testing.T) {
	register := func(r *gin.Engine, h *DepartmentHandler) { r.PUT("/leaders/:id", h.UpdateOrganizationLeader) }
	for _, body := range []string{
		`{}`,
		`{"department_ids":[1],"report":true,"reset_quota":false,"manage_members":false}`,
		`{"department_ids":null,"report":false,"reset_quota":false,"manage_members":false,"expected_version":"v"}`,
		`{"department_ids":[1],"report":null,"reset_quota":false,"manage_members":false,"expected_version":"v"}`,
		`{"department_ids":[],"report":true,"reset_quota":false,"manage_members":false,"expected_version":"v"}`,
		`{"department_ids":[1],"report":false,"reset_quota":false,"manage_members":false,"expected_version":"v"}`,
		`{"department_ids":[1,1],"report":true,"reset_quota":false,"manage_members":false,"expected_version":"v"}`,
		`{"department_ids":[1],"report":true,"reset_quota":false,"manage_members":false,"expected_version":"v","role":"admin"}`,
	} {
		t.Run(body, func(t *testing.T) {
			repo := &organizationLeaderRecorder{}
			response := serveOrganizationLeaderRequest(t, repo, register, http.MethodPut, "/leaders/9", body)
			require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
			require.Zero(t, repo.calls)
		})
	}
	repo := &organizationLeaderRecorder{}
	response := serveOrganizationLeaderRequest(t, repo, register, http.MethodPut, "/leaders/9", `{"department_ids":[3,1],"report":true,"reset_quota":false,"manage_members":true,"expected_version":"v"}`)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, []int64{1, 3}, *repo.leader.DepartmentIDs)
	require.Equal(t, []string{service.AdminPermissionOrganizationUsage, service.AdminPermissionDepartmentMembers}, repo.leader.Permissions())
	response = serveOrganizationLeaderRequest(t, repo, register, http.MethodPut, "/leaders/9", `{"department_ids":[],"report":false,"reset_quota":false,"manage_members":false,"expected_version":"v"}`)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String(), "empty departments with every capability off is a revocation")
}

func TestDepartmentManagedUpdateDistinguishesNullFromOmittedDepartment(t *testing.T) {
	register := func(r *gin.Engine, h *DepartmentHandler) { r.PUT("/members/:id", h.UpdateManagedMember) }
	repo := &organizationLeaderRecorder{}
	response := serveOrganizationLeaderRequest(t, repo, register, http.MethodPut, "/members/5", `{"expected_department_id":null,"expected_department_version":2,"username":"x"}`)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.True(t, repo.memberUpdate.ExpectedUnassigned)
	require.Nil(t, repo.memberUpdate.ExpectedDepartmentID)

	repo = &organizationLeaderRecorder{}
	response = serveOrganizationLeaderRequest(t, repo, register, http.MethodPut, "/members/5", `{"expected_department_version":2,"username":"x"}`)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.False(t, repo.memberUpdate.ExpectedUnassigned, "an omitted department is not a confirmation; the repository rejects it after the scope check")

	repo = &organizationLeaderRecorder{}
	response = serveOrganizationLeaderRequest(t, repo, register, http.MethodPut, "/members/5", `{"expected_department_id":4,"expected_department_version":2}`)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.False(t, repo.memberUpdate.ExpectedUnassigned)
	require.Equal(t, int64(4), *repo.memberUpdate.ExpectedDepartmentID)
}
