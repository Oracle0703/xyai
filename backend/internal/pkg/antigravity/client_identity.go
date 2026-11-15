package antigravity

import (
	"os"
	"regexp"
	"sort"
	"strings"
)

// Agent clients other than Claude Code, Codex and Pi announce themselves the same way
// ("You are Cline, an AI coding agent.", "You are OpenCode, the best coding agent on the
// planet."), and the Antigravity upstream rejects such a system instruction with the same
// bare 429 RESOURCE_EXHAUSTED. On 2026-11-15 Cline's opener alone returned 429 while
// "You are an AI agent." returned 200 on the same account and model, through a Claude model
// mapped on Antigravity. Instead of adding one opener per client, a leading
// "You are <Name>" that declares an agent product is rewritten to "You are an AI agent",
// keeping the rest of the sentence.
//
// <Name> is either
//   - a known product name (case-insensitive, no further check), or
//   - a capitalized name of up to three words whose own sentence describes an AI or
//     coding agent, so personas such as "You are Alice, a friendly tutor." stay untouched.
//
// Operators add product names through SUB2API_ANTIGRAVITY_IDENTITY_NAMES (comma separated),
// read once at startup. Google's own identities are never rewritten.
const clientIdentityNamesEnv = "SUB2API_ANTIGRAVITY_IDENTITY_NAMES"

var defaultClientIdentityNames = []string{
	"opencode", "cline", "kilo code", "kilocode", "roo code", "roo", "crush", "goose", "aider",
	"amp", "droid", "cursor", "windsurf", "cascade", "github copilot", "copilot", "trae", "qoder", "iflow", "kiro",
}

// The upstream accepts these, and the transformer relies on "You are Antigravity" to decide
// whether to inject its own identity patch.
var keptClientIdentityNames = map[string]bool{"antigravity": true, "gemini": true}

var (
	capitalizedClientIdentity = regexp.MustCompile(
		`^[ \t\r\n]*You are (?:called |named )?([A-Z][A-Za-z0-9_+-]*(?:\.[A-Za-z0-9]+)*(?: [A-Z][A-Za-z0-9_+-]*(?:\.[A-Za-z0-9]+)*){0,2})[ \t]*(?:[,.!:;(]|-|–|—|\r?\n|$)`)
	agentDescription    = regexp.MustCompile(`(?i)\b(?:agent|agentic|assistant|engineer|developer|programmer|coding|coder|code|cli|ide|software|llm|language model|ai)\b`)
	knownClientIdentity = compileKnownClientIdentity(os.Getenv(clientIdentityNamesEnv))
)

func compileKnownClientIdentity(extra string) *regexp.Regexp {
	seen := map[string]bool{}
	var names []string
	for _, name := range append(append([]string(nil), defaultClientIdentityNames...), strings.Split(extra, ",")...) {
		name = strings.ToLower(strings.Join(strings.Fields(name), " "))
		if name == "" || seen[name] || keptClientIdentityNames[strings.Fields(name)[0]] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	// Longest first, so "roo code" wins over "roo".
	sort.SliceStable(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })
	for i, name := range names {
		names[i] = strings.ReplaceAll(regexp.QuoteMeta(name), " ", `[ \t]+`)
	}
	return regexp.MustCompile(`(?i)^[ \t\r\n]*You are (?:called |named )?(` + strings.Join(names, "|") + `)\b`)
}

// neutralizeClientIdentity rewrites a leading agent product identity. It runs after the
// client-specific openers in NeutralizeSystemIdentity.
func neutralizeClientIdentity(text string) string {
	if loc := knownClientIdentity.FindStringSubmatchIndex(text); loc != nil {
		return "You are an AI agent" + text[loc[3]:]
	}
	loc := capitalizedClientIdentity.FindStringSubmatchIndex(text)
	if loc == nil {
		return text
	}
	name, rest := text[loc[2]:loc[3]], text[loc[3]:]
	if keptClientIdentityNames[strings.ToLower(strings.Fields(name)[0])] || !agentDescription.MatchString(firstSentence(rest)) {
		return text
	}
	return "You are an AI agent" + rest
}

func firstSentence(text string) string {
	if i := strings.IndexAny(text, "\r\n"); i >= 0 {
		text = text[:i]
	}
	if i := strings.Index(text, ". "); i >= 0 {
		text = text[:i]
	}
	if len(text) > 300 {
		text = text[:300]
	}
	return text
}
