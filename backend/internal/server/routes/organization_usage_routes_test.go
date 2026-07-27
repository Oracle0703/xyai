package routes

import (
	"testing"

	basehandler "github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRegisterUsageRoutes_RegistersOrganizationReportEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	h := &basehandler.Handlers{Admin: &basehandler.AdminHandlers{
		Usage:             &adminhandler.UsageHandler{},
		OrganizationUsage: &adminhandler.OrganizationUsageHandler{},
	}}
	registerUsageRoutes(engine.Group("/api/v1/admin"), h)

	routes := map[string]bool{}
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	require.True(t, routes["GET /api/v1/admin/usage/organization-report/summary"])
	require.True(t, routes["GET /api/v1/admin/usage/organization-report/periods"])
	require.True(t, routes["GET /api/v1/admin/usage/organization-report/trend"])
}
