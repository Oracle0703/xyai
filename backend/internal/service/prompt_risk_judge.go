package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// LLM 语义复核(judge):关键词命中且会真正 block 时,调一次 OpenAI 兼容 /v1/chat/completions
// 做语义精判,降低双用途词(vpn/proxy/代理…)的字面误杀。失败一律 fail-open(放行)。
// 本文件:进程内防递归标记 + 纯函数(payload/usermsg/parse,可单测)+ runPromptRiskJudge 客户端
// (照抄 callModerationOnceWithInput 骨架)。

// promptRiskJudgeContextKey 标记"judge 自身发起的回环请求",防同进程内直接递归。
type promptRiskJudgeContextKey struct{}

func withPromptRiskJudgeInFlight(ctx context.Context) context.Context {
	return context.WithValue(ctx, promptRiskJudgeContextKey{}, true)
}

func ctxHasPromptRiskJudgeInFlight(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(promptRiskJudgeContextKey{}).(bool)
	return v
}

// promptRiskJudgeResult 是一次 judge 调用的结果。Err != nil 表示需 fail-open(放行降级)。
type promptRiskJudgeResult struct {
	Risk      string // none / low / high(已归一)
	Reason    string
	LatencyMS int
	Skipped   bool // 因递归标记被跳过(未发请求)
	Err       error
}

// chat completions 响应的最小子集。
type promptRiskJudgeChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// promptRiskJudgeVerdict 是模型按约定输出的 JSON 内容。
type promptRiskJudgeVerdict struct {
	Risk   string `json:"risk"`
	Reason string `json:"reason"`
}

// buildPromptRiskJudgePayload 构造 chat completions 请求体(强制 JSON 输出、非流式、低温)。
func buildPromptRiskJudgePayload(model, systemPrompt, userMessage string) map[string]any {
	return map[string]any{
		"model": model,
		"messages": []map[string]any{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userMessage},
		},
		"stream":          false,
		"temperature":     0,
		"response_format": map[string]any{"type": "json_object"},
	}
}

// buildPromptRiskJudgeUserMessage 把命中的关键词证据 + 用户原文拼成 user 消息。
func buildPromptRiskJudgeUserMessage(text string, reasons []PromptRiskReason) string {
	var b strings.Builder
	if len(reasons) > 0 {
		parts := make([]string, 0, len(reasons))
		for _, r := range reasons {
			parts = append(parts, fmt.Sprintf("%s(%s,%s)", r.Keyword, r.Level, r.Source))
		}
		b.WriteString("被关键词规则命中的词：")
		b.WriteString(strings.Join(parts, "、"))
		b.WriteString("\n")
	}
	b.WriteString("用户原文：\n<<<\n")
	b.WriteString(text)
	b.WriteString("\n>>>\n请复核并只输出 JSON。")
	return b.String()
}

// parsePromptRiskJudgeContent 解析模型返回的 JSON 内容,容忍 ```json 代码块包裹与前后多余文本。
func parsePromptRiskJudgeContent(content string) (risk string, reason string, err error) {
	raw := extractPromptRiskJudgeJSON(content)
	if raw == "" {
		return "", "", fmt.Errorf("prompt risk judge: empty content")
	}
	var v promptRiskJudgeVerdict
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return "", "", fmt.Errorf("prompt risk judge: parse content: %w", err)
	}
	r := normalizePromptRiskJudgeRisk(v.Risk)
	if r == "" {
		return "", "", fmt.Errorf("prompt risk judge: invalid risk %q", v.Risk)
	}
	return r, strings.TrimSpace(v.Reason), nil
}

// extractPromptRiskJudgeJSON 从模型输出里截取首个 { 到末个 } 之间的 JSON 子串(去掉代码块/解释文字)。
func extractPromptRiskJudgeJSON(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	start := strings.IndexByte(content, '{')
	end := strings.LastIndexByte(content, '}')
	if start < 0 || end < 0 || end < start {
		return ""
	}
	return content[start : end+1]
}

func normalizePromptRiskJudgeRisk(risk string) string {
	switch strings.ToLower(strings.TrimSpace(risk)) {
	case PromptRiskJudgeRiskNone:
		return PromptRiskJudgeRiskNone
	case PromptRiskJudgeRiskLow:
		return PromptRiskJudgeRiskLow
	case PromptRiskJudgeRiskHigh:
		return PromptRiskJudgeRiskHigh
	default:
		return ""
	}
}

// runPromptRiskJudge 调用 judge 模型做语义复核。任何错误都返回 {Err},绝不 panic、不中断上层 Check。
func (s *ContentModerationService) runPromptRiskJudge(ctx context.Context, cfg *PromptRiskConfig, text string, reasons []PromptRiskReason) *promptRiskJudgeResult {
	if cfg == nil {
		return &promptRiskJudgeResult{Err: fmt.Errorf("prompt risk judge: nil config")}
	}
	j := cfg.Judge
	start := time.Now()
	latency := func() int { return int(time.Since(start).Milliseconds()) }

	endpoint, err := url.JoinPath(strings.TrimRight(j.BaseURL, "/"), "/v1/chat/completions")
	if err != nil {
		return &promptRiskJudgeResult{LatencyMS: latency(), Err: fmt.Errorf("prompt risk judge: join url: %w", err)}
	}
	systemPrompt := strings.TrimSpace(j.PromptTemplate)
	if systemPrompt == "" {
		systemPrompt = defaultPromptRiskJudgePrompt
	}
	userMessage := buildPromptRiskJudgeUserMessage(text, reasons)
	payload := buildPromptRiskJudgePayload(j.Model, systemPrompt, userMessage)
	raw, err := json.Marshal(payload)
	if err != nil {
		return &promptRiskJudgeResult{LatencyMS: latency(), Err: fmt.Errorf("prompt risk judge: marshal: %w", err)}
	}

	timeout := time.Duration(j.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = time.Duration(defaultPromptRiskJudgeTimeoutMS) * time.Millisecond
	}
	// 注入防递归标记:judge 的回环请求若同进程再入 Check 会被 evaluatePromptRiskStage 短路。
	reqCtx, cancel := context.WithTimeout(withPromptRiskJudgeInFlight(ctx), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return &promptRiskJudgeResult{LatencyMS: latency(), Err: fmt.Errorf("prompt risk judge: new request: %w", err)}
	}
	req.Header.Set("Authorization", "Bearer "+j.APIKey)
	req.Header.Set("Content-Type", "application/json")

	client := s.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return &promptRiskJudgeResult{LatencyMS: latency(), Err: fmt.Errorf("prompt risk judge: do request: %w", err)}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return &promptRiskJudgeResult{LatencyMS: latency(), Err: fmt.Errorf("prompt risk judge: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))}
	}
	var out promptRiskJudgeChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return &promptRiskJudgeResult{LatencyMS: latency(), Err: fmt.Errorf("prompt risk judge: decode: %w", err)}
	}
	if len(out.Choices) == 0 {
		return &promptRiskJudgeResult{LatencyMS: latency(), Err: fmt.Errorf("prompt risk judge: empty choices")}
	}
	risk, reason, err := parsePromptRiskJudgeContent(out.Choices[0].Message.Content)
	if err != nil {
		return &promptRiskJudgeResult{LatencyMS: latency(), Err: err}
	}
	return &promptRiskJudgeResult{Risk: risk, Reason: reason, LatencyMS: latency()}
}
