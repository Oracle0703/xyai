package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

// Request origin is an explainable heuristic about this sampling request, not
// proof of who authored its prompt. Never retain client metadata or prompt text.
type tokenAnalysisRequestOrigin struct {
	Kind       string `json:"kind"`
	Reason     string `json:"reason"`
	Confidence string `json:"confidence"`
	Version    int    `json:"version"`
}

// Version 2 adds the client shapes found by the 2026-10-27 production audit
// (docs/features/token-analysis-request-origin-design-cn.md, "v2 修订").
const tokenAnalysisOriginVersion = 2

type tokenAnalysisOriginContract struct {
	Kind       string
	Confidence string
}

// Reason decides kind and confidence. The frontend mirrors this table
// (requestOrigin.ts) to validate the contract; v1 rows keep their own table.
var tokenAnalysisOriginReasons = map[string]tokenAnalysisOriginContract{
	"subagent":               {"ai", "high"},
	"automation":             {"ai", "high"},
	"tool_result":            {"ai", "high"},
	"agent_message":          {"ai", "high"},
	"assistant_continuation": {"ai", "high"},
	"compaction":             {"ai", "high"},
	// A structural inference without a client marker, hence medium.
	"compaction_resume": {"ai", "medium"},
	"user_input":        {"human", "medium"},
	"no_signal":         {"unknown", "none"},
	"unsupported_shape": {"unknown", "none"},
	"body_truncated":    {"unknown", "none"},
	"count_tokens":      {"unknown", "none"},
}

func tokenAnalysisOrigin(reason string) tokenAnalysisRequestOrigin {
	contract, ok := tokenAnalysisOriginReasons[reason]
	if !ok {
		contract = tokenAnalysisOriginContract{Kind: "unknown", Confidence: "none"}
	}
	return tokenAnalysisRequestOrigin{Kind: contract.Kind, Reason: reason, Confidence: contract.Confidence, Version: tokenAnalysisOriginVersion}
}

func setTokenAnalysisRequestOrigin(summary *TokenAnalysisBodySummary, origin tokenAnalysisRequestOrigin) {
	if summary.SummaryJSON == nil {
		summary.SummaryJSON = map[string]any{}
	}
	summary.SummaryJSON["request_origin"] = origin
}

// tokenAnalysisTransportOrigin holds the facts only the archive line knows, in
// precedence order; they override whatever the body classification said.
// rawPath is the un-normalized request path (the endpoint folds /compact and
// /count_tokens into their base routes).
func tokenAnalysisTransportOrigin(rawPath string, isCountTokens, bodyTruncated bool) (tokenAnalysisRequestOrigin, bool) {
	switch {
	case isCountTokens:
		// count_tokens probes are not model calls, so no request origin applies.
		return tokenAnalysisOrigin("count_tokens"), true
	case bodyTruncated:
		// A truncated archive cannot establish the actual trailing input, even if
		// its retained bytes happen to be valid JSON or contain a complete prompt.
		return tokenAnalysisOrigin("body_truncated"), true
	case strings.HasSuffix(strings.TrimRight(rawPath, "/"), "/responses/compact"):
		return tokenAnalysisOrigin("compaction"), true
	}
	return tokenAnalysisRequestOrigin{}, false
}

func classifyTokenAnalysisRequestOrigin(root map[string]any) tokenAnalysisRequestOrigin {
	// The allowlist describes observed machine tasks. Metadata is client supplied;
	// composer/user/thread_source=user|automation|system and the mere presence of
	// tools prove nothing.
	metadata, _ := root["client_metadata"].(map[string]any)
	turn, _ := metadata["x-codex-turn-metadata"].(map[string]any)
	if raw, ok := metadata["x-codex-turn-metadata"].(string); ok {
		if err := json.Unmarshal([]byte(raw), &turn); err != nil {
			turn = nil
		}
	}
	// The two fields use different value sets (2026-10-23 production sample):
	// x-openai-subagent collab_spawn/guardian/memory_consolidation, while the
	// nested subagent_kind is thread_spawn/guardian. Do not interchange them.
	switch metadata["x-openai-subagent"] {
	case "collab_spawn", "guardian", "memory_consolidation":
		return tokenAnalysisOrigin("subagent")
	}
	switch turn["subagent_kind"] {
	case "thread_spawn", "guardian":
		return tokenAnalysisOrigin("subagent")
	}
	// Codex marks every compaction call, including those without the fixed
	// prompt; a trailing compaction instruction must not read as new user text.
	if turn["request_kind"] == "compaction" {
		return tokenAnalysisOrigin("compaction")
	}
	switch turn["turn_trigger"] {
	case "automation_cron_scheduled", "automation_heartbeat_scheduled", "goal", "memory_consolidation", "guardian_review", "guardian_classifier",
		"ambient_suggestions", "ambient_suggestion_safety", "thread_title", "thread_description":
		return tokenAnalysisOrigin("automation")
	case "app_tool_send_message":
		return tokenAnalysisOrigin("agent_message")
	}
	// codex-tui marks helper tasks only through thread_source (turn_trigger empty).
	switch turn["thread_source"] {
	case "thread_title", "thread_description", "ambient_suggestions", "ambient_suggestion_safety", "guardian_review", "guardian_classifier", "memory_consolidation":
		return tokenAnalysisOrigin("automation")
	}
	switch {
	case root["contents"] != nil:
		return tokenAnalysisOriginFromItems(root["contents"], true)
	case root["prompt"] != nil && root["messages"] == nil && root["input"] == nil:
		origin, _ := tokenAnalysisOriginFromUserContent(root["prompt"], false)
		return origin
	case root["input"] != nil:
		if input, ok := root["input"].(string); ok {
			origin, _ := tokenAnalysisOriginFromUserContent(input, false)
			return origin
		}
		return tokenAnalysisOriginFromItems(root["input"], false)
	case root["messages"] != nil:
		return tokenAnalysisOriginFromItems(root["messages"], false)
	default:
		return tokenAnalysisOrigin("unsupported_shape")
	}
}

func tokenAnalysisOriginFromItems(value any, gemini bool) tokenAnalysisRequestOrigin {
	items, ok := value.([]any)
	if !ok {
		return tokenAnalysisOrigin("unsupported_shape")
	}
	for i := len(items) - 1; i >= 0; i-- {
		item, ok := items[i].(map[string]any)
		if !ok {
			return tokenAnalysisOrigin("unsupported_shape")
		}
		role, _ := item["role"].(string)
		if role == "system" || role == "developer" {
			// Injected instructions carry no turn origin whatever their item type
			// (message, or Responses Lite additional_tools).
			continue
		}
		typ, _ := item["type"].(string)
		switch {
		case tokenAnalysisIsToolOutputItemType(typ):
			return tokenAnalysisOrigin("tool_result")
		case typ == "agent_message":
			return tokenAnalysisOrigin("agent_message")
		case typ == "compaction_trigger" || typ == "compaction":
			return tokenAnalysisOrigin("compaction")
		case typ == "reasoning" || strings.HasSuffix(typ, "_call"):
			return tokenAnalysisOrigin("assistant_continuation")
		case typ != "" && typ != "message":
			// Do not walk past an unfamiliar trailing event and mislabel an old
			// user message as a new manual request.
			return tokenAnalysisOrigin("unsupported_shape")
		}
		switch role {
		case "tool", "function":
			return tokenAnalysisOrigin("tool_result")
		case "assistant", "model":
			return tokenAnalysisOrigin("assistant_continuation")
		case "user", "":
			if role == "" && !gemini {
				return tokenAnalysisOrigin("unsupported_shape")
			}
			content := item["content"]
			if gemini {
				content = item["parts"]
			}
			origin, injectedOnly := tokenAnalysisOriginFromUserContent(content, gemini)
			if injectedOnly {
				continue
			}
			if origin.Reason == "user_input" && tokenAnalysisIsToolImageAttachment(content) && tokenAnalysisPreviousIsTool(items, i) {
				return tokenAnalysisOrigin("tool_result")
			}
			return origin
		default:
			return tokenAnalysisOrigin("unsupported_shape")
		}
	}
	return tokenAnalysisOrigin("no_signal")
}

// injectedOnly allows skipping a complete injected context message, never a
// user sentence that happens to quote or discuss the same XML tag.
func tokenAnalysisOriginFromUserContent(content any, gemini bool) (origin tokenAnalysisRequestOrigin, injectedOnly bool) {
	if text, ok := content.(string); ok {
		if tokenAnalysisInjectedContextOnly(text) {
			return tokenAnalysisOrigin("no_signal"), true
		}
		if reason, ok := tokenAnalysisMachineEventReason(text); ok {
			return tokenAnalysisOrigin(reason), false
		}
		if strings.TrimSpace(text) != "" {
			return tokenAnalysisOrigin("user_input"), false
		}
		return tokenAnalysisOrigin("no_signal"), false
	}
	blocks, ok := content.([]any)
	if !ok {
		return tokenAnalysisOrigin("unsupported_shape"), false
	}
	var human, tool, injected bool
	event := ""
	for _, raw := range blocks {
		block, ok := raw.(map[string]any)
		if !ok {
			return tokenAnalysisOrigin("unsupported_shape"), false
		}
		typ, _ := block["type"].(string)
		switch {
		case tokenAnalysisIsToolResultBlock(block, gemini):
			tool = true
		case tokenAnalysisIsTextBlock(block):
			text, ok := block["text"].(string)
			if !ok {
				return tokenAnalysisOrigin("unsupported_shape"), false
			}
			if tokenAnalysisInjectedContextOnly(text) {
				injected = true
			} else if reason, ok := tokenAnalysisMachineEventReason(text); ok {
				if event == "" {
					event = reason
				}
			} else if strings.TrimSpace(text) != "" {
				human = true
			}
		case tokenAnalysisIsMediaBlock(block, gemini):
			if typ != "" && len(block) <= 1 {
				return tokenAnalysisOrigin("unsupported_shape"), false
			}
			human = true
		default:
			return tokenAnalysisOrigin("unsupported_shape"), false
		}
	}
	switch {
	case human:
		return tokenAnalysisOrigin("user_input"), false
	case tool:
		return tokenAnalysisOrigin("tool_result"), false
	case event != "":
		return tokenAnalysisOrigin(event), false
	}
	return tokenAnalysisOrigin("no_signal"), injected
}

// tokenAnalysisIsToolOutputItemType 是 Responses 顶层工具输出条目的共享定义,
// 来源分类与摘要器的工具输出统计共用。
func tokenAnalysisIsToolOutputItemType(typ string) bool {
	return strings.HasSuffix(typ, "_call_output") || typ == "tool_search_output"
}

// tokenAnalysisIsToolResultBlock 是消息内容块中工具结果的共享定义: Anthropic
// tool_result, 以及 Gemini parts 中的 functionResponse/function_response。
func tokenAnalysisIsToolResultBlock(block map[string]any, gemini bool) bool {
	typ, _ := block["type"].(string)
	return typ == "tool_result" || (gemini && (block["functionResponse"] != nil || block["function_response"] != nil))
}

// tokenAnalysisIsTextBlock is the shared definition of a text content block for
// origin classification and user-input extraction (Gemini parts carry no type).
func tokenAnalysisIsTextBlock(block map[string]any) bool {
	typ, _ := block["type"].(string)
	return typ == "text" || typ == "input_text" || (typ == "" && block["text"] != nil)
}

// tokenAnalysisIsMediaBlock covers typed image/file/audio blocks and Gemini
// inline/file data parts, which carry no type.
func tokenAnalysisIsMediaBlock(block map[string]any, gemini bool) bool {
	switch typ, _ := block["type"].(string); typ {
	case "image", "image_url", "input_image", "input_file", "file", "document", "input_audio":
		return true
	case "":
		return gemini && (block["inlineData"] != nil || block["inline_data"] != nil || block["fileData"] != nil || block["file_data"] != nil)
	}
	return false
}

// Chat Completions cannot carry images in a tool message, so WorkBuddy follows
// the tool message with a user message holding this marker and the images.
const tokenAnalysisToolImageMarker = "Attached image(s) from tool result:"

// tokenAnalysisIsToolImageAttachment reports a user message made only of the
// tool-image marker, injected context and at least one media block. A plain
// image upload has no marker, so it stays user input.
func tokenAnalysisIsToolImageAttachment(content any) bool {
	blocks, ok := content.([]any)
	if !ok {
		return false
	}
	var marker, media bool
	for _, raw := range blocks {
		block, _ := raw.(map[string]any)
		switch {
		case block == nil:
			return false
		case tokenAnalysisIsTextBlock(block):
			text, _ := block["text"].(string)
			switch {
			case strings.TrimSpace(text) == tokenAnalysisToolImageMarker:
				marker = true
			case !tokenAnalysisInjectedContextOnly(text) && strings.TrimSpace(text) != "":
				return false
			}
		case tokenAnalysisIsMediaBlock(block, false):
			media = true
		default:
			return false
		}
	}
	return marker && media
}

// tokenAnalysisPreviousIsTool reports whether the effective item before
// items[i] is a tool result, skipping instructions and complete injected-only
// user messages the same way tokenAnalysisOriginFromItems does.
func tokenAnalysisPreviousIsTool(items []any, i int) bool {
	for j := i - 1; j >= 0; j-- {
		item, _ := items[j].(map[string]any)
		role, _ := item["role"].(string)
		if role == "system" || role == "developer" {
			continue
		}
		typ, _ := item["type"].(string)
		if role == "user" && (typ == "" || typ == "message") {
			if _, injectedOnly := tokenAnalysisOriginFromUserContent(item["content"], false); injectedOnly {
				continue
			}
		}
		return role == "tool" || role == "function" || tokenAnalysisIsToolOutputItemType(typ)
	}
	return false
}

// Client-injected wrappers that carry no new user intent and are skipped to
// the previous effective event: Codex <environment_context>, the Codex app's
// <in-app-browser-context> ambient UI state (prepended to the composer text,
// seen in production), Claude Code <system-reminder> (seen beside user text and
// as reminder-only continuations after an assistant turn), Copilot
// <skill-context> skill definitions and OpenCode <system-update> instruction
// diffs (both seen after tool results, 2026-10-26 production audit).
var tokenAnalysisInjectedContextTags = []string{"environment_context", "in-app-browser-context", "system-reminder", "skill-context", "system-update"}

// The Codex app introduces the user's own text with this heading after the
// ambient context; it is client framing only when a wrapper precedes it.
const tokenAnalysisMyRequestHeading = "## My request:"

// tokenAnalysisInjectedContextOnly reports whether text consists solely of one
// or more complete injected wrappers. Text outside a wrapper, or an unclosed
// wrapper, keeps the whole block as user input.
func tokenAnalysisInjectedContextOnly(text string) bool {
	rest, stripped := stripTokenAnalysisInjectedPrefix(text)
	return stripped && rest == ""
}

// stripTokenAnalysisInjectedPrefix removes the complete injected wrappers that
// lead the text, where clients put them. A wrapper later in the text or an
// unclosed one is kept: it may be the user quoting the tag.
func stripTokenAnalysisInjectedPrefix(text string) (rest string, stripped bool) {
	rest = strings.TrimSpace(text)
	for {
		tail, ok := cutTokenAnalysisInjectedWrapper(rest)
		if !ok {
			break
		}
		rest, stripped = strings.TrimSpace(tail), true
	}
	if stripped {
		if body, ok := strings.CutPrefix(rest, tokenAnalysisMyRequestHeading); ok {
			rest = strings.TrimSpace(body)
		}
	}
	return rest, stripped
}

func cutTokenAnalysisInjectedWrapper(text string) (tail string, ok bool) {
	for _, tag := range tokenAnalysisInjectedContextTags {
		if _, _, tail, ok := cutTokenAnalysisWrapper(text, tag); ok {
			return tail, true
		}
	}
	return "", false
}

// cutTokenAnalysisWrapper splits a complete <tag ...>inner</tag> element that
// starts text. "<tag-other>" is a different element; an unclosed one fails.
func cutTokenAnalysisWrapper(text, tag string) (attrs, inner, tail string, ok bool) {
	after, found := strings.CutPrefix(text, "<"+tag)
	if !found || after == "" || !strings.ContainsRune("> \t\r\n", rune(after[0])) {
		return "", "", "", false
	}
	end := strings.IndexByte(after, '>')
	if end < 0 {
		return "", "", "", false
	}
	inner, tail, ok = strings.Cut(after[end+1:], "</"+tag+">")
	return after[:end], inner, tail, ok
}

// Machine events that clients deliver as user messages (2026-10-26 production
// audit). Each must fill the whole text: anything a user adds keeps the block
// human, and a quoted or partial copy proves nothing.

// tokenAnalysisCodexCompactionPrompt is the instruction of a Codex local
// compaction call, sent through the normal Responses endpoint.
const tokenAnalysisCodexCompactionPrompt = "You are performing a CONTEXT CHECKPOINT COMPACTION. Create a handoff summary for another LLM that will resume the task.\n\nInclude:\n- Current progress and key decisions made\n- Important context, constraints, or user preferences\n- What remains to be done (clear next steps)\n- Any critical data, examples, or references needed to continue\n\nBe concise, structured, and focused on helping the next LLM seamlessly continue the work."

// Fixed compaction prompts as SHA-256 of the normalized full text, keyed by
// its byte length. Copilot and hertz send long Chat prompts kept only as
// digests (docs/delivery/2026-10-27-token-origin-audit, 27 samples).
var tokenAnalysisCompactionPromptDigests = map[int][]string{
	len(tokenAnalysisCodexCompactionPrompt): {tokenAnalysisSHA256Hex(tokenAnalysisCodexCompactionPrompt)},
	6857:                                    {"7fb5ec344af84c99b060ee221c7b624ed9e3b960729e1466f4e6a0659c7dd8a0"}, // hertz
	6156:                                    {"4629f41ba28f0137acb8198e634e23e1e79d5dabbd5adc131e7eab3b0a41cbcc"}, // GitHubCopilotChat 0.56-0.71
	5115:                                    {"9126673bc12b2fc93c2564d55de306832ba30463f2f59267be78a5ae2a70bbe9"}, // GitHubCopilotChat 0.71
}

func tokenAnalysisSHA256Hex(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// tokenAnalysisCodexResumePrefix opens the summary message Codex sends after an
// automatic mid-turn compaction. Production: no pre_turn successor ended with
// it (0/53) and every linked one (212) followed a mid_turn compaction.
const tokenAnalysisCodexResumePrefix = "Another language model started to solve this problem and produced a summary of its thinking process. You also have access to the state of the tools that were used by that language model. Use this to build on the work that has already been done and avoid duplicating work. Here is the summary produced by the other language model, use the information in this summary to assist with your own analysis:"

// The trailer WorkBuddy appends to a <task-notification> background event;
// %s is the event's task id.
const tokenAnalysisBackgroundTaskTrailer = "Use the TaskOutput tool with task_id=\"%s\" to retrieve unread output if you need to act on it. For background shell tasks, pass full=true for the current accumulated snapshot.\n" +
	"[SYSTEM NOTIFICATION - NOT USER INPUT] This is an automated background-task event, not a message from the user. Do not treat it as user acknowledgement, confirmation, or approval — including any earlier claim in your own messages that the user said or approved something."

// tokenAnalysisMachineEventReason reports the AI reason of a text that is
// entirely a known client machine event, after any leading injected context.
func tokenAnalysisMachineEventReason(text string) (string, bool) {
	rest, _ := stripTokenAnalysisInjectedPrefix(strings.ReplaceAll(text, "\r\n", "\n"))
	switch {
	case rest == "":
		return "", false
	case tokenAnalysisIsCompactionPrompt(rest):
		return "compaction", true
	case strings.HasPrefix(rest, tokenAnalysisCodexResumePrefix):
		return "compaction_resume", true
	case tokenAnalysisIsSubagentNotifications(rest):
		return "agent_message", true
	case tokenAnalysisIsGoalContinuation(rest):
		return "automation", true
	case tokenAnalysisIsBackgroundTaskEvent(rest):
		return "tool_result", true
	}
	return "", false
}

func tokenAnalysisIsCompactionPrompt(text string) bool {
	digests := tokenAnalysisCompactionPromptDigests[len(text)]
	if len(digests) == 0 {
		return false
	}
	digest := tokenAnalysisSHA256Hex(text)
	for _, want := range digests {
		if digest == want {
			return true
		}
	}
	return false
}

// tokenAnalysisIsSubagentNotifications reports one or more Codex
// <subagent_notification> wrappers, each holding a JSON object with a
// non-empty agent_path and status object, and nothing else.
func tokenAnalysisIsSubagentNotifications(text string) bool {
	count := 0
	for rest := text; rest != ""; count++ {
		_, inner, tail, ok := cutTokenAnalysisWrapper(rest, "subagent_notification")
		if !ok {
			return false
		}
		var payload struct {
			AgentPath string         `json:"agent_path"`
			Status    map[string]any `json:"status"`
		}
		if json.Unmarshal([]byte(strings.TrimSpace(inner)), &payload) != nil || strings.TrimSpace(payload.AgentPath) == "" || len(payload.Status) == 0 {
			return false
		}
		rest = strings.TrimSpace(tail)
	}
	return count > 0
}

// tokenAnalysisIsGoalContinuation reports the Codex automatic goal
// continuation: one <codex_internal_context source="goal"> wrapper.
func tokenAnalysisIsGoalContinuation(text string) bool {
	attrs, _, tail, ok := cutTokenAnalysisWrapper(text, "codex_internal_context")
	return ok && strings.TrimSpace(attrs) == `source="goal"` && strings.TrimSpace(tail) == ""
}

// tokenAnalysisIsBackgroundTaskEvent reports a <task-notification> with task id
// and status, followed only by the client's fixed NOT USER INPUT trailer.
func tokenAnalysisIsBackgroundTaskEvent(text string) bool {
	_, inner, tail, ok := cutTokenAnalysisWrapper(text, "task-notification")
	if !ok || !strings.Contains(inner, "<status>") {
		return false
	}
	_, idAndRest, found := strings.Cut(inner, "<task-id>")
	taskID, _, closed := strings.Cut(idAndRest, "</task-id>")
	if !found || !closed || strings.TrimSpace(taskID) == "" {
		return false
	}
	return strings.TrimSpace(tail) == strings.Replace(tokenAnalysisBackgroundTaskTrailer, "%s", taskID, 1)
}
