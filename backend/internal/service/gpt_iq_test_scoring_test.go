//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func gptIQGradedSpec(mode, expected string) GPTIQRevisionSpec {
	return GPTIQRevisionSpec{Kind: GPTIQKindGraded, ScoringMode: mode, ScoringExpected: expected}
}

func TestScoreGPTIQAnswerFinalLine(t *testing.T) {
	spec := gptIQGradedSpec(GPTIQScoringFinalLine, "21")
	tests := []struct {
		name   string
		answer string
		want   bool
	}{
		{"plain", "思路……\n最终答案＝21", true},
		{"full width digits and colon", "最终答案：２１", true},
		{"markdown bold", "推理\n**最终答案＝21**", true},
		{"english", "Reasoning...\nFinal answer: 21.", true},
		{"verification numbers after answer do not matter", "答案 21，验算 9 + 12\n最终答案＝21", true},
		{"wrong final line", "一开始以为 21\n最终答案＝29", false},
		{"last final line wins", "最终答案＝29\n更正：\n最终答案＝21", true},
		{"no final line is wrong", "答案是 21", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ScoreGPTIQAnswer(spec, tt.answer)
			require.NotNil(t, got)
			require.Equal(t, tt.want, *got)
		})
	}
	text := ScoreGPTIQAnswer(gptIQGradedSpec(GPTIQScoringFinalLine, "苹果"), "最终答案＝苹果。")
	require.True(t, *text)
}

// number_any 与脚本 `(?<!\d)21(?!\d)` 的 Python 语义保持一致（不做 NFKC）。
func TestScoreGPTIQAnswerNumberAnyMatchesScript(t *testing.T) {
	spec := gptIQGradedSpec(GPTIQScoringNumberAny, "21")
	tests := map[string]bool{
		"21":           true,
		"答案是21颗":       true,
		"不是21，而是29":    true,
		"121":          false,
		"212":          false,
		"21.5":         true,
		"２１":           false,
		"共２21":         false,
		"21２":          false,
		"29 and 21":    true,
		"no number":    false,
		"221 then 21x": true,
	}
	for answer, want := range tests {
		got := ScoreGPTIQAnswer(spec, answer)
		require.NotNil(t, got, answer)
		require.Equal(t, want, *got, answer)
	}
}

func TestScoreGPTIQAnswerOtherModes(t *testing.T) {
	require.Nil(t, ScoreGPTIQAnswer(GPTIQRevisionSpec{Kind: GPTIQKindChat}, "hi"))
	require.True(t, *ScoreGPTIQAnswer(gptIQGradedSpec(GPTIQScoringContains, "Hello"), "well, ＨＥＬＬＯ there"))
	require.False(t, *ScoreGPTIQAnswer(gptIQGradedSpec(GPTIQScoringContains, "bye"), "hello"))
	require.True(t, *ScoreGPTIQAnswer(gptIQGradedSpec(GPTIQScoringRegex, `^\s*\d+\s*$`), " 42 "))
	require.False(t, *ScoreGPTIQAnswer(gptIQGradedSpec(GPTIQScoringRegex, `^\d+$`), "42 apples"))
}

func gptIQValidInput() GPTIQCaseInput {
	return GPTIQCaseInput{Name: "糖果题", Enabled: true, Kind: GPTIQKindGraded, Prompt: "题面\r\n第二行", Model: "gpt-6.1-sol", ReasoningEffort: "High", Runs: 3, ScoringMode: GPTIQScoringFinalLine, ScoringExpected: " 21 "}
}

func TestNormalizeGPTIQCaseInput(t *testing.T) {
	in, spec, err := NormalizeGPTIQCaseInput(gptIQValidInput())
	require.NoError(t, err)
	require.Equal(t, "糖果题", in.Name)
	require.Equal(t, "题面\n第二行", spec.Prompt)
	require.Equal(t, "high", spec.ReasoningEffort)
	require.Equal(t, gptIQDefaultGradedTO, spec.TimeoutSeconds)
	require.Equal(t, "21", spec.ScoringExpected)
	require.Len(t, spec.PromptSHA256, 64)

	chat := gptIQValidInput()
	chat.Kind = GPTIQKindChat
	_, chatSpec, err := NormalizeGPTIQCaseInput(chat)
	require.NoError(t, err)
	require.Equal(t, GPTIQScoringNone, chatSpec.ScoringMode)
	require.Empty(t, chatSpec.ScoringExpected)
	require.Equal(t, gptIQDefaultChatTO, chatSpec.TimeoutSeconds)

	bad := map[string]func(*GPTIQCaseInput){
		"empty name":      func(in *GPTIQCaseInput) { in.Name = "  " },
		"long name":       func(in *GPTIQCaseInput) { in.Name = strings.Repeat("名", 41) },
		"kind":            func(in *GPTIQCaseInput) { in.Kind = "quiz" },
		"empty prompt":    func(in *GPTIQCaseInput) { in.Prompt = " \n " },
		"api key prompt":  func(in *GPTIQCaseInput) { in.Prompt = "use sk-abcdefghijklmnop" },
		"bearer prompt":   func(in *GPTIQCaseInput) { in.Prompt = "Authorization: Bearer abcdefghijklmnop" },
		"email prompt":    func(in *GPTIQCaseInput) { in.Prompt = "mail me@example.com" },
		"model chars":     func(in *GPTIQCaseInput) { in.Model = "gpt 6" },
		"effort":          func(in *GPTIQCaseInput) { in.ReasoningEffort = "turbo" },
		"runs":            func(in *GPTIQCaseInput) { in.Runs = 6 },
		"timeout":         func(in *GPTIQCaseInput) { in.TimeoutSeconds = 10 },
		"graded no mode":  func(in *GPTIQCaseInput) { in.ScoringMode = GPTIQScoringNone },
		"graded no value": func(in *GPTIQCaseInput) { in.ScoringExpected = "" },
		"bad regex": func(in *GPTIQCaseInput) {
			in.ScoringMode = GPTIQScoringRegex
			in.ScoringExpected = `(?<!\d)21`
		},
	}
	for name, mutate := range bad {
		in := gptIQValidInput()
		mutate(&in)
		_, _, err := NormalizeGPTIQCaseInput(in)
		require.Error(t, err, name)
	}
}

func TestNormalizeGPTIQKeepsWhitespaceForRegexAndContains(t *testing.T) {
	in := gptIQValidInput()
	in.ScoringMode, in.ScoringExpected = GPTIQScoringRegex, " 21$"
	_, spec, err := NormalizeGPTIQCaseInput(in)
	require.NoError(t, err)
	require.Equal(t, " 21$", spec.ScoringExpected)
	require.False(t, *ScoreGPTIQAnswer(spec, "121"))
	require.True(t, *ScoreGPTIQAnswer(spec, "答案 21"))
	in.ScoringMode, in.ScoringExpected = GPTIQScoringNumberAny, " 21 "
	_, spec, err = NormalizeGPTIQCaseInput(in)
	require.NoError(t, err)
	require.Equal(t, "21", spec.ScoringExpected)
	in.ScoringMode, in.ScoringExpected = GPTIQScoringContains, "   "
	_, _, err = NormalizeGPTIQCaseInput(in)
	require.Error(t, err)
}

func TestGPTIQTemplatesAreValid(t *testing.T) {
	templates := GPTIQCaseTemplates()
	require.Len(t, templates, 2)
	for _, tpl := range templates {
		_, _, err := NormalizeGPTIQCaseInput(tpl.Input)
		require.NoError(t, err, tpl.Key)
	}
	candy := templates[1].Input
	require.Equal(t, "21", candy.ScoringExpected)
	require.Equal(t, GPTIQScoringFinalLine, candy.ScoringMode)
	require.Contains(t, candy.Prompt, "最终答案＝<整数>")
	require.True(t, *ScoreGPTIQAnswer(GPTIQRevisionSpec{Kind: candy.Kind, ScoringMode: candy.ScoringMode, ScoringExpected: candy.ScoringExpected}, "9 颗圆形 + 12 颗星形\n最终答案＝21"))
}

// ---- 流解析 ----

func gptIQSSE(events ...string) io.Reader {
	var b strings.Builder
	for _, e := range events {
		b.WriteString("data: ")
		b.WriteString(e)
		b.WriteString("\n\n")
	}
	return strings.NewReader(b.String())
}

func gptIQFakeClock(start time.Time, step time.Duration) func() time.Time {
	current := start
	return func() time.Time {
		current = current.Add(step)
		return current
	}
}

func TestParseGPTIQStreamCollectsAnswerAndUsage(t *testing.T) {
	start := time.Date(2026, 11, 23, 8, 30, 0, 0, time.UTC)
	var res GPTIQProbeResult
	parseGPTIQStream(gptIQSSE(
		`{"type":"response.created"}`,
		`{"type":"response.output_text.delta","delta":"最终答案"}`,
		`{"type":"response.output_text.delta","delta":"＝21"}`,
		`{"type":"response.completed","response":{"model":"gpt-6.1-sol","usage":{"input_tokens":120,"input_tokens_details":{"cached_tokens":10},"output_tokens":800,"output_tokens_details":{"reasoning_tokens":700}}}}`,
	), start, gptIQFakeClock(start, time.Second), &res)
	require.Empty(t, res.Reason)
	require.Equal(t, "最终答案＝21", res.Answer)
	require.Equal(t, "gpt-6.1-sol", res.ModelResponded)
	require.Equal(t, 120, *res.InputTokens)
	require.Equal(t, 10, *res.CachedTokens)
	require.Equal(t, 800, *res.OutputTokens)
	require.Equal(t, 700, *res.ReasoningTokens)
	require.Equal(t, time.Second, res.TTFT)
	require.Equal(t, 2*time.Second, res.Duration)
}

func TestParseGPTIQStreamFallbacksAndFailures(t *testing.T) {
	start := time.Now()
	clock := gptIQFakeClock(start, time.Millisecond)

	var fallback GPTIQProbeResult
	parseGPTIQStream(gptIQSSE(`{"type":"response.completed","response":{"output":[{"type":"reasoning"},{"type":"message","content":[{"type":"output_text","text":"hello"}]}]}}`), start, clock, &fallback)
	require.Empty(t, fallback.Reason)
	require.Equal(t, "hello", fallback.Answer)
	require.Nil(t, fallback.ReasoningTokens, "missing usage stays nil, not 0")

	var empty GPTIQProbeResult
	parseGPTIQStream(gptIQSSE(`{"type":"response.completed","response":{}}`), start, clock, &empty)
	require.Equal(t, GPTIQReasonEmptyAnswer, empty.Reason)

	var failed GPTIQProbeResult
	parseGPTIQStream(gptIQSSE(`{"type":"response.failed","response":{"error":{"message":"bad token sk-abcdefghijklmnopqrst"}}}`), start, clock, &failed)
	require.Equal(t, GPTIQReasonResponseFailed, failed.Reason)
	require.NotContains(t, failed.ErrorSummary, "sk-abcdef")
	require.False(t, failed.Transient)

	var cut GPTIQProbeResult
	parseGPTIQStream(gptIQSSE(`{"type":"response.output_text.delta","delta":"part"}`), start, clock, &cut)
	require.Equal(t, GPTIQReasonStreamIncomplete, cut.Reason)
	require.True(t, cut.Transient)
	require.Equal(t, "part", cut.Answer)
}

// ---- 上游请求 ----

type gptIQStaticToken struct {
	token string
	calls int
}

func (s *gptIQStaticToken) GetAccessToken(context.Context, *Account) (string, error) {
	s.calls++
	return s.token, nil
}

func gptIQOAuthAccount() *Account {
	return &Account{
		ID: 89, Name: "c-01", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Concurrency: 1,
		Credentials: map[string]any{"access_token": "test-token", "chatgpt_account_id": "acct-1"},
	}
}

func gptIQCompletedResponse() *http.Response {
	resp := newJSONResponse(http.StatusOK, "")
	resp.Body = io.NopCloser(gptIQSSE(
		`{"type":"response.output_text.delta","delta":"hi"}`,
		`{"type":"response.completed","response":{"usage":{"input_tokens":5,"output_tokens":2}}}`,
	))
	resp.Header.Set("x-codex-primary-used-percent", "88")
	resp.Header.Set("x-codex-primary-window-minutes", "10080")
	return resp
}

// 执行器与测试连接走同一出站路径：URL、Host、身份头一致；载荷换成题面并带思考强度。
func TestGPTIQProbeMatchesAccountTestHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &queuedHTTPUpstream{responses: []*http.Response{gptIQCompletedResponse(), gptIQCompletedResponse()}}
	repo := &openAIAccountTestRepo{}
	tests := &AccountTestService{accountRepo: repo, httpUpstream: upstream}

	c, _ := newTestContext()
	require.NoError(t, tests.testOpenAIAccountConnection(c, gptIQOAuthAccount(), "gpt-5.4", "", ""))
	repo.updatedExtra = nil

	tokens := &gptIQStaticToken{token: "test-token"}
	prober := &gptIQUpstreamProber{tests: tests, tokens: tokens, now: time.Now}
	res := prober.Probe(context.Background(), gptIQOAuthAccount(), GPTIQProbeRequest{Model: "gpt-5.4", ReasoningEffort: "high", Prompt: "最终答案？", Timeout: time.Minute})
	require.Empty(t, res.Reason)
	require.True(t, res.Dispatched)
	require.Equal(t, "hi", res.Answer)
	require.Len(t, upstream.requests, 2)
	require.True(t, upstream.tlsFlags[1] == upstream.tlsFlags[0])

	ref, got := upstream.requests[0], upstream.requests[1]
	require.Equal(t, ref.URL.String(), got.URL.String())
	require.Equal(t, ref.Host, got.Host)
	for _, key := range []string{"Authorization", "Accept", "Openai-Beta", "Originator", "User-Agent", "Chatgpt-Account-Id", "Version", "Session_id"} {
		if ref.Header.Get(key) != "" {
			require.Equal(t, ref.Header.Get(key), got.Header.Get(key), key)
		}
	}
	require.Equal(t, HTTPUpstreamProfileOpenAI, HTTPUpstreamProfileFromContext(got.Context()))

	body, err := io.ReadAll(got.Body)
	require.NoError(t, err)
	require.Equal(t, "最终答案？", gjson.GetBytes(body, "input.0.content.0.text").String())
	require.Equal(t, "high", gjson.GetBytes(body, "reasoning.effort").String())
	require.False(t, gjson.GetBytes(body, "store").Bool())
	require.True(t, gjson.GetBytes(body, "stream").Bool())
	require.NotEmpty(t, gjson.GetBytes(body, "instructions").String())
	require.NotEmpty(t, gjson.GetBytes(body, "prompt_cache_key").String())

	// 测试连接会把 x-codex-* 写进 accounts.extra；执行器不写。
	require.Nil(t, repo.updatedExtra)
}

func TestGPTIQProbeHasNoAccountSideEffects(t *testing.T) {
	cases := []struct {
		name   string
		status int
		header map[string]string
		reason string
	}{
		{"unauthorized", http.StatusUnauthorized, nil, GPTQuotaStatusUnauthorized},
		{"rate limited", http.StatusTooManyRequests, map[string]string{"Retry-After": "120"}, GPTQuotaStatusRateLimited},
		{"server error", http.StatusBadGateway, nil, GPTQuotaStatusUpstreamError},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			resp := newJSONResponse(tt.status, `{"error":{"message":"token sk-abcdefghijklmnopqrstu for me@example.com"}}`)
			for k, v := range tt.header {
				resp.Header.Set(k, v)
			}
			upstream := &queuedHTTPUpstream{responses: []*http.Response{resp}}
			repo := &openAIAccountTestRepo{}
			prober := &gptIQUpstreamProber{tests: &AccountTestService{accountRepo: repo, httpUpstream: upstream}, tokens: &gptIQStaticToken{token: "t"}, now: time.Now}
			res := prober.Probe(context.Background(), gptIQOAuthAccount(), GPTIQProbeRequest{Model: "gpt-5.4", Prompt: "hi", Timeout: time.Minute})
			require.Equal(t, tt.reason, res.Reason)
			require.Equal(t, tt.status, res.HTTPStatus)
			require.NotContains(t, res.ErrorSummary, "sk-abc")
			require.NotContains(t, res.ErrorSummary, "me@example.com")
			if tt.status == http.StatusTooManyRequests {
				require.InDelta(t, (120 * time.Second).Seconds(), res.RetryAfter.Seconds(), 2)
			}
			require.Equal(t, tt.status >= 500, res.Transient)
			require.Zero(t, repo.setErrorID)
			require.Zero(t, repo.rateLimitedID)
			require.Nil(t, repo.updatedExtra)
			require.Empty(t, repo.bulkUpdatedIDs)
		})
	}
}

func TestGPTIQProbeTokenPrecheckSkipsProvider(t *testing.T) {
	account := gptIQOAuthAccount()
	expired := time.Now().Add(-time.Hour).Format(time.RFC3339)
	account.Credentials["expires_at"] = expired
	tokens := &gptIQStaticToken{token: "t"}
	upstream := &queuedHTTPUpstream{}
	prober := &gptIQUpstreamProber{tests: &AccountTestService{httpUpstream: upstream}, tokens: tokens, now: time.Now}
	res := prober.Probe(context.Background(), account, GPTIQProbeRequest{Model: "gpt-5.4", Prompt: "hi"})
	require.Equal(t, GPTQuotaStatusTokenUnavailable, res.Reason)
	require.False(t, res.Dispatched)
	require.Zero(t, tokens.calls, "token provider would disable an account without refresh token")
	require.Empty(t, upstream.requests)

	account.Credentials["refresh_token"] = "rt"
	require.True(t, gptIQTokenUsable(account, time.Now()))

	shadow := gptIQOAuthAccount()
	parent := int64(1)
	shadow.ParentAccountID = &parent
	require.Equal(t, GPTQuotaStatusNotSupported, prober.Probe(context.Background(), shadow, GPTIQProbeRequest{}).Reason)
}
