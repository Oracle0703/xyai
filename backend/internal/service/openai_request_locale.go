package service

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"go.uber.org/zap"
)

// Only parse field boundaries. Environment text is XML-like, but fields such as
// subagents and network can contain unescaped text and are opaque to this rewrite.
var openAIEnvironmentFieldStart = regexp.MustCompile(`^<([A-Za-z_][A-Za-z0-9_.:-]*)(?:[ \t\r\n]+(?:[^<>"']|"[^"]*"|'[^']*')*)?[ \t\r\n]*/?>`)

// Closing tags may contain whitespace before >. Match only the field name;
// nested fields and unescaped characters in the body remain opaque.
func findOpenAIEnvironmentFieldEnd(text, name string) (start, end int) {
	prefix := "</" + name
	for offset := 0; offset < len(text); {
		index := strings.Index(text[offset:], prefix)
		if index < 0 {
			break
		}
		start = offset + index
		offset = start + len(prefix)
		rest := strings.TrimLeft(text[offset:], " \t\r\n")
		if strings.HasPrefix(rest, ">") {
			return start, len(text) - len(rest) + 1
		}
	}
	return -1, -1
}

func rewriteOpenAIRequestEnvironment(text, timezone string) (rewritten, previous, reason string) {
	trimmed := strings.TrimSpace(text)
	root := openAIEnvironmentFieldStart.FindStringSubmatchIndex(trimmed)
	if root == nil || trimmed[root[2]:root[3]] != "environment_context" {
		return text, "", "no_environment_context"
	}
	rootCloseStart, rootCloseEnd := findOpenAIEnvironmentFieldEnd(trimmed[root[1]:], "environment_context")
	if strings.HasSuffix(trimmed[:root[1]], "/>") || rootCloseEnd != len(trimmed)-root[1] {
		return text, "", "invalid_environment_context"
	}
	content := trimmed[root[1] : root[1]+rootCloseStart]
	valueStart, valueEnd := -1, -1
	for offset := 0; offset < len(content); {
		rest := strings.TrimLeftFunc(content[offset:], unicode.IsSpace)
		offset = len(content) - len(rest)
		if rest == "" {
			break
		}
		field := openAIEnvironmentFieldStart.FindStringSubmatchIndex(rest)
		if field == nil {
			return text, "", "invalid_environment_context"
		}
		name := rest[field[2]:field[3]]
		if name == "environment_context" {
			return text, "", "invalid_environment_context"
		}
		selfClosing := strings.HasSuffix(rest[:field[1]], "/>")
		if name == "timezone" && (valueStart >= 0 || selfClosing) {
			return text, "", "invalid_environment_context"
		}
		offset += field[1]
		if selfClosing {
			continue
		}
		end, closeEnd := findOpenAIEnvironmentFieldEnd(content[offset:], name)
		if end < 0 {
			return text, "", "invalid_environment_context"
		}
		if name == "timezone" {
			value := content[offset : offset+end]
			previous = strings.TrimSpace(value)
			if previous == "" || strings.Contains(value, "<") {
				return text, "", "invalid_environment_context"
			}
			valueStart = offset + len(value) - len(strings.TrimLeftFunc(value, unicode.IsSpace))
			valueEnd = offset + len(strings.TrimRightFunc(value, unicode.IsSpace))
		}
		offset += closeEnd
	}
	if valueStart < 0 {
		return text, "", "no_timezone"
	}
	if previous == timezone {
		return text, previous, "already_target"
	}
	// Patch only the timezone value, preserving all original tags and whitespace.
	base := len(text) - len(strings.TrimLeftFunc(text, unicode.IsSpace)) + root[1]
	return text[:base+valueStart] + timezone + text[base+valueEnd:], previous, "replaced"
}

const openAIRequestLocaleLogMessage = "openai request timezone normalization"

// applyOpenAIRequestLocale is the single gate shared by every OpenAI forwarding
// path (HTTP, passthrough, compact, WS ingress and WS passthrough). The rewrite
// runs only when the effective account switch (the parent's for Spark shadows)
// and the global switch are both on; otherwise body is returned untouched and is
// not parsed. The global switch is read from the cached settings snapshot on every
// call, so WS connections pick up a change on their next response.create.
func (s *OpenAIGatewayService) applyOpenAIRequestLocale(ctx context.Context, c *gin.Context, account *Account, body []byte, transport string) []byte {
	if account == nil || !account.IsOpenAI() {
		return body
	}
	source := openAIRequestTimezoneSource(c, account)
	reason := ""
	switch {
	case source == nil:
		reason = "parent_unavailable"
	case !source.OpenAIRequestTimezoneRewriteEnabled():
		reason = "account_disabled"
	case s == nil || s.settingService == nil:
		reason = "settings_unavailable"
	default:
		if enabled, unavailable := s.settingService.OpenAIRequestTimezoneRewriteState(ctx); unavailable {
			reason = "settings_unavailable"
		} else if !enabled {
			reason = "global_disabled"
		}
	}
	if reason != "" {
		if entry := logger.FromContext(ctx).Check(zap.DebugLevel, openAIRequestLocaleLogMessage); entry != nil {
			entry.Write(zap.Int64("account_id", account.ID), zap.String("transport", transport),
				zap.Bool("enabled", false), zap.String("reason", reason))
		}
		return body
	}
	return normalizeOpenAIRequestLocale(ctx, account, source.OpenAIRequestTimezone(), body, transport)
}

// normalizeOpenAIRequestLocale rewrites the structured timezone fields of an
// enabled request. It is all-or-nothing: if any write fails the original body is
// returned. Logs carry counts and a reason only, never client values or prompt text.
func normalizeOpenAIRequestLocale(ctx context.Context, account *Account, timezone string, body []byte, transport string) []byte {
	out := body
	matched, replaced, alreadyTarget := 0, 0, 0
	invalidEnvironment, failed := false, false
	rewriteText := func(path string, text gjson.Result) {
		if failed || text.Type != gjson.String {
			return
		}
		next, _, reason := rewriteOpenAIRequestEnvironment(text.String(), timezone)
		switch reason {
		case "no_environment_context":
			return
		case "invalid_environment_context":
			invalidEnvironment = true
		case "already_target":
			alreadyTarget++
		case "replaced":
			updated, err := sjson.SetBytes(out, path, next)
			if err != nil {
				failed = true
				return
			}
			out = updated
			replaced++
		}
		matched++
	}
	input := gjson.GetBytes(body, "input")
	if input.IsArray() {
		for inputIndex, item := range input.Array() {
			if item.Get("role").String() != "user" {
				continue
			}
			content := item.Get("content")
			path := "input." + strconv.Itoa(inputIndex) + ".content"
			if content.Type == gjson.String {
				rewriteText(path, content)
			} else if content.IsArray() {
				for contentIndex, part := range content.Array() {
					if part.Get("type").String() == "input_text" {
						rewriteText(path+"."+strconv.Itoa(contentIndex)+".text", part.Get("text"))
					}
				}
			}
		}
	}
	webSearchReplaced := 0
	tools := gjson.GetBytes(body, "tools")
	if tools.IsArray() && !failed {
		for index, tool := range tools.Array() {
			kind := tool.Get("type").String()
			if kind != "web_search" && !strings.HasPrefix(kind, "web_search_") {
				continue
			}
			value := tool.Get("user_location.timezone")
			if value.Type != gjson.String || value.String() == timezone {
				continue
			}
			updated, err := sjson.SetBytes(out, "tools."+strconv.Itoa(index)+".user_location.timezone", timezone)
			if err != nil {
				failed = true
				break
			}
			out = updated
			webSearchReplaced++
		}
	}
	reason := "no_environment_context"
	switch {
	case failed:
		reason = "rewrite_failed"
		out, replaced, webSearchReplaced = body, 0, 0
	case replaced > 0 || webSearchReplaced > 0:
		reason = "replaced"
	case invalidEnvironment:
		reason = "invalid_environment_context"
	case alreadyTarget > 0:
		reason = "already_target"
	case matched > 0:
		reason = "no_timezone"
	}
	if entry := logger.FromContext(ctx).Check(zap.DebugLevel, openAIRequestLocaleLogMessage); entry != nil {
		entry.Write(zap.Int64("account_id", account.ID), zap.String("transport", transport),
			zap.Bool("enabled", true), zap.String("reason", reason), zap.String("target_timezone", timezone),
			zap.Int("matched_count", matched), zap.Int("replaced_count", replaced),
			zap.Int("web_search_replaced_count", webSearchReplaced))
	}
	return out
}
