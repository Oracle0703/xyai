package promptmetrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestIsGatewayPromptPath(t *testing.T) {
	cases := map[string]bool{
		"/v1/chat/completions":                  true,
		"/responses":                            true,
		"/backend-api/codex/responses":          true,
		"/antigravity/v1/messages":              true,
		"/api/v1/admin/prompt-metrics/overview": false,
		"/api/v1/admin/users":                   false,
		"/health":                               false,
	}
	for path, want := range cases {
		require.Equal(t, want, isGatewayPromptPath(path), path)
	}
}

func TestPromptCaptureShouldCaptureRequiresPostJSON(t *testing.T) {
	capture := NewPromptCapture(testPromptMetricsConfig(), &capturePublisherStub{}, NewExtractor())
	req, err := http.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	require.True(t, capture.shouldCapture(testGinContext(req)))

	req, err = http.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	require.False(t, capture.shouldCapture(testGinContext(req)))

	req, err = http.NewRequest(http.MethodPost, "/api/v1/admin/prompt-metrics/overview", nil)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	require.False(t, capture.shouldCapture(testGinContext(req)))
}

func TestPromptCaptureSkipsAbortedAndUnauthenticatedRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	publisher := &capturePublisherStub{}
	capture := NewPromptCapture(testPromptMetricsConfig(), publisher, NewExtractor())
	router := gin.New()
	router.Use(capture.Handler())
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		c.AbortWithStatus(http.StatusUnauthorized)
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"messages":[{"role":"user","content":"secret prompt"}]}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(httptest.NewRecorder(), req)

	require.Empty(t, publisher.events)

	router = gin.New()
	router.Use(capture.Handler())
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"messages":[{"role":"user","content":"secret prompt"}]}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(httptest.NewRecorder(), req)

	require.Empty(t, publisher.events)
}

func TestPromptCaptureReadsBodyWithinCaptureLimitAndRestoresBody(t *testing.T) {
	capture := NewPromptCapture(testPromptMetricsConfig(), &capturePublisherStub{}, NewExtractor())
	body := &countingReadCloser{reader: strings.NewReader(strings.Repeat("a", 4096))}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	c := testGinContext(req)

	preview, truncated, ok := capture.readAndResetBody(c)
	require.True(t, ok)
	require.True(t, truncated)
	require.Len(t, preview, int(capture.cfg.MaxPromptBytes))
	require.LessOrEqual(t, body.readBytes, int(capture.cfg.MaxPromptBytes)+1)

	restored, err := io.ReadAll(c.Request.Body)
	require.NoError(t, err)
	require.Len(t, restored, 4096)
}

func TestPromptCaptureRedactsExcerptAndDefaultsFullTextOff(t *testing.T) {
	capture := NewPromptCapture(config.PromptMetricsConfig{
		Enabled:             true,
		MaxPromptBytes:      1024,
		WorkerCount:         1,
		QueueSize:           1,
		WriteTimeoutSeconds: 1,
	}, &capturePublisherStub{}, NewExtractor())
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	c := testGinContext(req)
	c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{ID: 9, UserID: 7})

	event, ok := capture.buildEvent(c, []byte(`{"messages":[{"role":"user","content":"my password is hunter2 and token sk-test-abcdefghijklmnopqrstuvwxyz"}]}`), false)

	require.True(t, ok)
	require.Empty(t, event.PromptText)
	require.NotContains(t, strings.ToLower(event.PromptExcerpt), "hunter2")
	require.NotContains(t, event.PromptExcerpt, "sk-test-abcdefghijklmnopqrstuvwxyz")
	require.Contains(t, event.PromptExcerpt, "[REDACTED:")
}

func TestPromptCaptureBuildsTruncatedEventFromPreview(t *testing.T) {
	capture := NewPromptCapture(testPromptMetricsConfig(), &capturePublisherStub{}, NewExtractor())
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	c := testGinContext(req)
	c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{ID: 9, UserID: 7})
	body := []byte(`{"messages":[{"role":"user","content":"` + strings.Repeat("x", 1500) + `"}`)

	event, ok := capture.buildEvent(c, body[:capture.cfg.MaxPromptBytes], true)

	require.True(t, ok)
	require.True(t, event.Truncated)
	require.NotEmpty(t, event.PromptText)
	require.True(t, strings.HasPrefix(strings.Repeat("x", 1500), event.PromptText))
}

type capturePublisherStub struct {
	events []Event
}

func (s *capturePublisherStub) Publish(event Event) {
	s.events = append(s.events, event)
}

type countingReadCloser struct {
	reader    *strings.Reader
	readBytes int
}

func (r *countingReadCloser) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.readBytes += n
	return n, err
}

func (r *countingReadCloser) Close() error {
	return nil
}

func testPromptMetricsConfig() config.PromptMetricsConfig {
	return config.PromptMetricsConfig{
		Enabled:             true,
		StoreFullText:       true,
		MaxPromptBytes:      1024,
		WorkerCount:         1,
		QueueSize:           1,
		WriteTimeoutSeconds: 1,
	}
}

func testGinContext(req *http.Request) *gin.Context {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	return c
}
