package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"golang.org/x/text/unicode/norm"
)

// 降智测试的判分、测试项校验与模板。设计见 docs/features/gpt-iq-test-design-cn.md §4。

const (
	GPTIQKindChat   = "chat"
	GPTIQKindGraded = "graded"

	GPTIQScoringNone      = "none"
	GPTIQScoringFinalLine = "final_line"
	GPTIQScoringNumberAny = "number_any"
	GPTIQScoringContains  = "contains"
	GPTIQScoringRegex     = "regex"

	gptIQNameMaxRunes     = 40
	gptIQPromptMaxRunes   = 8000
	gptIQExpectedMaxRunes = 200
	gptIQModelMaxLen      = 100
	gptIQMaxRuns          = 5
	gptIQMinTimeout       = 30
	gptIQMaxTimeout       = 600
	gptIQDefaultChatTO    = 60
	gptIQDefaultGradedTO  = 300
)

var (
	gptIQModelPattern  = regexp.MustCompile(`^[A-Za-z0-9._:/-]+$`)
	gptIQEffortAllowed = map[string]bool{"": true, "none": true, "minimal": true, "low": true, "medium": true, "high": true, "xhigh": true, "max": true, "ultra": true}
	// 题面会进入数据库与公开页面；拒绝疑似密钥、Bearer 令牌与邮箱。
	gptIQPromptSecretPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bsk-[A-Za-z0-9_-]{8,}`),
		regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]{10,}`),
		regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`),
	}
	// final_line：回答中"最终答案＝X"或"Final answer: X"形式的行，取最后一行。
	gptIQFinalLinePattern = regexp.MustCompile(`(?i)^(?:最终答案|final\s*answer)\s*[:：=＝]\s*(.+)$`)
	gptIQIntegerPattern   = regexp.MustCompile(`^[-+]?[0-9]+$`)
	// gptIQFinalLineDecor 是答案行两端可忽略的 Markdown 符号（加粗、斜体、代码、标题、引用）。
	gptIQFinalLineDecor = "*_`#> "
)

// GPTIQCaseInput 是创建或修改测试项的请求体。
type GPTIQCaseInput struct {
	Name            string `json:"name"`
	Enabled         bool   `json:"enabled"`
	SortOrder       int    `json:"sort_order"`
	PublicPrompt    bool   `json:"public_prompt"`
	PublicAnswer    bool   `json:"public_answer"`
	Kind            string `json:"kind"`
	Prompt          string `json:"prompt"`
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoning_effort"`
	Runs            int    `json:"runs"`
	TimeoutSeconds  int    `json:"timeout_seconds"`
	ScoringMode     string `json:"scoring_mode"`
	ScoringExpected string `json:"scoring_expected"`
}

// GPTIQRevisionSpec 是不可变题目版本中影响结果解释的字段。
type GPTIQRevisionSpec struct {
	Kind            string `json:"kind"`
	Prompt          string `json:"prompt"`
	PromptSHA256    string `json:"prompt_sha256"`
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoning_effort"`
	Runs            int    `json:"runs"`
	TimeoutSeconds  int    `json:"timeout_seconds"`
	ScoringMode     string `json:"scoring_mode"`
	ScoringExpected string `json:"scoring_expected"`
}

// SameAs 判断两个版本是否等价（相同则修改测试项时不新建版本）。
func (s GPTIQRevisionSpec) SameAs(o GPTIQRevisionSpec) bool {
	return s.Kind == o.Kind && s.Prompt == o.Prompt && s.Model == o.Model && s.ReasoningEffort == o.ReasoningEffort &&
		s.Runs == o.Runs && s.TimeoutSeconds == o.TimeoutSeconds && s.ScoringMode == o.ScoringMode && s.ScoringExpected == o.ScoringExpected
}

// NormalizeGPTIQCaseInput 校验并规范化测试项输入，返回可变属性与版本字段。
func NormalizeGPTIQCaseInput(in GPTIQCaseInput) (GPTIQCaseInput, GPTIQRevisionSpec, error) {
	out := in
	out.Name = strings.TrimSpace(in.Name)
	if n := utf8.RuneCountInString(out.Name); n == 0 || n > gptIQNameMaxRunes || strings.ContainsFunc(out.Name, unicode.IsControl) {
		return out, GPTIQRevisionSpec{}, errors.New("name must be 1-40 characters without control characters")
	}
	spec := GPTIQRevisionSpec{
		Kind:            strings.TrimSpace(in.Kind),
		Prompt:          strings.ReplaceAll(in.Prompt, "\r\n", "\n"),
		Model:           strings.TrimSpace(in.Model),
		ReasoningEffort: strings.ToLower(strings.TrimSpace(in.ReasoningEffort)),
		Runs:            in.Runs,
		TimeoutSeconds:  in.TimeoutSeconds,
		ScoringMode:     strings.TrimSpace(in.ScoringMode),
		ScoringExpected: in.ScoringExpected,
	}
	if spec.Kind != GPTIQKindChat && spec.Kind != GPTIQKindGraded {
		return out, spec, errors.New("kind must be chat or graded")
	}
	if strings.TrimSpace(spec.Prompt) == "" || utf8.RuneCountInString(spec.Prompt) > gptIQPromptMaxRunes {
		return out, spec, errors.New("prompt must be 1-8000 characters")
	}
	for _, pattern := range gptIQPromptSecretPatterns {
		if pattern.MatchString(spec.Prompt) {
			return out, spec, errors.New("prompt looks like it contains a credential or email address")
		}
	}
	if spec.Model == "" || len(spec.Model) > gptIQModelMaxLen || !gptIQModelPattern.MatchString(spec.Model) {
		return out, spec, errors.New("model must be 1-100 characters of letters, digits, . _ : / -")
	}
	if !gptIQEffortAllowed[spec.ReasoningEffort] {
		return out, spec, errors.New("unsupported reasoning_effort")
	}
	if spec.Runs < 1 || spec.Runs > gptIQMaxRuns {
		return out, spec, errors.New("runs must be 1-5")
	}
	if spec.TimeoutSeconds == 0 {
		spec.TimeoutSeconds = gptIQDefaultChatTO
		if spec.Kind == GPTIQKindGraded {
			spec.TimeoutSeconds = gptIQDefaultGradedTO
		}
	}
	if spec.TimeoutSeconds < gptIQMinTimeout || spec.TimeoutSeconds > gptIQMaxTimeout {
		return out, spec, errors.New("timeout_seconds must be 30-600")
	}
	if spec.Kind == GPTIQKindChat {
		spec.ScoringMode = GPTIQScoringNone
		spec.ScoringExpected = ""
	} else {
		switch spec.ScoringMode {
		case GPTIQScoringFinalLine, GPTIQScoringNumberAny, GPTIQScoringContains, GPTIQScoringRegex:
		default:
			return out, spec, errors.New("graded cases need scoring_mode final_line, number_any, contains or regex")
		}
		// final_line / number_any 比较的是数字或答案文本，去首尾空白；contains / regex 可能依赖空白，原样保存。
		if spec.ScoringMode == GPTIQScoringFinalLine || spec.ScoringMode == GPTIQScoringNumberAny {
			spec.ScoringExpected = strings.TrimSpace(spec.ScoringExpected)
		}
		if strings.TrimSpace(spec.ScoringExpected) == "" || utf8.RuneCountInString(spec.ScoringExpected) > gptIQExpectedMaxRunes {
			return out, spec, errors.New("scoring_expected must be 1-200 characters")
		}
		if spec.ScoringMode == GPTIQScoringRegex {
			if _, err := regexp.Compile(spec.ScoringExpected); err != nil {
				return out, spec, errors.New("scoring_expected is not a valid RE2 regular expression")
			}
		}
	}
	spec.PromptSHA256 = gptIQPromptHash(spec.Prompt)
	return out, spec, nil
}

func gptIQPromptHash(prompt string) string {
	sum := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(sum[:])
}

// gptIQAuditAllowedFields 是测试项请求体中可以原样进入审计的字段（与 GPTIQCaseInput 及 expected_version 对应）。
var gptIQAuditAllowedFields = []string{
	"name", "enabled", "sort_order", "public_prompt", "public_answer", "kind", "model", "reasoning_effort",
	"runs", "timeout_seconds", "scoring_mode", "scoring_expected", "expected_version",
}

const gptIQAuditOmitted = "<omitted>"

// GPTIQAuditPromptDigest 供审计中间件使用，按白名单重建请求体，从不把原始请求体交回：
//   - 键名按 encoding/json 的规则大小写不敏感匹配（"Prompt"、"PROMPT" 同样会被绑定成题面）；
//   - 题面键的字符串值替换为 "sha256:<hex>"（与版本表 prompt_sha256 同一规范化），非字符串值替换为省略标记；
//   - 白名单字段只保留标量值，嵌套值替换为省略标记；其他字段丢弃；
//   - 无法解析成 JSON 对象时只记录省略标记。
func GPTIQAuditPromptDigest(raw []byte) []byte {
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil || body == nil {
		return []byte(`{"_body":"` + gptIQAuditOmitted + `"}`)
	}
	out := map[string]any{}
	dropped := 0
	for key, value := range body {
		if strings.EqualFold(key, "prompt") {
			if prompt, ok := value.(string); ok {
				out["prompt"] = "sha256:" + gptIQPromptHash(strings.ReplaceAll(prompt, "\r\n", "\n"))
			} else {
				out["prompt"] = gptIQAuditOmitted
			}
			continue
		}
		field := ""
		for _, allowed := range gptIQAuditAllowedFields {
			if strings.EqualFold(key, allowed) {
				field = allowed
				break
			}
		}
		if field == "" {
			dropped++
			continue
		}
		switch value.(type) {
		case nil, bool, float64, string:
			out[field] = value
		default:
			out[field] = gptIQAuditOmitted
		}
	}
	if dropped > 0 {
		out["_dropped_fields"] = dropped
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return []byte(`{"_body":"` + gptIQAuditOmitted + `"}`)
	}
	return encoded
}

// ScoreGPTIQAnswer 按版本的判分规则给出结果；chat 测试项返回 nil（不评分）。
func ScoreGPTIQAnswer(spec GPTIQRevisionSpec, answer string) *bool {
	if spec.Kind != GPTIQKindGraded {
		return nil
	}
	var ok bool
	switch spec.ScoringMode {
	case GPTIQScoringFinalLine:
		ok = gptIQScoreFinalLine(answer, spec.ScoringExpected)
	case GPTIQScoringNumberAny:
		ok = gptIQScoreNumberAny(answer, spec.ScoringExpected)
	case GPTIQScoringContains:
		ok = strings.Contains(strings.ToLower(norm.NFKC.String(answer)), strings.ToLower(norm.NFKC.String(spec.ScoringExpected)))
	case GPTIQScoringRegex:
		re, err := regexp.Compile(spec.ScoringExpected)
		ok = err == nil && re.MatchString(answer)
	default:
		return nil
	}
	return &ok
}

// gptIQScoreFinalLine 只看回答的最后一个非空行（去掉 Markdown 强调、引用、标题符号）：该行必须整行是
// "最终答案＝X" 或 "Final answer: X"。期望值是整数时，X（去尾部句号）必须完整解析为该整数——"21.5"、
// "不是21，是29"、答案行之后又追加更正都判错；否则比较 X 与期望文本（大小写不敏感）。
func gptIQScoreFinalLine(answer, expected string) bool {
	lines := strings.Split(norm.NFKC.String(answer), "\n")
	exp := norm.NFKC.String(strings.TrimSpace(expected))
	last := ""
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(strings.Trim(strings.TrimSpace(lines[i]), gptIQFinalLineDecor)); line != "" {
			last = line
			break
		}
	}
	m := gptIQFinalLinePattern.FindStringSubmatch(last)
	if len(m) != 2 {
		return false
	}
	value := strings.TrimRight(strings.TrimSpace(strings.Trim(strings.TrimSpace(m[1]), gptIQFinalLineDecor)), "。.!！")
	if want, err := strconv.ParseInt(exp, 10, 64); err == nil {
		if !gptIQIntegerPattern.MatchString(value) {
			return false
		}
		n, err := strconv.ParseInt(value, 10, 64)
		return err == nil && n == want
	}
	return strings.EqualFold(value, exp)
}

// gptIQScoreNumberAny 与糖果脚本 `(?<!\d)21(?!\d)` 等价：期望文本原样出现，且前后都不是
// Unicode 十进制数字（Python str 正则的 \d 语义）。不做 NFKC，全角 "２１" 与脚本一样判为不匹配。
// Go RE2 不支持后行断言，因此手工检查边界。
func gptIQScoreNumberAny(answer, expected string) bool {
	if expected == "" {
		return false
	}
	for start := 0; start <= len(answer)-len(expected); {
		idx := strings.Index(answer[start:], expected)
		if idx < 0 {
			return false
		}
		pos := start + idx
		before, _ := utf8.DecodeLastRuneInString(answer[:pos])
		after, _ := utf8.DecodeRuneInString(answer[pos+len(expected):])
		if (pos == 0 || !unicode.IsDigit(before)) && (pos+len(expected) == len(answer) || !unicode.IsDigit(after)) {
			return true
		}
		_, size := utf8.DecodeRuneInString(answer[pos:])
		start = pos + size
	}
	return false
}

// GPTIQCaseTemplate 是管理页"从模板添加"的预填内容，保存后与普通测试项无区别。
type GPTIQCaseTemplate struct {
	Key   string         `json:"key"`
	Input GPTIQCaseInput `json:"input"`
}

// gptIQCandyPrompt 按原题玩法自行表述，答案 21：形状可凭手感挑选，取 9 颗圆形 + 12 颗星形即可保证。
// 若要与 haowang02/codex-candy-eval 脚本逐字对照，改用其原题并把判分切换为 number_any。
const gptIQCandyPrompt = `不使用任何外部工具回答以下问题：

一个黑色袋子里装有苹果味、桃子味、西瓜味三种口味的糖果，每种糖果有圆形和五角星形两种形状，不同的形状靠手感可以分辨。各口味、各形状的数量如下表。参赛者需要在活动开始前决定摸出的糖果数目。那么，最少取出多少个糖果，才能保证手中同时拥有不同形状的苹果味和桃子味的糖？（手中同时有圆形苹果味与五角星桃子味，或者同时有圆形桃子味与五角星苹果味，都满足要求）

        苹果味  桃子味  西瓜味
圆形       7      9      8
五角星形   7      6      4

请在回答的最后单独一行写：最终答案＝<整数>`

// GPTIQCaseTemplates 返回内置模板。
func GPTIQCaseTemplates() []GPTIQCaseTemplate {
	return []GPTIQCaseTemplate{
		{Key: "hi", Input: GPTIQCaseInput{Name: "打招呼", Enabled: true, SortOrder: 10, PublicPrompt: true, PublicAnswer: true, Kind: GPTIQKindChat, Prompt: "hi", Model: openai.DefaultTestModel, Runs: 1, TimeoutSeconds: gptIQDefaultChatTO, ScoringMode: GPTIQScoringNone}},
		{Key: "candy", Input: GPTIQCaseInput{Name: "糖果题", Enabled: true, SortOrder: 20, PublicPrompt: true, PublicAnswer: true, Kind: GPTIQKindGraded, Prompt: gptIQCandyPrompt, Model: "gpt-6.1-sol", ReasoningEffort: "high", Runs: 3, TimeoutSeconds: gptIQDefaultGradedTO, ScoringMode: GPTIQScoringFinalLine, ScoringExpected: "21"}},
	}
}
