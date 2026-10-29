//go:build unit

package middleware

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestScopedAdminKeyPreservesDepartmentIdentityAndRouteScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	settings := service.NewSettingService(nil, nil)
	settings.SetScopedAdminKeyRepository(&adminAuthKeyRepo{})
	key, item, err := settings.CreateScopedAdminKey(context.Background(), service.ScopedAdminKeyInput{
		Name: "subscriptions", Enabled: true,
		Policy: adminaccess.Policy{GlobalPermissions: []string{"subscriptions.read"}},
	}, 9)
	require.NoError(t, err)
	users := service.NewUserService(&stubUserRepo{getByID: func(context.Context, int64) (*service.User, error) {
		return &service.User{ID: 9, Role: service.RoleAdmin, Status: service.StatusActive}, nil
	}}, nil, nil, nil)
	r := gin.New()
	r.Use(gin.HandlerFunc(NewAdminAuthMiddleware(nil, users, settings, nil)))
	r.GET("/api/v1/admin/subscriptions", func(c *gin.Context) {
		ctx := c.Request.Context()
		require.Equal(t, int64(9), service.DepartmentActorID(ctx))
		require.False(t, service.DepartmentActorIsAdminAPIKey(ctx), "scoped keys must retain creator status revalidation")
		principal := adminaccess.FromContext(ctx)
		require.NotNil(t, principal)
		require.Equal(t, item.ID, principal.KeyID)
		require.Equal(t, []string{"subscriptions.read"}, principal.Policy.GlobalPermissions)
		c.Status(204)
	})
	r.GET("/api/v1/admin/departments", func(c *gin.Context) { t.Fatal("local route must remain denied") })
	for _, tc := range []struct {
		path string
		code int
	}{{"/api/v1/admin/subscriptions", 204}, {"/api/v1/admin/departments", 403}} {
		req := httptest.NewRequest("GET", tc.path, nil)
		req.Header.Set("x-api-key", key)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, tc.code, w.Code)
	}
}
