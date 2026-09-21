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

type departmentAssignmentRecorder struct {
	service.DepartmentRepository
	input service.DepartmentAssignInput
	calls int
}

func (r *departmentAssignmentRecorder) Assign(_ context.Context, input service.DepartmentAssignInput) (int, error) {
	r.input = input
	r.calls++
	return len(input.Members), nil
}

func TestDepartmentAssignmentRequiresExplicitObjectAndTarget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, body := range []string{
		`null`, `[]`, `{}`, `{"members":[{"user_id":1}]}`,
		`{"department_id":0,"members":[{"user_id":1}]}`,
		`{"department_id":"7","members":[{"user_id":1}]}`,
		`{"department_id":null,"members":[]}`,
		`{"department_id":null,"members":[{"user_id":1},{"user_id":1}]}`,
		`{"department_id":null,"members":[{"user_id":1,"expected_department_version":-1}]}`,
		`{"department_id":null,"members":[{"user_id":1}],"unknown":true}`,
		`{"department_id":null,"members":[{"user_id":1}]} {}`,
	} {
		t.Run(body, func(t *testing.T) {
			repo := &departmentAssignmentRecorder{}
			h := NewDepartmentHandler(service.NewDepartmentService(repo))
			router := gin.New()
			router.POST("/assign", h.Assign)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/assign", strings.NewReader(body)))
			require.Equal(t, http.StatusBadRequest, response.Code)
			require.Zero(t, repo.calls)
		})
	}
	repo := &departmentAssignmentRecorder{}
	h := NewDepartmentHandler(service.NewDepartmentService(repo))
	router := gin.New()
	router.POST("/assign", h.Assign)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/assign", strings.NewReader(`{"department_id":null,"members":[{"user_id":1,"expected_department_id":7,"expected_department_version":2}]}`)))
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, 1, repo.calls)
	require.Nil(t, repo.input.DepartmentID, "explicit null clears membership")
	require.Equal(t, int64(2), repo.input.Members[0].ExpectedVersion)
	require.Equal(t, int64(7), *repo.input.Members[0].ExpectedDepartmentID)
}
