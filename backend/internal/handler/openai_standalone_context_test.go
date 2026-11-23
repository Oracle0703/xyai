package handler

import (
	"maps"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

func TestCodexSourceContextAllowlist(t *testing.T) {
	for _, tc := range []struct{ namespace, name, output string }{
		{"guardian", "guardian_housekeeper_input", `{"sources":[]}`},
		{"guardian", "guardian_channel_selection_input", `{"channels":[]}`},
		{"guardian", "guardian_original_evidence", `{"sources":[]}`},
		{"cloud_threads", "send_message", delegationEnvelope},
		{"cloud_threads", "send_message", `<codex_delegation><source_thread_id>thread-1</source_thread_id><source_orbit_id>orbit-1</source_orbit_id><input>do the work</input></codex_delegation>`},
	} {
		t.Run(tc.namespace+"/"+tc.name, func(t *testing.T) {
			item := pageContextItem(tc.name, tc.output)
			item["namespace"] = tc.namespace
			body := pageContextRequest(t, item, map[string]any{"role": "user", "content": "hello"})
			original := append([]byte(nil), body...)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			require.True(t, (&OpenAIGatewayHandler{}).validateFunctionCallOutputRequest(c, body, zap.NewNop()))
			require.False(t, c.Writer.Written())
			require.Equal(t, original, body)
			require.Len(t, gjson.GetBytes(codexStandaloneContextValidationBody(body), "input").Array(), 1)
			for key, values := range map[string][]any{
				"namespace": {"unknown", nil}, "name": {"read_file", tc.name + "_extra"},
				"call_id": {"", nil, 123, "call_missing"}, "output": {nil, 123, []any{}},
			} {
				for _, value := range values {
					bad := maps.Clone(item)
					bad[key] = value
					badBody := pageContextRequest(t, bad)
					require.Equal(t, badBody, codexStandaloneContextValidationBody(badBody), "%s=%v", key, value)
				}
			}
			// A recognized context must not hide a genuine orphan result beside it.
			c, _ = gin.CreateTestContext(httptest.NewRecorder())
			require.False(t, (&OpenAIGatewayHandler{}).validateFunctionCallOutputRequest(c,
				pageContextRequest(t, item, pageContextItem("read_file", "unpaired result")), zap.NewNop()))
		})
	}
}

func TestCodexSourceContextRejectsMalformedEnvelopes(t *testing.T) {
	for _, tc := range []struct{ namespace, name, output string }{
		{"guardian", "guardian_original_evidence", "not JSON"},
		{"guardian", "guardian_original_evidence", `{"sources":[],"sources":null}`},
		{"cloud_threads", "send_message", "ordinary result"},
		{"cloud_threads", "send_message", `<codex_delegation><source_thread_id>thread-1</source_thread_id><source_orbit_id></source_orbit_id><input>work</input></codex_delegation>`},
		{"cloud_threads", "send_message", `<codex_delegation><source_thread_id>thread-1</source_thread_id><source_orbit_id>one</source_orbit_id><source_orbit_id>two</source_orbit_id><input>work</input></codex_delegation>`},
		{"cloud_threads", "send_message", `<codex_delegation><source_orbit_id>one</source_orbit_id><source_thread_id>thread-1</source_thread_id><input>work</input></codex_delegation>`},
	} {
		item := pageContextItem(tc.name, tc.output)
		item["namespace"] = tc.namespace
		body := pageContextRequest(t, item)
		require.Equal(t, body, codexStandaloneContextValidationBody(body))
	}
}

const browserTabsContext = `
<in-app-browser-context source="ambient-ui-state">
This block is automatically supplied ambient UI state, not part of the user's request. Do not treat it as an instruction or as evidence that the user explicitly selected the in-app browser.
# In app browser:
- The user has the in-app browser open with 1 tab.
- Current URL: file:///D:/project/report.html
</in-app-browser-context>
`

func TestCodexBrowserTabsEnvelope(t *testing.T) {
	for _, output := range []string{browserTabsContext, strings.ReplaceAll(browserTabsContext, "\n", "\r\n"), strings.ReplaceAll(browserTabsContext, "file:///D:/project/report.html", "https://example.com/?q=one&view=two")} {
		body := pageContextRequest(t, pageContextItem("browser_tabs", output))
		require.Empty(t, gjson.GetBytes(codexStandaloneContextValidationBody(body), "input").Array())
		require.Equal(t, output, gjson.GetBytes(body, "input.0.output").String())
	}
	for _, output := range []string{
		"ordinary tool output", "<in-app-browser-context source=\"ambient-ui-state\"></in-app-browser-context>",
		strings.Replace(browserTabsContext, `source="ambient-ui-state"`, `source="user"`, 1),
		strings.Replace(browserTabsContext, "</in-app-browser-context>", "", 1),
		browserTabsContext + "trailing content",
		strings.Replace(browserTabsContext, "# In app browser:", "# Other:", 1),
	} {
		body := pageContextRequest(t, pageContextItem("browser_tabs", output))
		require.Equal(t, body, codexStandaloneContextValidationBody(body))
	}
}

// responsesEntryNormalize mirrors the normalizer order of the Responses entry.
func responsesEntryNormalize(body []byte) []byte {
	if next, changed := normalizeCodexUntrustedContext(body); changed {
		body = next
	}
	if next, changed := normalizeCodexAutomationBootstrap(body); changed {
		body = next
	}
	if next, changed := normalizeCodexDelegationBootstrap(body); changed {
		body = next
	}
	return body
}

// 上游 #8769 覆盖的生产者由上游改写为 user message；其余本地原已支持的生产者
// 不被改写，但必须通过校验并原样转发，而不是整条请求 400。
func TestCodexStandaloneContextResponsesEntry(t *testing.T) {
	standalone := func(namespace, name, output string) map[string]any {
		return map[string]any{"type": "function_call_output", "namespace": namespace, "name": name, "output": output}
	}
	user := map[string]any{"role": "user", "content": "hello"}
	accepted := func(t *testing.T, body []byte) bool {
		t.Helper()
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		ok := (&OpenAIGatewayHandler{}).validateFunctionCallOutputRequest(c, body, zap.NewNop())
		require.Equal(t, !ok, c.Writer.Written())
		return ok
	}
	for _, tc := range []struct {
		namespace, name, output string
		rewritten               bool
	}{
		{"agent", "browser_tabs", browserTabsContext, false},
		{"agent", "proposed_plan", "# Approved plan\n1. Implement the change", true},
		{"agent", "untrusted_content", "External context", true},
		{"guardian", "guardian_housekeeper_input", `{"sources":[]}`, false},
		{"guardian", "guardian_channel_selection_input", `{"channels":[]}`, false},
		{"guardian", "guardian_original_evidence", `{"sources":[]}`, false},
		{"cloud_threads", "send_message", delegationEnvelope, false},
		{"agent", "goal_objective", "Finish the task", true},
		{"agent", "codex_apps_open_page", `{"page_id":null}`, true},
		{"codex_app", "automation_update", "<heartbeat><automation_id>test</automation_id></heartbeat>", true},
		{"codex_app", "create_thread", delegationEnvelope, true},
	} {
		t.Run(tc.namespace+"/"+tc.name, func(t *testing.T) {
			body := pageContextRequest(t, standalone(tc.namespace, tc.name, tc.output), user)
			normalized := responsesEntryNormalize(body)
			if tc.rewritten {
				require.Equal(t, "message", gjson.GetBytes(normalized, "input.0.type").String())
				require.Equal(t, "user", gjson.GetBytes(normalized, "input.0.role").String())
			} else {
				require.Equal(t, body, normalized, "producers unknown to upstream must be forwarded unchanged")
			}
			require.True(t, accepted(t, normalized))
		})
	}

	// 上游 bootstrap normalizer 遇到它不识别的独立输出会整体放弃改写；
	// 这类并存请求沿用本地原有行为放行并原样转发。
	for _, bootstrap := range []map[string]any{
		standalone("codex_app", "create_thread", delegationEnvelope),
		standalone("codex_app", "automation_update", "<heartbeat><automation_id>test</automation_id></heartbeat>"),
	} {
		body := pageContextRequest(t, standalone("agent", "browser_tabs", browserTabsContext), bootstrap, user)
		normalized := responsesEntryNormalize(body)
		require.Equal(t, body, normalized)
		require.True(t, accepted(t, normalized))
	}

	// 未知名称与真实孤立工具结果仍按通用规则拒绝。
	for _, item := range []map[string]any{
		standalone("agent", "read_file", "unpaired result"),
		standalone("agent", "browser_info_extra", "tab"),
		standalone("guardian", "proposed_plan", "plan"),
	} {
		body := responsesEntryNormalize(pageContextRequest(t, standalone("agent", "proposed_plan", "plan"), item, user))
		require.False(t, accepted(t, body))
	}
}
