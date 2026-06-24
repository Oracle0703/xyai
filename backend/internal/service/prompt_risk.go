package service

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
)

// Prompt 风险前置审查:在请求送达上游(尤其 OpenAI/Codex 共享账号)之前,于网关侧对用户
// 文本做一道分级关键词审查。纯函数 + 表驱动,无 I/O(对标 token_analysis_risk.go)。
// 设计文档: docs/features/codex-prompt-risk-filter-design-cn.md

const (
	PromptRiskLevelLow    = "low"
	PromptRiskLevelMedium = "medium"
	PromptRiskLevelHigh   = "high"

	PromptRiskModeOff     = "off"
	PromptRiskModeObserve = "observe"
	PromptRiskModeBlock   = "block"

	PromptRiskActionAllow     = "allow"
	PromptRiskActionLogNotify = "log_notify"
	PromptRiskActionBlock     = "block"

	PromptRiskMatchContains = "contains"
	PromptRiskMatchRegex    = "regex"
	PromptRiskMatchWord     = "word"

	PromptRiskScopeNewest = "newest" // 仅评估本轮(最新)用户意图,默认
	PromptRiskScopeFull   = "full"   // 评估请求体内全部历史 user turn(显式 opt-in)

	defaultPromptRiskBlockStatus       = 403
	defaultPromptRiskEscalateThreshold = 1.0 // 任一高(1.0)或 ≥3 个不同中(0.4×3)才升级拦截;两个中(0.4×2=0.8)不升级,避免误杀常见双用途词对
	defaultPromptRiskHighScore         = 1.0
	defaultPromptRiskMediumScore       = 0.4
	maxPromptRiskScore                 = 99.999999 // 适配日志列 highest_score DECIMAL(8,6)

	defaultPromptRiskBlockMessage = "您的请求包含可能触发上游服务滥用风控的表述，已被网关本地拦截，以保护团队共享账号。"
	defaultPromptRiskRewriteHint  = "改写建议：聚焦合法工程目标（如网络性能优化、协议兼容性、自有资产的授权测试），" +
		"避免直接出现攻击工具名或针对第三方系统的攻击性动作；写明授权范围与目标归属后再试。"

	// LLM 语义复核(judge):关键词命中且会真正拦截时,调一次外部 chat 模型做语义精判,降低双用途词误杀。
	defaultPromptRiskJudgeTimeoutMS = 4000
	minPromptRiskJudgeTimeoutMS     = 500
	maxPromptRiskJudgeTimeoutMS     = 15000

	PromptRiskJudgeRiskNone = "none"
	PromptRiskJudgeRiskLow  = "low"
	PromptRiskJudgeRiskHigh = "high"

	// 内置 judge system 提示词(judge_prompt_template 为空时使用)。
	defaultPromptRiskJudgePrompt = "你是网关侧的安全语义复核员。一段用户 prompt 已被关键词规则标记为\"疑似滥用风险\"，现在做语义复核，降低对双用途技术词的误杀。\n" +
		"- 正常(none/low)：为自有/已授权资产做工程工作（配置自有 VPN/代理/隧道、优化网络、调试自有 OpenVPN、对授权且归属自己的目标做安全测试、学习协议、读文档）。\n" +
		"- 风险(high)：攻击未授权第三方、搭建翻墙/规避审查服务、规避本网关或上游风控、窃取凭据、批量爆破、投放后门木马。\n" +
		"仅凭出现 vpn/proxy/代理/exploit/nmap 等词不能判风险，要看意图与目标归属；无法判断时倾向 low。\n" +
		"严格只输出 JSON，无多余文字：{\"risk\":\"none|low|high\",\"reason\":\"一句话中文理由\"}"
)

// PromptRiskKeywordSet 一组同等级、同匹配模式的关键词。
type PromptRiskKeywordSet struct {
	Level     string   `json:"level"`
	MatchMode string   `json:"match_mode"`
	Keywords  []string `json:"keywords"`
	Score     float64  `json:"score"` // 0~1;命中累加(按 level+keyword 去重)
}

// PromptRiskExemption 命中即把生效等级封顶到 MaxLevel(只记不拦),用于安全/网络团队。
type PromptRiskExemption struct {
	GroupIDs  []int64 `json:"group_ids"`
	UserIDs   []int64 `json:"user_ids"`
	APIKeyIDs []int64 `json:"api_key_ids"`
	MaxLevel  string  `json:"max_level"`
}

// PromptRiskSubject 评估所需的身份(全部来自 ContentModerationCheckInput,无需改 input)。
type PromptRiskSubject struct {
	UserID   int64
	GroupID  *int64
	APIKeyID int64
	Provider string
}

// PromptRiskJudgeConfig 配置"关键词命中后的 LLM 语义复核"。指向一个 OpenAI 兼容
// /v1/chat/completions 端点(推荐用户自己的网关 + 专属便宜模型 + 专属 API Key)。
// 仅在关键词评估会真正 block 且命中等级 ∈ TriggerLevels 时触发,judge 判"非风险"则把
// block 降级为观察(放行),judge 判"风险"则保持拦截;调用失败一律 fail-open(放行)。
type PromptRiskJudgeConfig struct {
	Enabled        bool     `json:"enabled"`
	BaseURL        string   `json:"base_url"`        // 网关根地址,如 https://gw.example.com
	Model          string   `json:"model"`           // 便宜 chat 模型 id
	APIKey         string   `json:"api_key"`         // Bearer token(写入用;读取时掩码,见 api_key_masked)
	TimeoutMS      int      `json:"timeout_ms"`      // 默认 4000,clamp [500,15000]
	PromptTemplate string   `json:"prompt_template"` // 空则用内置 defaultPromptRiskJudgePrompt
	TriggerLevels  []string `json:"trigger_levels"`  // 默认 ["high"];只对命中这些等级的 would-be-block 复核
	FailOpen       bool     `json:"fail_open"`       // v1 固定 true(judge 失败放行)

	// 只读输出(GetPromptRiskConfig 填充,供前端展示;不参与匹配/落库)。
	APIKeyConfigured bool   `json:"api_key_configured,omitempty"`
	APIKeyMasked     string `json:"api_key_masked,omitempty"`
}

// PromptRiskConfig 独立设置(settings 表 key=prompt_risk_config),不依赖旧内容审核配置。
type PromptRiskConfig struct {
	Enabled           bool                   `json:"enabled"`
	Mode              string                 `json:"mode"`        // off / observe / block
	AllGroups         bool                   `json:"all_groups"`
	GroupIDs          []int64                `json:"group_ids"`   // group 级 opt-in
	InputScope        string                 `json:"input_scope"` // newest(默认,仅本轮用户意图) / full(全历史 user turn)
	BlockStatus       int                    `json:"block_status"`
	EscalateThreshold float64                `json:"escalate_threshold"`
	BlockMessage      string                 `json:"block_message"`
	RewriteSuggestion string                 `json:"rewrite_suggestion"`
	KeywordSets       []PromptRiskKeywordSet `json:"keyword_sets"`
	Exemptions        []PromptRiskExemption  `json:"exemptions"`
	Judge             PromptRiskJudgeConfig  `json:"judge"`
}

// PromptRiskReason 单条命中证据。
type PromptRiskReason struct {
	Level   string  `json:"level"`
	Keyword string  `json:"keyword"`
	Source  string  `json:"source"` // 匹配模式: contains / regex / word
	Score   float64 `json:"score"`
}

// PromptRiskDecision 评估结果(已套豁免封顶 + Mode 收敛)。
type PromptRiskDecision struct {
	Level   string             `json:"level"`
	Action  string             `json:"action"`
	Score   float64            `json:"score"`
	Reasons []PromptRiskReason `json:"reasons"`
}

// DefaultPromptRiskConfig 返回首版默认配置:功能关闭、Mode=observe(先观察校准再切拦截)、
// 仅评估最新一轮用户意图(newest)、词表按"双用途→观察 / 明确恶意→拦截"重排。
// 加载/仓储异常时一律 fail-open(返回本默认配置,Enabled=false ⇒ 不拦),不提供 fail-closed。
func DefaultPromptRiskConfig() PromptRiskConfig {
	return PromptRiskConfig{
		Enabled:           false,
		Mode:              PromptRiskModeObserve,
		AllGroups:         false,
		GroupIDs:          []int64{},
		InputScope:        PromptRiskScopeNewest,
		BlockStatus:       defaultPromptRiskBlockStatus,
		EscalateThreshold: defaultPromptRiskEscalateThreshold,
		BlockMessage:      defaultPromptRiskBlockMessage,
		RewriteSuggestion: defaultPromptRiskRewriteHint,
		KeywordSets: []PromptRiskKeywordSet{
			// 中(双用途、需观察)
			{Level: PromptRiskLevelMedium, MatchMode: PromptRiskMatchWord, Score: defaultPromptRiskMediumScore, Keywords: []string{
				"vpn", "proxy", "tunnel", "socks5", "openvpn", "shadowsocks", "v2ray", "trojan",
				"bypass", "payload", "spoof", "obfuscate", "stealth", "evade", "evasion", "enumerate", "exploit", "nmap",
			}},
			{Level: PromptRiskLevelMedium, MatchMode: PromptRiskMatchRegex, Score: defaultPromptRiskMediumScore, Keywords: []string{
				`\bport\s+scan\b`,
			}},
			{Level: PromptRiskLevelMedium, MatchMode: PromptRiskMatchContains, Score: defaultPromptRiskMediumScore, Keywords: []string{
				"代理", "翻墙", "梯子", "隧道", "绕过", "隐藏", "规避", "反检测", "反审计", "反追踪", "伪装", "渗透", "端口扫描", "枚举",
			}},
			// 高(明确攻击工具 / 恶意动作)
			{Level: PromptRiskLevelHigh, MatchMode: PromptRiskMatchWord, Score: defaultPromptRiskHighScore, Keywords: []string{
				"sqlmap", "metasploit", "masscan", "hydra", "nuclei", "gobuster", "ffuf", "nikto", "bruteforce", "backdoor",
			}},
			{Level: PromptRiskLevelHigh, MatchMode: PromptRiskMatchContains, Score: defaultPromptRiskHighScore, Keywords: []string{
				"爆破", "提权", "后门", "木马", "免杀", "撞库", "漏洞利用", "凭据窃取",
			}},
		},
		Exemptions: []PromptRiskExemption{},
		Judge: PromptRiskJudgeConfig{
			Enabled:       false,
			TimeoutMS:     defaultPromptRiskJudgeTimeoutMS,
			TriggerLevels: []string{PromptRiskLevelHigh},
			FailOpen:      true,
		},
	}
}

// EvaluatePromptRisk 评估文本风险:小写一次 → 逐词匹配累加(0~1,去重)→ 高/阈值=block、
// 中=log_notify、否则 allow → 套豁免 MaxLevel 封顶 → 按 Mode 收敛(observe 把 block 降为 log_notify)。
func EvaluatePromptRisk(cfg *PromptRiskConfig, text string, subj PromptRiskSubject) PromptRiskDecision {
	dec := PromptRiskDecision{Level: PromptRiskLevelLow, Action: PromptRiskActionAllow, Reasons: []PromptRiskReason{}}
	if cfg == nil {
		return dec
	}
	lowered := strings.ToLower(strings.TrimSpace(text))
	if lowered == "" {
		return dec
	}

	var score float64
	seen := make(map[string]struct{})
	highHit, mediumHit := false, false
	for _, set := range cfg.KeywordSets {
		level := normalizePromptRiskLevel(set.Level)
		mode := normalizePromptRiskMatchMode(set.MatchMode)
		points := set.Score
		if points <= 0 {
			points = defaultPromptRiskScoreForLevel(level)
		}
		for _, kw := range set.Keywords {
			k := strings.ToLower(strings.TrimSpace(kw))
			if k == "" {
				continue
			}
			dedupKey := level + "|" + k
			if _, ok := seen[dedupKey]; ok {
				continue
			}
			if !promptRiskKeywordMatches(mode, lowered, k) {
				continue
			}
			seen[dedupKey] = struct{}{}
			score += points
			dec.Reasons = append(dec.Reasons, PromptRiskReason{Level: level, Keyword: kw, Source: mode, Score: points})
			switch level {
			case PromptRiskLevelHigh:
				highHit = true
			case PromptRiskLevelMedium:
				mediumHit = true
			}
		}
	}
	dec.Score = clampPromptRiskScore(score)

	threshold := cfg.EscalateThreshold
	if threshold <= 0 {
		threshold = defaultPromptRiskEscalateThreshold
	}
	level := PromptRiskLevelLow
	action := PromptRiskActionAllow
	switch {
	case highHit || score >= threshold:
		level, action = PromptRiskLevelHigh, PromptRiskActionBlock
	case mediumHit:
		level, action = PromptRiskLevelMedium, PromptRiskActionLogNotify
	}

	// 豁免封顶(取所有命中豁免里最严格的上限)。
	if cap, ok := matchedExemptionMaxLevel(cfg, subj); ok {
		level, action = capPromptRiskLevel(level, action, cap)
	}

	// Mode 收敛:observe 只记不拦。
	if normalizePromptRiskMode(cfg.Mode) == PromptRiskModeObserve && action == PromptRiskActionBlock {
		action = PromptRiskActionLogNotify
	}

	dec.Level = level
	dec.Action = action
	return dec
}

// includesGroup 判断该请求所属分组是否在 prompt-risk 作用域内(group 级 opt-in)。
func (cfg *PromptRiskConfig) includesGroup(groupID *int64) bool {
	if cfg == nil {
		return false
	}
	if cfg.AllGroups {
		return true
	}
	if groupID == nil {
		return false
	}
	for _, id := range cfg.GroupIDs {
		if id == *groupID {
			return true
		}
	}
	return false
}

// normalize 填充默认值并归一化(load/update 后调用)。
func (cfg *PromptRiskConfig) normalize() {
	if cfg == nil {
		return
	}
	cfg.Mode = normalizePromptRiskMode(cfg.Mode)
	cfg.InputScope = normalizePromptRiskScope(cfg.InputScope)
	if cfg.BlockStatus < 400 || cfg.BlockStatus > 599 {
		cfg.BlockStatus = defaultPromptRiskBlockStatus
	}
	if cfg.EscalateThreshold <= 0 {
		cfg.EscalateThreshold = defaultPromptRiskEscalateThreshold
	}
	if strings.TrimSpace(cfg.BlockMessage) == "" {
		cfg.BlockMessage = defaultPromptRiskBlockMessage
	}
	cfg.GroupIDs = normalizeInt64IDs(cfg.GroupIDs)
	for i := range cfg.KeywordSets {
		cfg.KeywordSets[i].Level = normalizePromptRiskLevel(cfg.KeywordSets[i].Level)
		cfg.KeywordSets[i].MatchMode = normalizePromptRiskMatchMode(cfg.KeywordSets[i].MatchMode)
		if cfg.KeywordSets[i].Score <= 0 {
			cfg.KeywordSets[i].Score = defaultPromptRiskScoreForLevel(cfg.KeywordSets[i].Level)
		}
	}
	for i := range cfg.Exemptions {
		cfg.Exemptions[i].MaxLevel = normalizePromptRiskLevel(cfg.Exemptions[i].MaxLevel)
		cfg.Exemptions[i].GroupIDs = normalizeInt64IDs(cfg.Exemptions[i].GroupIDs)
		cfg.Exemptions[i].UserIDs = normalizeInt64IDs(cfg.Exemptions[i].UserIDs)
		cfg.Exemptions[i].APIKeyIDs = normalizeInt64IDs(cfg.Exemptions[i].APIKeyIDs)
	}
	cfg.Judge.normalize()
}

// normalize 填充 judge 默认值并归一化(随 PromptRiskConfig.normalize 调用)。
func (j *PromptRiskJudgeConfig) normalize() {
	if j == nil {
		return
	}
	j.BaseURL = strings.TrimRight(strings.TrimSpace(j.BaseURL), "/")
	j.Model = strings.TrimSpace(j.Model)
	j.APIKey = strings.TrimSpace(j.APIKey)
	j.PromptTemplate = strings.TrimSpace(j.PromptTemplate)
	if j.TimeoutMS == 0 {
		j.TimeoutMS = defaultPromptRiskJudgeTimeoutMS
	}
	if j.TimeoutMS < minPromptRiskJudgeTimeoutMS {
		j.TimeoutMS = minPromptRiskJudgeTimeoutMS
	}
	if j.TimeoutMS > maxPromptRiskJudgeTimeoutMS {
		j.TimeoutMS = maxPromptRiskJudgeTimeoutMS
	}
	levels := make([]string, 0, len(j.TriggerLevels))
	seen := make(map[string]struct{}, len(j.TriggerLevels))
	for _, lvl := range j.TriggerLevels {
		if !isValidPromptRiskLevel(lvl) {
			continue // 丢弃非法等级(normalizePromptRiskLevel 会把未知值强制成 medium,故先判原值)
		}
		l := normalizePromptRiskLevel(lvl)
		if _, ok := seen[l]; ok {
			continue
		}
		seen[l] = struct{}{}
		levels = append(levels, l)
	}
	if len(levels) == 0 {
		levels = []string{PromptRiskLevelHigh}
	}
	j.TriggerLevels = levels
	j.FailOpen = true // v1 固定 fail-open
}

// triggersLevel 判断某命中等级是否在 judge 触发范围内。
func (j *PromptRiskJudgeConfig) triggersLevel(level string) bool {
	if j == nil {
		return false
	}
	want := normalizePromptRiskLevel(level)
	for _, lvl := range j.TriggerLevels {
		if normalizePromptRiskLevel(lvl) == want {
			return true
		}
	}
	return false
}

// validateRaw 校验管理端提交的**原始**配置:非空但非法的枚举值直接拒绝,而不是静默纠正。
// 与 normalize 配合:先 validateRaw 拒绝错值(给管理员明确反馈),再 normalize 填默认,
// 最后 Validate 校验 regex/word 关键词可编译。空值视为"用默认",不报错。
func (cfg *PromptRiskConfig) validateRaw() error {
	if cfg == nil {
		return fmt.Errorf("prompt risk config is nil")
	}
	if m := strings.TrimSpace(cfg.Mode); m != "" {
		switch strings.ToLower(m) {
		case PromptRiskModeOff, PromptRiskModeObserve, PromptRiskModeBlock:
		default:
			return fmt.Errorf("invalid prompt risk mode: %q", cfg.Mode)
		}
	}
	if sc := strings.TrimSpace(cfg.InputScope); sc != "" {
		switch strings.ToLower(sc) {
		case PromptRiskScopeNewest, PromptRiskScopeFull:
		default:
			return fmt.Errorf("invalid prompt risk input_scope: %q", cfg.InputScope)
		}
	}
	if cfg.BlockStatus != 0 && (cfg.BlockStatus < 400 || cfg.BlockStatus > 599) {
		return fmt.Errorf("prompt risk block_status must be within 400-599")
	}
	for _, set := range cfg.KeywordSets {
		if l := strings.TrimSpace(set.Level); l != "" && !isValidPromptRiskLevel(l) {
			return fmt.Errorf("invalid prompt risk level: %q", set.Level)
		}
		if mm := strings.TrimSpace(set.MatchMode); mm != "" {
			switch strings.ToLower(mm) {
			case PromptRiskMatchContains, PromptRiskMatchRegex, PromptRiskMatchWord:
			default:
				return fmt.Errorf("invalid prompt risk match_mode: %q", set.MatchMode)
			}
		}
	}
	for _, ex := range cfg.Exemptions {
		if l := strings.TrimSpace(ex.MaxLevel); l != "" && !isValidPromptRiskLevel(l) {
			return fmt.Errorf("invalid exemption max_level: %q", ex.MaxLevel)
		}
	}
	if err := cfg.Judge.validateRaw(); err != nil {
		return err
	}
	return nil
}

// validateRaw 校验 judge 原始配置:仅在启用时强制 base_url/model/api_key,枚举/范围非法直接拒绝。
func (j *PromptRiskJudgeConfig) validateRaw() error {
	if j == nil || !j.Enabled {
		return nil
	}
	if strings.TrimSpace(j.BaseURL) == "" {
		return fmt.Errorf("prompt risk judge base_url is required when enabled")
	}
	if _, err := url.ParseRequestURI(strings.TrimSpace(j.BaseURL)); err != nil {
		return fmt.Errorf("invalid prompt risk judge base_url: %q", j.BaseURL)
	}
	if strings.TrimSpace(j.Model) == "" {
		return fmt.Errorf("prompt risk judge model is required when enabled")
	}
	// api_key 允许为空(表示沿用已存旧值);此处不强校验,由 UpdatePromptRiskConfig 合并后于 Validate 兜底。
	if j.TimeoutMS != 0 && (j.TimeoutMS < minPromptRiskJudgeTimeoutMS || j.TimeoutMS > maxPromptRiskJudgeTimeoutMS) {
		return fmt.Errorf("prompt risk judge timeout_ms must be within %d-%d", minPromptRiskJudgeTimeoutMS, maxPromptRiskJudgeTimeoutMS)
	}
	for _, lvl := range j.TriggerLevels {
		if l := strings.TrimSpace(lvl); l != "" && !isValidPromptRiskLevel(l) {
			return fmt.Errorf("invalid prompt risk judge trigger level: %q", lvl)
		}
	}
	return nil
}

// Validate 校验配置(保存时调用):mode/status/level 合法,且 word/regex 关键词可编译。
func (cfg *PromptRiskConfig) Validate() error {
	if cfg == nil {
		return fmt.Errorf("prompt risk config is nil")
	}
	switch normalizePromptRiskMode(cfg.Mode) {
	case PromptRiskModeOff, PromptRiskModeObserve, PromptRiskModeBlock:
	default:
		return fmt.Errorf("invalid prompt risk mode: %q", cfg.Mode)
	}
	if cfg.BlockStatus < 400 || cfg.BlockStatus > 599 {
		return fmt.Errorf("prompt risk block_status must be within 400-599")
	}
	for _, set := range cfg.KeywordSets {
		if !isValidPromptRiskLevel(set.Level) {
			return fmt.Errorf("invalid prompt risk level: %q", set.Level)
		}
		mode := normalizePromptRiskMatchMode(set.MatchMode)
		if mode != PromptRiskMatchRegex && mode != PromptRiskMatchWord {
			continue
		}
		for _, kw := range set.Keywords {
			k := strings.ToLower(strings.TrimSpace(kw))
			if k == "" {
				continue
			}
			if _, err := promptRiskCompiledRegex(promptRiskPattern(mode, k)); err != nil {
				return fmt.Errorf("invalid %s keyword %q: %w", mode, kw, err)
			}
		}
	}
	for _, ex := range cfg.Exemptions {
		if !isValidPromptRiskLevel(ex.MaxLevel) {
			return fmt.Errorf("invalid exemption max_level: %q", ex.MaxLevel)
		}
	}
	if cfg.Judge.Enabled {
		if strings.TrimSpace(cfg.Judge.BaseURL) == "" {
			return fmt.Errorf("prompt risk judge base_url is required when enabled")
		}
		if strings.TrimSpace(cfg.Judge.Model) == "" {
			return fmt.Errorf("prompt risk judge model is required when enabled")
		}
		if strings.TrimSpace(cfg.Judge.APIKey) == "" {
			return fmt.Errorf("prompt risk judge api_key is required when enabled")
		}
	}
	return nil
}

// matchedExemptionMaxLevel 返回命中豁免里最严格(等级最低)的封顶等级。
func matchedExemptionMaxLevel(cfg *PromptRiskConfig, subj PromptRiskSubject) (string, bool) {
	best := ""
	found := false
	for _, ex := range cfg.Exemptions {
		if !exemptionMatchesSubject(ex, subj) {
			continue
		}
		level := normalizePromptRiskLevel(ex.MaxLevel)
		if !found || promptRiskLevelRank(level) < promptRiskLevelRank(best) {
			best = level
			found = true
		}
	}
	return best, found
}

func exemptionMatchesSubject(ex PromptRiskExemption, subj PromptRiskSubject) bool {
	if subj.GroupID != nil && containsInt64(ex.GroupIDs, *subj.GroupID) {
		return true
	}
	if subj.UserID > 0 && containsInt64(ex.UserIDs, subj.UserID) {
		return true
	}
	if subj.APIKeyID > 0 && containsInt64(ex.APIKeyIDs, subj.APIKeyID) {
		return true
	}
	return false
}

// capPromptRiskLevel 把 level 封顶到 cap,并同步动作。
func capPromptRiskLevel(level, action, cap string) (string, string) {
	if promptRiskLevelRank(level) <= promptRiskLevelRank(cap) {
		return level, action
	}
	switch cap {
	case PromptRiskLevelHigh:
		return PromptRiskLevelHigh, PromptRiskActionBlock
	case PromptRiskLevelMedium:
		return PromptRiskLevelMedium, PromptRiskActionLogNotify
	default:
		return PromptRiskLevelLow, PromptRiskActionAllow
	}
}

// promptRiskKeywordMatches 关键词匹配:word 走词边界,regex 走正则,其余子串。文本与关键词均已小写。
func promptRiskKeywordMatches(mode, loweredText, keyword string) bool {
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	if keyword == "" || loweredText == "" {
		return false
	}
	switch normalizePromptRiskMatchMode(mode) {
	case PromptRiskMatchRegex:
		re, err := promptRiskCompiledRegex(keyword)
		return err == nil && re.MatchString(loweredText)
	case PromptRiskMatchWord:
		re, err := promptRiskCompiledRegex(promptRiskPattern(PromptRiskMatchWord, keyword))
		return err == nil && re.MatchString(loweredText)
	default:
		return strings.Contains(loweredText, keyword)
	}
}

// promptRiskPattern 构造匹配模式对应的正则源串。
func promptRiskPattern(mode, keyword string) string {
	if normalizePromptRiskMatchMode(mode) == PromptRiskMatchWord {
		// 词边界:左右不是字母/数字(中文按字母处理,不影响 Latin 词)。
		return `(^|[^\p{L}\p{N}])` + regexp.QuoteMeta(keyword) + `([^\p{L}\p{N}]|$)`
	}
	return keyword
}

// promptRiskCompiledRegex 进程内缓存已编译正则(按模式串去重),避免每请求 regexp.Compile。
var promptRiskRegexCache sync.Map // pattern string -> *regexp.Regexp | error

func promptRiskCompiledRegex(pattern string) (*regexp.Regexp, error) {
	if v, ok := promptRiskRegexCache.Load(pattern); ok {
		switch t := v.(type) {
		case *regexp.Regexp:
			return t, nil
		case error:
			return nil, t
		}
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		promptRiskRegexCache.Store(pattern, err)
		return nil, err
	}
	promptRiskRegexCache.Store(pattern, re)
	return re, nil
}

func normalizePromptRiskMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case PromptRiskModeOff:
		return PromptRiskModeOff
	case PromptRiskModeBlock:
		return PromptRiskModeBlock
	default:
		return PromptRiskModeObserve
	}
}

func normalizePromptRiskScope(scope string) string {
	if strings.ToLower(strings.TrimSpace(scope)) == PromptRiskScopeFull {
		return PromptRiskScopeFull
	}
	return PromptRiskScopeNewest
}

func normalizePromptRiskMatchMode(mode string) string {	switch strings.ToLower(strings.TrimSpace(mode)) {
	case PromptRiskMatchRegex:
		return PromptRiskMatchRegex
	case PromptRiskMatchWord:
		return PromptRiskMatchWord
	default:
		return PromptRiskMatchContains
	}
}

func normalizePromptRiskLevel(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case PromptRiskLevelHigh:
		return PromptRiskLevelHigh
	case PromptRiskLevelLow:
		return PromptRiskLevelLow
	default:
		return PromptRiskLevelMedium
	}
}

func isValidPromptRiskLevel(level string) bool {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case PromptRiskLevelLow, PromptRiskLevelMedium, PromptRiskLevelHigh:
		return true
	default:
		return false
	}
}

func promptRiskLevelRank(level string) int {
	switch normalizePromptRiskLevel(level) {
	case PromptRiskLevelHigh:
		return 2
	case PromptRiskLevelMedium:
		return 1
	default:
		return 0
	}
}

func defaultPromptRiskScoreForLevel(level string) float64 {
	switch normalizePromptRiskLevel(level) {
	case PromptRiskLevelHigh:
		return defaultPromptRiskHighScore
	case PromptRiskLevelMedium:
		return defaultPromptRiskMediumScore
	default:
		return 0
	}
}

func clampPromptRiskScore(score float64) float64 {
	if score < 0 {
		return 0
	}
	if score > maxPromptRiskScore {
		return maxPromptRiskScore
	}
	return score
}
