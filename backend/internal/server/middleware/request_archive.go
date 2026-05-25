package middleware

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

const (
	defaultRequestArchiveDir              = "data/request-archive"
	defaultRequestArchiveMaxRequestBytes  = int64(64 * 1024)
	defaultRequestArchiveMaxResponseBytes = int64(64 * 1024)
)

// RequestArchive records AI gateway request/response bodies to local JSONL files.
func RequestArchive(cfg config.GatewayRequestArchiveConfig) gin.HandlerFunc {
	cfg = normalizeRequestArchiveConfig(cfg)
	writer := &requestArchiveFileWriter{dir: cfg.Dir}

	return func(c *gin.Context) {
		if !cfg.Enabled || !shouldArchiveGatewayRequest(c) {
			c.Next()
			return
		}

		startedAt := time.Now()
		archiveID := newArchiveID()
		if c.Request.Body == nil {
			c.Request.Body = http.NoBody
		}
		body, readErr := io.ReadAll(c.Request.Body)
		if readErr != nil {
			logger.L().Warn("request_archive.read_body_failed", zap.Error(readErr))
			c.Next()
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(body))

		requestRecord := buildArchiveRecord(c, archiveID, "request", startedAt)
		requestRecord.Body, requestRecord.BodySize, requestRecord.BodySHA256, requestRecord.BodyTruncated = captureBytes(body, cfg.MaxRequestBodyBytes)
		requestRecord.Model = firstNonEmpty(requestRecord.Model, strings.TrimSpace(gjson.GetBytes(body, "model").String()))
		if err := writer.Write(requestRecord); err != nil {
			logger.L().Warn("request_archive.write_request_failed", zap.Error(err))
		}

		var captureWriter *requestArchiveResponseWriter
		originalWriter := c.Writer
		if cfg.CaptureResponse {
			captureWriter = newRequestArchiveResponseWriter(c.Writer, cfg.MaxResponseBodyBytes)
			c.Writer = captureWriter
			defer func() {
				if c.Writer == captureWriter {
					c.Writer = originalWriter
				}
			}()
		}

		c.Next()

		responseRecord := buildArchiveRecord(c, archiveID, "response", time.Now())
		responseRecord.DurationMs = time.Since(startedAt).Milliseconds()
		if c.Writer != nil {
			responseRecord.Status = c.Writer.Status()
		}
		if captureWriter != nil {
			responseRecord.Body = captureWriter.Body()
			responseRecord.BodySize = captureWriter.BodySize()
			responseRecord.BodySHA256 = captureWriter.BodySHA256()
			responseRecord.BodyTruncated = captureWriter.Truncated()
			responseRecord.Stream = captureWriter.Stream()
		}
		if err := writer.Write(responseRecord); err != nil {
			logger.L().Warn("request_archive.write_response_failed", zap.Error(err))
		}
	}
}

func shouldArchiveGatewayRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return false
	}
	if c.Request.Method != http.MethodPost {
		return false
	}
	path := strings.TrimRight(strings.TrimSpace(c.Request.URL.Path), "/")
	if path == "" {
		return false
	}
	return path == "/v1/messages" ||
		path == "/antigravity/v1/messages" ||
		path == "/v1/messages/count_tokens" ||
		path == "/antigravity/v1/messages/count_tokens" ||
		path == "/v1/responses" ||
		strings.HasPrefix(path, "/v1/responses/") ||
		path == "/responses" ||
		strings.HasPrefix(path, "/responses/") ||
		path == "/backend-api/codex/responses" ||
		strings.HasPrefix(path, "/backend-api/codex/responses/") ||
		path == "/v1/chat/completions" ||
		path == "/chat/completions" ||
		strings.HasPrefix(path, "/v1beta/models/") ||
		strings.HasPrefix(path, "/antigravity/v1beta/models/") ||
		path == "/v1/images/generations" ||
		path == "/v1/images/edits" ||
		path == "/images/generations" ||
		path == "/images/edits"
}

func normalizeRequestArchiveConfig(cfg config.GatewayRequestArchiveConfig) config.GatewayRequestArchiveConfig {
	if strings.TrimSpace(cfg.Dir) == "" {
		cfg.Dir = defaultRequestArchiveDir
	}
	if cfg.MaxRequestBodyBytes <= 0 {
		cfg.MaxRequestBodyBytes = defaultRequestArchiveMaxRequestBytes
	}
	if cfg.MaxResponseBodyBytes <= 0 {
		cfg.MaxResponseBodyBytes = defaultRequestArchiveMaxResponseBytes
	}
	return cfg
}

type requestArchiveRecord struct {
	ArchiveID     string         `json:"archive_id"`
	Event         string         `json:"event"`
	Timestamp     string         `json:"timestamp"`
	Method        string         `json:"method,omitempty"`
	Path          string         `json:"path,omitempty"`
	Endpoint      string         `json:"endpoint,omitempty"`
	UserID        *int64         `json:"user_id,omitempty"`
	APIKeyID      *int64         `json:"api_key_id,omitempty"`
	GroupID       *int64         `json:"group_id,omitempty"`
	AccountID     *int64         `json:"account_id,omitempty"`
	Model         string         `json:"model,omitempty"`
	ClientIP      string         `json:"client_ip,omitempty"`
	UserAgent     string         `json:"user_agent,omitempty"`
	Headers       map[string]any `json:"headers,omitempty"`
	Status        int            `json:"status,omitempty"`
	DurationMs    int64          `json:"duration_ms,omitempty"`
	Stream        bool           `json:"stream,omitempty"`
	Body          string         `json:"body,omitempty"`
	BodySize      int64          `json:"body_size"`
	BodySHA256    string         `json:"body_sha256,omitempty"`
	BodyTruncated bool           `json:"body_truncated,omitempty"`
}

func buildArchiveRecord(c *gin.Context, archiveID, event string, ts time.Time) requestArchiveRecord {
	record := requestArchiveRecord{
		ArchiveID: archiveID,
		Event:     event,
		Timestamp: ts.UTC().Format(time.RFC3339Nano),
		Headers:   map[string]any{},
	}
	if c == nil || c.Request == nil {
		return record
	}
	record.Method = c.Request.Method
	if c.Request.URL != nil {
		record.Path = c.Request.URL.Path
	}
	record.Endpoint = getStringContext(c, "_gateway_inbound_endpoint")
	record.Model = getStringContext(c, "ops_model")
	record.ClientIP = clientIP(c.Request)
	record.UserAgent = strings.TrimSpace(c.GetHeader("User-Agent"))
	record.Headers = archiveHeaderAllowlist(c.Request.Header)

	if subject, ok := GetAuthSubjectFromContext(c); ok && subject.UserID > 0 {
		record.UserID = ptrInt64(subject.UserID)
	}
	if apiKey, ok := GetAPIKeyFromContext(c); ok && apiKey != nil {
		if record.UserID == nil && apiKey.UserID > 0 {
			record.UserID = ptrInt64(apiKey.UserID)
		}
		if apiKey.ID > 0 {
			record.APIKeyID = ptrInt64(apiKey.ID)
		}
		if apiKey.GroupID != nil {
			record.GroupID = apiKey.GroupID
		}
	}
	if accountID := getInt64Context(c, "ops_account_id"); accountID > 0 {
		record.AccountID = ptrInt64(accountID)
	} else if accountID := getInt64Context(c, "_gateway_selected_account_id"); accountID > 0 {
		record.AccountID = ptrInt64(accountID)
	}
	return record
}

func archiveHeaderAllowlist(h http.Header) map[string]any {
	headers := map[string]any{}
	allowed := map[string]string{
		"Content-Type":        "content_type",
		"Accept":              "accept",
		"User-Agent":          "user_agent",
		"X-Request-Id":        "x_request_id",
		"X-Client-Request-Id": "x_client_request_id",
		"X-Forwarded-For":     "x_forwarded_for",
		"X-Real-Ip":           "x_real_ip",
		"Openai-Organization": "openai_organization",
		"Anthropic-Version":   "anthropic_version",
		"Anthropic-Beta":      "anthropic_beta",
	}
	for headerName, outputName := range allowed {
		if value := strings.TrimSpace(h.Get(headerName)); value != "" {
			headers[outputName] = value
		}
	}
	return headers
}

func captureBytes(data []byte, limit int64) (string, int64, string, bool) {
	sum := sha256.Sum256(data)
	size := int64(len(data))
	if limit < 0 {
		limit = 0
	}
	truncated := size > limit
	if truncated {
		data = data[:limit]
	}
	return string(data), size, hex.EncodeToString(sum[:]), truncated
}

type requestArchiveFileWriter struct {
	dir string
	mu  sync.Mutex
}

func (w *requestArchiveFileWriter) Write(record requestArchiveRecord) error {
	if w == nil {
		return nil
	}
	if err := os.MkdirAll(w.dir, 0o700); err != nil {
		return err
	}
	line, err := json.Marshal(record)
	if err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	path := filepath.Join(w.dir, time.Now().Format("2006-01-02")+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	return nil
}

type requestArchiveResponseWriter struct {
	gin.ResponseWriter
	buf       bytes.Buffer
	hasher    hashAccumulator
	limit     int64
	size      int64
	truncated bool
	stream    bool
}

func newRequestArchiveResponseWriter(w gin.ResponseWriter, limit int64) *requestArchiveResponseWriter {
	if limit < 0 {
		limit = 0
	}
	h := sha256.New()
	return &requestArchiveResponseWriter{
		ResponseWriter: w,
		hasher:         h,
		limit:          limit,
	}
}

func (w *requestArchiveResponseWriter) Write(data []byte) (int, error) {
	w.capture(data)
	return w.ResponseWriter.Write(data)
}

func (w *requestArchiveResponseWriter) WriteString(s string) (int, error) {
	w.capture([]byte(s))
	return w.ResponseWriter.WriteString(s)
}

func (w *requestArchiveResponseWriter) WriteHeaderNow() {
	if strings.Contains(strings.ToLower(w.Header().Get("Content-Type")), "text/event-stream") {
		w.stream = true
	}
	w.ResponseWriter.WriteHeaderNow()
}

func (w *requestArchiveResponseWriter) Flush() {
	w.stream = true
	w.ResponseWriter.Flush()
}

func (w *requestArchiveResponseWriter) capture(data []byte) {
	if len(data) == 0 {
		return
	}
	_, _ = w.hasher.Write(data)
	w.size += int64(len(data))
	if w.limit == 0 {
		w.truncated = true
		return
	}
	remaining := w.limit - int64(w.buf.Len())
	if remaining <= 0 {
		w.truncated = true
		return
	}
	if int64(len(data)) > remaining {
		w.buf.Write(data[:remaining])
		w.truncated = true
		return
	}
	w.buf.Write(data)
}

func (w *requestArchiveResponseWriter) Body() string {
	return w.buf.String()
}

func (w *requestArchiveResponseWriter) BodySize() int64 {
	return w.size
}

func (w *requestArchiveResponseWriter) BodySHA256() string {
	if w.hasher == nil {
		return ""
	}
	return hex.EncodeToString(w.hasher.Sum(nil))
}

func (w *requestArchiveResponseWriter) Truncated() bool {
	return w.truncated
}

func (w *requestArchiveResponseWriter) Stream() bool {
	return w.stream || strings.Contains(strings.ToLower(w.Header().Get("Content-Type")), "text/event-stream")
}

type hashAccumulator interface {
	io.Writer
	Sum([]byte) []byte
}

func newArchiveID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err == nil {
		return hex.EncodeToString(b[:])
	}
	return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
}

func clientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		if first := strings.TrimSpace(strings.Split(xff, ",")[0]); first != "" {
			return first
		}
	}
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func getStringContext(c *gin.Context, key string) string {
	if c == nil || key == "" {
		return ""
	}
	v, ok := c.Get(key)
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func getInt64Context(c *gin.Context, key string) int64 {
	if c == nil || key == "" {
		return 0
	}
	v, ok := c.Get(key)
	if !ok {
		return 0
	}
	switch t := v.(type) {
	case int64:
		return t
	case int:
		return int64(t)
	case int32:
		return int64(t)
	default:
		return 0
	}
}

func ptrInt64(v int64) *int64 {
	return &v
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
