package service

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Normalize at account selection, before typed protocol converters discard
// extension fields and before Responses strips its compatible "none" placeholder.
func (s *OpenAIGatewayService) prepareGrokAutoHTTPEffort(ctx context.Context, c *gin.Context, account *Account, body []byte, protocol, defaultMappedModel string) ([]byte, *string, error) {
	if account == nil || (!account.IsGrok() && !account.IsOpenAIApiKey()) {
		return body, nil, nil
	}
	model := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	var upstream string
	if protocol == APIProtocolChatCompletions {
		upstream = resolveOpenAIForwardModel(account, model, defaultMappedModel)
		if !gjson.GetBytes(body, "messages").Exists() && gjson.GetBytes(body, "input").Exists() {
			protocol = APIProtocolResponses
		}
	} else if account.IsGrok() {
		upstream = account.GetMappedModel(model)
	} else {
		compact := isOpenAIResponsesCompactPath(c)
		upstream = resolveOpenAIAccountUpstreamModelForRequest(account, model, compact)
		if compact && !shouldForwardOpenAIResponsesViaRawChatCompletions(account) {
			if fallback := s.resolveOpenAICompactFallbackModel(account, model); fallback != "" {
				upstream = fallback
			}
		}
	}
	if !grokDefaultsAutoEffortToHigh(upstream) {
		return body, nil, nil
	}
	out, requested, err := normalizeGrokAutoEffort(ctx, body, protocol)
	if IsReasoningEffortPolicyDenied(err) {
		MarkOpsClientBusinessLimited(c, OpsClientBusinessLimitedReasonLocalPolicyDenied)
		writeChatCompletionsError(c, http.StatusForbidden, "permission_error", err.Error())
	}
	return out, requested, err
}

func grokDefaultsAutoEffortToHigh(upstream string) bool {
	model := strings.ToLower(xai.ResolveGrokTextResponsesModelID(upstream))
	minor, ok := strings.CutPrefix(strings.TrimSuffix(model, "-latest"), "grok-4.")
	if !ok || minor == "" || minor[0] < '1' || minor[0] > '9' {
		return false
	}
	// 4.20 names an older xAI family, including its own reasoning variants.
	version, err := strconv.ParseUint(minor, 10, 32)
	// A newer standard version joins once the outbound capability table lists
	// it, so high never reaches a model not known to accept the parameter.
	return err == nil && version >= 7 && version != 20 && grokSupportsReasoningEffort(model)
}

// Only a literal auto or WeKnora's generic switch is rewritten. Explicit
// levels, off switches and other vendors' dialects are returned untouched.
func normalizeGrokAutoEffort(ctx context.Context, body []byte, protocol string) ([]byte, *string, error) {
	paths := []string{"reasoning.effort", "reasoning_effort", "reasoningEffort"}
	if protocol == APIProtocolChatCompletions {
		paths = []string{"reasoning_effort", "reasoningEffort", "reasoning.effort"}
	}
	auto := grokGenericThinkingEnabled(body)
	for _, path := range paths {
		if value := gjson.GetBytes(body, path); value.Exists() {
			auto = strings.EqualFold(strings.TrimSpace(value.Str), "auto")
			break
		}
	}
	if !auto {
		return body, nil, nil
	}
	effort := "high"
	// The handler saw auto or no effort at all, so the group policy has not
	// judged the materialized level yet.
	if policy, ok := ctx.Value(grokAutoEffortPolicyKey{}).(grokAutoEffortPolicy); ok {
		rank, _ := reasoningEffortRank(policy.maxEffort)
		var err error
		effort, err = evaluateReasoningEffort(effort, policy.model, policy.mappings, NormalizeMaxReasoningEffort(policy.maxEffort), rank, policy.overLimit == ReasoningEffortOverLimitDeny)
		if err != nil {
			return body, nil, err
		}
	}
	out, err := sjson.SetBytes(body, paths[0], effort)
	// Remove only what the conversion replaced. Other template kwargs and
	// reasoning options such as Responses reasoning.summary stay.
	for _, path := range append(paths[1:], "chat_template_kwargs.enable_thinking") {
		if err == nil && gjson.GetBytes(out, path).Exists() {
			out, err = sjson.DeleteBytes(out, path)
		}
	}
	for _, path := range []string{"reasoning", "chat_template_kwargs"} {
		if object := gjson.GetBytes(out, path); err == nil && object.IsObject() && len(object.Map()) == 0 {
			out, err = sjson.DeleteBytes(out, path)
		}
	}
	if err != nil {
		return body, nil, err
	}
	return out, optionalTrimmedStringPtr("auto"), nil
}

// WeKnora's generic provider sends "thinking on, level decided by the server"
// as chat_template_kwargs.enable_thinking. Other vendors' switches never
// trigger the conversion, and one that says off or is malformed vetoes it.
func grokGenericThinkingEnabled(body []byte) bool {
	if gjson.GetBytes(body, "chat_template_kwargs.enable_thinking").Type != gjson.True {
		return false
	}
	if gjson.GetBytes(body, "thinking").Type == gjson.False || strings.EqualFold(gjson.GetBytes(body, "thinking.type").Str, "disabled") {
		return false
	}
	for _, path := range []string{"enable_thinking", "reasoning.enabled"} {
		if flag := gjson.GetBytes(body, path); flag.Exists() && flag.Type != gjson.True {
			return false
		}
	}
	return true
}

func prepareGrokAutoWSEffort(ctx context.Context, account *Account, hooks *OpenAIWSIngressHooks, clientModel, upstreamModel string, body []byte) ([]byte, *string, error) {
	if (!account.IsGrok() && !account.IsOpenAIApiKey()) || !grokDefaultsAutoEffortToHigh(upstreamModel) {
		return body, nil, nil
	}
	if hooks != nil {
		ctx = WithGrokAutoEffortPolicy(ctx, clientModel, hooks.MaxReasoningEffort, hooks.ReasoningEffortMappings, hooks.MaxReasoningEffortOverLimit)
	}
	return normalizeGrokAutoEffort(ctx, body, APIProtocolResponses)
}
