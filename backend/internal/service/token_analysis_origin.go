package service

import (
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

// Reason decides kind; kind decides confidence. The frontend mirrors this table
// (requestOrigin.ts) to validate the v1 contract.
var tokenAnalysisOriginReasonKinds = map[string]string{
	"subagent":               "ai",
	"automation":             "ai",
	"tool_result":            "ai",
	"agent_message":          "ai",
	"assistant_continuation": "ai",
	"compaction":             "ai",
	"user_input":             "human",
	"no_signal":              "unknown",
	"unsupported_shape":      "unknown",
	"body_truncated":         "unknown",
	"count_tokens":           "unknown",
}

func tokenAnalysisOrigin(reason string) tokenAnalysisRequestOrigin {
	kind, ok := tokenAnalysisOriginReasonKinds[reason]
	if !ok {
		kind = "unknown"
	}
	confidence := "none"
	if kind == "ai" {
		confidence = "high"
	} else if kind == "human" {
		confidence = "medium"
	}
	return tokenAnalysisRequestOrigin{Kind: kind, Reason: reason, Confidence: confidence, Version: 1}
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
	// composer/user/thread_source and the mere presence of tools prove nothing.
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
	switch turn["turn_trigger"] {
	case "automation_cron_scheduled", "goal", "memory_consolidation", "guardian_review", "guardian_classifier", "ambient_suggestions", "thread_title":
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
			} else if strings.TrimSpace(text) != "" {
				human = true
			}
		case typ == "image" || typ == "image_url" || typ == "input_image" || typ == "input_file" || typ == "file" || typ == "document" || typ == "input_audio":
			if len(block) <= 1 {
				return tokenAnalysisOrigin("unsupported_shape"), false
			}
			human = true
		case gemini && (block["inlineData"] != nil || block["inline_data"] != nil || block["fileData"] != nil || block["file_data"] != nil):
			human = true
		default:
			return tokenAnalysisOrigin("unsupported_shape"), false
		}
	}
	if human {
		return tokenAnalysisOrigin("user_input"), false
	}
	if tool {
		return tokenAnalysisOrigin("tool_result"), false
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

// Client-injected wrappers that carry no new user intent: Codex
// <environment_context>, the Codex app's <in-app-browser-context> ambient UI
// state (prepended to the composer text, seen in production), and Claude Code
// <system-reminder> (seen beside user text and as reminder-only continuations
// after an assistant turn).
var tokenAnalysisInjectedContextTags = []string{"environment_context", "in-app-browser-context", "system-reminder"}

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
		after, found := strings.CutPrefix(text, "<"+tag)
		// "<tag>" or "<tag attr=...>"; "<tag-other>" is a different element.
		if !found || after == "" || !strings.ContainsRune("> \t\r\n", rune(after[0])) {
			continue
		}
		end := strings.IndexByte(after, '>')
		if end < 0 {
			return "", false
		}
		_, tail, closed := strings.Cut(after[end+1:], "</"+tag+">")
		return tail, closed
	}
	return "", false
}
