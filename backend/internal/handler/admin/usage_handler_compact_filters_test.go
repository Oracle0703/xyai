//go:build unit

package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUsageHandlerCompactFilterEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adminService := newStubAdminService()
	handler := NewUsageHandler(nil, nil, adminService, nil)
	router := gin.New()
	router.GET("/admin/usage/search-accounts", handler.SearchAccounts)
	router.GET("/admin/usage/search-groups", handler.SearchGroups)

	t.Run("accounts expose only id and name", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/admin/usage/search-accounts?q=acc", nil)
		router.ServeHTTP(recorder, request)

		require.Equal(t, http.StatusOK, recorder.Code)
		require.JSONEq(t, `{"code":0,"message":"success","data":[{"id":3,"name":"account"}]}`, recorder.Body.String())
		require.Equal(t, "acc", adminService.lastListAccounts.search)
	})

	t.Run("groups expose only id and name", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/admin/usage/search-groups", nil)
		router.ServeHTTP(recorder, request)

		require.Equal(t, http.StatusOK, recorder.Code)
		require.JSONEq(t, `{"code":0,"message":"success","data":[{"id":2,"name":"group"}]}`, recorder.Body.String())
	})
}
