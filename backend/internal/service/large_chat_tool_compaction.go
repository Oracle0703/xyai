package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

const (
	LargeRequestModeOff               = "off"
	LargeRequestModeWarn              = "warn"
	LargeRequestModeToolOutputCompact = "tool_output_compact"
)

type LargeRequestScope struct {
	Enabled bool
}

type LargeRequestIdentity struct {
	UserID   int64
	APIKeyID int64
	GroupID  int64
}

type LargeChatToolCompactionResult struct {
	Request                  apicompat.ChatCompletionsRequest
	Body                     []byte
	LargeRequestDetected     bool
	Compacted                bool
	Mode                     string
	ToolOutputBytes          int64
	ToolMessageCount         int
	CompressedToolMessages   int
	CompressedMessageIndices []int
	CompressedOriginalBytes  int64
	CompressedFinalBytes     int64
	RequestBodySizeBefore    int64
	RequestBodySizeAfter     int64
	PromptCacheKeyInjected   bool
	PromptCacheKey           string
	SkipReasons              map[string]int
}

func LargeRequestScopeFromConfig(cfg config.GatewayLargeRequestConfig, identity LargeRequestIdentity) LargeRequestScope {
	if !cfg.Enabled || normalizeLargeRequestMode(cfg.Mode) != LargeRequestModeToolOutputCompact {
		return LargeRequestScope{Enabled: false}
	}
	if len(cfg.EnabledUserIDs) == 0 && len(cfg.EnabledAPIKeyIDs) == 0 && len(cfg.EnabledGroupIDs) == 0 {
		return LargeRequestScope{Enabled: false}
	}
	return LargeRequestScope{
		Enabled: containsInt64(cfg.EnabledUserIDs, identity.UserID) ||
			containsInt64(cfg.EnabledAPIKeyIDs, identity.APIKeyID) ||
			containsInt64(cfg.EnabledGroupIDs, identity.GroupID),
	}
}

func MaybeInjectLargeRequestPromptCacheKey(result *LargeChatToolCompactionResult, identity LargeRequestIdentity) {
	if result == nil || !result.LargeRequestDetected || strings.TrimSpace(result.Request.PromptCacheKey) != "" {
		return
	}
	seed := largeRequestPromptCacheSeed(result.Request, identity)
	if seed == "" {
		return
	}
	sum := sha256.Sum256([]byte(seed))
	key := "sub2api-large-" + hex.EncodeToString(sum[:])[:32]
	result.Request.PromptCacheKey = key
	result.PromptCacheKey = key
	result.PromptCacheKeyInjected = true
	body, err := marshalLargeChatRequest(result.Request)
	if err == nil {
		result.Body = body
		result.RequestBodySizeAfter = int64(len(body))
	}
}

type largeToolCandidate struct {
	index int
	bytes int64
}

func CompactLargeChatToolOutputs(
	req apicompat.ChatCompletionsRequest,
	originalBody []byte,
	cfg config.GatewayLargeRequestConfig,
	scope LargeRequestScope,
) (LargeChatToolCompactionResult, error) {
	result := LargeChatToolCompactionResult{
		Request:               req,
		Body:                  append([]byte(nil), originalBody...),
		Mode:                  normalizeLargeRequestMode(cfg.Mode),
		RequestBodySizeBefore: int64(len(originalBody)),
		RequestBodySizeAfter:  int64(len(originalBody)),
		SkipReasons:           map[string]int{},
	}
	if !cfg.Enabled || result.Mode == LargeRequestModeOff {
		return result, nil
	}

	toolIndices, messageBytes, lastUserIndex := collectLargeToolStats(req.Messages, &result)
	if result.RequestBodySizeBefore <= cfg.BodyThresholdBytes || result.ToolOutputBytes <= cfg.ToolTotalThresholdBytes {
		return result, nil
	}
	result.LargeRequestDetected = true
	if result.Mode != LargeRequestModeToolOutputCompact || !scope.Enabled {
		return result, nil
	}

	candidates := selectLargeToolCandidates(req.Messages, toolIndices, messageBytes, lastUserIndex, cfg, &result)
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].bytes == candidates[j].bytes {
			return candidates[i].index < candidates[j].index
		}
		return candidates[i].bytes > candidates[j].bytes
	})

	currentBody := append([]byte(nil), originalBody...)
	currentSize := int64(len(currentBody))
	for _, candidate := range candidates {
		if cfg.TargetBodyBytes > 0 && currentSize <= cfg.TargetBodyBytes {
			break
		}
		content := rawJSONStringValue(result.Request.Messages[candidate.index].Content)
		replacement := compactToolContent(content, candidate.bytes, cfg.HeadChars, cfg.TailChars)
		if replacement == content {
			continue
		}
		result.Request.Messages[candidate.index].Content = mustMarshalRawStringValue(replacement)
		updated, err := marshalLargeChatRequest(result.Request)
		if err != nil {
			return result, fmt.Errorf("marshal compacted chat request: %w", err)
		}
		result.Compacted = true
		result.CompressedToolMessages++
		result.CompressedMessageIndices = append(result.CompressedMessageIndices, candidate.index)
		result.CompressedOriginalBytes += candidate.bytes
		result.CompressedFinalBytes += int64(len(result.Request.Messages[candidate.index].Content))
		currentBody = updated
		currentSize = int64(len(updated))
	}
	result.Body = currentBody
	result.RequestBodySizeAfter = currentSize
	return result, nil
}

func marshalLargeChatRequest(req apicompat.ChatCompletionsRequest) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(req); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}

func collectLargeToolStats(messages []apicompat.ChatMessage, result *LargeChatToolCompactionResult) ([]int, []int64, int) {
	toolIndices := make([]int, 0)
	messageBytes := make([]int64, len(messages))
	lastUserIndex := -1
	for i, msg := range messages {
		messageBytes[i] = int64(len(msg.Content))
		switch msg.Role {
		case "user":
			lastUserIndex = i
		case "tool":
			toolIndices = append(toolIndices, i)
			if result != nil {
				result.ToolMessageCount++
				result.ToolOutputBytes += messageBytes[i]
			}
		}
	}
	return toolIndices, messageBytes, lastUserIndex
}

func selectLargeToolCandidates(
	messages []apicompat.ChatMessage,
	toolIndices []int,
	messageBytes []int64,
	lastUserIndex int,
	cfg config.GatewayLargeRequestConfig,
	result *LargeChatToolCompactionResult,
) []largeToolCandidate {
	absoluteKeep := lastNIndexSet(toolIndices, cfg.AbsoluteRecentToolKeep)
	recentKeep := lastNIndexSet(toolIndices, cfg.RecentToolKeep)
	candidates := make([]largeToolCandidate, 0)
	for _, idx := range toolIndices {
		msg := messages[idx]
		bytesLen := messageBytes[idx]
		switch {
		case absoluteKeep[idx]:
			result.SkipReasons["absolute_recent_tool_keep"]++
			continue
		case lastUserIndex >= 0 && idx > lastUserIndex:
			result.SkipReasons["after_last_user_keep"]++
			continue
		case !isJSONString(msg.Content):
			result.SkipReasons["non_string_tool_content"]++
			continue
		case looksLikeBase64ToolContent(rawJSONStringValue(msg.Content)):
			result.SkipReasons["base64_or_binary"]++
			continue
		case recentKeep[idx] && bytesLen <= cfg.GiantToolThresholdBytes:
			result.SkipReasons["recent_tool_keep_below_giant_threshold"]++
			continue
		case bytesLen <= cfg.NormalToolThresholdBytes:
			result.SkipReasons["below_normal_threshold"]++
			continue
		default:
			candidates = append(candidates, largeToolCandidate{index: idx, bytes: bytesLen})
		}
	}
	return candidates
}

func normalizeLargeRequestMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case LargeRequestModeOff:
		return LargeRequestModeOff
	case LargeRequestModeToolOutputCompact:
		return LargeRequestModeToolOutputCompact
	default:
		return LargeRequestModeWarn
	}
}

func largeRequestPromptCacheSeed(req apicompat.ChatCompletionsRequest, identity LargeRequestIdentity) string {
	var b strings.Builder
	b.WriteString(req.Model)
	b.WriteString("|u=")
	b.WriteString(fmt.Sprint(identity.UserID))
	b.WriteString("|k=")
	b.WriteString(fmt.Sprint(identity.APIKeyID))
	b.WriteString("|g=")
	b.WriteString(fmt.Sprint(identity.GroupID))
	for _, msg := range req.Messages {
		switch msg.Role {
		case "system", "developer":
			b.WriteString("|msg=")
			b.Write(msg.Content)
		}
	}
	if len(req.Tools) > 0 {
		body, _ := json.Marshal(req.Tools)
		b.WriteString("|tools=")
		b.Write(body)
	}
	return b.String()
}

func lastNIndexSet(indices []int, n int) map[int]bool {
	out := map[int]bool{}
	if n <= 0 {
		return out
	}
	start := len(indices) - n
	if start < 0 {
		start = 0
	}
	for _, idx := range indices[start:] {
		out[idx] = true
	}
	return out
}

func isJSONString(raw json.RawMessage) bool {
	var s string
	return json.Unmarshal(raw, &s) == nil
}

func rawJSONStringValue(raw json.RawMessage) string {
	var s string
	_ = json.Unmarshal(raw, &s)
	return s
}

func compactToolContent(content string, originalBytes int64, headChars, tailChars int) string {
	if headChars <= 0 {
		headChars = 8000
	}
	if tailChars <= 0 {
		tailChars = 8000
	}
	runes := []rune(content)
	if len(runes) <= headChars+tailChars {
		return content
	}
	sum := sha256.Sum256([]byte(content))
	head := string(runes[:headChars])
	tail := string(runes[len(runes)-tailChars:])
	omitted := len(runes) - headChars - tailChars
	return fmt.Sprintf("[Sub2API compressed historical tool output]\noriginal_bytes: %d\nsha256: %s\nkept_head_chars: %d\nkept_tail_chars: %d\nomitted_chars: %d\nreason: large historical role=tool output\n\n--- kept head ---\n%s\n--- omitted middle ---\n--- kept tail ---\n%s",
		originalBytes,
		hex.EncodeToString(sum[:]),
		headChars,
		tailChars,
		omitted,
		head,
		tail,
	)
}

func mustMarshalRawStringValue(value string) json.RawMessage {
	body, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`""`)
	}
	return body
}

func looksLikeBase64ToolContent(content string) bool {
	trimmed := strings.TrimSpace(content)
	if strings.HasPrefix(trimmed, "data:") {
		return true
	}
	if len(trimmed) < 32768 {
		return false
	}
	compact := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\n', '\r', '\t':
			return -1
		default:
			return r
		}
	}, trimmed)
	if len(compact)%4 != 0 {
		return false
	}
	for _, r := range compact {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && r != '+' && r != '/' && r != '=' {
			return false
		}
	}
	return true
}
