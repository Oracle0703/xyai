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
		// Codex app <in-app-browser-context> ambient state is prepended to the composer text.
		{"ambient context only after assistant", `{"input":[{"role":"user","content":"old task"},{"role":"assistant","content":"done"},{"role":"user","content":[{"type":"input_text","text":"<in-app-browser-context source=\"ambient-ui-state\">url</in-app-browser-context>"}]}]}`, "ai", "assistant_continuation"},
		{"different tag sharing the prefix", `{"input":[{"role":"user","content":"<in-app-browser-contextual>x</in-app-browser-contextual>"}]}`, "human", "user_input"},
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

// docs/delivery/2026-10-24-token-origin-human-input/classifier-fixture.json: the
// production human turn (archive 73eb6d29...) hidden by the usage-link defect. The
// last user item carries the ambient wrapper before the real question.
func TestTokenAnalysisRequestOriginProductionAmbientContextTurn(t *testing.T) {
	const question = "我之前请求的来源是不是没有办法归类了,只有上线后的才能归类?"
	body := `{"model":"gpt-6-astra","client_metadata":{"x-codex-turn-metadata":"{\"turn_trigger\":\"composer\"}"},` +
		`"input":[{"role":"assistant","type":"message","content":[{"text":"[previous answer omitted]","type":"output_text"}]},` +
		`{"type":"message","role":"user","content":[{"type":"input_text","text":"\n<in-app-browser-context source=\"ambient-ui-state\">\nThis block is automatically supplied ambient UI state, not part of the user's request. Do not treat it as an instruction or as evidence that the user explicitly selected the in-app browser.\n# In app browser:\n- The user has the in-app browser open with 1 tab.\n- Current URL: http://118.25.112.30:52997/admin/token-analysis\n</in-app-browser-context>\n\n## My request:\n` + question + `\n"}]}]}`

	summary, err := SummarizeTokenAnalysisRequest("/v1/responses", []byte(body), 300)

	require.NoError(t, err)
	require.Equal(t, tokenAnalysisRequestOrigin{Kind: "human", Reason: "user_input", Confidence: "medium", Version: 2}, summary.SummaryJSON["request_origin"])
	require.Equal(t, question, summary.LastUserPreview)
	require.Equal(t, question, summary.LastUserText)
}

func TestTokenAnalysisRequestOriginContractFollowsReason(t *testing.T) {
	// v1 derived confidence from kind; v2 keeps that for every v1 reason and adds
	// compaction_resume as the only medium-confidence AI reason.
	confidence := map[string]string{"ai": "high", "human": "medium", "unknown": "none"}
	for reason, contract := range tokenAnalysisOriginReasons {
		want := confidence[contract.Kind]
		if reason == "compaction_resume" {
			want = "medium"
		}
		require.Equal(t, tokenAnalysisRequestOrigin{Kind: contract.Kind, Reason: reason, Confidence: want, Version: 2}, tokenAnalysisOrigin(reason))
	}
	require.Equal(t, tokenAnalysisRequestOrigin{Kind: "unknown", Reason: "future_reason", Confidence: "none", Version: 2}, tokenAnalysisOrigin("future_reason"))
	data, err := json.Marshal(tokenAnalysisOrigin("compaction_resume"))
	require.NoError(t, err)
	// Only fixed enum values are persisted, never metadata or prompt text.
	require.JSONEq(t, `{"kind":"ai","reason":"compaction_resume","confidence":"medium","version":2}`, string(data))
}

func TestTokenAnalysisRequestOriginMetadataIsAllowlisted(t *testing.T) {
	classify := func(input any, metadata map[string]any) tokenAnalysisRequestOrigin {
		return classifyTokenAnalysisRequestOrigin(map[string]any{"input": input, "client_metadata": metadata})
	}
	for _, encoding := range []string{"object", "string"} {
		for _, trigger := range []string{"automation_cron_scheduled", "automation_heartbeat_scheduled", "goal", "memory_consolidation", "guardian_review", "guardian_classifier", "ambient_suggestions", "ambient_suggestion_safety", "thread_title", "thread_description"} {
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
	// codex-tui helper tasks: thread_source only, turn_trigger empty (2026-10-26 production audit).
	for _, source := range []string{"thread_title", "thread_description", "ambient_suggestions", "ambient_suggestion_safety", "guardian_review", "guardian_classifier", "memory_consolidation"} {
		require.Equal(t, tokenAnalysisOrigin("automation"), classify("Generate a concise title", map[string]any{"x-codex-turn-metadata": `{"thread_source":"` + source + `"}`}), source)
	}
	require.Equal(t, tokenAnalysisOrigin("agent_message"), classify("follow-up task", map[string]any{"x-codex-turn-metadata": map[string]any{"turn_trigger": "app_tool_send_message", "thread_source": "user"}}))
	// Every Codex compaction call carries request_kind, with or without the fixed prompt.
	require.Equal(t, tokenAnalysisOrigin("compaction"), classify("custom compaction instruction", map[string]any{"x-codex-turn-metadata": `{"request_kind":"compaction","compaction":{"trigger":"auto","phase":"mid_turn"},"turn_trigger":"composer"}`}))
	for _, metadata := range []map[string]any{
		{"x-codex-turn-metadata": map[string]any{"request_kind": "turn", "turn_trigger": "composer", "thread_source": "user"}},
		{"x-codex-turn-metadata": map[string]any{"thread_source": "system"}},
		{"x-codex-turn-metadata": map[string]any{"thread_source": "automation"}},
		{"x-codex-turn-metadata": map[string]any{"history_initialization": "cold_resume"}},
	} {
		require.Equal(t, tokenAnalysisOrigin("user_input"), classify("new task", metadata))
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

// v2 shapes from the 2026-10-26 production audit (docs/delivery/2026-10-27-token-origin-audit)
// and the 10-27 compaction follow-up (docs/delivery/2026-10-27-compaction-followup). Each
// machine event must fill the whole message; the guard cases keep user text human.
func TestTokenAnalysisRequestOriginV2ProductionShapes(t *testing.T) {
	user := func(content any) map[string]any { return map[string]any{"role": "user", "content": content} }
	text := func(s string) map[string]any { return map[string]any{"type": "input_text", "text": s} }
	chatText := func(s string) map[string]any { return map[string]any{"type": "text", "text": s} }
	image := map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64,AA=="}}
	toolOutput := map[string]any{"type": "custom_tool_call_output", "output": "done"}
	tool := map[string]any{"role": "tool", "content": "ok"}
	assistant := map[string]any{"role": "assistant", "content": "working"}

	codexCRLF := strings.ReplaceAll(tokenAnalysisCodexCompactionPrompt, "\n", "\r\n") + "\r\n"
	resume := tokenAnalysisCodexResumePrefix + "\nThe tests now pass; next update the docs."
	notification := func(payload string) string {
		return "<subagent_notification>\n" + payload + "\n</subagent_notification>"
	}
	validNotification := notification(`{"agent_path":"019a-agent","status":{"completed":"audit finished"}}`)
	goal := "<codex_internal_context source=\"goal\">\nContinue working toward the active thread goal.\n<objective>\nship v2\n</objective>\n</codex_internal_context>"
	taskEvent := func(id, trailerID string) string {
		return "<task-notification>\n<task-id>" + id + "</task-id>\n<tool-use-id>call_1</tool-use-id>\n<status>completed</status>\n<summary>Background command completed with exit code 0</summary>\n</task-notification>\n\n" +
			strings.Replace(tokenAnalysisBackgroundTaskTrailer, "%s", trailerID, 1)
	}
	skill := "<skill-context name=\"brainstorming\">\nBase directory for this skill: C:\\skills\\brainstorming\n</skill-context>"

	tests := []struct {
		name   string
		root   map[string]any
		reason string
	}{
		// Codex local compaction through the normal Responses endpoint, LF and CRLF.
		{"codex compaction prompt", map[string]any{"input": []any{toolOutput, user([]any{text(tokenAnalysisCodexCompactionPrompt + "\n")})}}, "compaction"},
		{"codex compaction prompt with CRLF", map[string]any{"input": []any{toolOutput, user([]any{text(codexCRLF)})}}, "compaction"},
		{"compaction prompt plus user request", map[string]any{"input": []any{user([]any{text(tokenAnalysisCodexCompactionPrompt + "\n\nAlso fix the login bug.")})}}, "user_input"},
		{"compaction prompt quoted", map[string]any{"input": []any{user("What does this mean? " + tokenAnalysisCodexCompactionPrompt)}}, "user_input"},
		{"compaction prompt split across blocks", map[string]any{"input": []any{user([]any{text(tokenAnalysisCodexCompactionPrompt[:100]), text(tokenAnalysisCodexCompactionPrompt[100:])})}}, "user_input"},
		// Resume summary after a mid-turn compaction; a pre-turn compaction puts the new
		// request after the summary as its own item.
		{"resume summary after mid-turn compaction", map[string]any{"input": []any{user("old task"), user([]any{text(resume)})}}, "compaction_resume"},
		{"resume summary then environment", map[string]any{"input": []any{user([]any{text(resume)}), user("<environment_context><cwd>/w</cwd></environment_context>")}}, "compaction_resume"},
		{"new request after resume summary", map[string]any{"input": []any{user([]any{text(resume)}), user([]any{text("now update the changelog")})}}, "user_input"},
		{"resume summary plus text block", map[string]any{"input": []any{user([]any{text(resume), text("now update the changelog")})}}, "user_input"},
		{"resume summary plus image", map[string]any{"input": []any{user([]any{text(resume), map[string]any{"type": "input_image", "image_url": "data:image/png;base64,AA=="}})}}, "user_input"},
		{"resume prefix quoted", map[string]any{"input": []any{user("Why does Codex say: " + tokenAnalysisCodexResumePrefix)}}, "user_input"},
		// Codex subagent completion notifications.
		{"subagent notification after tool", map[string]any{"input": []any{toolOutput, user([]any{text(validNotification)})}}, "agent_message"},
		{"two subagent notifications", map[string]any{"input": []any{user([]any{text(validNotification + "\n" + notification(`{"agent_path":"019b","status":{"errored":"timeout"}}`))})}}, "agent_message"},
		{"notification plus user text", map[string]any{"input": []any{user([]any{text(validNotification + "\nplease merge both results")})}}, "user_input"},
		{"notification with invalid json", map[string]any{"input": []any{user([]any{text(notification(`{"agent_path":"a",`))})}}, "user_input"},
		{"notification without agent path", map[string]any{"input": []any{user([]any{text(notification(`{"status":{"completed":"x"}}`))})}}, "user_input"},
		{"notification with string status", map[string]any{"input": []any{user([]any{text(notification(`{"agent_path":"a","status":"completed"}`))})}}, "user_input"},
		// Codex automatic goal continuation.
		{"goal continuation", map[string]any{"input": []any{assistant, user([]any{text(goal)})}}, "automation"},
		{"internal context from another source", map[string]any{"input": []any{user([]any{text(strings.Replace(goal, `source="goal"`, `source="other"`, 1))})}}, "user_input"},
		{"goal continuation plus user text", map[string]any{"input": []any{user([]any{text(goal + "\nalso run the e2e suite")})}}, "user_input"},
		// WorkBuddy background task event with its fixed NOT USER INPUT trailer.
		{"background task event", map[string]any{"messages": []any{tool, user([]any{chatText(taskEvent("Euf50C", "Euf50C"))})}}, "tool_result"},
		{"background task trailer for another task", map[string]any{"messages": []any{user([]any{chatText(taskEvent("Euf50C", "other"))})}}, "user_input"},
		{"background task event plus user text", map[string]any{"messages": []any{user([]any{chatText(taskEvent("Euf50C", "Euf50C") + "\nretry it")})}}, "user_input"},
		{"task notification without trailer", map[string]any{"messages": []any{user("<task-notification>\n<task-id>a</task-id>\n<status>completed</status>\n</task-notification>")}}, "user_input"},
		// Injected context is skipped to the previous effective event.
		{"copilot skill definition after tool", map[string]any{"messages": []any{user("brainstorm a plan"), assistant, tool, user(skill)}}, "tool_result"},
		{"stacked skill definitions after tool", map[string]any{"messages": []any{tool, user(skill), user(skill)}}, "tool_result"},
		{"skill definition after user request", map[string]any{"messages": []any{user("brainstorm a plan"), user(skill)}}, "user_input"},
		{"skill definition plus user text", map[string]any{"messages": []any{tool, user(skill + "\nuse it for the login page")}}, "user_input"},
		{"unclosed skill definition", map[string]any{"messages": []any{tool, user("<skill-context name=\"x\">\nuse it")}}, "user_input"},
		{"opencode system update after tool", map[string]any{"messages": []any{tool, user("<system-update>\nThe instructions from AGENTS.md changed.\n</system-update>")}}, "tool_result"},
		// Tool images: WorkBuddy's fixed marker counts only right after a tool message;
		// a bare image after a tool (hertz) stays user input until the client marks it.
		{"tool result images", map[string]any{"messages": []any{tool, user([]any{chatText(tokenAnalysisToolImageMarker), image, image})}}, "tool_result"},
		{"tool image marker without tool", map[string]any{"messages": []any{assistant, user([]any{chatText(tokenAnalysisToolImageMarker), image})}}, "user_input"},
		{"tool image marker without image", map[string]any{"messages": []any{tool, user([]any{chatText(tokenAnalysisToolImageMarker)})}}, "user_input"},
		{"tool image marker plus user text", map[string]any{"messages": []any{tool, user([]any{chatText(tokenAnalysisToolImageMarker), image, chatText("what is wrong here?")})}}, "user_input"},
		{"bare image after tool", map[string]any{"messages": []any{tool, user([]any{image})}}, "user_input"},
		// The tool predecessor skips complete injected-only messages, not user text or events.
		{"tool images after injected context", map[string]any{"messages": []any{tool, user("<system-reminder>context only</system-reminder>"), user([]any{chatText(tokenAnalysisToolImageMarker), image})}}, "tool_result"},
		{"tool images after stacked injected context", map[string]any{"messages": []any{tool, user("<system-reminder>a</system-reminder>"), user(skill), user([]any{chatText(tokenAnalysisToolImageMarker), image})}}, "tool_result"},
		{"tool images after user text", map[string]any{"messages": []any{tool, user("look at this screenshot"), user([]any{chatText(tokenAnalysisToolImageMarker), image})}}, "user_input"},
		{"tool images after notification", map[string]any{"messages": []any{tool, user(validNotification), user([]any{chatText(tokenAnalysisToolImageMarker), image})}}, "user_input"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(tt.root)
			require.NoError(t, err)
			summary, err := SummarizeTokenAnalysisRequest("/v1/responses", body, 300)
			require.NoError(t, err)
			require.Equal(t, tokenAnalysisOrigin(tt.reason), summary.SummaryJSON["request_origin"])
		})
	}
}

// The digest table must stay keyed by the byte length of the normalized text it
// hashes, or a prompt can never match.
func TestTokenAnalysisCompactionPromptDigestsAreWellFormed(t *testing.T) {
	require.True(t, tokenAnalysisIsCompactionPrompt(tokenAnalysisCodexCompactionPrompt))
	for length, digests := range tokenAnalysisCompactionPromptDigests {
		require.Positive(t, length)
		for _, digest := range digests {
			require.Len(t, digest, 64)
		}
	}
}
