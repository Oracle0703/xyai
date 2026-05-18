package middleware

import (
	"errors"
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

func TestRequestInterceptGreetingResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rulesFile := writeInterceptRules(t, `
version: 1
rules:
  - id: greeting
    match: exact
    keywords: ["hi", "你好"]
    reply: "你好，我是迅游AI，有什么可以帮助你？"
`)
	called := false
	router := gin.New()
	router.Use(RequestIntercept(config.GatewayRequestInterceptConfig{
		Enabled:   true,
		RulesFile: rulesFile,
	}))
	router.POST("/v1/responses", func(c *gin.Context) {
		called = true
		c.JSON(http.StatusOK, gin.H{"upstream": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5","input":"你好"}`))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.False(t, called)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "你好，我是迅游AI")
	require.Contains(t, w.Body.String(), `"object":"response"`)
}

func TestRequestInterceptPolicyChatCompletions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rulesFile := writeInterceptRules(t, `
version: 1
rules:
  - id: policy
    match: contains
    keywords: ["示例敏感词"]
    reply: "你的问题超出法律规定，请问一些其他的。"
`)
	called := false
	router := gin.New()
	router.Use(RequestIntercept(config.GatewayRequestInterceptConfig{
		Enabled:   true,
		RulesFile: rulesFile,
	}))
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		called = true
		c.JSON(http.StatusOK, gin.H{"upstream": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5","messages":[{"role":"user","content":"这里有示例敏感词"}]}`))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.False(t, called)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "你的问题超出法律规定")
	require.Contains(t, w.Body.String(), `"chat.completion"`)
}

func TestRequestInterceptMissRestoresBodyAndCallsNext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rulesFile := writeInterceptRules(t, `
version: 1
rules:
  - id: greeting
    match: exact
    keywords: ["hi"]
    reply: "你好，我是迅游AI，有什么可以帮助你？"
`)
	router := gin.New()
	router.Use(RequestIntercept(config.GatewayRequestInterceptConfig{
		Enabled:   true,
		RulesFile: rulesFile,
	}))
	router.POST("/v1/messages", func(c *gin.Context) {
		body, err := c.GetRawData()
		require.NoError(t, err)
		c.String(http.StatusAccepted, string(body))
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-3","messages":[{"role":"user","content":"正常问题"}]}`))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusAccepted, w.Code)
	require.Contains(t, w.Body.String(), "正常问题")
}

func TestRequestInterceptProviderRulesOverrideFileFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rulesFile := writeInterceptRules(t, `
version: 1
rules:
  - id: file
    match: exact
    keywords: ["hi"]
    reply: "file reply"
`)
	called := false
	router := gin.New()
	router.Use(RequestInterceptWithProvider(config.GatewayRequestInterceptConfig{
		Enabled:   true,
		RulesFile: rulesFile,
	}, func(c *gin.Context) ([]service.RequestInterceptRuleConfig, error) {
		return []service.RequestInterceptRuleConfig{{
			ID:              "provider",
			Name:            "provider",
			Enabled:         true,
			Priority:        1,
			MatchMode:       service.RequestInterceptMatchExact,
			Keywords:        []string{"hi"},
			Reply:           "provider reply",
			Scopes:          []string{service.RequestInterceptScopeAll},
			Normalize:       service.DefaultRequestInterceptNormalization(),
			CaseInsensitive: true,
		}}, nil
	}))
	router.POST("/v1/responses", func(c *gin.Context) {
		called = true
		c.JSON(http.StatusOK, gin.H{"upstream": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5","input":"hi"}`))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.False(t, called)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "provider reply")
	require.NotContains(t, w.Body.String(), "file reply")
}

func TestRequestInterceptFallsBackToFileWhenProviderEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rulesFile := writeInterceptRules(t, `
version: 1
rules:
  - id: file
    match: exact
    keywords: ["hi"]
    reply: "file reply"
`)
	router := gin.New()
	router.Use(RequestInterceptWithProvider(config.GatewayRequestInterceptConfig{
		Enabled:   true,
		RulesFile: rulesFile,
	}, func(c *gin.Context) ([]service.RequestInterceptRuleConfig, error) {
		return nil, nil
	}))
	router.POST("/v1/responses", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"upstream": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5","input":"hi"}`))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "file reply")
}

func TestRequestInterceptFallsBackToFileWhenProviderFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rulesFile := writeInterceptRules(t, `
version: 1
rules:
  - id: file
    match: exact
    keywords: ["hi"]
    reply: "file reply"
`)
	router := gin.New()
	router.Use(RequestInterceptWithProvider(config.GatewayRequestInterceptConfig{
		Enabled:   true,
		RulesFile: rulesFile,
	}, func(c *gin.Context) ([]service.RequestInterceptRuleConfig, error) {
		return nil, errors.New("db unavailable")
	}))
	router.POST("/v1/responses", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"upstream": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5","input":"hi"}`))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "file reply")
}

func writeInterceptRules(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "request_intercept_rules.yaml")
	require.NoError(t, os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o600))
	return path
}
