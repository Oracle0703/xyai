package service

import (
	"fmt"
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
