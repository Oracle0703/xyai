package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

type RequestInterceptRulesProvider func(*gin.Context) ([]service.RequestInterceptRuleConfig, error)
type RequestInterceptEnabledProvider func(*gin.Context) (bool, error)

type requestInterceptRulesFile struct {
	Version int                    `yaml:"version"`
	Rules   []requestInterceptRule `yaml:"rules"`
}

type requestInterceptRule struct {
	ID         string   `yaml:"id"`
	Match      string   `yaml:"match"`
	MatchScope string   `yaml:"match_scope"`
	Keywords   []string `yaml:"keywords"`
	Reply      string   `yaml:"reply"`
}

// RequestIntercept intercepts matching model requests and returns a fixed response.
func RequestIntercept(cfg config.GatewayRequestInterceptConfig) gin.HandlerFunc {
	return RequestInterceptWithProvider(cfg, nil)
}

// RequestInterceptWithProvider intercepts matching model requests using DB-backed rules first,
// then falls back to the configured YAML rules file.
func RequestInterceptWithProvider(cfg config.GatewayRequestInterceptConfig, provider RequestInterceptRulesProvider) gin.HandlerFunc {
	return RequestInterceptWithProviders(cfg, provider, nil)
}

func RequestInterceptWithProviders(cfg config.GatewayRequestInterceptConfig, provider RequestInterceptRulesProvider, enabledProvider RequestInterceptEnabledProvider) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !cfg.Enabled || !shouldInterceptGatewayRequest(c) {
			c.Next()
			return
		}
		if enabledProvider != nil {
			enabled, err := enabledProvider(c)
			if err != nil {
				logger.L().Warn("request_intercept.load_enabled_failed", zap.Error(err))
			} else if !enabled {
				c.Next()
				return
			}
		}
		if c.Request.Body == nil {
			c.Request.Body = http.NoBody
		}
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			logger.L().Warn("request_intercept.read_body_failed", zap.Error(err))
			c.Next()
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(body))

		rules, err := loadRequestInterceptRulesFromProvider(c, cfg, provider)
		if err != nil || len(rules) == 0 {
			c.Next()
			return
		}
		text := extractRequestInterceptText(body)
		fullContextText := extractRequestInterceptFullContextText(body)
		path := ""
		if c.Request != nil && c.Request.URL != nil {
			path = c.Request.URL.Path
		}
		decision, ok := service.MatchRequestInterceptRules(rules, service.RequestInterceptMatchInput{
			Text:            text,
			FullContextText: fullContextText,
			Endpoint:        path,
		})
		if !ok {
			c.Next()
			return
		}

		writeInterceptResponse(c, body, decision.Reply)
		c.Abort()
	}
}

func shouldInterceptGatewayRequest(c *gin.Context) bool {
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
		path == "/v1/responses" ||
		strings.HasPrefix(path, "/v1/responses/") ||
		path == "/responses" ||
		strings.HasPrefix(path, "/responses/") ||
		path == "/backend-api/codex/responses" ||
		strings.HasPrefix(path, "/backend-api/codex/responses/") ||
		path == "/v1/chat/completions" ||
		path == "/chat/completions" ||
		isGeminiGenerateContentPath(path)
}

func NewRequestInterceptRulesProvider(svc *service.RequestInterceptRulesService) RequestInterceptRulesProvider {
	if svc == nil {
		return nil
	}
	return func(c *gin.Context) ([]service.RequestInterceptRuleConfig, error) {
		return svc.ListRules(c.Request.Context())
	}
}

func NewRequestInterceptEnabledProvider(svc *service.RequestInterceptRulesService) RequestInterceptEnabledProvider {
	if svc == nil {
		return nil
	}
	return func(c *gin.Context) (bool, error) {
		cfg, err := svc.GetConfig(c.Request.Context())
		return cfg.Enabled, err
	}
}

func loadRequestInterceptRules(path string) ([]requestInterceptRule, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file requestInterceptRulesFile
	if err := yaml.Unmarshal(raw, &file); err != nil {
		return nil, err
	}
	return file.Rules, nil
}

func loadRequestInterceptRulesFromProvider(c *gin.Context, cfg config.GatewayRequestInterceptConfig, provider RequestInterceptRulesProvider) ([]service.RequestInterceptRuleConfig, error) {
	if provider != nil {
		rules, err := provider(c)
		if err != nil {
			logger.L().Warn("request_intercept.load_provider_rules_failed", zap.Error(err))
		} else if len(rules) > 0 {
			return rules, nil
		}
	}
	if strings.TrimSpace(cfg.RulesFile) == "" {
		return nil, nil
	}
	fileRules, err := loadRequestInterceptRules(cfg.RulesFile)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.L().Warn("request_intercept.load_rules_failed", zap.String("rules_file", cfg.RulesFile), zap.Error(err))
		}
		return nil, err
	}
	rules := make([]service.RequestInterceptRuleConfig, 0, len(fileRules))
	for _, rule := range fileRules {
		rules = append(rules, service.RequestInterceptRuleConfig{
			ID:              strings.TrimSpace(rule.ID),
			Name:            strings.TrimSpace(firstNonEmpty(rule.ID, "YAML rule")),
			Enabled:         true,
			Priority:        0,
			MatchMode:       rule.Match,
			MatchScope:      rule.MatchScope,
			Keywords:        rule.Keywords,
			Reply:           rule.Reply,
			Scopes:          []string{service.RequestInterceptScopeAll},
			Normalize:       service.DefaultRequestInterceptNormalization(),
			CaseInsensitive: true,
		})
	}
	return rules, nil
}

func extractRequestInterceptText(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	if text := extractResponsesInputText(gjson.GetBytes(body, "input")); text != "" {
		return text
	}
	if text := extractLastUserContent(gjson.GetBytes(body, "messages")); text != "" {
		return text
	}
	if text := extractLastGeminiUserContent(gjson.GetBytes(body, "contents")); text != "" {
		return text
	}
	return ""
}

func extractRequestInterceptFullContextText(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var parts []string
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			parts = append(parts, value)
		}
	}

	add(gjson.GetBytes(body, "input").String())
	if result := gjson.GetBytes(body, "messages.#.content"); result.Exists() {
		result.ForEach(func(_, value gjson.Result) bool {
			addJSONText(value, add)
			return true
		})
	}
	if result := gjson.GetBytes(body, "contents.#.parts.#.text"); result.Exists() {
		result.ForEach(func(_, value gjson.Result) bool {
			add(value.String())
			return true
		})
	}
	if result := gjson.GetBytes(body, "system"); result.Exists() {
		addJSONText(result, add)
	}
	return strings.Join(parts, "\n")
}

func extractResponsesInputText(input gjson.Result) string {
	if !input.Exists() {
		return ""
	}
	if input.Type == gjson.String {
		return strings.TrimSpace(input.String())
	}
	if !input.IsArray() {
		return extractJSONText(input)
	}

	var fallback string
	var lastUser string
	input.ForEach(func(_, item gjson.Result) bool {
		text := extractResponsesInputItemText(item)
		if strings.TrimSpace(text) == "" {
			return true
		}
		fallback = text
		if strings.EqualFold(strings.TrimSpace(item.Get("role").String()), "user") || strings.EqualFold(strings.TrimSpace(item.Get("type").String()), "input_text") {
			lastUser = text
		}
		return true
	})
	if lastUser != "" {
		return strings.TrimSpace(lastUser)
	}
	return strings.TrimSpace(fallback)
}

func extractResponsesInputItemText(item gjson.Result) string {
	if item.Type == gjson.String {
		return item.String()
	}
	if item.Get("type").String() == "input_text" {
		return item.Get("text").String()
	}
	if text := extractJSONText(item.Get("content")); text != "" {
		return text
	}
	return extractJSONText(item)
}

func extractLastUserContent(messages gjson.Result) string {
	if !messages.IsArray() {
		return ""
	}
	var fallback string
	var lastUser string
	messages.ForEach(func(_, message gjson.Result) bool {
		text := extractJSONText(message.Get("content"))
		if strings.TrimSpace(text) == "" {
			return true
		}
		fallback = text
		if strings.EqualFold(strings.TrimSpace(message.Get("role").String()), "user") {
			lastUser = text
		}
		return true
	})
	if lastUser != "" {
		return strings.TrimSpace(lastUser)
	}
	return strings.TrimSpace(fallback)
}

func extractLastGeminiUserContent(contents gjson.Result) string {
	if !contents.IsArray() {
		return ""
	}
	var fallback string
	var lastUser string
	contents.ForEach(func(_, content gjson.Result) bool {
		text := extractGeminiPartsText(content.Get("parts"))
		if strings.TrimSpace(text) == "" {
			return true
		}
		fallback = text
		if strings.EqualFold(strings.TrimSpace(content.Get("role").String()), "user") {
			lastUser = text
		}
		return true
	})
	if lastUser != "" {
		return strings.TrimSpace(lastUser)
	}
	return strings.TrimSpace(fallback)
}

func extractGeminiPartsText(parts gjson.Result) string {
	if !parts.IsArray() {
		return ""
	}
	var values []string
	parts.ForEach(func(_, part gjson.Result) bool {
		if text := strings.TrimSpace(part.Get("text").String()); text != "" {
			values = append(values, text)
		}
		return true
	})
	return strings.Join(values, "\n")
}

func extractJSONText(value gjson.Result) string {
	var parts []string
	addJSONText(value, func(text string) {
		if text = strings.TrimSpace(text); text != "" {
			parts = append(parts, text)
		}
	})
	return strings.Join(parts, "\n")
}

func addJSONText(value gjson.Result, add func(string)) {
	switch {
	case value.Type == gjson.String:
		add(value.String())
	case value.IsArray():
		value.ForEach(func(_, item gjson.Result) bool {
			if item.Type == gjson.String {
				add(item.String())
				return true
			}
			add(item.Get("text").String())
			return true
		})
	case value.IsObject():
		add(value.Get("text").String())
	}
}

func writeInterceptResponse(c *gin.Context, body []byte, reply string) {
	path := ""
	if c.Request != nil && c.Request.URL != nil {
		path = c.Request.URL.Path
	}
	model := firstNonEmpty(
		strings.TrimSpace(gjson.GetBytes(body, "model").String()),
		extractModelFromPath(path),
		"intercept",
	)
	stream := isRequestInterceptStream(c, body)
	switch {
	case strings.Contains(path, "/chat/completions"):
		if stream {
			writeChatCompletionsStreamIntercept(c, model, reply)
			return
		}
		writeChatCompletionsIntercept(c, model, reply)
	case strings.Contains(path, "/v1beta/models/") || strings.Contains(path, "/antigravity/v1beta/models/"):
		if stream {
			writeGeminiStreamIntercept(c, model, reply)
			return
		}
		writeGeminiIntercept(c, model, reply)
	case strings.Contains(path, "/responses"):
		if stream {
			writeResponsesStreamIntercept(c, model, reply)
			return
		}
		writeResponsesIntercept(c, model, reply)
	default:
		if stream {
			writeMessagesStreamIntercept(c, model, reply)
			return
		}
		writeMessagesIntercept(c, model, reply)
	}
}

func isRequestInterceptStream(c *gin.Context, body []byte) bool {
	if gjson.GetBytes(body, "stream").Bool() {
		return true
	}
	if c == nil || c.Request == nil {
		return false
	}
	if c.Request.URL != nil && strings.Contains(c.Request.URL.Path, ":streamGenerateContent") {
		return true
	}
	return strings.Contains(strings.ToLower(c.GetHeader("Accept")), "text/event-stream")
}

func isGeminiGenerateContentPath(path string) bool {
	return (strings.HasPrefix(path, "/v1beta/models/") || strings.HasPrefix(path, "/antigravity/v1beta/models/")) &&
		(strings.Contains(path, ":generateContent") || strings.Contains(path, ":streamGenerateContent"))
}

func writeResponsesIntercept(c *gin.Context, model, reply string) {
	c.JSON(http.StatusOK, gin.H{
		"id":      "resp_intercept_" + time.Now().Format("20060102150405"),
		"object":  "response",
		"created": time.Now().Unix(),
		"model":   model,
		"output": []gin.H{{
			"type": "message",
			"role": "assistant",
			"content": []gin.H{{
				"type": "output_text",
				"text": reply,
			}},
		}},
	})
}

func writeResponsesStreamIntercept(c *gin.Context, model, reply string) {
	now := time.Now()
	responseID := "resp_intercept_" + now.Format("20060102150405")
	itemID := "msg_intercept_" + now.Format("20060102150405")
	created := now.Unix()
	content := gin.H{
		"type":        "output_text",
		"text":        reply,
		"annotations": []gin.H{},
	}
	doneItem := gin.H{
		"id":      itemID,
		"type":    "message",
		"status":  "completed",
		"role":    "assistant",
		"content": []gin.H{content},
	}
	completedResponse := gin.H{
		"id":         responseID,
		"object":     "response",
		"created":    created,
		"created_at": created,
		"status":     "completed",
		"model":      model,
		"output":     []gin.H{doneItem},
		"usage": gin.H{
			"input_tokens":  0,
			"output_tokens": 0,
			"total_tokens":  0,
		},
	}

	writeRequestInterceptSSEHeaders(c)
	writeRequestInterceptSSEJSON(c, "response.created", gin.H{
		"type": "response.created",
		"response": gin.H{
			"id":         responseID,
			"object":     "response",
			"created":    created,
			"created_at": created,
			"status":     "in_progress",
			"model":      model,
			"output":     []gin.H{},
		},
	})
	writeRequestInterceptSSEJSON(c, "response.output_item.added", gin.H{
		"type":         "response.output_item.added",
		"output_index": 0,
		"item": gin.H{
			"id":      itemID,
			"type":    "message",
			"status":  "in_progress",
			"role":    "assistant",
			"content": []gin.H{},
		},
	})
	writeRequestInterceptSSEJSON(c, "response.content_part.added", gin.H{
		"type":          "response.content_part.added",
		"item_id":       itemID,
		"output_index":  0,
		"content_index": 0,
		"part": gin.H{
			"type":        "output_text",
			"text":        "",
			"annotations": []gin.H{},
		},
	})
	writeRequestInterceptSSEJSON(c, "response.output_text.delta", gin.H{
		"type":          "response.output_text.delta",
		"item_id":       itemID,
		"output_index":  0,
		"content_index": 0,
		"delta":         reply,
	})
	writeRequestInterceptSSEJSON(c, "response.output_text.done", gin.H{
		"type":          "response.output_text.done",
		"item_id":       itemID,
		"output_index":  0,
		"content_index": 0,
		"text":          reply,
	})
	writeRequestInterceptSSEJSON(c, "response.content_part.done", gin.H{
		"type":          "response.content_part.done",
		"item_id":       itemID,
		"output_index":  0,
		"content_index": 0,
		"part":          content,
	})
	writeRequestInterceptSSEJSON(c, "response.output_item.done", gin.H{
		"type":         "response.output_item.done",
		"output_index": 0,
		"item":         doneItem,
	})
	writeRequestInterceptSSEJSON(c, "response.completed", gin.H{
		"type":     "response.completed",
		"response": completedResponse,
	})
	writeRequestInterceptSSERawData(c, "[DONE]")
	c.Writer.Flush()
}

func writeChatCompletionsIntercept(c *gin.Context, model, reply string) {
	c.JSON(http.StatusOK, gin.H{
		"id":      "chatcmpl_intercept_" + time.Now().Format("20060102150405"),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []gin.H{{
			"index": 0,
			"message": gin.H{
				"role":    "assistant",
				"content": reply,
			},
			"finish_reason": "stop",
		}},
	})
}

func writeChatCompletionsStreamIntercept(c *gin.Context, model, reply string) {
	now := time.Now()
	id := "chatcmpl_intercept_" + now.Format("20060102150405")
	created := now.Unix()
	chunk := func(delta gin.H, finishReason any) gin.H {
		return gin.H{
			"id":      id,
			"object":  "chat.completion.chunk",
			"created": created,
			"model":   model,
			"choices": []gin.H{{
				"index":         0,
				"delta":         delta,
				"finish_reason": finishReason,
			}},
		}
	}

	writeRequestInterceptSSEHeaders(c)
	writeRequestInterceptSSEJSON(c, "", chunk(gin.H{"role": "assistant"}, nil))
	writeRequestInterceptSSEJSON(c, "", chunk(gin.H{"content": reply}, nil))
	writeRequestInterceptSSEJSON(c, "", chunk(gin.H{}, "stop"))
	writeRequestInterceptSSERawData(c, "[DONE]")
	c.Writer.Flush()
}

func writeMessagesIntercept(c *gin.Context, model, reply string) {
	c.JSON(http.StatusOK, gin.H{
		"id":            "msg_intercept_" + time.Now().Format("20060102150405"),
		"type":          "message",
		"role":          "assistant",
		"model":         model,
		"content":       []gin.H{{"type": "text", "text": reply}},
		"stop_reason":   "end_turn",
		"stop_sequence": nil,
		"usage":         gin.H{"input_tokens": 0, "output_tokens": 0},
	})
}

func writeMessagesStreamIntercept(c *gin.Context, model, reply string) {
	now := time.Now()
	id := "msg_intercept_" + now.Format("20060102150405")

	writeRequestInterceptSSEHeaders(c)
	writeRequestInterceptSSEJSON(c, "message_start", gin.H{
		"type": "message_start",
		"message": gin.H{
			"id":            id,
			"type":          "message",
			"role":          "assistant",
			"model":         model,
			"content":       []gin.H{},
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage":         gin.H{"input_tokens": 0, "output_tokens": 0},
		},
	})
	writeRequestInterceptSSEJSON(c, "content_block_start", gin.H{
		"type":  "content_block_start",
		"index": 0,
		"content_block": gin.H{
			"type": "text",
			"text": "",
		},
	})
	writeRequestInterceptSSEJSON(c, "content_block_delta", gin.H{
		"type":  "content_block_delta",
		"index": 0,
		"delta": gin.H{
			"type": "text_delta",
			"text": reply,
		},
	})
	writeRequestInterceptSSEJSON(c, "content_block_stop", gin.H{
		"type":  "content_block_stop",
		"index": 0,
	})
	writeRequestInterceptSSEJSON(c, "message_delta", gin.H{
		"type": "message_delta",
		"delta": gin.H{
			"stop_reason":   "end_turn",
			"stop_sequence": nil,
		},
		"usage": gin.H{"output_tokens": 0},
	})
	writeRequestInterceptSSEJSON(c, "message_stop", gin.H{
		"type": "message_stop",
	})
	c.Writer.Flush()
}

func writeGeminiIntercept(c *gin.Context, model, reply string) {
	c.JSON(http.StatusOK, gin.H{
		"candidates": []gin.H{{
			"content": gin.H{
				"role":  "model",
				"parts": []gin.H{{"text": reply}},
			},
			"finishReason": "STOP",
		}},
		"modelVersion": model,
	})
}

func writeGeminiStreamIntercept(c *gin.Context, model, reply string) {
	writeRequestInterceptSSEHeaders(c)
	writeRequestInterceptSSEJSON(c, "", gin.H{
		"candidates": []gin.H{{
			"content": gin.H{
				"role":  "model",
				"parts": []gin.H{{"text": reply}},
			},
			"finishReason": "STOP",
		}},
		"modelVersion": model,
	})
	c.Writer.Flush()
}

func writeRequestInterceptSSEHeaders(c *gin.Context) {
	header := c.Writer.Header()
	header.Set("Content-Type", "text/event-stream; charset=utf-8")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	header.Set("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
}

func writeRequestInterceptSSEJSON(c *gin.Context, event string, data any) {
	raw, err := json.Marshal(data)
	if err != nil {
		logger.L().Warn("request_intercept.marshal_sse_failed", zap.String("event", event), zap.Error(err))
		return
	}
	if event != "" {
		if _, err := c.Writer.Write([]byte("event: " + event + "\n")); err != nil {
			logger.L().Warn("request_intercept.write_sse_failed", zap.String("event", event), zap.Error(err))
			return
		}
	}
	if _, err := c.Writer.Write(append(append([]byte("data: "), raw...), []byte("\n\n")...)); err != nil {
		logger.L().Warn("request_intercept.write_sse_failed", zap.String("event", event), zap.Error(err))
	}
}

func writeRequestInterceptSSERawData(c *gin.Context, data string) {
	if _, err := c.Writer.Write([]byte("data: " + data + "\n\n")); err != nil {
		logger.L().Warn("request_intercept.write_sse_failed", zap.String("data", data), zap.Error(err))
	}
}

func extractModelFromPath(path string) string {
	idx := strings.Index(path, "/models/")
	if idx < 0 {
		return ""
	}
	raw := strings.TrimPrefix(path[idx+len("/models/"):], "/")
	if colon := strings.Index(raw, ":"); colon >= 0 {
		raw = raw[:colon]
	}
	return strings.TrimSpace(raw)
}
