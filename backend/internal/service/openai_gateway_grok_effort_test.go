//go:build unit

package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGrokAutoEffortInputs(t *testing.T) {
	const weKnora = `"chat_template_kwargs":{"enable_thinking":true}`
	for _, tt := range []struct {
		name, fields    string
		chat, responses bool // converted to high under that protocol
	}{
		{"auto", `"reasoning_effort":"auto"`, true, true},
		{"spaced auto", `"reasoning_effort":" Auto "`, true, true},
		{"nested auto", `"reasoning":{"effort":"auto","summary":"detailed"}`, true, true},
		{"camel auto", `"reasoningEffort":"auto"`, true, true},
		{"WeKnora", weKnora, true, true},
		{"WeKnora with other kwargs", `"chat_template_kwargs":{"enable_thinking":true,"keep":1}`, true, true},
		{"auto with WeKnora switch", `"reasoning_effort":"auto",` + weKnora, true, true},
		{"protocol precedence", `"reasoning":{"effort":"auto"},"reasoning_effort":"low"`, false, true},
		// Everything below is a normal call and must reach the forwarders unchanged.
		{"missing", `"stream":false`, false, false},
		{"low with WeKnora switch", `"reasoning_effort":"low",` + weKnora, false, false},
		{"explicit none", `"reasoning_effort":"none",` + weKnora, false, false},
		{"null effort", `"reasoning_effort":null,` + weKnora, false, false},
		{"empty effort", `"reasoning_effort":"",` + weKnora, false, false},
		{"bad effort", `"reasoning_effort":"invalid",` + weKnora, false, false},
		{"WeKnora off", `"chat_template_kwargs":{"enable_thinking":false,"keep":1}`, false, false},
		{"unrelated kwargs", `"chat_template_kwargs":{"keep":1}`, false, false},
		{"top-level switch", `"enable_thinking":true`, false, false},
		{"OpenRouter switch", `"reasoning":{"enabled":true}`, false, false},
		{"OpenRouter off", `"reasoning":{"enabled":false}`, false, false},
		{"thinking type", `"thinking":{"type":"enabled"}`, false, false},
		{"conflicting switches", weKnora + `,"enable_thinking":false`, false, false},
		{"disabled", `"thinking":{"type":"disabled"},` + weKnora, false, false},
		{"disabled boolean", `"thinking":false,` + weKnora, false, false},
		{"string true", `"chat_template_kwargs":{"enable_thinking":"true"}`, false, false},
		{"number", `"chat_template_kwargs":{"enable_thinking":1}`, false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for protocol, converted := range map[string]bool{APIProtocolChatCompletions: tt.chat, APIProtocolResponses: tt.responses} {
				body := []byte(`{"model":"grok-4.7",` + tt.fields + `}`)
				out, requested, err := normalizeGrokAutoEffort(context.Background(), body, protocol)
				require.NoError(t, err)
				if !converted {
					require.Equal(t, string(body), string(out), protocol)
					require.Nil(t, requested, protocol)
					continue
				}
				path, other := "reasoning.effort", "reasoning_effort"
				if protocol == APIProtocolChatCompletions {
					path, other = other, path
				}
				require.Equal(t, "high", gjson.GetBytes(out, path).String(), protocol)
				require.Equal(t, "auto", optionalStringValue(requested), protocol)
				for _, replaced := range []string{other, "reasoningEffort", "chat_template_kwargs.enable_thinking"} {
					require.False(t, gjson.GetBytes(out, replaced).Exists(), replaced)
				}
				for _, kept := range []string{"reasoning.summary", "chat_template_kwargs.keep"} {
					require.Equal(t, gjson.GetBytes(body, kept).Raw, gjson.GetBytes(out, kept).Raw, kept)
				}
				require.Equal(t, gjson.GetBytes(body, "chat_template_kwargs.keep").Exists(), gjson.GetBytes(out, "chat_template_kwargs").Exists())
			}
		})
	}
}

func TestGrokAutoEffortForwarding(t *testing.T) {
	const model = "grok-4.7"
	for _, stream := range []bool{false, true} {
		for _, tc := range []struct {
			name, inbound, outbound, fields string
			compatible                      bool
		}{
			{"WeKnora direct", grokChatRawEndpoint, grokChatRawEndpoint, `"chat_template_kwargs":{"enable_thinking":true}`, false},
			{"Responses direct", grokChatResponsesEndpoint, grokChatResponsesEndpoint, `"reasoning":{"effort":"auto"}`, false},
			{"Chat to Responses", grokChatRawEndpoint, grokChatResponsesEndpoint, `"chat_template_kwargs":{"enable_thinking":true}`, true},
			{"Responses to Chat", grokChatResponsesEndpoint, grokChatRawEndpoint, `"reasoning":{"effort":"auto"}`, true},
		} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, stream), func(t *testing.T) {
				c, _ := grokAuditContext(tc.inbound)
				account := grokAuditAPIKeyAccount(710)
				if tc.compatible {
					account = rawChatCompletionsTestAccount()
					account.Extra = map[string]any{"openai_responses_supported": tc.outbound == grokChatResponsesEndpoint}
				}
				account.Credentials["model_mapping"] = map[string]any{"helper": model}
				upstream := &httpUpstreamRecorder{}
				if tc.outbound == grokChatRawEndpoint {
					upstream.resp = grokAuditHTTPResponse("application/json", grokAuditChatJSON(model))
					if stream {
						upstream.resp = grokAuditHTTPResponse("text/event-stream", grokAuditChatSSE(model))
					}
				} else {
					upstream.resp = grokAuditHTTPResponse("application/json", grokAuditResponsesJSON(model))
					if stream || tc.inbound == grokChatRawEndpoint {
						upstream.resp = grokAuditHTTPResponse("text/event-stream", grokAuditResponsesSSE("", model))
					}
				}
				svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
				ctx := context.Background()
				wantEffort := "high"
				if tc.compatible {
					ctx = WithGrokAutoEffortPolicy(ctx, "client-alias", "medium", nil, "downgrade")
					wantEffort = "medium"
				}
				input := `"input":"hi"`
				if tc.inbound == grokChatRawEndpoint {
					input = `"messages":[{"role":"user","content":"hi"}]`
				}
				raw := fmt.Sprintf(`{"model":"helper","stream":%t,%s,%s}`, stream, input, tc.fields)
				body := []byte(raw)
				var result *OpenAIForwardResult
				var err error
				if tc.inbound == grokChatRawEndpoint {
					result, err = svc.ForwardAsChatCompletions(ctx, c, account, body, "", "")
				} else {
					result, err = svc.Forward(ctx, c, account, body)
				}
				require.NoError(t, err)
				require.Equal(t, raw, string(body))
				require.Equal(t, tc.outbound, upstream.lastReq.URL.Path)
				path := "reasoning.effort"
				if tc.outbound == grokChatRawEndpoint {
					path = "reasoning_effort"
				}
				require.Equal(t, wantEffort, gjson.GetBytes(upstream.lastBody, path).String())
				require.Equal(t, model, gjson.GetBytes(upstream.lastBody, "model").String())
				require.False(t, gjson.GetBytes(upstream.lastBody, "chat_template_kwargs").Exists())
				require.Equal(t, "auto", optionalStringValue(result.RequestedReasoningEffort))
				require.Equal(t, wantEffort, optionalStringValue(result.ReasoningEffort))
			})
		}
	}
}

func TestGrokAutoEffortModel(t *testing.T) {
	for _, model := range []string{"grok-4.7", "GROK-4.7", "grok-4.7-latest", "x-ai/grok-4.7"} {
		require.True(t, grokDefaultsAutoEffortToHigh(model), model)
	}
	// 4.8 and later join once grokSupportsReasoningEffort lists them.
	for _, model := range []string{"", "grok-4.6", "grok-4.8", "grok-4.8-latest", "grok-4.10", "grok-4.20", "grok-4.20-latest", "grok-4.20-reasoning", "grok-4.21", "grok-4.7-fast", "grok-4.7-build", "grok-4.07", "grok-4.7.1", "grok-4.+7", "grok-5", "custom/grok-4.7"} {
		require.False(t, grokDefaultsAutoEffortToHigh(model), model)
	}
}

func TestGrokAutoEffortPolicyAndModelScope(t *testing.T) {
	svc := &OpenAIGatewayService{httpUpstream: &httpUpstreamRecorder{}}
	for _, model := range []string{"grok-4.6", "grok-4.8", "grok-4.7-fast", "grok-4.7-build", "gpt-5"} {
		// A fresh account avoids stale cached model mappings across cases.
		a := rawChatCompletionsTestAccount()
		a.Credentials["model_mapping"] = map[string]any{"grok-4.7": model}
		c, _ := grokAuditContext(grokChatRawEndpoint)
		body := []byte(`{"model":"grok-4.7","chat_template_kwargs":{"enable_thinking":true}}`)
		out, requested, err := svc.prepareGrokAutoHTTPEffort(context.Background(), c, a, body, APIProtocolChatCompletions, "")
		require.NoError(t, err)
		require.Nil(t, requested)
		require.Equal(t, body, out)
	}
	account := rawChatCompletionsTestAccount()
	account.Credentials["model_mapping"] = map[string]any{"routed": "grok-4.7-latest"}
	for _, ctx := range []context.Context{
		WithGrokAutoEffortPolicy(context.Background(), "client", "medium", nil, "deny"),
		WithGrokAutoEffortPolicy(context.Background(), "client", "", []ReasoningEffortMapping{{From: "high", To: "deny", MatchType: "exact", Model: "client"}}, ""),
	} {
		c, rec := grokAuditContext(grokChatRawEndpoint)
		_, err := svc.ForwardAsChatCompletions(ctx, c, account, []byte(`{"model":"routed","messages":[],"chat_template_kwargs":{"enable_thinking":true}}`), "", "")
		require.True(t, IsReasoningEffortPolicyDenied(err))
		require.Equal(t, 403, rec.Code)
		require.Nil(t, svc.httpUpstream.(*httpUpstreamRecorder).lastReq)
	}
}

// A normal call is not rewritten: a third-party upstream still receives the
// client's off switch, other vendors' switches and unrelated template kwargs.
func TestGrokAutoEffortLeavesNormalCallsToUpstream(t *testing.T) {
	for _, tc := range []struct{ fields, path, raw string }{
		{`"reasoning":{"enabled":false}`, "reasoning.enabled", "false"},
		{`"chat_template_kwargs":{"enable_thinking":false}`, "chat_template_kwargs.enable_thinking", "false"},
		{`"chat_template_kwargs":{"keep":1}`, "chat_template_kwargs.keep", "1"},
		{`"enable_thinking":true`, "enable_thinking", "true"},
	} {
		c, _ := grokAuditContext(grokChatRawEndpoint)
		account := rawChatCompletionsTestAccount()
		account.Extra = map[string]any{"openai_responses_supported": false}
		account.Credentials["model_mapping"] = map[string]any{"helper": "grok-4.7"}
		upstream := &httpUpstreamRecorder{resp: grokAuditHTTPResponse("application/json", grokAuditChatJSON("grok-4.7"))}
		svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
		body := []byte(`{"model":"helper","stream":false,"messages":[{"role":"user","content":"hi"}],` + tc.fields + `}`)
		result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
		require.NoError(t, err, tc.fields)
		require.Equal(t, tc.raw, gjson.GetBytes(upstream.lastBody, tc.path).Raw, tc.fields)
		require.False(t, gjson.GetBytes(upstream.lastBody, "reasoning_effort").Exists(), tc.fields)
		require.Nil(t, result.RequestedReasoningEffort, tc.fields)
	}
}

func TestGrokAutoResponsesDoesNotUpgradeAfterStrippingNone(t *testing.T) {
	account := grokAuditAPIKeyAccount(710)
	upstream := &httpUpstreamRecorder{resp: grokAuditHTTPResponse("application/json", grokAuditResponsesJSON("grok-4.7"))}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	c, _ := grokAuditContext(grokChatResponsesEndpoint)
	body := []byte(`{"model":"grok-4.7-latest","messages":[{"role":"user","content":"hi"}],"reasoning":{"effort":"none"},"reasoning_effort":"auto","chat_template_kwargs":{"enable_thinking":true}}`)
	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.Equal(t, "grok-4.7", gjson.GetBytes(upstream.lastBody, "model").String())
	require.True(t, gjson.GetBytes(upstream.lastBody, "input").IsArray())
	// Explicit none wins under Responses precedence, so this is a normal call.
	for _, field := range []string{"reasoning.effort", "reasoning_effort"} {
		require.False(t, gjson.GetBytes(upstream.lastBody, field).Exists(), field)
	}
	require.Nil(t, result.ReasoningEffort)
}
