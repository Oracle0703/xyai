package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIGatewayService_StripsUnsupportedTextFormatSchemaFormatBeforeFirstForward(t *testing.T) {
	body := []byte(`{
		"model": "gpt-5.5",
		"stream": false,
		"input": "Return structured data.",
		"metadata": {"format": "uri"},
		"text": {
			"format": {
				"type": "json_schema",
				"name": "codex_output_schema",
				"strict": true,
				"schema": {
					"type": "object",
					"properties": {
						"source_url": {"type": "string", "format": "uri"},
						"contact": {"type": "string", "format": "email"}
					},
					"required": ["source_url", "contact"],
					"additionalProperties": false
				}
			}
		}
	}`)
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		newOpenAIRejectedFieldTestResponse(http.StatusOK, `{"id":"resp_schema_ok","output":[],"usage":{"input_tokens":1,"output_tokens":1,"input_tokens_details":{"cached_tokens":0}}}`),
	}}

	result, err := newOpenAIRejectedFieldTestService(upstream).Forward(
		context.Background(),
		newOpenAIRejectedFieldTestContext(body),
		newOpenAIRejectedFieldTestAccount(),
		body,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 1)
	require.False(t, gjson.GetBytes(upstream.bodies[0], "text.format.schema.properties.source_url.format").Exists())
	require.Equal(t, "email", gjson.GetBytes(upstream.bodies[0], "text.format.schema.properties.contact.format").String())
	require.Equal(t, "uri", gjson.GetBytes(upstream.bodies[0], "metadata.format").String())
}
