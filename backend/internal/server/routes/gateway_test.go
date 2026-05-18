package routes

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newGatewayRoutesTestRouter() *gin.Engine {
	return newGatewayRoutesTestRouterWithConfig(&config.Config{})
}

func newGatewayRoutesTestRouterWithConfig(cfg *config.Config) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	if cfg == nil {
		cfg = &config.Config{}
	}

	RegisterGatewayRoutes(
		router,
		&handler.Handlers{
			Gateway:       &handler.GatewayHandler{},
			OpenAIGateway: &handler.OpenAIGatewayHandler{},
		},
		servermiddleware.APIKeyAuthMiddleware(func(c *gin.Context) {
			groupID := int64(1)
			c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{
				GroupID: &groupID,
				Group:   &service.Group{Platform: service.PlatformOpenAI},
			})
			c.Next()
		}),
		nil,
		nil,
		nil,
		nil,
		cfg,
	)

	return router
}

func TestGatewayRoutesOpenAIResponsesCompactPathIsRegistered(t *testing.T) {
	router := newGatewayRoutesTestRouter()

	for _, path := range []string{
		"/v1/responses/compact",
		"/responses/compact",
		"/backend-api/codex/responses",
		"/backend-api/codex/responses/compact",
	} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"gpt-5"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		require.NotEqual(t, http.StatusNotFound, w.Code, "path=%s should hit OpenAI responses handler", path)
	}
}

func TestGatewayRoutesOpenAIImagesPathsAreRegistered(t *testing.T) {
	router := newGatewayRoutesTestRouter()

	for _, path := range []string{
		"/v1/images/generations",
		"/v1/images/edits",
		"/images/generations",
		"/images/edits",
	} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"gpt-image-2","prompt":"draw a cat"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		require.NotEqual(t, http.StatusNotFound, w.Code, "path=%s should hit OpenAI images handler", path)
	}
}

func TestGatewayRoutesRequestArchiveRunsForOpenAIResponsesAlias(t *testing.T) {
	dir := t.TempDir()
	router := newGatewayRoutesTestRouterWithConfig(&config.Config{
		Gateway: config.GatewayConfig{
			MaxBodySize: 1024 * 1024,
			RequestArchive: config.GatewayRequestArchiveConfig{
				Enabled:              true,
				Dir:                  dir,
				MaxRequestBodyBytes:  1024,
				MaxResponseBodyBytes: 1024,
				CaptureResponse:      true,
			},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/backend-api/codex/responses", strings.NewReader(`{"model":"gpt-5","input":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "route-archive-test/1.0")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.NotEqual(t, http.StatusNotFound, w.Code)
	records := readGatewayRouteArchiveRecords(t, dir)
	require.Len(t, records, 2)
	require.Equal(t, "request", records[0]["event"])
	require.Equal(t, "response", records[1]["event"])
	require.Equal(t, records[0]["archive_id"], records[1]["archive_id"])
	require.Equal(t, "/v1/responses", records[0]["endpoint"])
	require.Contains(t, records[0]["body"], `"input":"hello"`)
}

func TestGatewayRoutesRequestInterceptRunsAfterArchive(t *testing.T) {
	dir := t.TempDir()
	rulesFile := filepath.Join(dir, "rules.yaml")
	require.NoError(t, os.WriteFile(rulesFile, []byte(`
version: 1
rules:
  - id: greeting
    match: exact
    keywords: ["hi"]
    reply: "你好，我是迅游AI，有什么可以帮助你？"
`), 0o600))
	archiveDir := filepath.Join(dir, "archive")
	router := newGatewayRoutesTestRouterWithConfig(&config.Config{
		Gateway: config.GatewayConfig{
			MaxBodySize: 1024 * 1024,
			RequestArchive: config.GatewayRequestArchiveConfig{
				Enabled:              true,
				Dir:                  archiveDir,
				MaxRequestBodyBytes:  1024,
				MaxResponseBodyBytes: 1024,
				CaptureResponse:      true,
			},
			RequestIntercept: config.GatewayRequestInterceptConfig{
				Enabled:   true,
				RulesFile: rulesFile,
			},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5","input":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "迅游AI")
	records := readGatewayRouteArchiveRecords(t, archiveDir)
	require.Len(t, records, 2)
	require.Equal(t, "request", records[0]["event"])
	require.Equal(t, "response", records[1]["event"])
	require.Contains(t, records[1]["body"], "迅游AI")
}

func readGatewayRouteArchiveRecords(t *testing.T, dir string) []map[string]any {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	require.NoError(t, err)
	require.Len(t, files, 1)

	f, err := os.Open(files[0])
	require.NoError(t, err)
	defer f.Close()

	var records []map[string]any
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var record map[string]any
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &record))
		records = append(records, record)
	}
	require.NoError(t, scanner.Err())
	return records
}
