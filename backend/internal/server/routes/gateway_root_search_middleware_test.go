package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRootXSearchUsesRequestArchiveBeforeRequestIntercept(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	var handlerNames []string
	router.Use(func(c *gin.Context) {
		if c.Request.URL.Path == "/x_search" {
			handlerNames = c.HandlerNames()
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	handlers := &handler.Handlers{
		Gateway:       &handler.GatewayHandler{},
		OpenAIGateway: &handler.OpenAIGatewayHandler{},
		AsyncImage:    &handler.AsyncImageHandler{},
		BatchImage:    &handler.BatchImageHandler{},
	}
	apiKeyAuth := servermiddleware.APIKeyAuthMiddleware(func(c *gin.Context) { c.Next() })
	RegisterGatewayRoutes(router, handlers, apiKeyAuth, nil, nil, nil, nil, nil, &config.Config{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/x_search", strings.NewReader(`{"query":"codex"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusNoContent, recorder.Code)

	archiveIndex := handlerNameIndex(handlerNames, "RequestArchiveWithProvider")
	interceptIndex := handlerNameIndex(handlerNames, "RequestInterceptWithProviders")
	require.NotEqual(t, -1, archiveIndex, "root /x_search handler chain must include RequestArchive: %v", handlerNames)
	require.NotEqual(t, -1, interceptIndex, "root /x_search handler chain must include RequestIntercept: %v", handlerNames)
	require.Less(t, archiveIndex, interceptIndex, "RequestArchive must run before RequestIntercept: %v", handlerNames)
}

func handlerNameIndex(handlerNames []string, fragment string) int {
	for index, name := range handlerNames {
		if strings.Contains(name, fragment) {
			return index
		}
	}
	return -1
}
