package antigravity

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// The opener of the Cline request that the Antigravity upstream rejected with a bare 429
// on 2026-11-15 (the same request with "You are an AI agent." returned 200).
const clineOpener = "You are Cline, an AI coding agent. Your primary goal is to assist users with various coding tasks."

func TestNeutralizeSystemIdentity_ClientIdentity(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{"cline", clineOpener, "You are an AI agent, an AI coding agent. Your primary goal is to assist users with various coding tasks."},
		{"cline classic", "You are Cline, a highly skilled software engineer with extensive knowledge.\nRules.",
			"You are an AI agent, a highly skilled software engineer with extensive knowledge.\nRules."},
		{"opencode", "You are OpenCode, the best coding agent on the planet.", "You are an AI agent, the best coding agent on the planet."},
		{"opencode lower case", "You are opencode, an interactive CLI tool that helps users with software engineering tasks.",
			"You are an AI agent, an interactive CLI tool that helps users with software engineering tasks."},
		{"kilo code", "You are Kilo Code, a highly skilled software engineer.", "You are an AI agent, a highly skilled software engineer."},
		{"roo", "You are Roo, a highly skilled software engineer.", "You are an AI agent, a highly skilled software engineer."},
		{"amp", "You are Amp, a powerful AI coding agent built by Sourcegraph.", "You are an AI agent, a powerful AI coding agent built by Sourcegraph."},
		{"known name without description", "You are Cline.\nHelp the user.", "You are an AI agent.\nHelp the user."},
		{"unknown agent product", "  \nYou are Foobot, an AI coding assistant. Rest.", "You are an AI agent, an AI coding assistant. Rest."},
		{"unknown multi-word product", "You are Acme Dev Agent - a software engineering agent.", "You are an AI agent - a software engineering agent."},

		// Left untouched.
		{"description only", "You are a helpful assistant.", "You are a helpful assistant."},
		{"persona", "You are Alice, a friendly tutor for kids.", "You are Alice, a friendly tutor for kids."},
		{"capitalized name without description", "You are Foobot.\nYou are an AI coding agent.", "You are Foobot.\nYou are an AI coding agent."},
		{"antigravity", "You are Antigravity, a powerful agentic AI coding assistant.", "You are Antigravity, a powerful agentic AI coding assistant."},
		{"gemini", "You are Gemini CLI, an interactive CLI agent.", "You are Gemini CLI, an interactive CLI agent."},
		{"not at start", "Example: " + clineOpener, "Example: " + clineOpener},
		{"mention", "Use Cline or OpenCode to edit files.", "Use Cline or OpenCode to edit files."},
		{"name prefix", "You are Roomba's coding assistant.", "You are Roomba's coding assistant."},
		{"already neutral", "You are an AI agent, an AI coding agent.", "You are an AI agent, an AI coding agent."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NeutralizeSystemIdentity(tt.text)
			require.Equal(t, tt.want, got)
			require.Equal(t, got, NeutralizeSystemIdentity(got), "idempotent")
		})
	}
}

func TestCompileKnownClientIdentity_ExtraNames(t *testing.T) {
	re := compileKnownClientIdentity(" Foo  Bar , ,gemini,baz,cline")
	rewrite := func(text string) string {
		if loc := re.FindStringSubmatchIndex(text); loc != nil {
			return "You are an AI agent" + text[loc[3]:]
		}
		return text
	}

	require.Equal(t, "You are an AI agent, the helper.", rewrite("You are foo bar, the helper."))
	require.Equal(t, "You are an AI agent.", rewrite("You are BAZ."))
	require.Equal(t, "You are an AI agent, an AI coding agent.", rewrite("You are Cline, an AI coding agent."))
	require.Equal(t, "You are Gemini, a model.", rewrite("You are Gemini, a model."), "Google identities cannot be configured away")
	require.Equal(t, "You are bazooka.", rewrite("You are bazooka."))
}

func TestTransformClaudeToGemini_NeutralizesClientIdentity(t *testing.T) {
	system, err := json.Marshal([]map[string]string{{"type": "text", "text": clineOpener}})
	require.NoError(t, err)
	req := &ClaudeRequest{
		System:   system,
		Messages: []ClaudeMessage{{Role: "user", Content: json.RawMessage(`"You are Cline, an AI coding agent."`)}},
	}

	for _, model := range []string{"gemini-3-flash", "claude-opus-4-6"} {
		body, err := TransformClaudeToGeminiWithOptions(req, "project", model, DefaultTransformOptions())
		require.NoError(t, err)
		var got V1InternalRequest
		require.NoError(t, json.Unmarshal(body, &got))
		var system string
		for _, part := range got.Request.SystemInstruction.Parts {
			system += part.Text
		}
		require.NotContains(t, system, "You are Cline", model)
		require.Contains(t, system, "You are an AI agent, an AI coding agent.", model)
		require.Equal(t, "You are Cline, an AI coding agent.", got.Request.Contents[0].Parts[0].Text, "user content is not rewritten")
	}
}
