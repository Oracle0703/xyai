package middleware

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRequestArchiveWritesCorrelatedRequestAndResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	router := gin.New()
	router.Use(useRequestArchive(t, config.GatewayRequestArchiveConfig{
		Enabled:              true,
		Dir:                  dir,
		MaxRequestBodyBytes:  1024,
		MaxResponseBodyBytes: 1024,
		CaptureResponse:      true,
	}))
	router.POST("/v1/messages", func(c *gin.Context) {
		body := make([]byte, c.Request.ContentLength)
		_, err := c.Request.Body.Read(body)
		require.NoError(t, err)
		c.JSON(http.StatusCreated, gin.H{"echo": strings.TrimSpace(string(body))})
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-3","messages":[{"role":"user","content":"hello"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "archive-test-client/1.0")
	req.Header.Set("Authorization", "Bearer should-not-be-written")
	req.Header.Set("Cookie", "session=should-not-be-written")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
	records := readArchiveRecords(t, dir, 2)
	require.Len(t, records, 2)
	require.Equal(t, "request", records[0]["event"])
	require.Equal(t, "response", records[1]["event"])
	require.NotEmpty(t, records[0]["archive_id"])
	require.Equal(t, records[0]["archive_id"], records[1]["archive_id"])
	require.Contains(t, records[0]["body"], `"content":"hello"`)
	require.Contains(t, records[1]["body"], `"echo"`)
	require.Equal(t, "archive-test-client/1.0", records[0]["user_agent"])

	headers, ok := records[0]["headers"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "application/json", headers["content_type"])
	require.NotContains(t, headers, "authorization")
	require.NotContains(t, headers, "cookie")
}

func TestRequestArchiveDisabledDoesNotWriteFiles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	router := gin.New()
	router.Use(useRequestArchive(t, config.GatewayRequestArchiveConfig{
		Enabled: false,
		Dir:     dir,
	}))
	var handlerBody string
	var bodyReadOK bool
	router.POST("/v1/messages", func(c *gin.Context) {
		// disabled 时中间件不得提前读取 body，handler 仍能读到完整内容。
		raw, err := io.ReadAll(c.Request.Body)
		bodyReadOK = err == nil
		handlerBody = string(raw)
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-3"}`))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, bodyReadOK)
	require.Equal(t, `{"model":"claude-3"}`, handlerBody)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestRequestArchiveRuntimeDisabledDoesNotReadBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	router := gin.New()
	provider := func(*gin.Context) (config.GatewayRequestArchiveConfig, error) {
		return config.GatewayRequestArchiveConfig{Enabled: false}, nil
	}
	router.Use(useRequestArchiveWithProvider(t, config.GatewayRequestArchiveConfig{
		Enabled: true,
		Dir:     dir,
	}, provider))
	var handlerBody string
	router.POST("/v1/messages", func(c *gin.Context) {
		raw, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		handlerBody = string(raw)
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-3"}`))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, `{"model":"claude-3"}`, handlerBody)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestRequestArchiveCaptureResponseDisabledDoesNotWrapWriter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	router := gin.New()
	router.Use(useRequestArchive(t, config.GatewayRequestArchiveConfig{
		Enabled:         true,
		Dir:             dir,
		CaptureResponse: false,
	}))
	var wrapped bool
	router.POST("/v1/messages", func(c *gin.Context) {
		_, wrapped = c.Writer.(*requestArchiveResponseWriter)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-3"}`))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.False(t, wrapped, "capture_response=false 不应包装 ResponseWriter")

	// 请求仍应被归档，但响应体不应被捕获（无 body）。
	records := readArchiveRecords(t, dir, 2)
	require.Len(t, records, 2)
	require.Equal(t, "response", records[1]["event"])
	require.NotContains(t, records[1], "body")
}

func TestAsyncRequestArchiveWriterDropsWhenQueueFull(t *testing.T) {
	// 不启动后台 goroutine，使 channel 永不被消费，模拟队列满场景。
	w := &asyncRequestArchiveWriter{
		dir: t.TempDir(),
		ch:  make(chan requestArchiveRecord, 2),
	}
	w.Enqueue(requestArchiveRecord{ArchiveID: "1"})
	w.Enqueue(requestArchiveRecord{ArchiveID: "2"})

	done := make(chan struct{})
	go func() {
		w.Enqueue(requestArchiveRecord{ArchiveID: "3"})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Enqueue blocked when queue was full")
	}

	require.Equal(t, int64(1), w.dropped.Load())
}

func TestRequestArchiveSkipsNonModelGatewayQueries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	router := gin.New()
	router.Use(useRequestArchive(t, config.GatewayRequestArchiveConfig{
		Enabled: true,
		Dir:     dir,
	}))
	router.GET("/v1/models", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"object": "list"})
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestRequestArchiveTruncatesBodies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	router := gin.New()
	router.Use(useRequestArchive(t, config.GatewayRequestArchiveConfig{
		Enabled:              true,
		Dir:                  dir,
		MaxRequestBodyBytes:  8,
		MaxResponseBodyBytes: 9,
		CaptureResponse:      true,
	}))
	router.POST("/v1/messages", func(c *gin.Context) {
		c.String(http.StatusOK, "response-body-is-long")
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader("request-body-is-long"))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	records := readArchiveRecords(t, dir, 2)
	require.Len(t, records, 2)
	require.Equal(t, "request-", records[0]["body"])
	require.Equal(t, true, records[0]["body_truncated"])
	require.Equal(t, "response-", records[1]["body"])
	require.Equal(t, true, records[1]["body_truncated"])
}

func TestRequestArchiveRestoresResponseWriterForOuterMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Next()
		require.Equal(t, http.StatusOK, c.Writer.Status())
	})
	router.Use(func(c *gin.Context) {
		originalWriter := c.Writer
		w := &releasedOnReturnWriter{ResponseWriter: originalWriter}
		defer func() {
			if c.Writer == w {
				c.Writer = originalWriter
			}
			w.ResponseWriter = nil
		}()
		c.Writer = w
		c.Next()
	})
	router.Use(useRequestArchive(t, config.GatewayRequestArchiveConfig{
		Enabled:              true,
		Dir:                  dir,
		MaxRequestBodyBytes:  1024,
		MaxResponseBodyBytes: 1024,
		CaptureResponse:      true,
	}))
	router.POST("/v1/messages", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-3"}`))
	w := httptest.NewRecorder()

	require.NotPanics(t, func() {
		router.ServeHTTP(w, req)
	})
	require.Equal(t, http.StatusOK, w.Code)
}

type releasedOnReturnWriter struct {
	gin.ResponseWriter
}

func (w *releasedOnReturnWriter) Status() int {
	if w.ResponseWriter == nil {
		panic("released writer status")
	}
	return w.ResponseWriter.Status()
}

func (w *releasedOnReturnWriter) Written() bool {
	if w.ResponseWriter == nil {
		panic("released writer written")
	}
	return w.ResponseWriter.Written()
}

func useRequestArchive(t *testing.T, cfg config.GatewayRequestArchiveConfig) gin.HandlerFunc {
	return useRequestArchiveWithProvider(t, cfg, nil)
}

func useRequestArchiveWithProvider(t *testing.T, cfg config.GatewayRequestArchiveConfig, provider RequestArchiveConfigProvider) gin.HandlerFunc {
	t.Helper()
	cfg = normalizeRequestArchiveConfig(cfg)
	if provider == nil && !cfg.Enabled {
		return RequestArchiveWithProvider(cfg, nil)
	}
	writer := newAsyncRequestArchiveWriter(cfg.Dir, cfg.QueueSize)
	removeRequestArchiveWriterForTest(writer)
	t.Cleanup(writer.Close)
	return newRequestArchiveHandler(cfg, writer, provider)
}

func removeRequestArchiveWriterForTest(target *asyncRequestArchiveWriter) {
	requestArchiveWriterMu.Lock()
	defer requestArchiveWriterMu.Unlock()
	for i, writer := range requestArchiveActiveWriters {
		if writer == target {
			requestArchiveActiveWriters = append(requestArchiveActiveWriters[:i], requestArchiveActiveWriters[i+1:]...)
			return
		}
	}
}

func readArchiveRecords(t *testing.T, dir string, want int) []map[string]any {
	t.Helper()

	var records []map[string]any
	require.Eventually(t, func() bool {
		records = loadArchiveRecords(t, dir)
		return len(records) >= want
	}, 2*time.Second, 5*time.Millisecond, "expected at least %d archive records", want)
	return records
}

func loadArchiveRecords(t *testing.T, dir string) []map[string]any {
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
	scanner.Buffer(make([]byte, 0, 1024*1024), 8*1024*1024)
	for scanner.Scan() {
		var record map[string]any
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &record))
		records = append(records, record)
	}
	require.NoError(t, scanner.Err())
	return records
}

func TestRequestArchiveIncludesIdentityWhenAvailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		groupID := int64(30)
		c.Set(string(ContextKeyUser), AuthSubject{UserID: 20, Concurrency: 1})
		c.Set(string(ContextKeyAPIKey), &service.APIKey{ID: 10, GroupID: &groupID})
		c.Set("_gateway_inbound_endpoint", "/v1/messages")
		c.Set("_gateway_selected_account_id", int64(40))
		c.Next()
	})
	router.Use(useRequestArchive(t, config.GatewayRequestArchiveConfig{
		Enabled:              true,
		Dir:                  dir,
		MaxRequestBodyBytes:  1024,
		MaxResponseBodyBytes: 1024,
		CaptureResponse:      true,
	}))
	router.POST("/v1/messages", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-3"}`))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	records := readArchiveRecords(t, dir, 2)
	require.Equal(t, float64(20), records[0]["user_id"])
	require.Equal(t, float64(10), records[0]["api_key_id"])
	require.Equal(t, float64(30), records[0]["group_id"])
	require.Equal(t, float64(40), records[1]["account_id"])
	require.Equal(t, "/v1/messages", records[0]["endpoint"])
	require.Equal(t, "claude-3", records[0]["model"])
}

func TestRequestArchiveFallsBackToAPIKeyUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(ContextKeyAPIKey), &service.APIKey{ID: 10, UserID: 55})
		c.Next()
	})
	router.Use(useRequestArchive(t, config.GatewayRequestArchiveConfig{
		Enabled:              true,
		Dir:                  dir,
		MaxRequestBodyBytes:  1024,
		MaxResponseBodyBytes: 1024,
		CaptureResponse:      true,
	}))
	router.POST("/v1/messages", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-3"}`))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	records := readArchiveRecords(t, dir, 2)
	require.Equal(t, float64(55), records[0]["user_id"])
	require.Equal(t, float64(10), records[0]["api_key_id"])
}

// TestAsyncWriterCloseDuringConcurrentEnqueueDoesNotPanic 守护 H1 修复:
// Close() 通过 quit channel 发出停止信号, 不关闭数据 channel ch, 因此即使在
// 大量并发 Enqueue 在途时调用 Close() 也不会触发 "send on closed channel" panic。
// 若有人退回到 close(w.ch) 的实现, 本用例会以 panic 崩溃, 起到回归告警作用。
func TestAsyncWriterCloseDuringConcurrentEnqueueDoesNotPanic(t *testing.T) {
	w := newAsyncRequestArchiveWriter(t.TempDir(), 8)
	removeRequestArchiveWriterForTest(w)

	const goroutines = 16
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					w.Enqueue(requestArchiveRecord{ArchiveID: "concurrent"})
				}
			}
		}()
	}

	time.Sleep(5 * time.Millisecond) // 让发送方持续在途
	w.Close()                        // 必须安全返回, 不得 panic
	close(stop)
	wg.Wait()

	// Close 后再次 Enqueue 也不得 panic(队列无人消费 -> 走 drop 分支)。
	require.NotPanics(t, func() {
		w.Enqueue(requestArchiveRecord{ArchiveID: "after-close"})
	})
	// 幂等 Close。
	require.NotPanics(t, w.Close)
}
