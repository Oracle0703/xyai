package promptmetrics

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

const (
	defaultMaxPromptBytes = 256 * 1024
	defaultExcerptChars   = 600
)

var sensitivePromptPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(password|passwd|pwd)\b\s*(?:[:=]|\bis\b)?\s*["']?[^"',;\s]+["']?`),
	regexp.MustCompile(`(?i)\b(token|api[_ -]?key|secret|private[_ -]?key)\b\s*(?:[:=]|\bis\b)?\s*["']?[^"',;\s]+["']?`),
	regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]{12,}`),
	regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{16,}\b`),
	regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9_]{20,}\b`),
	regexp.MustCompile(`\b[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{10,}\b`),
}

// Publisher 定义异步写入接口, middleware 只依赖该最小契约.
type Publisher interface {
	Publish(event Event)
}

// PromptCapture 是全局挂载的 AOP 采集中间件.
// 它通过路径白名单限制作用域, 确保管理端和普通前端 API 不触发提示词采集.
type PromptCapture struct {
	cfg       config.PromptMetricsConfig
	publisher Publisher
	extractor *Extractor
}

// NewPromptCapture 创建采集中间件实例.
// cfg.Enabled=false 时仍可注册, Handler 会快速旁路.
func NewPromptCapture(cfg config.PromptMetricsConfig, publisher Publisher, extractor *Extractor) *PromptCapture {
	if extractor == nil {
		extractor = NewExtractor()
	}
	return &PromptCapture{cfg: normalizeConfig(cfg), publisher: publisher, extractor: extractor}
}

// Handler 返回 gin 中间件, 在主请求完成后异步发布事件.
// 读取 body 后会恢复完整 body, 避免影响后续网关 handler.
func (m *PromptCapture) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !m.shouldCapture(c) {
			c.Next()
			return
		}
		body, truncated, ok := m.readAndResetBody(c)
		if !ok {
			c.Next()
			return
		}
		c.Next()
		if !m.shouldPublish(c) {
			return
		}
		event, ok := m.buildEvent(c, body, truncated)
		if !ok {
			return
		}
		m.publisher.Publish(event)
	}
}

func (m *PromptCapture) shouldCapture(c *gin.Context) bool {
	if m == nil || !m.cfg.Enabled || m.publisher == nil || c == nil || c.Request == nil {
		return false
	}
	if c.Request.Method != http.MethodPost {
		return false
	}
	if !strings.Contains(strings.ToLower(c.GetHeader("Content-Type")), "json") {
		return false
	}
	return isGatewayPromptPath(c.Request.URL.Path)
}

func (m *PromptCapture) readAndResetBody(c *gin.Context) ([]byte, bool, bool) {
	if c.Request.Body == nil {
		return nil, false, true
	}
	originalBody := c.Request.Body
	maxBytes := m.cfg.MaxPromptBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxPromptBytes
	}
	readLimit := maxBytes + 1
	preview, err := io.ReadAll(io.LimitReader(originalBody, readLimit))
	if err != nil {
		c.Request.Body = &replayReadCloser{
			reader: io.MultiReader(bytes.NewReader(preview), originalBody),
			closer: originalBody,
		}
		return nil, false, false
	}
	c.Request.Body = &replayReadCloser{
		reader: io.MultiReader(bytes.NewReader(preview), originalBody),
		closer: originalBody,
	}
	if int64(len(preview)) > maxBytes {
		return preview[:maxBytes], true, true
	}
	return preview, false, true
}

func (m *PromptCapture) shouldPublish(c *gin.Context) bool {
	if c == nil || c.IsAborted() || c.Writer.Status() >= http.StatusBadRequest {
		return false
	}
	apiKey, ok := servermiddleware.GetAPIKeyFromContext(c)
	return ok && apiKey != nil
}

func (m *PromptCapture) buildEvent(c *gin.Context, body []byte, bodyTruncated bool) (Event, bool) {
	extracted := m.extractor.Extract(c.Request.URL.Path, body)
	if strings.TrimSpace(extracted.Text) == "" && bodyTruncated {
		extracted = m.extractor.ExtractTruncated(c.Request.URL.Path, body)
	}
	if strings.TrimSpace(extracted.Text) == "" {
		return Event{}, false
	}
	clientCtx := DetectContext(c, body, extracted.Text)
	apiKey, _ := servermiddleware.GetAPIKeyFromContext(c)
	var userID, apiKeyID, groupID *int64
	if apiKey != nil {
		userID = ptrInt64(apiKey.UserID)
		apiKeyID = ptrInt64(apiKey.ID)
		groupID = apiKey.GroupID
	}
	requestID := resolveRequestID(c)
	promptHash := hashPrompt(extracted.Text)
	return Event{
		RequestID:             requestID,
		UserID:                userID,
		APIKeyID:              apiKeyID,
		GroupID:               groupID,
		Model:                 extracted.RequestedModel,
		RequestedModel:        extracted.RequestedModel,
		Endpoint:              c.Request.URL.Path,
		SourceProtocol:        extracted.SourceProtocol,
		PromptText:            maybeFullText(m.cfg.StoreFullText, extracted.Text),
		PromptExcerpt:         excerpt(redactSensitivePromptText(extracted.Text), defaultExcerptChars),
		PromptHash:            promptHash,
		PromptChars:           utf8.RuneCountInString(extracted.Text),
		PromptSegments:        extracted.Segments,
		PromptTokensEstimated: estimateTokens(extracted.Text),
		ProjectName:           clientCtx.ProjectName,
		GitBranch:             clientCtx.GitBranch,
		ClientName:            clientCtx.ClientName,
		ClientVersion:         clientCtx.ClientVersion,
		UserAgent:             c.GetHeader("User-Agent"),
		IPAddress:             ip.GetTrustedClientIP(c),
		Truncated:             bodyTruncated || extracted.PromptTruncated,
		AnalysisStatus:        AnalysisStatusPending,
		CreatedAt:             time.Now(),
	}, true
}

func isGatewayPromptPath(path string) bool {
	path = strings.ToLower(strings.TrimSpace(path))
	if path == "" || strings.HasPrefix(path, "/api/") {
		return false
	}
	prefixes := []string{
		"/v1/",
		"/v1beta/",
		"/responses",
		"/chat/completions",
		"/images/",
		"/backend-api/codex/",
		"/antigravity/v1/",
		"/antigravity/v1beta/",
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func resolveRequestID(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	if v, _ := c.Request.Context().Value(ctxkey.ClientRequestID).(string); strings.TrimSpace(v) != "" {
		return "client:" + strings.TrimSpace(v)
	}
	if v, _ := c.Request.Context().Value(ctxkey.RequestID).(string); strings.TrimSpace(v) != "" {
		return "local:" + strings.TrimSpace(v)
	}
	return strings.TrimSpace(c.GetHeader("X-Request-Id"))
}

func hashPrompt(text string) string {
	normalized := strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(text))), " ")
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

func excerpt(text string, maxRunes int) string {
	text = strings.TrimSpace(text)
	if maxRunes <= 0 || utf8.RuneCountInString(text) <= maxRunes {
		return text
	}
	runes := []rune(text)
	return string(runes[:maxRunes])
}

func maybeFullText(enabled bool, text string) string {
	if !enabled {
		return ""
	}
	return text
}

func redactSensitivePromptText(text string) string {
	redacted := text
	for _, pattern := range sensitivePromptPatterns {
		redacted = pattern.ReplaceAllString(redacted, "[REDACTED:value]")
	}
	return redacted
}

func estimateTokens(text string) int {
	chars := utf8.RuneCountInString(strings.TrimSpace(text))
	if chars == 0 {
		return 0
	}
	return (chars + 3) / 4
}

func ptrInt64(v int64) *int64 {
	return &v
}

func normalizeConfig(cfg config.PromptMetricsConfig) config.PromptMetricsConfig {
	if cfg.MaxPromptBytes <= 0 {
		cfg.MaxPromptBytes = defaultMaxPromptBytes
	}
	if cfg.WorkerCount <= 0 {
		cfg.WorkerCount = 4
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 1024
	}
	if cfg.WriteTimeoutSeconds <= 0 {
		cfg.WriteTimeoutSeconds = 3
	}
	return cfg
}

type replayReadCloser struct {
	reader io.Reader
	closer io.Closer
}

func (r *replayReadCloser) Read(p []byte) (int, error) {
	return r.reader.Read(p)
}

func (r *replayReadCloser) Close() error {
	if r.closer == nil {
		return nil
	}
	return r.closer.Close()
}
