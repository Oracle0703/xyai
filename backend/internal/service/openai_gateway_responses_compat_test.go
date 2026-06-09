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

func TestOpenAIGatewayService_ResponsesCompatSanitizesUnsupportedRequestParameters(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.5","stream":true,"temperature":0.7,"max_output_tokens":1000,"input":"hello","tools":[{"type":"function","name":"lookup","description":"Lookup data","parameters":{"type":"object"}}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_responses_compat_params"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"id":"chatcmpl_params_1","object":"chat.completion.chunk","created":1710000000,"model":"gpt-5.5","choices":[{"index":0,"delta":{"content":"ok"}}]}`,
			"",
			`data: {"id":"chatcmpl_params_1","object":"chat.completion.chunk","created":1710000000,"model":"gpt-5.5","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
			"",
			"data: [DONE]",
			"",
		}, "\n"))),
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
		ID:          204,
		Name:        "openai-compatible-apikey-params",
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
	require.Equal(t, "gpt-5.5", gjson.GetBytes(upstreamReqBody, "model").String())
	require.True(t, gjson.GetBytes(upstreamReqBody, "stream").Bool())
	require.Equal(t, int64(1000), gjson.GetBytes(upstreamReqBody, "max_completion_tokens").Int())
	require.False(t, gjson.GetBytes(upstreamReqBody, "max_output_tokens").Exists())
	require.False(t, gjson.GetBytes(upstreamReqBody, "temperature").Exists())
	require.Equal(t, "hello", gjson.GetBytes(upstreamReqBody, `messages.#(role=="user").content`).String())
	require.Equal(t, "function", gjson.GetBytes(upstreamReqBody, "tools.0.type").String())
	require.Equal(t, "lookup", gjson.GetBytes(upstreamReqBody, "tools.0.function.name").String())
}

func TestOpenAIGatewayService_ResponsesCompatPreservesCompatibleCacheUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"glm-5.1","stream":false,"input":[{"role":"user","content":"hello"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_responses_compat_cache"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"chatcmpl_compat_cache","object":"chat.completion","created":1710000000,"model":"glm-5.1","choices":[{"index":0,"message":{"role":"assistant","content":"hello back"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12,"cache_read_input_tokens":4,"cache_creation_input_tokens":6,"cached_tokens":99}}`,
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
		ID:          203,
		Name:        "openai-compatible-apikey-cache",
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
	require.Equal(t, 10, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)
	require.Equal(t, 4, result.Usage.CacheReadInputTokens)
	require.Equal(t, 6, result.Usage.CacheCreationInputTokens)
	require.Equal(t, int64(4), gjson.GetBytes(rec.Body.Bytes(), "usage.input_tokens_details.cached_tokens").Int())
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

	frames := parseTestOpenAIResponsesSSEFrames(rec.Body.String())
	added := findTestOpenAIResponsesSSEFrame(t, frames, "response.output_item.added")
	addedData := gjson.Parse(added.data)
	messageID := addedData.Get("item.id").String()
	require.NotEmpty(t, messageID)
	require.Equal(t, "message", addedData.Get("item.type").String())
	require.Equal(t, "assistant", addedData.Get("item.role").String())
	require.Equal(t, "in_progress", addedData.Get("item.status").String())
	require.Equal(t, int64(0), addedData.Get("output_index").Int())
	require.Equal(t, "output_text", addedData.Get("item.content.0.type").String())

	partAdded := gjson.Parse(findTestOpenAIResponsesSSEFrame(t, frames, "response.content_part.added").data)
	require.Equal(t, messageID, partAdded.Get("item_id").String())
	require.Equal(t, int64(0), partAdded.Get("content_index").Int())
	require.Equal(t, "output_text", partAdded.Get("part.type").String())
	require.Equal(t, "", partAdded.Get("part.text").String())

	textDelta := gjson.Parse(findTestOpenAIResponsesSSEFrame(t, frames, "response.output_text.delta").data)
	require.Equal(t, messageID, textDelta.Get("item_id").String())
	require.Equal(t, "hello", textDelta.Get("delta").String())

	textDone := gjson.Parse(findTestOpenAIResponsesSSEFrame(t, frames, "response.output_text.done").data)
	require.Equal(t, messageID, textDone.Get("item_id").String())
	require.Equal(t, "hello", textDone.Get("text").String())

	partDone := gjson.Parse(findTestOpenAIResponsesSSEFrame(t, frames, "response.content_part.done").data)
	require.Equal(t, messageID, partDone.Get("item_id").String())
	require.Equal(t, "hello", partDone.Get("part.text").String())

	itemDone := gjson.Parse(findTestOpenAIResponsesSSEFrame(t, frames, "response.output_item.done").data)
	require.Equal(t, messageID, itemDone.Get("item.id").String())
	require.Equal(t, "completed", itemDone.Get("item.status").String())
	require.Equal(t, "hello", itemDone.Get("item.content.0.text").String())

	completed := gjson.Parse(findTestOpenAIResponsesSSEFrame(t, frames, "response.completed").data)
	require.Equal(t, messageID, completed.Get("response.output.0.id").String())
	require.Equal(t, "hello", completed.Get("response.output.0.content.0.text").String())
	require.Contains(t, rec.Body.String(), `data: [DONE]`)
	require.Equal(t, 4, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)
}

type testOpenAIResponsesSSEFrame struct {
	event string
	data  string
}

func parseTestOpenAIResponsesSSEFrames(body string) []testOpenAIResponsesSSEFrame {
	blocks := strings.Split(body, "\n\n")
	frames := make([]testOpenAIResponsesSSEFrame, 0, len(blocks))
	for _, block := range blocks {
		var frame testOpenAIResponsesSSEFrame
		for _, line := range strings.Split(block, "\n") {
			line = strings.TrimSpace(line)
			if event, ok := extractOpenAISSEEventLine(line); ok {
				frame.event = event
				continue
			}
			if data, ok := extractOpenAISSEDataLine(line); ok {
				frame.data = strings.TrimSpace(data)
			}
		}
		if frame.event != "" || frame.data != "" {
			frames = append(frames, frame)
		}
	}
	return frames
}

func findTestOpenAIResponsesSSEFrame(t *testing.T, frames []testOpenAIResponsesSSEFrame, event string) testOpenAIResponsesSSEFrame {
	t.Helper()
	for _, frame := range frames {
		if frame.event == event {
			require.NotEmpty(t, frame.data)
			return frame
		}
	}
	require.Failf(t, "missing SSE event", "event %q not found in frames: %#v", event, frames)
	return testOpenAIResponsesSSEFrame{}
}
