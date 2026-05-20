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

func TestRequestInterceptUsesLastUserMessageForChatCompletions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rulesFile := writeInterceptRules(t, `
version: 1
rules:
  - id: greeting
    match: exact
    keywords: ["hi", "hello", "你好", "您好"]
    reply: "greeting reply"
  - id: policy
    match: contains
    keywords: ["台独"]
    reply: "policy reply"
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

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{
		"model":"gpt-5",
		"messages":[
			{"role":"system","content":"历史策略说明包含台独这个词"},
			{"role":"user","content":"之前聊过台独"},
			{"role":"assistant","content":"历史回复"},
			{"role":"user","content":"hi"}
		]
	}`))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.False(t, called)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "greeting reply")
	require.NotContains(t, w.Body.String(), "policy reply")
}

func TestRequestInterceptUsesLastUserInputForResponsesArray(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rulesFile := writeInterceptRules(t, `
version: 1
rules:
  - id: greeting
    match: exact
    keywords: ["hi", "hello", "你好", "您好"]
    reply: "greeting reply"
  - id: policy
    match: contains
    keywords: ["台独"]
    reply: "policy reply"
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

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{
		"model":"gpt-5",
		"input":[
			{"role":"user","content":[{"type":"input_text","text":"之前聊过台独"}]},
			{"role":"assistant","content":[{"type":"output_text","text":"历史回复"}]},
			{"role":"user","content":[{"type":"input_text","text":"hi"}]}
		]
	}`))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.False(t, called)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "greeting reply")
	require.NotContains(t, w.Body.String(), "policy reply")
}

func TestRequestInterceptFullContextScopeCanMatchHistory(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rulesFile := writeInterceptRules(t, `
version: 1
rules:
  - id: full-context-policy
    match: contains
    match_scope: full_context
    keywords: ["台独"]
    reply: "policy reply"
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

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{
		"model":"gpt-5",
		"messages":[
			{"role":"user","content":"之前聊过台独"},
			{"role":"assistant","content":"历史回复"},
			{"role":"user","content":"hi"}
		]
	}`))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.False(t, called)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "policy reply")
}

func TestRequestInterceptResponsesStreamReturnsSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rulesFile := writeInterceptRules(t, `
version: 1
rules:
  - id: stream
    match: contains
    keywords: ["intercept-test"]
    reply: "intercepted locally"
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

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.5","input":"please trigger intercept-test","stream":true}`))
	req.Header.Set("Accept", "text/event-stream")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.False(t, called)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Header().Get("Content-Type"), "text/event-stream")
	require.Contains(t, w.Body.String(), "event: response.completed")
	require.Contains(t, w.Body.String(), "event: response.output_text.delta")
	require.Contains(t, w.Body.String(), "intercepted locally")
	require.Contains(t, w.Body.String(), "data: [DONE]")
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

func TestRequestInterceptChatCompletionsStreamReturnsSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rulesFile := writeInterceptRules(t, `
version: 1
rules:
  - id: stream
    match: contains
    keywords: ["intercept-test"]
    reply: "chat stream intercepted"
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

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.5","messages":[{"role":"user","content":"please trigger intercept-test"}],"stream":true}`))
	req.Header.Set("Accept", "text/event-stream")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.False(t, called)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Header().Get("Content-Type"), "text/event-stream")
	require.Contains(t, w.Body.String(), `"object":"chat.completion.chunk"`)
	require.Contains(t, w.Body.String(), "chat stream intercepted")
	require.Contains(t, w.Body.String(), `"finish_reason":"stop"`)
	require.Contains(t, w.Body.String(), "data: [DONE]")
}

func TestRequestInterceptMessagesStreamReturnsSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rulesFile := writeInterceptRules(t, `
version: 1
rules:
  - id: stream
    match: contains
    keywords: ["intercept-test"]
    reply: "message stream intercepted"
`)
	called := false
	router := gin.New()
	router.Use(RequestIntercept(config.GatewayRequestInterceptConfig{
		Enabled:   true,
		RulesFile: rulesFile,
	}))
	router.POST("/v1/messages", func(c *gin.Context) {
		called = true
		c.JSON(http.StatusOK, gin.H{"upstream": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-3","messages":[{"role":"user","content":"please trigger intercept-test"}],"stream":true}`))
	req.Header.Set("Accept", "text/event-stream")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.False(t, called)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Header().Get("Content-Type"), "text/event-stream")
	require.Contains(t, w.Body.String(), "event: message_start")
	require.Contains(t, w.Body.String(), "event: content_block_delta")
	require.Contains(t, w.Body.String(), "message stream intercepted")
	require.Contains(t, w.Body.String(), "event: message_stop")
}

func TestRequestInterceptGeminiStreamGenerateContentReturnsSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rulesFile := writeInterceptRules(t, `
version: 1
rules:
  - id: stream
    match: contains
    keywords: ["intercept-test"]
    reply: "gemini stream intercepted"
`)
	called := false
	router := gin.New()
	router.Use(RequestIntercept(config.GatewayRequestInterceptConfig{
		Enabled:   true,
		RulesFile: rulesFile,
	}))
	router.POST("/v1beta/models/gemini-2.5-pro:streamGenerateContent", func(c *gin.Context) {
		called = true
		c.JSON(http.StatusOK, gin.H{"upstream": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-pro:streamGenerateContent", strings.NewReader(`{"contents":[{"role":"user","parts":[{"text":"please trigger intercept-test"}]}]}`))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.False(t, called)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Header().Get("Content-Type"), "text/event-stream")
	require.Contains(t, w.Body.String(), `"text":"gemini stream intercepted"`)
	require.Contains(t, w.Body.String(), `"finishReason":"STOP"`)
}

func TestRequestInterceptSkipsCountTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rulesFile := writeInterceptRules(t, `
version: 1
rules:
  - id: all
    match: contains
    keywords: ["intercept-test"]
    reply: "should not intercept"
`)
	called := false
	router := gin.New()
	router.Use(RequestIntercept(config.GatewayRequestInterceptConfig{
		Enabled:   true,
		RulesFile: rulesFile,
	}))
	router.POST("/v1/messages/count_tokens", func(c *gin.Context) {
		called = true
		c.JSON(http.StatusAccepted, gin.H{"upstream": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", strings.NewReader(`{"model":"claude-3","messages":[{"role":"user","content":"please trigger intercept-test"}]}`))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.True(t, called)
	require.Equal(t, http.StatusAccepted, w.Code)
	require.Contains(t, w.Body.String(), `"upstream":true`)
	require.NotContains(t, w.Body.String(), "should not intercept")
}

func TestRequestInterceptSkipsImages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rulesFile := writeInterceptRules(t, `
version: 1
rules:
  - id: all
    match: contains
    keywords: ["intercept-test"]
    reply: "should not intercept"
`)
	called := false
	router := gin.New()
	router.Use(RequestIntercept(config.GatewayRequestInterceptConfig{
		Enabled:   true,
		RulesFile: rulesFile,
	}))
	router.POST("/v1/images/generations", func(c *gin.Context) {
		called = true
		c.JSON(http.StatusAccepted, gin.H{"upstream": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"gpt-image-1","prompt":"please trigger intercept-test"}`))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.True(t, called)
	require.Equal(t, http.StatusAccepted, w.Code)
	require.Contains(t, w.Body.String(), `"upstream":true`)
	require.NotContains(t, w.Body.String(), "should not intercept")
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

func TestRequestInterceptRuntimeDisabledCallsNext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	called := false
	router := gin.New()
	router.Use(RequestInterceptWithProviders(config.GatewayRequestInterceptConfig{
		Enabled: true,
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
	}, func(c *gin.Context) (bool, error) {
		return false, nil
	}))
	router.POST("/v1/responses", func(c *gin.Context) {
		called = true
		c.JSON(http.StatusAccepted, gin.H{"upstream": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5","input":"hi"}`))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.True(t, called)
	require.Equal(t, http.StatusAccepted, w.Code)
	require.Contains(t, w.Body.String(), `"upstream":true`)
	require.NotContains(t, w.Body.String(), "provider reply")
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
