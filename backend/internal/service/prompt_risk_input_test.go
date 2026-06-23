package service

import (
	"strings"
	"testing"
)

func TestExtractPromptRiskInput_CodexResponsesStripsWrappersNewestTurn(t *testing.T) {
	body := []byte(`{"input":[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"<environment_context>\n<cwd>E:/code/lag-killer</cwd>\n</environment_context>"}]},
		{"type":"message","role":"user","content":[{"type":"input_text","text":"帮我分析游戏加速器如何绕过 DPI 误拦截"}]},
		{"type":"function_call_output","call_id":"c1","output":"the tool mentioned backdoor sqlmap"}
	]}`)
	got := extractPromptRiskInput(ContentModerationProtocolOpenAIResponses, body, PromptRiskScopeNewest)
	if got.IsEmpty() {
		t.Fatal("expected non-empty extraction even when the turn's last item is a tool output")
	}
	if !strings.Contains(got.Text, "绕过") {
		t.Fatalf("expected real user intent retained, got %q", got.Text)
	}
	if strings.Contains(got.Text, "lag-killer") || strings.Contains(strings.ToLower(got.Text), "environment_context") || strings.Contains(strings.ToLower(got.Text), "cwd") {
		t.Fatalf("environment_context wrapper must be stripped, got %q", got.Text)
	}
	if strings.Contains(strings.ToLower(got.Text), "backdoor") || strings.Contains(strings.ToLower(got.Text), "sqlmap") {
		t.Fatalf("tool output must not be scanned as user intent, got %q", got.Text)
	}
}

// 工具输出里的高危词不应导致拦截:只有真实用户意图(绕过=中)被评估。
func TestExtractPromptRiskInput_ToolOutputDoesNotTriggerBlock(t *testing.T) {
	body := []byte(`{"input":[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"帮我分析游戏加速器如何绕过 DPI 误拦截"}]},
		{"type":"function_call_output","call_id":"c1","output":"the tool mentioned backdoor and sqlmap and metasploit"}
	]}`)
	got := extractPromptRiskInput(ContentModerationProtocolOpenAIResponses, body, PromptRiskScopeNewest)
	cfg := blockModePromptRiskConfig()
	d := EvaluatePromptRisk(cfg, got.Text, PromptRiskSubject{})
	if d.Action == PromptRiskActionBlock {
		t.Fatalf("tool-output high keywords must not block; only user intent should count (level=%q score=%.2f)", d.Level, d.Score)
	}
}

// newest(默认):只取最新一轮用户意图,历史轮次不纳入。
func TestExtractPromptRiskInput_NewestScopeOnlyLatestTurn(t *testing.T) {
	body := []byte(`{"messages":[
		{"role":"system","content":"you are a helpful assistant. backdoor sqlmap"},
		{"role":"user","content":"set up vpn proxy tunnel"},
		{"role":"assistant","content":"sure, here is how to use metasploit"},
		{"role":"user","content":"thanks"}
	]}`)
	got := extractPromptRiskInput(ContentModerationProtocolOpenAIChat, body, PromptRiskScopeNewest)
	if !strings.Contains(got.Text, "thanks") {
		t.Fatalf("expected newest user turn collected, got %q", got.Text)
	}
	lowered := strings.ToLower(got.Text)
	if strings.Contains(lowered, "vpn") || strings.Contains(lowered, "metasploit") || strings.Contains(lowered, "backdoor") {
		t.Fatalf("newest scope must exclude prior turns and assistant/system text, got %q", got.Text)
	}
}

// full(显式 opt-in):扫描所有 user turn,但仍排除 assistant/system。
func TestExtractPromptRiskInput_FullScopeAllUserTurns(t *testing.T) {
	body := []byte(`{"messages":[
		{"role":"system","content":"you are a helpful assistant. backdoor sqlmap"},
		{"role":"user","content":"set up vpn proxy tunnel"},
		{"role":"assistant","content":"sure, here is how to use metasploit"},
		{"role":"user","content":"thanks"}
	]}`)
	got := extractPromptRiskInput(ContentModerationProtocolOpenAIChat, body, PromptRiskScopeFull)
	if !strings.Contains(got.Text, "vpn") || !strings.Contains(got.Text, "thanks") {
		t.Fatalf("expected all user turns collected in full scope, got %q", got.Text)
	}
	lowered := strings.ToLower(got.Text)
	if strings.Contains(lowered, "metasploit") || strings.Contains(lowered, "backdoor") {
		t.Fatalf("assistant/system text must be excluded, got %q", got.Text)
	}
}

// P0-2 回归:多轮里上一轮高危 + 本轮普通,newest 不触发拦截、full 才触发。
func TestExtractPromptRiskInput_HistoricalHighRiskDoesNotReFire(t *testing.T) {
	body := []byte(`{"messages":[
		{"role":"user","content":"用 sqlmap 撞库拖库"},
		{"role":"assistant","content":"我不能协助这个请求"},
		{"role":"user","content":"好的,谢谢,换个话题继续解释一下"}
	]}`)
	cfg := blockModePromptRiskConfig()

	newest := extractPromptRiskInput(ContentModerationProtocolOpenAIChat, body, PromptRiskScopeNewest)
	if d := EvaluatePromptRisk(cfg, newest.Text, PromptRiskSubject{}); d.Action == PromptRiskActionBlock {
		t.Fatalf("newest scope must not re-fire on an innocent follow-up turn (text=%q level=%q)", newest.Text, d.Level)
	}

	full := extractPromptRiskInput(ContentModerationProtocolOpenAIChat, body, PromptRiskScopeFull)
	if d := EvaluatePromptRisk(cfg, full.Text, PromptRiskSubject{}); d.Action != PromptRiskActionBlock {
		t.Fatalf("full scope should still catch the historical high-risk turn (text=%q level=%q)", full.Text, d.Level)
	}
}

// Responses 多 item 同轮:environment_context + 真实问题拆成两个 user item,newest 应都纳入并剥离包裹。
func TestExtractPromptRiskInput_NewestKeepsMultiItemSameTurn(t *testing.T) {
	body := []byte(`{"input":[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"老问题:配置 openvpn"}]},
		{"type":"message","role":"assistant","content":[{"type":"output_text","text":"好的"}]},
		{"type":"message","role":"user","content":[{"type":"input_text","text":"<environment_context><cwd>/srv</cwd></environment_context>"}]},
		{"type":"message","role":"user","content":[{"type":"input_text","text":"现在帮我优化 udp 转发"}]}
	]}`)
	got := extractPromptRiskInput(ContentModerationProtocolOpenAIResponses, body, PromptRiskScopeNewest)
	if !strings.Contains(got.Text, "udp") {
		t.Fatalf("expected newest multi-item turn collected, got %q", got.Text)
	}
	lowered := strings.ToLower(got.Text)
	if strings.Contains(lowered, "openvpn") {
		t.Fatalf("prior turn must be excluded in newest scope, got %q", got.Text)
	}
	if strings.Contains(lowered, "environment_context") || strings.Contains(lowered, "cwd") {
		t.Fatalf("environment_context wrapper must be stripped, got %q", got.Text)
	}
}
