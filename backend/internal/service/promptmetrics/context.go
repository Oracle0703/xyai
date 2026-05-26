package promptmetrics

import (
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

var (
	cwdPattern    = regexp.MustCompile(`(?i)(?:cwd|project|workspace)\s*[:=]\s*([^\n\r]+)`)
	branchPattern = regexp.MustCompile(`(?i)(?:branch|git\s*branch)\s*[:=]\s*([A-Za-z0-9._/\-]+)`)
)

// DetectContext 结合 header, User-Agent 和请求体文本推断项目, 分支和客户端信息.
// header 是最稳定来源, body 解析仅作为客户端未显式上传时的兜底.
func DetectContext(c *gin.Context, body []byte, promptText string) ClientContext {
	if c == nil || c.Request == nil {
		return ClientContext{}
	}
	ctx := ClientContext{
		ProjectName:   firstHeader(c, "X-Client-Project", "X-Project-Name"),
		GitBranch:     firstHeader(c, "X-Client-Branch", "X-Git-Branch"),
		ClientName:    firstHeader(c, "X-Client-Name"),
		ClientVersion: firstHeader(c, "X-Client-Version"),
	}
	ua := c.GetHeader("User-Agent")
	if ctx.ClientName == "" {
		ctx.ClientName, ctx.ClientVersion = detectClientFromUA(ua)
	} else if ctx.ClientVersion == "" {
		_, ctx.ClientVersion = detectClientFromUA(ua)
	}
	if ctx.ProjectName == "" || ctx.GitBranch == "" {
		ctx = detectContextFromBody(ctx, body, promptText)
	}
	return ctx
}

func firstHeader(c *gin.Context, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(c.GetHeader(name)); value != "" {
			return value
		}
	}
	return ""
}

func detectClientFromUA(ua string) (string, string) {
	lower := strings.ToLower(strings.TrimSpace(ua))
	switch {
	case strings.Contains(lower, "codex-cli"):
		return "codex-cli", tokenAfter(lower, "codex-cli/")
	case strings.Contains(lower, "claude-code"):
		return "claude-code", tokenAfter(lower, "claude-code/")
	case strings.Contains(lower, "gemini-cli"):
		return "gemini-cli", tokenAfter(lower, "gemini-cli/")
	default:
		return "", ""
	}
}

func tokenAfter(s, marker string) string {
	idx := strings.Index(s, marker)
	if idx < 0 {
		return ""
	}
	rest := s[idx+len(marker):]
	for i, r := range rest {
		if r == ' ' || r == ';' || r == ')' {
			return rest[:i]
		}
	}
	return rest
}

func detectContextFromBody(ctx ClientContext, body []byte, promptText string) ClientContext {
	text := strings.TrimSpace(promptText)
	if gjson.ValidBytes(body) {
		json := string(body)
		text = strings.Join([]string{
			text,
			gjson.Get(json, "instructions").String(),
			gjson.Get(json, "system").String(),
		}, "\n")
	}
	if ctx.ProjectName == "" {
		if match := cwdPattern.FindStringSubmatch(text); len(match) > 1 {
			ctx.ProjectName = sanitizeContextToken(match[1])
		}
	}
	if ctx.GitBranch == "" {
		if match := branchPattern.FindStringSubmatch(text); len(match) > 1 {
			ctx.GitBranch = sanitizeContextToken(match[1])
		}
	}
	return ctx
}

func sanitizeContextToken(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "`'\"")
	if idx := strings.IndexAny(value, "\n\r"); idx >= 0 {
		value = value[:idx]
	}
	if len(value) > 255 {
		value = value[:255]
	}
	return strings.TrimSpace(value)
}
