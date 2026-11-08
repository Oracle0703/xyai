package managementmcp

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 本地订阅/部门仓储按 DepartmentActor 判定操作者并对缺失身份 fail-closed；
// MCP admin 内部分发必须像 JWT/管理密钥一样注入，user scope 不注入。
func TestMCPAdminDispatchSetsDepartmentActor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var adminActor, userActor int64 = -1, -1
	inner := gin.New()
	inner.GET("/api/v1/admin/subscriptions", DispatchAuth(true), func(c *gin.Context) {
		adminActor = service.DepartmentActorID(c.Request.Context())
		c.JSON(http.StatusOK, gin.H{"data": []any{}})
	})
	inner.GET("/api/v1/keys", DispatchAuth(false), func(c *gin.Context) {
		userActor = service.DepartmentActorID(c.Request.Context())
		c.JSON(http.StatusOK, gin.H{"data": []any{}})
	})
	r, _ := newTestEndpoint(inner)

	_, result := rpc(t, r, "admin-reader", "tools/call", map[string]any{"name": "admin_list_subscriptions", "arguments": map[string]any{}})
	require.NotContains(t, result, "error")
	require.Equal(t, int64(3), adminActor)

	_, result = rpc(t, r, "writer", "tools/call", map[string]any{"name": "user_list_api_keys", "arguments": map[string]any{}})
	require.NotContains(t, result, "error")
	require.Equal(t, int64(0), userActor)
}
