package middleware

import (
	"bytes"
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
	ID       string   `yaml:"id"`
	Match    string   `yaml:"match"`
	Keywords []string `yaml:"keywords"`
	Reply    string   `yaml:"reply"`
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
		if !cfg.Enabled || !shouldArchiveGatewayRequest(c) {
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
		path := ""
		if c.Request != nil && c.Request.URL != nil {
			path = c.Request.URL.Path
		}
		decision, ok := service.MatchRequestInterceptRules(rules, service.RequestInterceptMatchInput{
			Text:     text,
			Endpoint: path,
		})
		if !ok {
			c.Next()
			return
		}

		writeInterceptResponse(c, body, decision.Reply)
		c.Abort()
	}
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
	switch {
	case strings.Contains(path, "/chat/completions"):
		writeChatCompletionsIntercept(c, model, reply)
	case strings.Contains(path, "/v1beta/models/") || strings.Contains(path, "/antigravity/v1beta/models/"):
		writeGeminiIntercept(c, model, reply)
	case strings.Contains(path, "/responses"):
		writeResponsesIntercept(c, model, reply)
	default:
		writeMessagesIntercept(c, model, reply)
	}
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
