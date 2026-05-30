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
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

const (
	defaultRequestArchiveDir              = "data/request-archive"
	defaultRequestArchiveMaxRequestBytes  = int64(64 * 1024)
	defaultRequestArchiveMaxResponseBytes = int64(64 * 1024)
	defaultRequestArchiveQueueSize        = 1024
)

type RequestArchiveConfigProvider func(*gin.Context) (config.GatewayRequestArchiveConfig, error)

var (
	requestArchiveWriterMu       sync.Mutex
	requestArchiveActiveWriters  []*asyncRequestArchiveWriter
	requestArchiveRegisterWriter = registerRequestArchiveWriter
)

// RequestArchive records AI gateway request/response bodies to local JSONL files.
//
// 写入采用异步有界队列：请求热路径只做入队，后台 goroutine 持有文件句柄
// 并按日期轮转。队列满时直接丢弃归档记录，不阻塞请求。
func RequestArchive(cfg config.GatewayRequestArchiveConfig) gin.HandlerFunc {
	return RequestArchiveWithProvider(cfg, nil)
}

func RequestArchiveWithProvider(cfg config.GatewayRequestArchiveConfig, provider RequestArchiveConfigProvider) gin.HandlerFunc {
	cfg = normalizeRequestArchiveConfig(cfg)
	if provider == nil && !cfg.Enabled {
		return func(c *gin.Context) {
			c.Next()
		}
	}
	writer := newAsyncRequestArchiveWriter(cfg.Dir, cfg.QueueSize)
	return newRequestArchiveHandler(cfg, writer, provider)
}

func NewRequestArchiveConfigProvider(svc *service.SettingService) RequestArchiveConfigProvider {
	if svc == nil {
		return nil
	}
	return func(c *gin.Context) (config.GatewayRequestArchiveConfig, error) {
		return svc.GetRequestArchiveRuntimeConfig(c.Request.Context()), nil
	}
}

// archiveEnqueuer 抽象归档记录入队，便于测试注入可关闭的 writer。
type archiveEnqueuer interface {
	Enqueue(record requestArchiveRecord)
}

func newRequestArchiveHandler(cfg config.GatewayRequestArchiveConfig, writer archiveEnqueuer, provider RequestArchiveConfigProvider) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !shouldArchiveGatewayRequest(c) {
			c.Next()
			return
		}

		runtimeCfg := cfg
		if provider != nil {
			providedCfg, err := provider(c)
			if err != nil {
				logger.L().Warn("request_archive.load_runtime_config_failed", zap.Error(err))
			} else {
				runtimeCfg = mergeRequestArchiveRuntimeConfig(cfg, providedCfg)
			}
		}
		runtimeCfg = normalizeRequestArchiveConfig(runtimeCfg)
		if !runtimeCfg.Enabled {
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
		requestRecord.Body, requestRecord.BodySize, requestRecord.BodySHA256, requestRecord.BodyTruncated = captureBytes(body, runtimeCfg.MaxRequestBodyBytes)
		requestRecord.Model = firstNonEmpty(requestRecord.Model, strings.TrimSpace(gjson.GetBytes(body, "model").String()))
		writer.Enqueue(requestRecord)

		var captureWriter *requestArchiveResponseWriter
		originalWriter := c.Writer
		if runtimeCfg.CaptureResponse {
			captureWriter = newRequestArchiveResponseWriter(c.Writer, runtimeCfg.MaxResponseBodyBytes)
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
		writer.Enqueue(responseRecord)
	}
}

// mergeRequestArchiveRuntimeConfig 用运行态配置覆盖基础配置中真正"按请求生效"的字段。
// 注意: Dir 与 QueueSize 在 asyncRequestArchiveWriter 构造时即固化, 运行态无法改变
// 实际落盘目录与队列容量, 因此这里不合并这两个字段(避免产生"已生效"的错觉)。
// 仅 Enabled / CaptureResponse / Max*BodyBytes 在请求热路径按运行态取值生效。
func mergeRequestArchiveRuntimeConfig(base, runtime config.GatewayRequestArchiveConfig) config.GatewayRequestArchiveConfig {
	runtime.Dir = base.Dir
	runtime.QueueSize = base.QueueSize
	if runtime.MaxRequestBodyBytes <= 0 {
		runtime.MaxRequestBodyBytes = base.MaxRequestBodyBytes
	}
	if runtime.MaxResponseBodyBytes <= 0 {
		runtime.MaxResponseBodyBytes = base.MaxResponseBodyBytes
	}
	return runtime
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
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = defaultRequestArchiveQueueSize
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

// asyncRequestArchiveWriter 以后台单 goroutine + 有界 channel 的方式写入归档。
// 请求热路径只调用 Enqueue，永不阻塞；channel 满时直接丢弃并累计 dropped。
// 后台 writer 持有当日文件句柄，跨天时关闭旧句柄并重新打开，避免每条记录 open/close。
type asyncRequestArchiveWriter struct {
	dir     string
	ch      chan requestArchiveRecord
	quit    chan struct{}
	done    chan struct{}
	closing atomic.Bool
	dropped atomic.Int64

	file     *os.File
	fileDate string
}

func newAsyncRequestArchiveWriter(dir string, queueSize int) *asyncRequestArchiveWriter {
	if queueSize <= 0 {
		queueSize = defaultRequestArchiveQueueSize
	}
	w := &asyncRequestArchiveWriter{
		dir:  dir,
		ch:   make(chan requestArchiveRecord, queueSize),
		quit: make(chan struct{}),
		done: make(chan struct{}),
	}
	go w.run()
	requestArchiveRegisterWriter(w)
	return w
}

func registerRequestArchiveWriter(w *asyncRequestArchiveWriter) {
	requestArchiveWriterMu.Lock()
	requestArchiveActiveWriters = append(requestArchiveActiveWriters, w)
	requestArchiveWriterMu.Unlock()
}

// CloseRequestArchiveWritersForTest drains and closes writers created by route-level tests.
func CloseRequestArchiveWritersForTest() {
	requestArchiveWriterMu.Lock()
	writers := requestArchiveActiveWriters
	requestArchiveActiveWriters = nil
	requestArchiveWriterMu.Unlock()
	for _, writer := range writers {
		writer.Close()
	}
}

// Enqueue 非阻塞入队；队列满时丢弃记录，避免拖慢请求尾延迟。
func (w *asyncRequestArchiveWriter) Enqueue(record requestArchiveRecord) {
	if w == nil {
		return
	}
	select {
	case w.ch <- record:
	default:
		dropped := w.dropped.Add(1)
		if dropped%256 == 1 {
			logger.L().Warn("request_archive.queue_full_dropped",
				zap.Int64("dropped_total", dropped),
				zap.Int("queue_size", cap(w.ch)))
		}
	}
}

func (w *asyncRequestArchiveWriter) run() {
	defer close(w.done)
	defer func() {
		if w.file != nil {
			_ = w.file.Close()
			w.file = nil
		}
	}()
	for {
		select {
		case record := <-w.ch:
			if err := w.writeRecord(record); err != nil {
				logger.L().Warn("request_archive.write_failed", zap.Error(err))
			}
		case <-w.quit:
			// 收到停止信号后尽力排空已入队记录, 再退出。
			for {
				select {
				case record := <-w.ch:
					if err := w.writeRecord(record); err != nil {
						logger.L().Warn("request_archive.write_failed", zap.Error(err))
					}
				default:
					return
				}
			}
		}
	}
}

// Close 停止后台 writer 并关闭文件句柄。
// 通过关闭独立的 quit channel 发出停止信号, 不关闭数据 channel ch ——
// 因为 ch 有多个并发发送者(Enqueue), 由非发送方关闭 ch 会触发
// "send on closed channel" panic。改用 quit 信号后, Enqueue 永不 panic,
// Close 可安全用于生产优雅停机。
func (w *asyncRequestArchiveWriter) Close() {
	if w == nil {
		return
	}
	if !w.closing.CompareAndSwap(false, true) {
		<-w.done
		return
	}
	close(w.quit)
	<-w.done
}

func (w *asyncRequestArchiveWriter) writeRecord(record requestArchiveRecord) error {
	line, err := json.Marshal(record)
	if err != nil {
		return err
	}
	f, err := w.fileForToday()
	if err != nil {
		return err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	return nil
}

// fileForToday 返回当日归档文件句柄，跨天时轮转。
func (w *asyncRequestArchiveWriter) fileForToday() (*os.File, error) {
	today := time.Now().Format("2006-01-02")
	if w.file != nil && w.fileDate == today {
		return w.file, nil
	}
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}
	if err := os.MkdirAll(w.dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(w.dir, today+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	w.file = f
	w.fileDate = today
	return f, nil
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
