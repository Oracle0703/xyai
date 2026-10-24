package routes

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSubAdminPermissionWhitelistMatchesRegisteredAdminRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	passthrough := func(c *gin.Context) { c.Next() }
	RegisterAdminRoutes(
		router.Group("/api/v1"),
		&handler.Handlers{Admin: &handler.AdminHandlers{}},
		servermiddleware.AdminAuthMiddleware(passthrough),
		servermiddleware.AuditLogMiddleware(passthrough),
		servermiddleware.StepUpAuthMiddleware(passthrough),
		nil,
		nil,
	)
	registered := map[string]struct{}{}
	for _, route := range router.Routes() {
		registered[route.Method+" "+route.Path] = struct{}{}
	}

	for _, item := range service.AdminPermissionCatalog() {
		for _, route := range service.AdminPermissionRouteTemplates(item.Code) {
			_, ok := registered[route]
			require.True(t, ok, "%s whitelists unregistered route %s", item.Code, route)
		}
	}
}
