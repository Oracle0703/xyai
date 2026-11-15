package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Other agent clients' openers are neutralized on every Antigravity protocol, whether the
// account maps the request to a Gemini or a Claude model (the 2026-11-15 Cline 429 went
// through /v1/chat/completions to a Claude model on Antigravity).
func TestAntigravityClientIdentityAcrossProtocols(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const task = "\nKeep this task description."
	identities := map[string]string{
		"You are Cline, an AI coding agent.":                     "You are an AI agent, an AI coding agent.",
		"You are OpenCode, the best coding agent on the planet.": "You are an AI agent, the best coding agent on the planet.",
	}
	for identity, want := range identities {
		for _, model := range []string{"gemini-3.1-pro-high", "claude-sonnet-4-6"} {
			for _, protocol := range []string{"responses", "chat", "messages"} {
				t.Run(fmt.Sprintf("%s/%s/%s", identity, model, protocol), func(t *testing.T) {
					text := identity + task
					request := map[string]any{"model": model, "max_tokens": 128}
					switch protocol {
					case "responses":
						request["instructions"] = text
						request["input"] = identity
					case "chat":
						request["messages"] = []map[string]any{{"role": "system", "content": text}, {"role": "user", "content": identity}}
					case "messages":
						request["system"] = []map[string]any{{"type": "text", "text": text}}
						request["messages"] = []map[string]any{{"role": "user", "content": identity}}
					}
					body, err := json.Marshal(request)
					require.NoError(t, err)
					var forwarded []byte
					upstream := &queuedHTTPUpstreamStub{
						responses: []*http.Response{antigravityCompatSuccessResponse()},
						onCall: func(req *http.Request, _ *queuedHTTPUpstreamStub) {
							forwarded, err = io.ReadAll(req.Body)
							require.NoError(t, err)
						},
					}
					svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, upstream)
					account := newAntigravityCompatAccount(AccountTypeOAuth)
					c, _ := newAntigravityCompatContext(http.MethodPost, "/v1/"+protocol, body)
					switch protocol {
					case "responses":
						_, err = svc.ForwardAsResponses(context.Background(), c, account, body, nil)
					case "chat":
						_, err = svc.ForwardAsChatCompletions(context.Background(), c, account, body, nil)
					case "messages":
						_, err = svc.Forward(context.Background(), c, account, body, false)
					}
					require.NoError(t, err)
					systemText := gjson.GetBytes(forwarded, "request.systemInstruction.parts").Raw
					require.Contains(t, systemText, want)
					require.Contains(t, systemText, "Keep this task description.")
					require.NotContains(t, systemText, identity)
					require.Equal(t, identity, gjson.GetBytes(forwarded, "request.contents.0.parts.0.text").String(), "user content is not rewritten")
				})
			}
		}
	}
}

func TestInjectAntigravityGeminiClientIdentity(t *testing.T) {
	body, err := json.Marshal(map[string]any{
		"systemInstruction": map[string]any{"parts": []map[string]string{{"text": "You are Cline, an AI coding agent.\nRules."}}},
		"contents":          []map[string]any{{"role": "user", "parts": []map[string]string{{"text": "hi"}}}},
	})
	require.NoError(t, err)

	got, err := injectIdentityPatchToGeminiRequest(body)
	require.NoError(t, err)
	parts := gjson.GetBytes(got, "systemInstruction.parts.#.text").Array()
	require.Len(t, parts, 2)
	require.Contains(t, parts[0].String(), "You are Antigravity")
	require.Equal(t, "You are an AI agent, an AI coding agent.\nRules.", parts[1].String())

	again, err := injectIdentityPatchToGeminiRequest(got)
	require.NoError(t, err)
	require.JSONEq(t, string(got), string(again), "the injected Antigravity identity is kept and not injected twice")
}
