package service

import (
	"context"
	"hash/fnv"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const (
	// TokenAnalysisClientFamilyOther 是未命中任何规则(含空 UA)时的客户端族。
	TokenAnalysisClientFamilyOther = "other"
	// TokenAnalysisClientFamilySDKScript 是 SDK/脚本类调用, 不属于 agent 族。
	TokenAnalysisClientFamilySDKScript = "sdk_script"

	// tokenAnalysisUserAgentMaxChars 与 usage_logs.user_agent 的 varchar(512) 对齐。
	tokenAnalysisUserAgentMaxChars = 512
	// tokenAnalysisClientFamilyRefreshBatch 是每轮自动索引重新映射的行数上限。
	tokenAnalysisClientFamilyRefreshBatch = 2000
	// tokenAnalysisClientFamilyRecheckInterval 是扫完一遍后再次复查的间隔: 滚动发布
	// 期间旧实例仍可能写入旧规则版本的行, 低频复查保证它们最终被补映射。
	tokenAnalysisClientFamilyRecheckInterval = time.Hour
)

// defaultTokenAnalysisClientFamilyRules 是内置映射, 按顺序首个命中生效。顺序有
// 依赖: codex 早于 copilot(codex_vscode、codex-tui 的 UA 都带 vscode), claude_code
// 早于 sdk_script(claude-cli 的 UA 带 agent-sdk)。
var defaultTokenAnalysisClientFamilyRules = []config.TokenAnalysisClientFamilyRule{
	{Family: "claude_code", Pattern: `claude-cli|claude-code`},
	{Family: "codex", Pattern: `codex`},
	{Family: "cursor", Pattern: `cursor`},
	{Family: "copilot", Pattern: `copilot|vscode`},
	{Family: "cline_like", Pattern: `cline|roo-?code|kilo-?code`},
	{Family: "gemini_cli", Pattern: `gemini-cli`},
	{Family: "opencode", Pattern: `opencode`},
	// hertz 是公司自有的类 Claude Code 客户端。
	{Family: "hertz", Pattern: `^hertz\b`},
	{Family: TokenAnalysisClientFamilySDKScript, Pattern: `openai[-/](python|node|js)|anthropic|python-requests|python-urllib|axios|node-fetch|curl/|go-http-client|okhttp`},
}

type tokenAnalysisClientFamilyRule struct {
	family  string
	pattern *regexp.Regexp
}

// tokenAnalysisClientFamilyMatcher 把 User-Agent 映射为客户端族; version 是规则
// 内容的哈希, 规则变化后已存行按存下的 UA 重新映射(不需要重读原文)。
type tokenAnalysisClientFamilyMatcher struct {
	rules   []tokenAnalysisClientFamilyRule
	version int32
}

// newTokenAnalysisClientFamilyMatcher 编译规则; 配置已在加载时校验, 这里遇到
// 无法编译的规则直接跳过, 不让单条坏规则拖垮索引。
func newTokenAnalysisClientFamilyMatcher(rules []config.TokenAnalysisClientFamilyRule) *tokenAnalysisClientFamilyMatcher {
	if len(rules) == 0 {
		rules = defaultTokenAnalysisClientFamilyRules
	}
	h := fnv.New32a()
	compiled := make([]tokenAnalysisClientFamilyRule, 0, len(rules))
	for _, rule := range rules {
		_, _ = h.Write([]byte(rule.Family))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(rule.Pattern))
		_, _ = h.Write([]byte{0})
		re, err := regexp.Compile("(?i)" + rule.Pattern)
		if err != nil {
			continue
		}
		compiled = append(compiled, tokenAnalysisClientFamilyRule{family: rule.Family, pattern: re})
	}
	// 0 保留给"未识别", 版本只取正数。
	version := int32(h.Sum32() & 0x7fffffff)
	if version == 0 {
		version = 1
	}
	return &tokenAnalysisClientFamilyMatcher{rules: compiled, version: version}
}

func (m *tokenAnalysisClientFamilyMatcher) Match(userAgent string) string {
	if strings.TrimSpace(userAgent) == "" {
		return TokenAnalysisClientFamilyOther
	}
	for _, rule := range m.rules {
		if rule.pattern.MatchString(userAgent) {
			return rule.family
		}
	}
	return TokenAnalysisClientFamilyOther
}

// IsTokenAnalysisAgentFamily 判断客户端族是否属于 agent 族(用于连续活跃统计)。
func IsTokenAnalysisAgentFamily(family string) bool {
	return family != "" && family != TokenAnalysisClientFamilyOther && family != TokenAnalysisClientFamilySDKScript
}

// normalizeTokenAnalysisUserAgent 清洗并截断归档 UA, 与 usage_logs 列宽一致。
func normalizeTokenAnalysisUserAgent(userAgent string) string {
	userAgent = strings.TrimSpace(sanitizeTokenAnalysisText(userAgent))
	if runes := []rune(userAgent); len(runes) > tokenAnalysisUserAgentMaxChars {
		userAgent = string(runes[:tokenAnalysisUserAgentMaxChars])
	}
	return userAgent
}

// tokenAnalysisClientFamilyRefresh 是重新映射的扫描进度: 按 id 游标分批扫完一遍后,
// 隔 tokenAnalysisClientFamilyRecheckInterval 再开始下一遍。
type tokenAnalysisClientFamilyRefresh struct {
	mu          sync.Mutex
	cursor      int64
	nextRoundAt time.Time
}

// refreshTokenAnalysisClientFamilies 按当前规则重新映射一批旧版本的行, 返回读出行数。
// 只处理已识别的行(版本非 0); 列上线前的历史行等受控重索引补 UA。写回按读出快照
// 条件提交, 期间被改过的行跳过, 由下一遍重新读取。
func (s *TokenAnalysisService) refreshTokenAnalysisClientFamilies(ctx context.Context, limit int, now time.Time) (int, error) {
	progress := &s.clientFamilyRefresh
	progress.mu.Lock()
	defer progress.mu.Unlock()
	if progress.cursor == 0 && now.Before(progress.nextRoundAt) {
		return 0, nil
	}
	matcher := s.clientFamilies
	rows, err := s.repo.ListStaleClientFamilies(ctx, matcher.version, progress.cursor, limit)
	if err != nil {
		return 0, err
	}
	for i := range rows {
		rows[i].Family = matcher.Match(rows[i].UserAgent)
	}
	if err := s.repo.UpdateClientFamilies(ctx, matcher.version, rows); err != nil {
		return 0, err
	}
	if len(rows) < limit {
		// 本遍扫完: 游标归零, 到期后从头复查。
		progress.cursor = 0
		progress.nextRoundAt = now.Add(tokenAnalysisClientFamilyRecheckInterval)
	} else {
		progress.cursor = rows[len(rows)-1].ID
	}
	return len(rows), nil
}
