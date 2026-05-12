package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIGatewayService_ResponsesFallsBackToRawChatCompletionsForOpenAICompatibleAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"glm-5.1","stream":false,"input":[{"role":"user","content":"hello"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_responses_compat"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"chatcmpl_compat_1","object":"chat.completion","created":1710000000,"model":"glm-5.1","choices":[{"index":0,"message":{"role":"assistant","content":"hello back"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`,
		)),
	}}

	svc := &OpenAIGatewayService{
		cfg: &config.Config{
			Security: config.SecurityConfig{
				URLAllowlist: config.URLAllowlistConfig{
					Enabled:           false,
					AllowInsecureHTTP: true,
				},
			},
		},
		httpUpstream: upstream,
	}
	account := &Account{
		ID:          201,
		Name:        "openai-compatible-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "http://upstream.example",
		},
		Extra: map[string]any{
			"openai_responses_supported": false,
		},
	}

	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "http://upstream.example/v1/chat/completions", upstream.lastReq.URL.String())
	require.Equal(t, 3, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)
	require.Equal(t, "response", gjson.GetBytes(rec.Body.Bytes(), "object").String())
	require.Equal(t, "hello back", gjson.GetBytes(rec.Body.Bytes(), "output.0.content.0.text").String())
}

func TestOpenAIGatewayService_ResponsesStreamFallsBackToRawChatCompletionsForOpenAICompatibleAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"glm-5.1","stream":true,"input":[{"role":"user","content":"hello"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"id":"chatcmpl_stream_1","object":"chat.completion.chunk","created":1710000000,"model":"glm-5.1","choices":[{"index":0,"delta":{"content":"hello"}}]}`,
		"",
		`data: {"id":"chatcmpl_stream_1","object":"chat.completion.chunk","created":1710000000,"model":"glm-5.1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":2,"total_tokens":6}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_responses_compat_stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}

	svc := &OpenAIGatewayService{
		cfg: &config.Config{
			Security: config.SecurityConfig{
				URLAllowlist: config.URLAllowlistConfig{
					Enabled:           false,
					AllowInsecureHTTP: true,
				},
			},
		},
		httpUpstream: upstream,
	}
	account := &Account{
		ID:          202,
		Name:        "openai-compatible-apikey-stream",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "http://upstream.example",
		},
		Extra: map[string]any{
			"openai_responses_supported": false,
		},
	}

	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, upstream.lastReq)
	upstreamReqBody, err := io.ReadAll(upstream.lastReq.Body)
	require.NoError(t, err)
	require.True(t, gjson.GetBytes(upstreamReqBody, "stream_options.include_usage").Bool())
	require.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
	require.Contains(t, rec.Body.String(), `event: response.created`)
	require.Contains(t, rec.Body.String(), `event: response.output_item.added`)
	require.Contains(t, rec.Body.String(), `event: response.content_part.added`)
	require.Contains(t, rec.Body.String(), `event: response.output_text.delta`)
	require.Contains(t, rec.Body.String(), `event: response.output_text.done`)
	require.Contains(t, rec.Body.String(), `event: response.content_part.done`)
	require.Contains(t, rec.Body.String(), `event: response.output_item.done`)
	require.Contains(t, rec.Body.String(), `event: response.completed`)
	require.Contains(t, rec.Body.String(), `"output_index":0`)
	require.Contains(t, rec.Body.String(), `"content_index":0`)
	require.Contains(t, rec.Body.String(), `"item":{"type":"message","id":"item_msg_0","role":"assistant","content":[],"status":"in_progress"}`)
	require.Contains(t, rec.Body.String(), `"part":{"type":"output_text","text":""}`)
	require.Contains(t, rec.Body.String(), `"item_id":"item_`)
	require.Contains(t, rec.Body.String(), `"item":{"type":"message","id":"item_msg_0","role":"assistant","content":[{"type":"output_text","text":"hello"}],"status":"completed"}`)
	require.Contains(t, rec.Body.String(), `"output":[{"type":"message","id":"item_msg_0","role":"assistant","content":[{"type":"output_text","text":"hello"}],"status":"completed"}]`)
	require.Contains(t, rec.Body.String(), `data: [DONE]`)
	require.Equal(t, 4, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)
}
