package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type managedDepartmentRecorder struct {
	service.DepartmentRepository
	calls int
	input service.ManagedMemberUpdateInput
	err   error
}

func (r *managedDepartmentRecorder) UpdateManagedMember(_ context.Context, id int64, in service.ManagedMemberUpdateInput) (*service.ManagedDepartmentMember, bool, error) {
	r.calls++
	r.input = in
	if r.err != nil {
		return nil, false, r.err
	}
	return &service.ManagedDepartmentMember{ID: id, DepartmentID: *in.ExpectedDepartmentID, DepartmentVersion: *in.ExpectedDepartmentVersion}, false, nil
}

func (r *managedDepartmentRecorder) ManagedAssign(_ context.Context, in service.DepartmentAssignInput) (int, error) {
	r.calls++
	return len(in.Members), r.err
}

func (r *managedDepartmentRecorder) ManagedCandidates(context.Context, int64, string) ([]service.DepartmentMember, error) {
	r.calls++
	return []service.DepartmentMember{}, r.err
}

func TestDepartmentManagedUpdateRejectsInvalidAndUnknownFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, body := range []string{
		`null`, `[]`, `{}`, `{"expected_department_id":7,"username":"new"}`,
		`{"expected_department_id":7,"expected_department_version":-1}`,
		`{"expected_department_id":7,"expected_department_version":0} {}`,
		`{"expected_department_id":7,"expected_department_version":0,"status":"banned"}`,
		`{"expected_department_id":7,"expected_department_version":0,"notes":"administrator secret"}`,
		`{"expected_department_id":7,"expected_department_version":0,"email":"other@example.com"}`,
		`{"expected_department_id":7,"expected_department_version":0,"role":"admin"}`,
		`{"expected_department_id":7,"expected_department_version":0,"password":"x"}`,
		`{"expected_department_id":7,"expected_department_version":0,"balance":100}`,
		`{"expected_department_id":7,"expected_department_version":0,"disabled_source":"department_manager"}`,
	} {
		t.Run(body, func(t *testing.T) {
			repo := &managedDepartmentRecorder{}
			h := NewDepartmentHandler(service.NewDepartmentService(repo, nil))
			router := gin.New()
			router.PUT("/members/:id", h.UpdateManagedMember)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPut, "/members/42", strings.NewReader(body)))
			require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
			require.Zero(t, repo.calls)
		})
	}
}

func TestDepartmentManagedUpdateKeepsExplicitFieldMaskAndMapsErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name   string
		err    error
		status int
		reason string
	}{
		{"success", nil, http.StatusOK, ""},
		{"scope", service.ErrDepartmentScopeDenied, http.StatusForbidden, "DEPARTMENT_SCOPE_DENIED"},
		{"version", service.ErrDepartmentConflict, http.StatusConflict, "DEPARTMENT_CONFLICT"},
		{"enable", service.ErrDepartmentMemberEnableDenied, http.StatusForbidden, "DEPARTMENT_MEMBER_ENABLE_DENIED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &managedDepartmentRecorder{err: tc.err}
			h := NewDepartmentHandler(service.NewDepartmentService(repo, nil))
			router := gin.New()
			router.PUT("/members/:id", h.UpdateManagedMember)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPut, "/members/42", strings.NewReader(`{"expected_department_id":7,"expected_department_version":0,"department_note":""}`)))
			require.Equal(t, tc.status, response.Code, response.Body.String())
			require.Equal(t, 1, repo.calls)
			require.NotNil(t, repo.input.DepartmentNote, "empty string is an explicit note clear")
			require.Empty(t, *repo.input.DepartmentNote)
			require.Nil(t, repo.input.Username)
			require.Nil(t, repo.input.Status, "editing a note must not write back a stale status")
			if tc.reason != "" {
				require.Contains(t, response.Body.String(), tc.reason)
			}
		})
	}
}

func TestDepartmentManagedAssignmentUsesStrictExistingDecoder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, body := range []string{
		`null`, `{}`, `{"members":[{"user_id":42}]}`,
		`{"department_id":null,"members":[{"user_id":42,"notes":"secret"}]}`,
		`{"department_id":null,"members":[{"user_id":42}]} {}`,
	} {
		t.Run(body, func(t *testing.T) {
			repo := &managedDepartmentRecorder{}
			h := NewDepartmentHandler(service.NewDepartmentService(repo, nil))
			router := gin.New()
			router.POST("/assign", h.ManagedAssign)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/assign", strings.NewReader(body)))
			require.Equal(t, http.StatusBadRequest, response.Code)
			require.Zero(t, repo.calls)
		})
	}
}

func TestDepartmentManagedCandidatesRequireDepartmentAndQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, query := range []string{"", "?department_id=all&q=ab", "?department_id=0&q=ab", "?department_id=7&q=a"} {
		t.Run(query, func(t *testing.T) {
			repo := &managedDepartmentRecorder{}
			h := NewDepartmentHandler(service.NewDepartmentService(repo, nil))
			router := gin.New()
			router.GET("/candidates", h.ManagedCandidates)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/candidates"+query, nil))
			require.Equal(t, http.StatusBadRequest, response.Code)
			require.Zero(t, repo.calls)
			var envelope map[string]any
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
		})
	}
}
