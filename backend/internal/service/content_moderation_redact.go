package service

import (
	"regexp"
	"strings"
)

// contentModerationRedactSecrets 控制风控日志摘要(InputExcerpt)与 cyber policy 错误体
// 是否对 URL、Bearer/JWT/api_key 等做脱敏。
//
// 本部署风控日志仅管理员本人可见,且脱敏会把触发词所在的整条 URL 抹成 [已脱敏],
// 反而看不出 Prompt 风险命中了什么,故显式关闭(命中词本身在 category_scores 里另有
// 未脱敏记录)。若日后日志面向多人、或落库被外发/备份共享,改回 true 即恢复脱敏。
const contentModerationRedactSecrets = false

var contentModerationSecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bhttps?://[^\s"'<>，。；、]+`),
	regexp.MustCompile(`(?i)\b((?:api[_-]?key|apikey|access[_-]?token|refresh[_-]?token|id[_-]?token|session[_-]?token|token|session|cookie|set[_-]?cookie|authorization|bearer|password|passwd|pwd|secret|client[_-]?secret|private[_-]?key)\s*[:=]\s*)(["']?)[^"'\s,;，。；、]{6,}`),
	regexp.MustCompile(`(?i)\b(Bearer\s+)[A-Za-z0-9._~+/=-]{12,}`),
	regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b`),
	regexp.MustCompile(`(?i)\b(?:sk|sk-proj|sk-ant|sess|rk|pk|ak|api|key|token|secret)[_-][A-Za-z0-9._~+/=-]{12,}\b`),
	regexp.MustCompile(`\b[0-9a-fA-F]{32,}\b`),
	regexp.MustCompile(`\b[A-Za-z0-9_-]{48,}\b`),
	regexp.MustCompile(`\b[A-Za-z0-9+/]{48,}={0,2}\b`),
	regexp.MustCompile(`\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b`),
}

// redactContentModerationSecrets 按 contentModerationRedactSecrets 开关对文本脱敏;
// 关闭时原样返回(仅 TrimSpace)。
func redactContentModerationSecrets(text string) string {
	text = strings.TrimSpace(text)
	if text == "" || !contentModerationRedactSecrets {
		return text
	}
	return applyContentModerationSecretRedaction(text)
}

// applyContentModerationSecretRedaction 执行实际的密钥/URL 脱敏(始终可用,供开关开启时调用与单测覆盖)。
func applyContentModerationSecretRedaction(text string) string {
	out := text
	for idx, pattern := range contentModerationSecretPatterns {
		switch idx {
		case 1:
			out = pattern.ReplaceAllString(out, `${1}${2}[已脱敏]`)
		case 2:
			out = pattern.ReplaceAllString(out, `${1}[已脱敏]`)
		default:
			out = pattern.ReplaceAllString(out, `[已脱敏]`)
		}
	}
	return out
}
