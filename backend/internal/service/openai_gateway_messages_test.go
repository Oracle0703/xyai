package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIGatewayServiceHandleAnthropicStreamingResponse_TopLevelUsageCacheRead(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	upstreamBody := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_stream","model":"gpt-5.5","status":"in_progress"}}`,
		``,
		`data: {"type":"response.completed","response":{"id":"resp_stream","model":"gpt-5.5","status":"completed","output":[]},"usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12,"cache_read_input_tokens":7}}`,
		``,
	}, "\n")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"x-request-id": []string{"req_stream_usage"},
		},
		Body: io.NopCloser(strings.NewReader(upstreamBody)),
	}

	svc := &OpenAIGatewayService{}
	result, err := svc.handleAnthropicStreamingResponse(resp, c, "gpt-5.5", "gpt-5.5", "gpt-5.5", time.Now())

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Stream)
	require.Equal(t, "resp_stream", result.ResponseID)
	require.Equal(t, 10, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)
	require.Equal(t, 7, result.Usage.CacheReadInputTokens)
	require.Contains(t, rec.Body.String(), `"cache_read_input_tokens":7`)
}
