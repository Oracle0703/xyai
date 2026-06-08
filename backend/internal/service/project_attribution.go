package service

import (
	"encoding/json"
	"regexp"
	"strings"
)

// ProjectAttribution 是从网关请求体中离线提取的项目归因信息。
// 提取规则基于 2026-06-05 全天真实归档流量(10,856 请求)三轮扫描验证,
// 各客户端命中率见 docs/features/token-analysis-project-attribution-design-cn.md。
type ProjectAttribution struct {
	// Workdir 归一化后的工作目录, 如 E:/code/lag-killer; 字典命中时可能为空。
	Workdir string
	// Project 仓库名(Workdir 的 basename 或已知根命中)。
	Project string
	// Branch 仅 Claude Code 流量可得, 留存不做统计维度。
	Branch string
	// Source 提取来源: system / env-context / instructions-md /
	// workspace-folders / raw-head / known-root; 空表示未归因。
	Source string
	// PathHints 在无直接 cwd 时收集的绝对路径(已小写归一), 供调用方
	// 与已知仓库根做前缀匹配(Copilot 附件路径场景), 不落库。
	PathHints []string
}

// Attributed 返回是否已直接归因(不含 PathHints 待匹配的情况)。
func (a ProjectAttribution) Attributed() bool {
	return a.Project != ""
}

const (
	projectAttributionHeadWindow    = 300 * 1024
	projectAttributionMaxPathHints  = 60
	projectAttributionMaxCwdLen     = 250
	projectAttributionMaxBaseLen    = 80
	projectAttributionSourceSystem  = "system"
	projectAttributionSourceEnvCtx  = "env-context"
	projectAttributionSourceInstrMD = "instructions-md"
	projectAttributionSourceWSFold  = "workspace-folders"
	projectAttributionSourceRawHead = "raw-head"
	// ProjectAttributionSourceKnownRoot 由调用方在 PathHints 命中已知根时使用。
	ProjectAttributionSourceKnownRoot = "known-root"
)

var (
	// Claude Code / trae 等 system prompt 内的工作目录与分支标记。
	reAttribCwd    = regexp.MustCompile(`(?:Primary working|Working) director(?:y|ies): ([^\n]+)`)
	reAttribBranch = regexp.MustCompile(`Current branch: ([^\s\\"]+)`)
	// Codex environment_context。
	reAttribCodexCwd = regexp.MustCompile(`<cwd>([^<\n]+)</cwd>`)
	// Copilot: 仓库根 /.github/copilot-instructions.md 反推。
	reAttribCopilotInstr = regexp.MustCompile(`([A-Za-z]:[\\/](?:[^\\/\n"'<>|*?]+[\\/])*?)\.github[\\/]copilot-instructions\.md`)
	// Copilot: workspace 文件夹列表文案。
	reAttribCopilotWS = regexp.MustCompile(`(?i)workspace with the following folders[^\n]*\n\s*[-*]?\s*([A-Za-z]:[\\/][^\n<"']+|/[^\n<"']+)`)
	// Copilot: 附件 filePath 属性(容忍 JSON 转义引号 \")与盘符绝对路径
	// (左边界非字母数字, 防 https:// 截成 s://)。
	reAttribAttachPath = regexp.MustCompile(`filePath=\\?["']([^"'\\]+)`)
	reAttribWinPath    = regexp.MustCompile(`(^|[^A-Za-z0-9])([A-Za-z]:[\\/][^\s"'<>|*?:\n]{3,200})`)
	reAttribDrivePath  = regexp.MustCompile(`^[A-Za-z]:/`)
)

// projectAttributionBlacklist 过滤 Copilot system prompt 自带示例路径与
// 用户级配置目录等非工作区路径(均为真实流量中确认的噪声来源)。
var projectAttributionBlacklist = []string{
	"users/someone", "users\\someone", "pygorithm", ".copilot",
	"appdata", "code/user/prompts", "code\\user\\prompts", "%3a",
	"program files", "/windows/", "\\windows\\",
}

// ExtractProjectAttribution 从请求体中提取项目归因。endpoint 为归一化的
// inbound endpoint(如 /v1/messages), userAgent 为客户端 UA, body 为归档的
// 请求体(可能因超限被尾部截断, 各规则只依赖头部窗口)。
func ExtractProjectAttribution(endpoint, userAgent, body string) ProjectAttribution {
	if strings.TrimSpace(body) == "" {
		return ProjectAttribution{}
	}
	ua := strings.ToLower(userAgent)
	switch {
	case strings.Contains(ua, "copilot"):
		return extractCopilotAttribution(body)
	case strings.Contains(ua, "claude-cli") || strings.Contains(ua, "claude-code") || strings.HasPrefix(endpoint, "/v1/messages"):
		return extractAnthropicAttribution(body)
	case strings.Contains(ua, "codex") || endpoint == "/v1/responses":
		return extractCodexAttribution(body)
	default:
		return extractGenericAttribution(body)
	}
}

// extractAnthropicAttribution 优先解析 body JSON 的 system 字段(字段锚定,
// 避免会话正文里引用的标记文本造成误提取), 失败时降级头部窗口正则。
func extractAnthropicAttribution(body string) ProjectAttribution {
	var root map[string]any
	if err := json.Unmarshal([]byte(body), &root); err == nil {
		text := anthropicAttributionSystemText(root)
		attr := ProjectAttribution{}
		if m := reAttribCwd.FindStringSubmatch(text); m != nil {
			attr.setWorkdir(m[1], projectAttributionSourceSystem)
		}
		if m := reAttribBranch.FindStringSubmatch(text); m != nil {
			attr.Branch = m[1]
		}
		return attr
	}
	// body 超限截断等导致非完整 JSON: env 块固定在 system 头部, 头部窗口可覆盖。
	head := attributionHead(body)
	attr := ProjectAttribution{}
	if m := reAttribCwd.FindStringSubmatch(head); m != nil {
		attr.setWorkdir(strings.TrimRight(m[1], `\"`), projectAttributionSourceRawHead)
	}
	if m := reAttribBranch.FindStringSubmatch(head); m != nil {
		attr.Branch = m[1]
	}
	return attr
}

func extractCodexAttribution(body string) ProjectAttribution {
	head := attributionHead(body)
	attr := ProjectAttribution{}
	if m := reAttribCodexCwd.FindStringSubmatch(head); m != nil {
		attr.setWorkdir(m[1], projectAttributionSourceEnvCtx)
	}
	if m := reAttribBranch.FindStringSubmatch(head); m != nil {
		attr.Branch = m[1]
	}
	return attr
}

// extractCopilotAttribution: Copilot 请求没有 cwd 概念, 依次尝试
// ① copilot-instructions.md 路径反推仓库根 ② workspace 文件夹列表
// ③ 收集附件/盘符绝对路径作为 PathHints 供已知根前缀匹配。
func extractCopilotAttribution(body string) ProjectAttribution {
	// 工具结果等嵌套 JSON 文本中的路径分隔符按嵌套层数翻倍(2/4/8 重
	// 反斜杠), 循环折叠到单反斜杠为止。
	nbody := body
	for strings.Contains(nbody, `\\`) {
		nbody = strings.ReplaceAll(nbody, `\\`, `\`)
	}
	attr := ProjectAttribution{}
	for _, m := range reAttribCopilotInstr.FindAllStringSubmatch(nbody, -1) {
		if root := normalizeAttributionPath(m[1]); root != "" && !attributionBlacklisted(root) {
			attr.setWorkdir(root, projectAttributionSourceInstrMD)
			break
		}
	}
	if attr.Workdir == "" {
		for _, m := range reAttribCopilotWS.FindAllStringSubmatch(nbody, -1) {
			if root := normalizeAttributionPath(cutAttributionLiteralNewline(m[1])); root != "" && !attributionBlacklisted(root) {
				attr.setWorkdir(root, projectAttributionSourceWSFold)
				break
			}
		}
	}
	if attr.Workdir == "" {
		seen := map[string]struct{}{}
		collect := func(raw string) {
			p := normalizeAttributionPath(raw)
			if p == "" || attributionBlacklisted(p) {
				return
			}
			if !strings.HasPrefix(p, "/") && !reAttribDrivePath.MatchString(p) {
				return
			}
			low := strings.ToLower(p)
			if _, ok := seen[low]; ok || len(attr.PathHints) >= projectAttributionMaxPathHints {
				return
			}
			seen[low] = struct{}{}
			attr.PathHints = append(attr.PathHints, low)
		}
		for _, m := range reAttribAttachPath.FindAllStringSubmatch(nbody, -1) {
			collect(m[1])
		}
		for _, m := range reAttribWinPath.FindAllStringSubmatch(nbody, -1) {
			collect(m[2])
		}
	}
	return attr
}

func extractGenericAttribution(body string) ProjectAttribution {
	head := attributionHead(body)
	attr := ProjectAttribution{}
	if m := reAttribCwd.FindStringSubmatch(head); m != nil {
		attr.setWorkdir(strings.TrimRight(m[1], `\"`), projectAttributionSourceRawHead)
	} else if m := reAttribCodexCwd.FindStringSubmatch(head); m != nil {
		attr.setWorkdir(m[1], projectAttributionSourceRawHead)
	}
	if m := reAttribBranch.FindStringSubmatch(head); m != nil {
		attr.Branch = m[1]
	}
	return attr
}

// setWorkdir 归一化、截断多值行并校验 cwd, 合法时填充 Workdir/Project/Source。
func (a *ProjectAttribution) setWorkdir(raw, source string) {
	c := normalizeAttributionPath(cutAttributionLiteralNewline(raw))
	if !validAttributionCwd(c) {
		return
	}
	a.Workdir = c
	a.Project = AttributionProjectOf(c)
	a.Source = source
}

// AttributionProjectOf 返回归一化工作目录的 basename 作为项目名。
func AttributionProjectOf(workdir string) string {
	if workdir == "" {
		return ""
	}
	if idx := strings.LastIndex(workdir, "/"); idx >= 0 {
		return workdir[idx+1:]
	}
	return workdir
}

func attributionHead(body string) string {
	if len(body) > projectAttributionHeadWindow {
		return body[:projectAttributionHeadWindow]
	}
	return body
}

func normalizeAttributionPath(p string) string {
	p = strings.Trim(strings.TrimSpace(p), "`'\" ")
	p = strings.ReplaceAll(p, `\\`, `\`)
	p = strings.ReplaceAll(p, `\`, "/")
	return strings.TrimRight(p, "/")
}

// cutAttributionLiteralNewline 截断嵌套 JSON 文本里的字面量 \n 多值行
// (如 trae 的 "Working directories: a\n- Operating system: ...")。
func cutAttributionLiteralNewline(v string) string {
	for _, sep := range []string{`\n`, "/n- ", "/n  "} {
		if i := strings.Index(v, sep); i > 0 {
			v = v[:i]
		}
	}
	return v
}

func validAttributionCwd(p string) bool {
	if p == "" || len(p) > projectAttributionMaxCwdLen || strings.ContainsAny(p, "\n") {
		return false
	}
	if !strings.HasPrefix(p, "/") && !strings.HasPrefix(p, "~") && !reAttribDrivePath.MatchString(p) {
		return false
	}
	base := AttributionProjectOf(p)
	return base != "" && len(base) <= projectAttributionMaxBaseLen && !strings.Contains(base, ":")
}

func attributionBlacklisted(p string) bool {
	low := strings.ToLower(p)
	for _, b := range projectAttributionBlacklist {
		if strings.Contains(low, b) {
			return true
		}
	}
	return false
}

// MatchAttributionKnownRoot 在已知仓库根集合(小写归一)中为 PathHints 做
// 最长前缀匹配, 命中返回根路径。roots 需按长度降序排列。
func MatchAttributionKnownRoot(hints []string, rootsByLenDesc []string) string {
	for _, hint := range hints {
		for _, root := range rootsByLenDesc {
			if hint == root || strings.HasPrefix(hint, root+"/") {
				return root
			}
		}
	}
	return ""
}

func anthropicAttributionSystemText(root map[string]any) string {
	switch sys := root["system"].(type) {
	case string:
		return sys
	case []any:
		var sb strings.Builder
		for _, item := range sys {
			if block, ok := item.(map[string]any); ok {
				if text, ok := block["text"].(string); ok {
					sb.WriteString(text)
					sb.WriteString("\n")
				}
			}
		}
		return sb.String()
	}
	return ""
}
