package middleware

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
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRequestArchiveWritesCorrelatedRequestAndResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	router := gin.New()
	router.Use(RequestArchive(config.GatewayRequestArchiveConfig{
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
	records := readArchiveRecords(t, dir)
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
	router.Use(RequestArchive(config.GatewayRequestArchiveConfig{
		Enabled: false,
		Dir:     dir,
	}))
	router.POST("/v1/messages", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-3"}`))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestRequestArchiveSkipsNonModelGatewayQueries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	router := gin.New()
	router.Use(RequestArchive(config.GatewayRequestArchiveConfig{
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
	router.Use(RequestArchive(config.GatewayRequestArchiveConfig{
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
	records := readArchiveRecords(t, dir)
	require.Len(t, records, 2)
	require.Equal(t, "request-", records[0]["body"])
	require.Equal(t, true, records[0]["body_truncated"])
	require.Equal(t, "response-", records[1]["body"])
	require.Equal(t, true, records[1]["body_truncated"])
}

func readArchiveRecords(t *testing.T, dir string) []map[string]any {
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
	router.Use(RequestArchive(config.GatewayRequestArchiveConfig{
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

	records := readArchiveRecords(t, dir)
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
	router.Use(RequestArchive(config.GatewayRequestArchiveConfig{
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

	records := readArchiveRecords(t, dir)
	require.Equal(t, float64(55), records[0]["user_id"])
	require.Equal(t, float64(10), records[0]["api_key_id"])
}
