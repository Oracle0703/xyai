package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// Case names tagged "#N" are rebuilt from docs/delivery/2026-10-24-token-origin-samples
// (sanitized structure of the 2026-10-23 production archive); "prod history" shapes come
// from its messages-history-audit.json.
func TestTokenAnalysisRequestOriginUsesLatestEffectiveInput(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		kind   string
		reason string
	}{
		{"responses tool output (#23)", `{"input":[{"role":"user","content":"old task"},{"type":"function_call_output","output":"done"}]}`, "ai", "tool_result"},
		{"custom tool output", `{"input":[{"type":"custom_tool_call_output","output":"done"}]}`, "ai", "tool_result"},
		{"tool search output", `{"input":[{"type":"tool_search_output","output":"done"}]}`, "ai", "tool_result"},
		{"agent message (#15)", `{"input":[{"type":"agent_message","content":"child result"}]}`, "ai", "agent_message"},
		{"agent message then environment (#5)", `{"input":[{"type":"agent_message","content":[{"type":"input_text","text":"done"},{"type":"encrypted_content"}]},{"role":"user","type":"message","content":[{"type":"input_text","text":"<environment_context><cwd>/w</cwd></environment_context>"}]}]}`, "ai", "agent_message"},
		{"assistant continuation", `{"input":[{"role":"user","content":"old task"},{"role":"assistant","content":"working"}]}`, "ai", "assistant_continuation"},
		{"reasoning continuation", `{"input":[{"type":"reasoning","summary":[]}]}`, "ai", "assistant_continuation"},
		{"compaction trigger", `{"input":[{"type":"compaction_trigger"}]}`, "ai", "compaction"},
		{"compaction item", `{"input":[{"type":"compaction","encrypted_content":"opaque"}]}`, "ai", "compaction"},
		{"new user after tools", `{"input":[{"type":"function_call_output","output":"old result"},{"role":"user","content":"new task"}]}`, "human", "user_input"},
		{"responses string", `{"input":"new task","previous_response_id":"old-response","tools":[{"type":"function"}]}`, "human", "user_input"},
		{"chat tool", `{"messages":[{"role":"user","content":"old task"},{"role":"tool","content":"done"}]}`, "ai", "tool_result"},
		{"copilot request after tool history (#210)", `{"messages":[{"role":"tool","content":"r"},{"role":"assistant","content":"a"},{"role":"user","content":"<attachments>a</attachments><userRequest>new task</userRequest>"}]}`, "human", "user_input"},
		{"legacy function", `{"messages":[{"role":"function","content":"done"}]}`, "ai", "tool_result"},
		{"anthropic tool plus system footer (#230)", `{"messages":[{"role":"user","content":"old task"},{"role":"user","content":[{"type":"tool_result","content":"done"}]},{"role":"system","content":"footer"}]}`, "ai", "tool_result"},
		{"mixed tool and new human text", `{"messages":[{"role":"user","content":[{"type":"tool_result","content":"done"},{"type":"text","text":"new instruction"}]}]}`, "human", "user_input"},
		{"image only", `{"input":[{"role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,AA=="}]}]}`, "human", "user_input"},
		// A plain user message cannot prove who sent it; scripts stay human-medium by design.
		{"structured json plus image stays human (#193)", `{"input":[{"role":"user","content":[{"type":"input_text","text":"{\"stage\":\"preflight\"}"},{"type":"input_image","image_url":"data:image/png;base64,AA=="}]}]}`, "human", "user_input"},
		{"document only", `{"messages":[{"role":"user","content":[{"type":"document","source":{"type":"text","data":"contents"}}]}]}`, "human", "user_input"},
		{"image prompt", `{"prompt":"draw a tree","model":"image-model"}`, "human", "user_input"},
		{"numeric prompt", `{"prompt":42}`, "unknown", "unsupported_shape"},
		{"gemini tool", `{"contents":[{"role":"user","parts":[{"text":"old task"}]},{"role":"user","parts":[{"functionResponse":{"name":"lookup","response":{}}}]}]}`, "ai", "tool_result"},
		{"gemini snake case tool", `{"contents":[{"parts":[{"function_response":{"name":"lookup","response":{}}}]}]}`, "ai", "tool_result"},
		{"gemini new user", `{"contents":[{"role":"model","parts":[{"text":"old answer"}]},{"role":"user","parts":[{"text":"new task"}]}]}`, "human", "user_input"},
		{"gemini model", `{"contents":[{"role":"model","parts":[{"text":"answer"}]}]}`, "ai", "assistant_continuation"},
		{"gemini image", `{"contents":[{"parts":[{"inlineData":{"mimeType":"image/png","data":"AA=="}}]}]}`, "human", "user_input"},
		{"environment and developer footers (#2/#153)", `{"input":[{"type":"custom_tool_call_output","output":"done"},{"role":"user","content":[{"type":"input_text","text":" <environment_context><cwd>/work</cwd></environment_context> "}]},{"role":"developer","content":"footer"}]}`, "ai", "tool_result"},
		{"developer additional_tools is skipped (#184)", `{"input":[{"type":"function_call_output"},{"role":"developer","type":"additional_tools"}]}`, "ai", "tool_result"},
		{"environment after human", `{"messages":[{"role":"user","content":"new task"},{"role":"user","content":"<environment_context>context</environment_context>"}]}`, "human", "user_input"},
		{"environment only", `{"input":[{"role":"user","content":"<environment_context>context</environment_context>"}]}`, "unknown", "no_signal"},
		{"quoted environment tag", `{"input":[{"role":"user","content":"Explain <environment_context>context</environment_context>"}]}`, "human", "user_input"},
		{"environment followed by instruction", `{"input":[{"role":"user","content":"<environment_context>context</environment_context> now edit the file"}]}`, "human", "user_input"},
		{"unknown trailing event", `{"input":[{"role":"user","content":"old task"},{"type":"future_event"}]}`, "unknown", "unsupported_shape"},
		{"unknown trailing user block", `{"messages":[{"role":"user","content":"old task"},{"role":"user","content":[{"type":"future_content"}]}]}`, "unknown", "unsupported_shape"},
		{"unknown mixed block", `{"messages":[{"role":"user","content":[{"type":"text","text":"text"},{"type":"future_content"}]}]}`, "unknown", "unsupported_shape"},
		{"empty trailing user", `{"input":[{"role":"user","content":"old task"},{"role":"user","content":""}]}`, "unknown", "no_signal"},
		{"missing role", `{"input":[{"type":"message","content":"ambiguous"}]}`, "unknown", "unsupported_shape"},
		{"non object trailing item", `{"input":[{"role":"user","content":"old task"},7]}`, "unknown", "unsupported_shape"},
		{"only instructions", `{"instructions":"internal instructions"}`, "unknown", "unsupported_shape"},
		{"empty input", `{"input":[]}`, "unknown", "no_signal"},
		// Claude Code <system-reminder> wrappers are injected context, not new user input.
		{"tool result with stacked reminders", `{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"},{"type":"text","text":"<system-reminder>a</system-reminder>\n<system-reminder>b</system-reminder>"},{"type":"text","text":"<system-reminder>c</system-reminder>"}]}]}`, "ai", "tool_result"},
		{"tool result with reminder and new text", `{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"},{"type":"text","text":"<system-reminder>a</system-reminder>"},{"type":"text","text":"stop and run the tests"}]}]}`, "human", "user_input"},
		{"prompt with context reminders (prod history)", `{"messages":[{"role":"user","content":[{"type":"text","text":"<system-reminder>project instructions</system-reminder>"},{"type":"text","text":"fix the login bug"}]}]}`, "human", "user_input"},
		{"reminder-only user after assistant (prod history)", `{"messages":[{"role":"user","content":"old task"},{"role":"assistant","content":"working"},{"role":"user","content":[{"type":"text","text":"<system-reminder>\nbackground notice\n</system-reminder>"}]}]}`, "ai", "assistant_continuation"},
		{"environment and reminder in one block", `{"input":[{"type":"function_call_output","output":"done"},{"role":"user","content":"<environment_context>cwd</environment_context>\n<system-reminder>note</system-reminder>"}]}`, "ai", "tool_result"},
		{"quoted reminder tag", `{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"},{"type":"text","text":"why does <system-reminder>x</system-reminder> appear here?"}]}]}`, "human", "user_input"},
		{"unclosed reminder", `{"messages":[{"role":"user","content":"<system-reminder>please continue with the refactor"}]}`, "human", "user_input"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary, err := SummarizeTokenAnalysisRequest("/v1/responses", []byte(tt.body), 300)
			require.NoError(t, err)
			origin := summary.SummaryJSON["request_origin"]
			require.Equal(t, tokenAnalysisOrigin(tt.reason), origin)
			require.Equal(t, tt.kind, origin.(tokenAnalysisRequestOrigin).Kind)
		})
	}
}

func TestTokenAnalysisRequestOriginContractFollowsReason(t *testing.T) {
	confidence := map[string]string{"ai": "high", "human": "medium", "unknown": "none"}
	for reason, kind := range tokenAnalysisOriginReasonKinds {
		require.Equal(t, tokenAnalysisRequestOrigin{Kind: kind, Reason: reason, Confidence: confidence[kind], Version: 1}, tokenAnalysisOrigin(reason))
	}
	data, err := json.Marshal(tokenAnalysisOrigin("automation"))
	require.NoError(t, err)
	// Only fixed enum values are persisted, never metadata or prompt text.
	require.JSONEq(t, `{"kind":"ai","reason":"automation","confidence":"high","version":1}`, string(data))
}

func TestTokenAnalysisRequestOriginMetadataIsAllowlisted(t *testing.T) {
	classify := func(input any, metadata map[string]any) tokenAnalysisRequestOrigin {
		return classifyTokenAnalysisRequestOrigin(map[string]any{"input": input, "client_metadata": metadata})
	}
	for _, encoding := range []string{"object", "string"} {
		for _, trigger := range []string{"automation_cron_scheduled", "goal", "memory_consolidation", "guardian_review", "guardian_classifier", "ambient_suggestions", "thread_title"} {
			turn := map[string]any{"turn_trigger": trigger, "thread_id": "private-id", "thread_source": "user"}
			var encoded any = turn
			if encoding == "string" {
				data, err := json.Marshal(turn)
				require.NoError(t, err)
				encoded = string(data)
			}
			require.Equal(t, tokenAnalysisOrigin("automation"), classify("automated task", map[string]any{"x-codex-turn-metadata": encoded}), encoding+"/"+trigger)
		}
	}
	for _, trigger := range []string{"composer", "user", "edit_user_message", "queue", "resume_interrupted_task", "future_trigger"} {
		metadata := map[string]any{"x-codex-turn-metadata": map[string]any{"turn_trigger": trigger, "thread_source": "automation"}}
		require.Equal(t, tokenAnalysisOrigin("user_input"), classify("new task", metadata), trigger)
		require.Equal(t, tokenAnalysisOrigin("tool_result"), classify([]any{map[string]any{"type": "function_call_output"}}, metadata), trigger)
	}
	// The two fields have different observed value sets and must not be interchanged.
	for _, metadata := range []map[string]any{
		{"x-openai-subagent": "collab_spawn"},
		{"x-openai-subagent": "guardian"},
		{"x-openai-subagent": "memory_consolidation"},
		{"x-codex-turn-metadata": map[string]any{"subagent_kind": "thread_spawn"}},
		{"x-codex-turn-metadata": `{"subagent_kind":"guardian"}`},
	} {
		require.Equal(t, tokenAnalysisOrigin("subagent"), classify("delegated task", metadata))
	}
	for _, metadata := range []map[string]any{
		{"x-openai-subagent": "thread_spawn"},
		{"x-codex-turn-metadata": map[string]any{"subagent_kind": "collab_spawn"}},
		{"x-codex-turn-metadata": map[string]any{"subagent_kind": "memory_consolidation"}},
		{"x-openai-subagent": "future_subagent"},
		{"x-openai-subagent": true},
		{"x-codex-turn-metadata": "{malformed"},
		{"x-codex-turn-metadata": `{"turn_trigger":"goal","subagent_kind":"guardian",}`},
		{"x-codex-turn-metadata": map[string]any{"thread_source": "subagent"}},
		{"x-codex-turn-metadata": map[string]any{"trigger": "goal"}},
	} {
		require.Equal(t, tokenAnalysisOrigin("user_input"), classify("new task", metadata))
	}
}

// Facts only the archive line knows override the body classification:
// count_tokens > truncation > raw /compact path.
func TestTokenAnalysisRequestOriginTransportOverrides(t *testing.T) {
	const multipart = "--origin-test\r\nContent-Disposition: form-data; name=\"prompt\"\r\n\r\ndraw a tree\r\n--origin-test--\r\n"
	tests := []struct {
		name      string
		path      string
		body      string
		truncated bool
		reason    string
	}{
		{"truncated user text", "/v1/responses", `{"input":"new task"}`, true, "body_truncated"},
		{"truncated tool output", "/v1/responses", `{"input":[{"type":"function_call_output","output":"done"}]}`, true, "body_truncated"},
		{"truncated subagent", "/v1/responses", `{"input":"automated task","client_metadata":{"x-openai-subagent":"guardian"}}`, true, "body_truncated"},
		{"truncated unparsable", "/v1/responses", `{"input":[`, true, "body_truncated"},
		{"truncated multipart", "/v1/images/edits", multipart, true, "body_truncated"},
		{"truncation beats compaction", "/v1/responses/compact", `{"input":"new task"}`, true, "body_truncated"},
		{"raw compact path", "/backend-api/codex/responses/compact", `{"input":"old user text"}`, false, "compaction"},
		{"count_tokens", "/v1/messages/count_tokens", `{"messages":[{"role":"user","content":"new task"}]}`, false, "count_tokens"},
		{"count_tokens beats truncation", "/v1/messages/count_tokens", `{"messages":[{"role":"user","content":"new task"}]}`, true, "count_tokens"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := indexTokenAnalysisTestLine(t, &config.Config{}, map[string]any{
				"endpoint": "/v1/responses", "path": tt.path, "body": tt.body, "body_truncated": tt.truncated,
			})
			require.Equal(t, tokenAnalysisOrigin(tt.reason), repo.upserts[0].SummaryJSON["request_origin"])
		})
	}
}

func TestTokenAnalysisRequestOriginMultipart(t *testing.T) {
	body := "--origin-test\r\nContent-Disposition: form-data; name=\"prompt\"\r\n\r\ndraw a tree\r\n--origin-test--\r\n"
	summary, ok := SummarizeTokenAnalysisMultipartRequest("/v1/images/edits", "multipart/form-data; boundary=origin-test", []byte(body), 300)
	require.True(t, ok)
	require.Equal(t, tokenAnalysisOrigin("user_input"), summary.SummaryJSON["request_origin"])
	summary, ok = SummarizeTokenAnalysisMultipartRequest("/v1/images/edits", "multipart/form-data; boundary=origin-test", []byte(strings.ReplaceAll(body, "draw a tree", "")), 300)
	require.True(t, ok)
	require.Equal(t, tokenAnalysisOrigin("no_signal"), summary.SummaryJSON["request_origin"])
}
