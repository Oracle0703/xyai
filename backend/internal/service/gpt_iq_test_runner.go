package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/google/uuid"
)

// 降智测试执行器：对单个 OpenAI OAuth 账号直接请求 ChatGPT Codex 上游，返回结构化结果。
// 与 /admin/accounts 测试连接使用相同的身份头、代理、TLS 指纹与插件链（只调用其辅助函数，
// 不修改上游文件 account_test_service.go），但没有任何账号副作用：不写 accounts.extra、
// 不 SetError、不改 429 调度状态。设计见 docs/features/gpt-iq-test-design-cn.md §5–§6。

const (
	gptIQMaxStreamBytes   = 8 << 20
	gptIQMaxErrorBodyRead = 4 << 10
	gptIQErrorSummaryMax  = 500
)

// 执行失败类别（reason）。除新增的三项外沿用额度展示的类别。
const (
	GPTIQReasonStreamIncomplete = "stream_incomplete"
	GPTIQReasonEmptyAnswer      = "empty_answer"
	GPTIQReasonResponseFailed   = "response_failed"
)

var gptIQSummaryRedactions = []*regexp.Regexp{
	regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._~+/=-]+`),
	regexp.MustCompile(`(?i)\bsk-[A-Za-z0-9_-]+`),
	regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`),
	regexp.MustCompile(`[A-Za-z0-9_-]{32,}`),
}

// GPTIQProbeRequest 是一次上游调用的参数。
type GPTIQProbeRequest struct {
	Model           string
	ReasoningEffort string
	Prompt          string
	Timeout         time.Duration
}

// GPTIQProbeResult 是一次上游调用的结构化结果。Reason 为空表示成功拿到非空回答。
type GPTIQProbeResult struct {
	Answer          string
	ModelRequested  string
	ModelResponded  string
	InputTokens     *int
	CachedTokens    *int
	OutputTokens    *int
	ReasoningTokens *int
	TTFT            time.Duration
	Duration        time.Duration
	HTTPStatus      int
	Reason          string
	ErrorSummary    string
	RetryAfter      time.Duration
	// Transient 表示超时、5xx、流中断等可重试一次的失败。
	Transient bool
	// Dispatched 为 false 表示请求未发出（预检失败），不占用每日派发名额。
	Dispatched bool

	attempts int
}

// GPTIQProber 抽象上游调用，便于测试替换。
type GPTIQProber interface {
	Probe(ctx context.Context, account *Account, req GPTIQProbeRequest) GPTIQProbeResult
}

type gptIQAccessTokenSource interface {
	GetAccessToken(ctx context.Context, account *Account) (string, error)
}

// gptIQUpstreamProber 是生产实现。access token 经 OpenAITokenProvider 获取：临近过期且有
// refresh token 时会正常刷新并持久化新凭证（与额度展示采集相同，属于允许的凭证轮换）；
// 过期且无 refresh token 时在调用 provider 之前拦截，避免 provider 禁用账号。
type gptIQUpstreamProber struct {
	tests  *AccountTestService
	tokens gptIQAccessTokenSource
	now    func() time.Time
}

func newGPTIQUpstreamProber(tests *AccountTestService, tokens *OpenAITokenProvider) GPTIQProber {
	p := &gptIQUpstreamProber{tests: tests, now: time.Now}
	if tokens != nil {
		p.tokens = tokens
	}
	return p
}

// gptIQTokenUsable 是 token 预检：access token 已过期（含刷新提前量）且没有 refresh token 时不可用。
func gptIQTokenUsable(account *Account, now time.Time) bool {
	if account == nil || account.IsOpenAIPersonalAccessToken() {
		return account != nil
	}
	expiresAt := account.GetOpenAITokenExpiresAt()
	return expiresAt == nil || !now.Add(openAITokenRefreshSkew).After(*expiresAt) || strings.TrimSpace(account.GetOpenAIRefreshToken()) != ""
}

func (p *gptIQUpstreamProber) Probe(ctx context.Context, account *Account, req GPTIQProbeRequest) GPTIQProbeResult {
	result := GPTIQProbeResult{ModelRequested: req.Model}
	if p == nil || p.tests == nil || p.tokens == nil || account == nil {
		result.Reason = GPTQuotaStatusInternalError
		return result
	}
	if !account.IsOpenAIOAuth() || account.IsShadow() || account.IsOpenAIAgentIdentity() {
		result.Reason = GPTQuotaStatusNotSupported
		return result
	}
	if !gptIQTokenUsable(account, p.now()) {
		result.Reason = GPTQuotaStatusTokenUnavailable
		return result
	}
	token, err := p.tokens.GetAccessToken(ctx, account)
	if err != nil || strings.TrimSpace(token) == "" {
		if category := openAIQuotaContextCategory(ctx); category != "" {
			result.Reason = category
		} else {
			result.Reason = GPTQuotaStatusTokenUnavailable
		}
		return result
	}

	upstreamModel := normalizeOpenAIModelForUpstream(account, account.GetMappedModel(req.Model))
	result.ModelRequested = upstreamModel
	httpReq, err := buildGPTIQProbeRequest(ctx, account, token, upstreamModel, req)
	if err != nil {
		result.Reason = GPTQuotaStatusInternalError
		return result
	}

	timeout := req.Timeout
	if timeout <= 0 {
		timeout = gptIQDefaultChatTO * time.Second
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	httpReq = httpReq.WithContext(WithHTTPUpstreamProfile(callCtx, HTTPUpstreamProfileOpenAI))

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	started := p.now()
	result.Dispatched = true
	resp, err := p.tests.doOpenAIAccountTestUpstream(httpReq, proxyURL, account, true)
	if err != nil {
		result.Duration = p.now().Sub(started)
		p.classifyTransportError(ctx, callCtx, err, &result)
		return result
	}
	defer func() { _ = resp.Body.Close() }()
	result.HTTPStatus = resp.StatusCode
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, gptIQMaxErrorBodyRead))
		result.Duration = p.now().Sub(started)
		result.ErrorSummary = gptIQSanitizeSummary(string(body))
		classifyGPTIQHTTPStatus(resp, p.now(), &result)
		return result
	}
	parseGPTIQStream(io.LimitReader(resp.Body, gptIQMaxStreamBytes), started, p.now, &result)
	if result.Reason == GPTIQReasonStreamIncomplete {
		if category := openAIQuotaContextCategory(ctx); category != "" {
			result.Reason, result.Transient = category, false
		} else if callCtx.Err() != nil {
			result.Reason, result.Transient = GPTQuotaStatusTimeout, true
		}
	}
	return result
}

// buildGPTIQProbeRequest 与测试连接的 OAuth 分支保持相同的 URL、Host 与身份头；载荷改为
// 按模型选择官方 Codex base instructions，并带上题面与思考强度。
func buildGPTIQProbeRequest(ctx context.Context, account *Account, token, model string, req GPTIQProbeRequest) (*http.Request, error) {
	payload := map[string]any{
		"model":        model,
		"instructions": openai.CodexBaseInstructionsForModel(model),
		"input": []map[string]any{{
			"role":    "user",
			"content": []map[string]any{{"type": "input_text", "text": req.Prompt}},
		}},
		"stream":           true,
		"store":            false,
		"prompt_cache_key": uuid.NewString(),
	}
	if effort := strings.TrimSpace(req.ReasoningEffort); effort != "" {
		payload["reasoning"] = map[string]any{"effort": effort}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, chatgptCodexAPIURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Host = "chatgpt.com"
	httpReq.Header.Set("accept", "text/event-stream")
	httpReq.Header.Set("OpenAI-Beta", "responses=experimental")
	canonical := resolveCodexOutboundIdentity("")
	httpReq.Header.Set("Originator", canonical.originator)
	if customUA := strings.TrimSpace(account.GetOpenAIUserAgent()); customUA != "" {
		httpReq.Header.Set("User-Agent", customUA)
	} else {
		httpReq.Header.Set("User-Agent", canonical.userAgent)
	}
	setOpenAIChatGPTAccountHeaders(httpReq.Header, account)
	enforceCodexIdentityHeadersWithUA(httpReq.Header, account.GetOpenAIUserAgent())
	account.ApplyHeaderOverrides(withoutHeaderOverrideRequestScope(httpReq.Context()), httpReq.Header)
	return httpReq, nil
}

func (p *gptIQUpstreamProber) classifyTransportError(parent, callCtx context.Context, err error, result *GPTIQProbeResult) {
	switch {
	case openAIQuotaContextCategory(parent) != "":
		result.Reason = openAIQuotaContextCategory(parent)
	case callCtx.Err() != nil:
		result.Reason, result.Transient = GPTQuotaStatusTimeout, true
	default:
		result.Reason, result.Transient = GPTQuotaStatusRequestFailed, true
		result.ErrorSummary = gptIQSanitizeSummary(err.Error())
	}
}

func classifyGPTIQHTTPStatus(resp *http.Response, now time.Time, result *GPTIQProbeResult) {
	switch status := resp.StatusCode; {
	case status == http.StatusUnauthorized:
		result.Reason = GPTQuotaStatusUnauthorized
	case status == http.StatusForbidden:
		result.Reason = GPTQuotaStatusForbidden
	case status == http.StatusTooManyRequests:
		result.Reason = GPTQuotaStatusRateLimited
		if retryAt := parseRetryAfterResetTime(resp.Header, now); retryAt != nil && retryAt.After(now) {
			result.RetryAfter = retryAt.Sub(now)
		}
	case status >= 500:
		result.Reason, result.Transient = GPTQuotaStatusUpstreamError, true
	default:
		result.Reason = GPTQuotaStatusUpstreamError
	}
}

type gptIQStreamUsage struct {
	InputTokens        *int `json:"input_tokens"`
	OutputTokens       *int `json:"output_tokens"`
	InputTokensDetails *struct {
		CachedTokens *int `json:"cached_tokens"`
	} `json:"input_tokens_details"`
	OutputTokensDetails *struct {
		ReasoningTokens *int `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
}

type gptIQStreamEvent struct {
	Type     string `json:"type"`
	Delta    string `json:"delta"`
	Message  string `json:"message"`
	Response *struct {
		Model  string            `json:"model"`
		Usage  *gptIQStreamUsage `json:"usage"`
		Output []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	} `json:"response"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// parseGPTIQStream 解析 Responses SSE：累积 output_text 增量（首个增量记首字耗时），
// response.completed 取 usage 与实际模型；没有增量时回退到 completed 中最后一条 message 的文本。
func parseGPTIQStream(body io.Reader, started time.Time, now func() time.Time, result *GPTIQProbeResult) {
	reader := bufio.NewReaderSize(body, 64<<10)
	var answer strings.Builder
	sawDelta := false
	for {
		line, err := reader.ReadString('\n')
		if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
			if data != "" && data != "[DONE]" {
				var event gptIQStreamEvent
				if json.Unmarshal([]byte(data), &event) == nil {
					switch event.Type {
					case "response.output_text.delta":
						if event.Delta != "" {
							if !sawDelta {
								sawDelta = true
								result.TTFT = now().Sub(started)
							}
							_, _ = answer.WriteString(event.Delta)
						}
					case "response.completed", "response.done":
						result.Duration = now().Sub(started)
						applyGPTIQCompleted(&event, &answer, sawDelta, result)
						return
					case "response.failed", "error":
						result.Duration = now().Sub(started)
						result.Reason = GPTIQReasonResponseFailed
						msg := event.Message
						if event.Error != nil && event.Error.Message != "" {
							msg = event.Error.Message
						} else if event.Response != nil && event.Response.Error != nil {
							msg = event.Response.Error.Message
						}
						result.ErrorSummary = gptIQSanitizeSummary(msg)
						result.Answer = answer.String()
						return
					}
				}
			}
		}
		if err != nil {
			result.Duration = now().Sub(started)
			result.Answer = answer.String()
			result.Reason, result.Transient = GPTIQReasonStreamIncomplete, true
			if !errors.Is(err, io.EOF) {
				result.ErrorSummary = gptIQSanitizeSummary(err.Error())
			}
			return
		}
	}
}

func applyGPTIQCompleted(event *gptIQStreamEvent, answer *strings.Builder, sawDelta bool, result *GPTIQProbeResult) {
	text := answer.String()
	if event.Response != nil {
		result.ModelResponded = strings.TrimSpace(event.Response.Model)
		if usage := event.Response.Usage; usage != nil {
			result.InputTokens = usage.InputTokens
			result.OutputTokens = usage.OutputTokens
			if usage.InputTokensDetails != nil {
				result.CachedTokens = usage.InputTokensDetails.CachedTokens
			}
			if usage.OutputTokensDetails != nil {
				result.ReasoningTokens = usage.OutputTokensDetails.ReasoningTokens
			}
		}
		if !sawDelta {
			for i := len(event.Response.Output) - 1; i >= 0; i-- {
				item := event.Response.Output[i]
				if item.Type != "message" {
					continue
				}
				var b strings.Builder
				for _, part := range item.Content {
					if part.Type == "output_text" {
						_, _ = b.WriteString(part.Text)
					}
				}
				if b.Len() > 0 {
					text = b.String()
					break
				}
			}
		}
	}
	result.Answer = text
	if strings.TrimSpace(text) == "" {
		result.Reason = GPTIQReasonEmptyAnswer
	}
}

// gptIQSanitizeSummary 去掉令牌、密钥、邮箱与长随机串后截断，只给管理页看。
func gptIQSanitizeSummary(raw string) string {
	s := strings.TrimSpace(strings.ReplaceAll(raw, "\x00", ""))
	for _, pattern := range gptIQSummaryRedactions {
		s = pattern.ReplaceAllString(s, "[redacted]")
	}
	s = strings.ToValidUTF8(s, "")
	if utf8.RuneCountInString(s) > gptIQErrorSummaryMax {
		runes := []rune(s)
		s = string(runes[:gptIQErrorSummaryMax])
	}
	return s
}
