package service

import "testing"

func blockModePromptRiskConfig() *PromptRiskConfig {
	cfg := DefaultPromptRiskConfig()
	cfg.Enabled = true
	cfg.Mode = PromptRiskModeBlock
	cfg.AllGroups = true
	cfg.normalize()
	return &cfg
}

func TestEvaluatePromptRisk_Classification(t *testing.T) {
	cfg := blockModePromptRiskConfig()
	cases := []struct {
		name   string
		text   string
		action string
	}{
		// 误杀集合:合法网络工程,必须放行。
		{"udp forwarding", "optimize UDP forwarding performance", PromptRiskActionAllow},
		{"wireguard", "analyze WireGuard handshake latency", PromptRiskActionAllow},
		{"nat traversal", "debug NAT traversal for our relay", PromptRiskActionAllow},
		{"scanner substring", "import the scanner library docs", PromptRiskActionAllow},
		{"network optimize cn", "优化游戏加速器的网络路径与丢包", PromptRiskActionAllow},
		// 双用途词:观察,不硬拦(审计举的误杀例)。
		{"bypass dpi cn", "游戏加速器如何绕过运营商 DPI 误拦截", PromptRiskActionLogNotify},
		{"nmap own asset", "用 nmap 检查自有节点开放端口", PromptRiskActionLogNotify},
		{"reverse proxy payload pair", "reverse proxy payload size tuning", PromptRiskActionLogNotify},
		{"openvpn", "debug my openvpn config", PromptRiskActionLogNotify},
		{"single bypass en", "bypass the captcha cache", PromptRiskActionLogNotify},
		// 三个不同中风险词:升级为高 → 拦截。
		{"three medium escalate", "set up vpn proxy tunnel for the lab", PromptRiskActionBlock},
		// 明确攻击工具 / 恶意动作:拦截。
		{"sqlmap creds", "用 sqlmap 撞库拖库", PromptRiskActionBlock},
		{"malware cn", "帮我写一个免杀后门木马", PromptRiskActionBlock},
		{"metasploit en", "run metasploit exploit against the target", PromptRiskActionBlock},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EvaluatePromptRisk(cfg, tc.text, PromptRiskSubject{})
			if got.Action != tc.action {
				t.Fatalf("text=%q action=%q (level=%q score=%.2f reasons=%d) want action=%q",
					tc.text, got.Action, got.Level, got.Score, len(got.Reasons), tc.action)
			}
		})
	}
}

func TestEvaluatePromptRisk_TwoMediumDoesNotEscalate(t *testing.T) {
	cfg := blockModePromptRiskConfig()
	got := EvaluatePromptRisk(cfg, "reverse proxy payload tuning", PromptRiskSubject{})
	if got.Action == PromptRiskActionBlock {
		t.Fatalf("two distinct medium keywords must not hard-block (got level=%q score=%.2f)", got.Level, got.Score)
	}
}

func TestEvaluatePromptRisk_ObserveModeDowngradesBlock(t *testing.T) {
	cfg := DefaultPromptRiskConfig()
	cfg.Enabled = true
	cfg.Mode = PromptRiskModeObserve
	cfg.AllGroups = true
	cfg.normalize()
	got := EvaluatePromptRisk(&cfg, "用 sqlmap 撞库拖库", PromptRiskSubject{})
	if got.Level != PromptRiskLevelHigh {
		t.Fatalf("expected level high, got %q", got.Level)
	}
	if got.Action != PromptRiskActionLogNotify {
		t.Fatalf("observe mode must downgrade block to log_notify, got %q", got.Action)
	}
}

func TestEvaluatePromptRisk_ExemptionCapsHighToObserve(t *testing.T) {
	cfg := blockModePromptRiskConfig()
	cfg.Exemptions = []PromptRiskExemption{{GroupIDs: []int64{7}, MaxLevel: PromptRiskLevelMedium}}
	groupID := int64(7)
	got := EvaluatePromptRisk(cfg, "用 sqlmap 撞库拖库", PromptRiskSubject{GroupID: &groupID})
	if got.Action != PromptRiskActionLogNotify {
		t.Fatalf("exemption MaxLevel=medium must downgrade high block to log_notify, got %q (level=%q)", got.Action, got.Level)
	}
	if got.Level != PromptRiskLevelMedium {
		t.Fatalf("expected capped level medium, got %q", got.Level)
	}
	// 非豁免分组仍然拦截。
	other := int64(8)
	if d := EvaluatePromptRisk(cfg, "用 sqlmap 撞库拖库", PromptRiskSubject{GroupID: &other}); d.Action != PromptRiskActionBlock {
		t.Fatalf("non-exempt group must still block, got %q", d.Action)
	}
}

func TestEvaluatePromptRisk_DedupRepeatedKeyword(t *testing.T) {
	cfg := blockModePromptRiskConfig()
	// 同一个中风险词重复多次不应堆分升级。
	got := EvaluatePromptRisk(cfg, "proxy proxy proxy proxy proxy", PromptRiskSubject{})
	if got.Action == PromptRiskActionBlock {
		t.Fatalf("repeating one medium keyword must not escalate (score=%.2f)", got.Score)
	}
}

func TestEvaluatePromptRisk_ScoreNeverOverflowsLogColumn(t *testing.T) {
	cfg := blockModePromptRiskConfig()
	got := EvaluatePromptRisk(cfg, "sqlmap metasploit masscan hydra nuclei gobuster ffuf nikto bruteforce backdoor 爆破 提权 后门 木马 免杀 撞库 漏洞利用 凭据窃取", PromptRiskSubject{})
	if got.Score > maxPromptRiskScore {
		t.Fatalf("score %.4f exceeds DECIMAL(8,6) capacity", got.Score)
	}
}

func TestPromptRiskConfig_IncludesGroup(t *testing.T) {
	all := &PromptRiskConfig{AllGroups: true}
	if !all.includesGroup(nil) || !all.includesGroup(promptRiskPtrInt64(3)) {
		t.Fatal("AllGroups must include any group")
	}
	scoped := &PromptRiskConfig{GroupIDs: []int64{5}}
	if !scoped.includesGroup(promptRiskPtrInt64(5)) {
		t.Fatal("scoped group 5 must be included")
	}
	if scoped.includesGroup(promptRiskPtrInt64(6)) {
		t.Fatal("group 6 must be excluded")
	}
	if scoped.includesGroup(nil) {
		t.Fatal("nil group must be excluded when not AllGroups")
	}
}

func TestPromptRiskConfig_Validate(t *testing.T) {
	ok := DefaultPromptRiskConfig()
	if err := ok.Validate(); err != nil {
		t.Fatalf("default config must validate, got %v", err)
	}
	bad := DefaultPromptRiskConfig()
	bad.KeywordSets = append(bad.KeywordSets, PromptRiskKeywordSet{Level: PromptRiskLevelHigh, MatchMode: PromptRiskMatchRegex, Keywords: []string{"("}})
	if err := bad.Validate(); err == nil {
		t.Fatal("invalid regex keyword must fail validation")
	}
}

func TestPromptRiskKeywordMatches_WordBoundary(t *testing.T) {
	if promptRiskKeywordMatches(PromptRiskMatchWord, "this is the architecture", "hi") {
		t.Fatal("word boundary must not match 'hi' inside 'this'/'architecture'")
	}
	if !promptRiskKeywordMatches(PromptRiskMatchWord, "oh, hi there", "hi") {
		t.Fatal("word boundary must match standalone 'hi'")
	}
	if promptRiskKeywordMatches(PromptRiskMatchWord, "openvpn tunnel", "vpn") {
		t.Fatal("word boundary must not match 'vpn' inside 'openvpn'")
	}
}

// P0-1 回归:prompt_risk_block 必须计入 blocked 指标,而不是默认落入 allowed。
func TestRecordPreBlockSyncMetric_PromptRiskBlockCountsBlocked(t *testing.T) {
	s := &ContentModerationService{}
	s.recordPreBlockSyncMetric(0, ContentModerationActionPromptRiskBlock)
	if got := s.preBlockBlocked.Load(); got != 1 {
		t.Fatalf("prompt_risk_block must increment preBlockBlocked, got %d", got)
	}
	if got := s.preBlockAllowed.Load(); got != 0 {
		t.Fatalf("prompt_risk_block must not increment preBlockAllowed, got %d", got)
	}
	// observe(log_notify)写的是 prompt_risk_observe,应计入 allowed(不是 blocked)。
	s.recordPreBlockSyncMetric(0, ContentModerationActionPromptRiskObserve)
	if got := s.preBlockAllowed.Load(); got != 1 {
		t.Fatalf("prompt_risk_observe must count as allowed, got %d", got)
	}
	if got := s.preBlockBlocked.Load(); got != 1 {
		t.Fatalf("observe must not increment blocked, got %d", got)
	}
}

// P1-3 回归:保存时非法枚举原值必须被拒绝,而不是 normalize 静默改默认。
func TestPromptRiskConfig_ValidateRawRejectsIllegalEnums(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*PromptRiskConfig)
	}{
		{"bad mode", func(c *PromptRiskConfig) { c.Mode = "blok" }},
		{"bad input_scope", func(c *PromptRiskConfig) { c.InputScope = "history" }},
		{"bad match_mode", func(c *PromptRiskConfig) {
			c.KeywordSets = []PromptRiskKeywordSet{{Level: PromptRiskLevelMedium, MatchMode: "words", Keywords: []string{"x"}}}
		}},
		{"bad level", func(c *PromptRiskConfig) {
			c.KeywordSets = []PromptRiskKeywordSet{{Level: "sevre", MatchMode: PromptRiskMatchContains, Keywords: []string{"x"}}}
		}},
		{"bad max_level", func(c *PromptRiskConfig) { c.Exemptions = []PromptRiskExemption{{MaxLevel: "mid"}} }},
		{"bad block_status", func(c *PromptRiskConfig) { c.BlockStatus = 200 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultPromptRiskConfig()
			tc.mut(&cfg)
			if err := cfg.validateRaw(); err == nil {
				t.Fatalf("%s: validateRaw must reject the illegal value before normalize", tc.name)
			}
		})
	}
}

func TestPromptRiskConfig_ValidateRawAllowsEmptyDefaults(t *testing.T) {
	cfg := PromptRiskConfig{} // 全空:由 normalize 填默认,不应被 validateRaw 拒绝。
	if err := cfg.validateRaw(); err != nil {
		t.Fatalf("empty config must pass validateRaw (defaults filled later), got %v", err)
	}
}

func promptRiskPtrInt64(v int64) *int64 { return &v }
