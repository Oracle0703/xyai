package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractProjectAttributionAnthropicSystem(t *testing.T) {
	body := `{"model":"claude-sonnet-4-6","system":[{"type":"text","text":"You are Claude Code.\nPrimary working directory: E:\\code\\XunYouCrossPlatform\\lag-killer\ngitStatus: Current branch: feature/hy/0607\nMain branch: main"}],"messages":[{"role":"user","content":"hi"}]}`
	attr := ExtractProjectAttribution("/v1/messages", "claude-cli/2.1.162 (external, cli)", body)
	require.Equal(t, "E:/code/XunYouCrossPlatform/lag-killer", attr.Workdir)
	require.Equal(t, "lag-killer", attr.Project)
	require.Equal(t, "feature/hy/0607", attr.Branch)
	require.Equal(t, "system", attr.Source)
}

func TestExtractProjectAttributionAnthropicIgnoresConversationQuote(t *testing.T) {
	// 标记文本出现在 messages 正文而非 system: 字段锚定后不应误提取。
	body := `{"model":"claude-sonnet-4-6","system":"plain assistant","messages":[{"role":"user","content":"my log says Working directory: /tmp/evil and Current branch: hacked"}]}`
	attr := ExtractProjectAttribution("/v1/messages", "claude-cli/2.1.162", body)
	require.Empty(t, attr.Project)
	require.Empty(t, attr.Branch)
}

func TestExtractProjectAttributionAnthropicTruncatedBodyFallsBackToHead(t *testing.T) {
	// 截断导致非完整 JSON 时降级头部窗口正则。
	body := `{"model":"claude-sonnet-4-6","system":"Working directory: /Users/admin/Desktop/workspace/lag-killer\nIs a git repository: true","messages":[{"role":"user","content":"` + strings.Repeat("x", 100) // 故意不闭合
	attr := ExtractProjectAttribution("/v1/messages", "claude-cli/2.1.162", body)
	require.Equal(t, "lag-killer", attr.Project)
	require.Equal(t, "raw-head", attr.Source)
}

func TestExtractProjectAttributionCodexEnvContext(t *testing.T) {
	body := `{"model":"gpt-5.4","instructions":"You are Codex.","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"<environment_context>\n<cwd>D:\\project\\AndroidSDK</cwd>\n<approval_policy>never</approval_policy>\n</environment_context>"}]}]}`
	attr := ExtractProjectAttribution("/v1/responses", "codex_vscode/0.5.0", body)
	require.Equal(t, "D:/project/AndroidSDK", attr.Workdir)
	require.Equal(t, "AndroidSDK", attr.Project)
	require.Equal(t, "env-context", attr.Source)
	require.Empty(t, attr.Branch)
}

func TestExtractProjectAttributionCopilotInstructionsMD(t *testing.T) {
	// 嵌套 JSON 工具结果里的双反斜杠路径(真实流量形态)。
	body := `{"model":"xy-gpt-5.5","messages":[{"role":"user","content":"<attachment filePath=\"c:\\\\Work\\\\AI\\\\ai-tracehub\\\\ai-tracehub\\\\.github\\\\copilot-instructions.md\">rules</attachment>"}]}`
	attr := ExtractProjectAttribution("/v1/chat/completions", "GitHubCopilotChat/0.51.0", body)
	require.Equal(t, "c:/Work/AI/ai-tracehub/ai-tracehub", attr.Workdir)
	require.Equal(t, "ai-tracehub", attr.Project)
	require.Equal(t, "instructions-md", attr.Source)
}

func TestExtractProjectAttributionCopilotPathHintsAndKnownRoot(t *testing.T) {
	body := `{"model":"xy-gpt-5.5","messages":[{"role":"user","content":"<attachment filePath=\"E:\\\\code\\\\XunYouCrossPlatform\\\\lag-killer-dev\\\\src\\\\main.cpp\">code</attachment> see https://github.com/x and c:\\Users\\someone\\pygorithm\\searching\\binary_search.py"}]}`
	attr := ExtractProjectAttribution("/v1/chat/completions", "GitHubCopilotChat/0.51.0", body)
	require.Empty(t, attr.Project)
	require.NotEmpty(t, attr.PathHints)
	// 黑名单路径与 https:// 残段不应进入 hints。
	for _, h := range attr.PathHints {
		require.NotContains(t, h, "pygorithm")
		require.False(t, strings.HasPrefix(h, "s://"), h)
	}
	roots := []string{"e:/code/xunyoucrossplatform/lag-killer-dev", "e:/code/xunyoucrossplatform/lag-killer"}
	require.Equal(t, "e:/code/xunyoucrossplatform/lag-killer-dev", MatchAttributionKnownRoot(attr.PathHints, roots))
}

func TestExtractProjectAttributionGenericMultiValueWorkdirLine(t *testing.T) {
	// trae 形态: 多值 Working directories 行内含字面量 \n, 应在 \n 处截断。
	body := `{"model":"gpt-5.5","messages":[{"role":"system","content":"env:\nWorking directories: d:/tools/trae_test\\n- Operating system: windows\\n- Today: 2026-06-05"},{"role":"user","content":"hi"}]}`
	attr := ExtractProjectAttribution("/v1/chat/completions", "trae/1.0", body)
	require.Equal(t, "d:/tools/trae_test", attr.Workdir)
	require.Equal(t, "trae_test", attr.Project)
}

func TestExtractProjectAttributionRejectsGarbageCwd(t *testing.T) {
	cases := []string{
		`{"messages":[{"role":"system","content":"Working directory: n"}]}`,                                   // 非路径
		`{"messages":[{"role":"system","content":"Working directory: not/a/rooted/path"}]}`,                   // 无根
		`{"messages":[{"role":"system","content":"Working directory: /x/` + strings.Repeat("a", 100) + `"}]}`, // basename 超长
	}
	for _, body := range cases {
		attr := ExtractProjectAttribution("/v1/chat/completions", "trae/1.0", body)
		require.Empty(t, attr.Project, body[:60])
	}
}

func TestExtractProjectAttributionEmptyBody(t *testing.T) {
	require.False(t, ExtractProjectAttribution("/v1/messages", "claude-cli/2.0", "").Attributed())
}

func TestExtractProjectAttributionRejectsNULBytes(t *testing.T) {
	// JSON 字符串里合法的 NUL 转义解码后是真实 0x00: 含 NUL 的"路径"是坏数据,
	// 直接拒绝归因; branch 剥离 NUL 后保留。两者都不得把 0x00 带进 text 列。
	sys := "Primary working directory: E:/code/ev\x00il\nCurrent branch: ma\x00in"
	body, err := json.Marshal(map[string]any{"model": "claude-sonnet-4-6", "system": sys})
	require.NoError(t, err)
	attr := ExtractProjectAttribution("/v1/messages", "claude-cli/2.1.162", string(body))
	require.Empty(t, attr.Workdir)
	require.Empty(t, attr.Project)
	require.Equal(t, "main", attr.Branch)
}
