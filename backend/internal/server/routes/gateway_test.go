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
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type gatewayRoutesTestOption func(*gatewayRoutesTestConfig)

type gatewayRoutesTestConfig struct {
	cfg      *config.Config
	platform string
}

func withGatewayRoutesTestConfig(cfg *config.Config) gatewayRoutesTestOption {
	return func(opts *gatewayRoutesTestConfig) {
		opts.cfg = cfg
	}
}

func withGatewayRoutesTestPlatform(platform string) gatewayRoutesTestOption {
	return func(opts *gatewayRoutesTestConfig) {
		opts.platform = platform
	}
}

func newGatewayRoutesTestRouter(options ...gatewayRoutesTestOption) *gin.Engine {
	opts := gatewayRoutesTestConfig{
		cfg:      &config.Config{},
		platform: service.PlatformOpenAI,
	}
	for _, option := range options {
		option(&opts)
	}
	if opts.cfg == nil {
		opts.cfg = &config.Config{}
	}
	if opts.platform == "" {
		opts.platform = service.PlatformOpenAI
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()

	RegisterGatewayRoutes(
		router,
		&handler.Handlers{
			Gateway:       &handler.GatewayHandler{},
			OpenAIGateway: &handler.OpenAIGatewayHandler{},
			AsyncImage:    handler.NewAsyncImageHandler(nil, nil),
		},
		servermiddleware.APIKeyAuthMiddleware(func(c *gin.Context) {
			groupID := int64(1)
			c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{
				GroupID: &groupID,
				Group:   &service.Group{Platform: opts.platform},
			})
			c.Next()
		}),
		nil,
		nil,
		nil,
		nil,
		opts.cfg,
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

func TestGatewayRoutesOpenAIAlphaSearchPathsAreRegistered(t *testing.T) {
	router := newGatewayRoutesTestRouter()
	registered := make(map[string]bool)
	for _, route := range router.Routes() {
		if route.Method == http.MethodPost {
			registered[route.Path] = true
		}
	}

	for _, path := range []string{
		"/v1/alpha/search",
		"/alpha/search",
		"/backend-api/codex/alpha/search",
	} {
		require.True(t, registered[path], "POST %s should be registered", path)
	}
}

func TestGatewayRoutesAlphaSearchRejectsNonOpenAIGroup(t *testing.T) {
	router := newGatewayRoutesTestRouter(withGatewayRoutesTestPlatform(service.PlatformGrok))
	req := httptest.NewRequest(http.MethodPost, "/v1/alpha/search", strings.NewReader(`{"model":"gpt-5.6-sol"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
	require.Contains(t, w.Body.String(), "only available for OpenAI groups")
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
	t.Cleanup(servermiddleware.CloseRequestArchiveWritersForTest)
	router := newGatewayRoutesTestRouter(withGatewayRoutesTestConfig(&config.Config{
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
	}))

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
	t.Cleanup(servermiddleware.CloseRequestArchiveWritersForTest)
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
	router := newGatewayRoutesTestRouter(withGatewayRoutesTestConfig(&config.Config{
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
	}))

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
	// 响应归档已瘦身为 usage 摘要(6dbec443): 不再写响应正文,
	// 拦截回复经捕获器只留 body_size/body_sha256。
	require.NotContains(t, records[1], "body")
	require.Greater(t, records[1]["body_size"], float64(0))
	require.NotEmpty(t, records[1]["body_sha256"])
}

func TestGatewayRoutesAsyncImagesPathsAreRegistered(t *testing.T) {
	router := newGatewayRoutesTestRouter()
	registered := make(map[string]bool)
	for _, route := range router.Routes() {
		registered[route.Method+" "+route.Path] = true
	}

	for _, route := range []string{
		"POST /v1/images/generations/async",
		"POST /v1/images/edits/async",
		"GET /v1/images/tasks/:task_id",
		"POST /images/generations/async",
		"POST /images/edits/async",
		"GET /images/tasks/:task_id",
	} {
		require.True(t, registered[route], "%s should be registered", route)
	}
}

func TestGatewayRoutesGrokImagesAndVideosPathsAreRegistered(t *testing.T) {
	router := newGatewayRoutesTestRouter(withGatewayRoutesTestPlatform(service.PlatformGrok))

	for _, path := range []string{
		"/v1/images/generations",
		"/v1/images/edits",
		"/images/generations",
		"/images/edits",
		"/v1/videos/generations",
		"/videos/generations",
		"/v1/videos/edits",
		"/videos/edits",
		"/v1/videos/extensions",
		"/videos/extensions",
	} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"grok-imagine","prompt":"draw a cat"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		require.NotEqual(t, http.StatusNotFound, w.Code, "path=%s should hit Grok media handler", path)
		require.NotContains(t, w.Body.String(), "not supported for this platform")
	}

	for _, path := range []string{
		"/v1/videos/request-123",
		"/videos/request-123",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		require.NotEqual(t, http.StatusNotFound, w.Code, "path=%s should hit Grok video handler", path)
		require.NotContains(t, w.Body.String(), "not supported for this platform")
	}
}

func TestGatewayRoutesNonGrokVideosAreRejectedAtPlatformGate(t *testing.T) {
	router := newGatewayRoutesTestRouter(withGatewayRoutesTestPlatform(service.PlatformOpenAI))

	for _, tc := range []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/v1/videos/generations", `{"model":"grok-imagine-video-1.5","prompt":"waves"}`},
		{http.MethodPost, "/videos/generations", `{"model":"grok-imagine-video-1.5","prompt":"waves"}`},
		{http.MethodPost, "/v1/videos/edits", `{"model":"grok-imagine-video","prompt":"waves","video":{"url":"https://example.com/in.mp4"}}`},
		{http.MethodPost, "/videos/edits", `{"model":"grok-imagine-video","prompt":"waves","video":{"url":"https://example.com/in.mp4"}}`},
		{http.MethodPost, "/v1/videos/extensions", `{"model":"grok-imagine-video","prompt":"waves","video":{"url":"https://example.com/in.mp4"}}`},
		{http.MethodPost, "/videos/extensions", `{"model":"grok-imagine-video","prompt":"waves","video":{"url":"https://example.com/in.mp4"}}`},
		{http.MethodGet, "/v1/videos/request-123", ""},
		{http.MethodGet, "/videos/request-123", ""},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusNotFound, w.Code, "method=%s path=%s", tc.method, tc.path)
		require.Contains(t, w.Body.String(), "Videos API is not supported for this platform")
	}
}

func TestGatewayRoutesGrokAllowsCLICompatibilityEntrypoints(t *testing.T) {
	router := newGatewayRoutesTestRouter(withGatewayRoutesTestPlatform(service.PlatformGrok))

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/v1/messages"},
		{http.MethodPost, "/v1/chat/completions"},
		{http.MethodPost, "/chat/completions"},
		{http.MethodGet, "/v1/responses"},
		{http.MethodGet, "/responses"},
		{http.MethodGet, "/backend-api/codex/responses"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{"model":"grok"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		require.NotEqual(t, http.StatusNotFound, w.Code, "method=%s path=%s", tc.method, tc.path)
		require.NotContains(t, w.Body.String(), "not supported for Grok groups")
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", strings.NewReader(`{"model":"grok","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
	require.Contains(t, w.Body.String(), "Token counting is not supported for this platform")

	for _, path := range []string{
		"/v1/responses",
		"/responses",
		"/backend-api/codex/responses",
	} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"grok","input":"hi"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		require.NotEqual(t, http.StatusNotFound, w.Code, "path=%s should still reach Responses handler", path)
	}
}

func readGatewayRouteArchiveRecords(t *testing.T, dir string) []map[string]any {
	t.Helper()
	var records []map[string]any
	require.Eventually(t, func() bool {
		records = loadGatewayRouteArchiveRecords(t, dir)
		return len(records) >= 2
	}, 2*time.Second, 5*time.Millisecond)
	return records
}

func loadGatewayRouteArchiveRecords(t *testing.T, dir string) []map[string]any {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	require.NoError(t, err)
	if len(files) == 0 {
		return nil
	}
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

func TestGatewayRoutesOpenAICountTokensPathIsRegistered(t *testing.T) {
	router := newGatewayRoutesTestRouter(withGatewayRoutesTestPlatform(service.PlatformOpenAI))

	req := httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", strings.NewReader(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	require.NotEqual(t, http.StatusNotFound, w.Code)
}
