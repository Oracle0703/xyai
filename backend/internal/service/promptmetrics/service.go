package promptmetrics

import (
	"context"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Service 封装管理端操作和本地规则分析.
// 后续若接入 LLM worker, 应保持这里暴露的 API 契约稳定.
type Service struct {
	repo *Repository
}

// NewService 创建 prompt metrics 服务.
func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Overview(ctx context.Context, filters Filters) (*Overview, error) {
	return s.repo.Overview(ctx, filters)
}

func (s *Service) Trend(ctx context.Context, filters Filters, bucket string) ([]TrendPoint, error) {
	return s.repo.Trend(ctx, filters, bucket)
}

func (s *Service) Rank(ctx context.Context, filters Filters, dimension string, limit int) ([]RankItem, error) {
	return s.repo.Rank(ctx, filters, dimension, limit)
}

func (s *Service) ListEvents(ctx context.Context, filters Filters, page, pageSize int) ([]Event, Pagination, error) {
	return s.repo.ListEvents(ctx, filters, page, pageSize)
}

func (s *Service) EventByID(ctx context.Context, id int64) (*Event, error) {
	return s.repo.EventByID(ctx, id)
}

// Reanalyze 用本地规则重新生成提示词分析结果.
// 它先标记 pending, 再读取详情并 upsert 结果, 失败时标记 failed.
func (s *Service) Reanalyze(ctx context.Context, id int64) (*Analysis, error) {
	if err := s.repo.MarkAnalysisStatus(ctx, id, AnalysisStatusPending); err != nil {
		return nil, err
	}
	event, err := s.repo.EventByID(ctx, id)
	if err != nil {
		_ = s.repo.MarkAnalysisStatus(ctx, id, AnalysisStatusFailed)
		return nil, err
	}
	analysis := analyzePromptDetail(*event)
	if err := s.repo.UpsertAnalysis(ctx, analysis); err != nil {
		_ = s.repo.MarkAnalysisStatus(ctx, id, AnalysisStatusFailed)
		return nil, err
	}
	return &analysis, nil
}

// analyzePromptDetail 基于可解释的本地规则给出质量分和改进建议.
// 规则故意保守, 作为没有外部分析 worker 时的兜底, 不声称是最终 AI 评价.
func analyzePromptDetail(event Event) Analysis {
	text := strings.TrimSpace(event.PromptText)
	if text == "" {
		text = strings.TrimSpace(event.PromptExcerpt)
	}
	chars := utf8.RuneCountInString(text)
	clarity := scoreClarity(text)
	contextScore := scoreContext(text, event)
	actionability := scoreActionability(text)
	constraint := scoreConstraint(text)
	risk := scoreRisk(text)
	quality := clampScore((clarity+contextScore+actionability+constraint+(100-risk))/5, 0, 100)
	categories := categoriesForPrompt(text, event)
	suggestions := suggestionsForScores(clarity, contextScore, actionability, constraint, risk)
	return Analysis{
		PromptEventID:          event.ID,
		Summary:                buildSummary(text, chars),
		QualityScore:           quality,
		ClarityScore:           clarity,
		ContextScore:           contextScore,
		ActionabilityScore:     actionability,
		ConstraintScore:        constraint,
		RiskScore:              risk,
		Categories:             categories,
		ImprovementSuggestions: suggestions,
		AnalyzerModel:          "local-rules-v1",
		AnalyzedAt:             time.Now(),
	}
}

func scoreClarity(text string) int {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return 0
	}
	score := 50
	if utf8.RuneCountInString(trimmed) >= 20 {
		score += 15
	}
	if strings.ContainsAny(trimmed, "?.?!。？！") {
		score += 10
	}
	if hasStructuredLines(trimmed) {
		score += 15
	}
	if repeatedWhitespaceRatio(trimmed) > 0.25 {
		score -= 15
	}
	return clampScore(score, 0, 100)
}

func scoreContext(text string, event Event) int {
	score := 35
	lower := strings.ToLower(text)
	if event.ProjectName != "" || event.GitBranch != "" {
		score += 15
	}
	for _, marker := range []string{"context", "背景", "文件", "代码", "error", "日志", "环境", "版本"} {
		if strings.Contains(lower, marker) {
			score += 8
		}
	}
	return clampScore(score, 0, 100)
}

func scoreActionability(text string) int {
	score := 40
	lower := strings.ToLower(text)
	for _, marker := range []string{"请", "帮我", "实现", "修复", "分析", "生成", "对比", "review", "fix", "implement", "explain"} {
		if strings.Contains(lower, marker) {
			score += 10
		}
	}
	if strings.Contains(lower, "怎么") || strings.Contains(lower, "how") {
		score += 8
	}
	return clampScore(score, 0, 100)
}

func scoreConstraint(text string) int {
	score := 30
	lower := strings.ToLower(text)
	for _, marker := range []string{"不要", "必须", "限制", "格式", "输出", "只", "不得", "must", "only", "without", "format"} {
		if strings.Contains(lower, marker) {
			score += 10
		}
	}
	if strings.Contains(text, "\n") {
		score += 10
	}
	return clampScore(score, 0, 100)
}

func scoreRisk(text string) int {
	score := 0
	lower := strings.ToLower(text)
	for _, marker := range []string{"password", "secret", "token", "api key", "私钥", "密码", "密钥", "删除", "drop table", "rm -rf"} {
		if strings.Contains(lower, marker) {
			score += 20
		}
	}
	if utf8.RuneCountInString(text) > 8000 {
		score += 10
	}
	return clampScore(score, 0, 100)
}

func buildSummary(text string, chars int) string {
	if strings.TrimSpace(text) == "" {
		return "未采集到可分析的提示词全文, 已基于摘要字段生成兜底分析."
	}
	firstLine := strings.TrimSpace(strings.Split(text, "\n")[0])
	if utf8.RuneCountInString(firstLine) > 80 {
		firstLine = string([]rune(firstLine)[:80])
	}
	return "用户提示词约 " + intToString(chars) + " 字符, 主题摘要: " + firstLine
}

func categoriesForPrompt(text string, event Event) []string {
	categories := []string{"general"}
	lower := strings.ToLower(text)
	if strings.Contains(lower, "代码") || strings.Contains(lower, "code") || strings.Contains(lower, "bug") {
		categories = append(categories, "coding")
	}
	if strings.Contains(lower, "分析") || strings.Contains(lower, "analyze") {
		categories = append(categories, "analysis")
	}
	if event.ProjectName != "" {
		categories = append(categories, "project_context")
	}
	return uniqueStrings(categories)
}

func suggestionsForScores(clarity, contextScore, actionability, constraint, risk int) []string {
	suggestions := make([]string, 0)
	if clarity < 60 {
		suggestions = append(suggestions, "补充明确问题和期望结果, 减少模糊表达.")
	}
	if contextScore < 60 {
		suggestions = append(suggestions, "补充项目背景, 文件路径, 错误信息或相关输入输出示例.")
	}
	if actionability < 60 {
		suggestions = append(suggestions, "把请求拆成可执行任务, 明确需要模型完成的动作.")
	}
	if constraint < 60 {
		suggestions = append(suggestions, "说明输出格式, 禁止事项, 边界条件和验收标准.")
	}
	if risk >= 40 {
		suggestions = append(suggestions, "检查提示词中是否包含密钥, 密码或高风险破坏性操作.")
	}
	if len(suggestions) == 0 {
		suggestions = append(suggestions, "暂无明显改进建议.")
	}
	return suggestions
}

func hasStructuredLines(text string) bool {
	lines := strings.Split(text, "\n")
	structured := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "-") || strings.HasPrefix(line, "*") || strings.HasPrefix(line, "1.") || strings.Contains(line, ":") {
			structured++
		}
	}
	return structured >= 2
}

func repeatedWhitespaceRatio(text string) float64 {
	if text == "" {
		return 0
	}
	spaces := 0
	for _, r := range text {
		if unicode.IsSpace(r) {
			spaces++
		}
	}
	return float64(spaces) / float64(utf8.RuneCountInString(text))
}

func clampScore(v, minValue, maxValue int) int {
	if v < minValue {
		return minValue
	}
	if v > maxValue {
		return maxValue
	}
	return v
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func intToString(v int) string {
	if v == 0 {
		return "0"
	}
	digits := make([]byte, 0, 12)
	for v > 0 {
		digits = append(digits, byte('0'+v%10))
		v /= 10
	}
	for i, j := 0, len(digits)-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}
	return string(digits)
}
