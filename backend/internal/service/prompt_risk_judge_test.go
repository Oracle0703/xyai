package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ---- 纯函数 ----

func TestBuildPromptRiskJudgePayload(t *testing.T) {
	p := buildPromptRiskJudgePayload("m1", "sys", "usr")
	require.Equal(t, "m1", p["model"])
	require.Equal(t, false, p["stream"])
	require.Equal(t, map[string]any{"type": "json_object"}, p["response_format"])
	msgs, ok := p["messages"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, msgs, 2)
	require.Equal(t, "system", msgs[0]["role"])
	require.Equal(t, "sys", msgs[0]["content"])
	require.Equal(t, "user", msgs[1]["role"])
	require.Equal(t, "usr", msgs[1]["content"])
}

func TestBuildPromptRiskJudgeUserMessage(t *testing.T) {
	msg := buildPromptRiskJudgeUserMessage("set up vpn proxy", []PromptRiskReason{
		{Level: PromptRiskLevelMedium, Keyword: "vpn", Source: PromptRiskMatchWord},
		{Level: PromptRiskLevelMedium, Keyword: "proxy", Source: PromptRiskMatchWord},
	})
	require.Contains(t, msg, "vpn(medium,word)")
	require.Contains(t, msg, "proxy(medium,word)")
	require.Contains(t, msg, "set up vpn proxy")
}

func TestParsePromptRiskJudgeContent(t *testing.T) {
	cases := []struct {
		name    string
		content string
		risk    string
		wantErr bool
	}{
		{"plain", `{"risk":"none","reason":"自有vpn"}`, PromptRiskJudgeRiskNone, false},
		{"high", `{"risk":"HIGH","reason":"x"}`, PromptRiskJudgeRiskHigh, false},
		{"code block", "```json\n{\"risk\":\"low\",\"reason\":\"y\"}\n```", PromptRiskJudgeRiskLow, false},
		{"extra text", "判定如下：{\"risk\":\"high\"} 完毕", PromptRiskJudgeRiskHigh, false},
		{"invalid risk", `{"risk":"maybe"}`, "", true},
		{"missing risk", `{"reason":"z"}`, "", true},
		{"not json", `totally not json`, "", true},
		{"empty", ``, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			risk, _, err := parsePromptRiskJudgeContent(tc.content)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.risk, risk)
		})
	}
}

// ---- 配置 normalize / validateRaw ----

func TestPromptRiskJudge_Normalize(t *testing.T) {
	cfg := DefaultPromptRiskConfig()
	cfg.Judge = PromptRiskJudgeConfig{
		Enabled:       true,
		BaseURL:       "https://gw.example.com/",
		Model:         " m ",
		TimeoutMS:     50, // < min,应 clamp 到 500
		TriggerLevels: []string{"HIGH", "bogus", "high"},
	}
	cfg.normalize()
	require.Equal(t, "https://gw.example.com", cfg.Judge.BaseURL)
	require.Equal(t, "m", cfg.Judge.Model)
	require.Equal(t, minPromptRiskJudgeTimeoutMS, cfg.Judge.TimeoutMS)
	require.Equal(t, []string{PromptRiskLevelHigh}, cfg.Judge.TriggerLevels) // 去重 + 过滤非法
	require.True(t, cfg.Judge.FailOpen)

	empty := DefaultPromptRiskConfig()
	empty.Judge = PromptRiskJudgeConfig{Enabled: true}
	empty.normalize()
	require.Equal(t, defaultPromptRiskJudgeTimeoutMS, empty.Judge.TimeoutMS)
	require.Equal(t, []string{PromptRiskLevelHigh}, empty.Judge.TriggerLevels)
}

func TestPromptRiskJudge_ValidateRawRejects(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*PromptRiskJudgeConfig)
	}{
		{"missing base_url", func(j *PromptRiskJudgeConfig) { j.BaseURL = ""; j.Model = "m" }},
		{"bad base_url", func(j *PromptRiskJudgeConfig) { j.BaseURL = "://bad"; j.Model = "m" }},
		{"missing model", func(j *PromptRiskJudgeConfig) { j.BaseURL = "https://x"; j.Model = "" }},
		{"bad timeout", func(j *PromptRiskJudgeConfig) { j.BaseURL = "https://x"; j.Model = "m"; j.TimeoutMS = 99999 }},
		{"bad trigger level", func(j *PromptRiskJudgeConfig) {
			j.BaseURL = "https://x"
			j.Model = "m"
			j.TriggerLevels = []string{"sevre"}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultPromptRiskConfig()
			cfg.Judge.Enabled = true
			tc.mut(&cfg.Judge)
			require.Error(t, cfg.validateRaw())
		})
	}

	// 禁用时不校验。
	disabled := DefaultPromptRiskConfig()
	disabled.Judge = PromptRiskJudgeConfig{Enabled: false}
	require.NoError(t, disabled.validateRaw())
}

// ---- judge 客户端(mock httpClient via httptest)----

func judgeChatResponse(verdict string) string {
	body, _ := json.Marshal(map[string]any{
		"choices": []map[string]any{
			{"message": map[string]any{"content": verdict}},
		},
	})
	return string(body)
}

func newJudgeService(t *testing.T) *ContentModerationService {
	t.Helper()
	return NewContentModerationService(
		&contentModerationTestSettingRepo{values: map[string]string{}},
		nil, nil, nil, nil, nil, nil,
	)
}

func TestRunPromptRiskJudge_Success(t *testing.T) {
	var gotAuth, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(judgeChatResponse(`{"risk":"none","reason":"自有实验室vpn"}`)))
	}))
	defer server.Close()

	svc := newJudgeService(t)
	cfg := DefaultPromptRiskConfig()
	cfg.Judge = PromptRiskJudgeConfig{Enabled: true, BaseURL: server.URL, Model: "m", APIKey: "sk-judge", TimeoutMS: 4000}
	cfg.normalize()

	res := svc.runPromptRiskJudge(context.Background(), &cfg, "config our vpn", nil)
	require.NoError(t, res.Err)
	require.Equal(t, PromptRiskJudgeRiskNone, res.Risk)
	require.Equal(t, "自有实验室vpn", res.Reason)
	require.Equal(t, "Bearer sk-judge", gotAuth)
	require.Equal(t, "/v1/chat/completions", gotPath)
}

func TestRunPromptRiskJudge_Non2xxFailOpen(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	}))
	defer server.Close()

	svc := newJudgeService(t)
	cfg := DefaultPromptRiskConfig()
	cfg.Judge = PromptRiskJudgeConfig{Enabled: true, BaseURL: server.URL, Model: "m", APIKey: "sk", TimeoutMS: 4000}
	cfg.normalize()

	res := svc.runPromptRiskJudge(context.Background(), &cfg, "x", nil)
	require.Error(t, res.Err)
	require.Empty(t, res.Risk)
}

func TestRunPromptRiskJudge_TimeoutFailOpen(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(900 * time.Millisecond)
		_, _ = w.Write([]byte(judgeChatResponse(`{"risk":"high"}`)))
	}))
	defer server.Close()

	svc := newJudgeService(t)
	cfg := DefaultPromptRiskConfig()
	cfg.Judge = PromptRiskJudgeConfig{Enabled: true, BaseURL: server.URL, Model: "m", APIKey: "sk", TimeoutMS: 500}
	cfg.normalize()

	res := svc.runPromptRiskJudge(context.Background(), &cfg, "x", nil)
	require.Error(t, res.Err)
}

func TestRunPromptRiskJudge_BadJSONFailOpen(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(judgeChatResponse(`not a verdict`)))
	}))
	defer server.Close()

	svc := newJudgeService(t)
	cfg := DefaultPromptRiskConfig()
	cfg.Judge = PromptRiskJudgeConfig{Enabled: true, BaseURL: server.URL, Model: "m", APIKey: "sk", TimeoutMS: 4000}
	cfg.normalize()

	res := svc.runPromptRiskJudge(context.Background(), &cfg, "x", nil)
	require.Error(t, res.Err)
}

// ---- 融合 / fail-open / 防递归(端到端 Check)----

// judgeServerWithCount 返回固定 verdict 并统计被调用次数。
func judgeServerWithCount(verdict string, count *int64) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(count, 1)
		_, _ = w.Write([]byte(judgeChatResponse(verdict)))
	}))
}

// buildJudgePromptRiskSettings 生成启用 judge 的 prompt_risk_config JSON。
func buildJudgePromptRiskSettings(t *testing.T, judge PromptRiskJudgeConfig, exemptions []PromptRiskExemption) string {
	t.Helper()
	cfg := DefaultPromptRiskConfig()
	cfg.Enabled = true
	cfg.Mode = PromptRiskModeBlock
	cfg.AllGroups = true
	cfg.Judge = judge
	if exemptions != nil {
		cfg.Exemptions = exemptions
	}
	cfg.normalize()
	raw, err := json.Marshal(&cfg)
	require.NoError(t, err)
	return string(raw)
}

func newCheckServiceWithJudge(t *testing.T, settingsJSON string) (*ContentModerationService, *contentModerationTestRepo) {
	t.Helper()
	repo := &contentModerationTestRepo{}
	svc := NewContentModerationService(
		&contentModerationTestSettingRepo{values: map[string]string{
			SettingKeyRiskControlEnabled: "true",
			SettingKeyPromptRiskConfig:   settingsJSON,
		}},
		repo, nil, nil, nil, nil, nil,
	)
	return svc, repo
}

// 三中升级 block 的合法语义请求 + judge=none → 降级放行(消除误杀)。
func TestPromptRiskJudge_FusionDowngradesOnNone(t *testing.T) {
	var calls int64
	server := judgeServerWithCount(`{"risk":"none","reason":"自有实验室"}`, &calls)
	defer server.Close()

	settings := buildJudgePromptRiskSettings(t, PromptRiskJudgeConfig{
		Enabled: true, BaseURL: server.URL, Model: "m", APIKey: "sk", TimeoutMS: 4000,
		TriggerLevels: []string{PromptRiskLevelHigh},
	}, nil)
	svc, repo := newCheckServiceWithJudge(t, settings)

	decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
		UserID:   1,
		Protocol: ContentModerationProtocolOpenAIChat,
		Body:     []byte(`{"messages":[{"role":"user","content":"set up vpn proxy tunnel for the lab"}]}`),
	})
	require.NoError(t, err)
	require.False(t, decision.Blocked, "judge=none 应降级放行")
	require.Equal(t, int64(1), atomic.LoadInt64(&calls))

	logs := requireContentModerationLogCount(t, repo, 1)
	require.Equal(t, ContentModerationActionPromptRiskObserve, logs[0].Action)
	require.Equal(t, float64(1), logs[0].CategoryScores["judge:downgraded"])
	require.Equal(t, float64(1), logs[0].CategoryScores["judge:risk:none"])
}

// judge=high → 保持拦截。
func TestPromptRiskJudge_FusionKeepsBlockOnHigh(t *testing.T) {
	var calls int64
	server := judgeServerWithCount(`{"risk":"high","reason":"搭翻墙"}`, &calls)
	defer server.Close()

	settings := buildJudgePromptRiskSettings(t, PromptRiskJudgeConfig{
		Enabled: true, BaseURL: server.URL, Model: "m", APIKey: "sk", TimeoutMS: 4000,
		TriggerLevels: []string{PromptRiskLevelHigh},
	}, nil)
	svc, repo := newCheckServiceWithJudge(t, settings)

	decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
		UserID:   1,
		Protocol: ContentModerationProtocolOpenAIChat,
		Body:     []byte(`{"messages":[{"role":"user","content":"set up vpn proxy tunnel for the lab"}]}`),
	})
	require.NoError(t, err)
	require.True(t, decision.Blocked, "judge=high 应保持拦截")
	require.Equal(t, ContentModerationActionPromptRiskBlock, decision.Action)
	require.Equal(t, int64(1), atomic.LoadInt64(&calls))

	logs := requireContentModerationLogCount(t, repo, 1)
	require.Equal(t, float64(1), logs[0].CategoryScores["judge:risk:high"])
	require.NotContains(t, logs[0].CategoryScores, "judge:downgraded")
}

// judge 不可达 → fail-open 放行。
func TestPromptRiskJudge_FailOpenOnUnreachable(t *testing.T) {
	settings := buildJudgePromptRiskSettings(t, PromptRiskJudgeConfig{
		Enabled: true, BaseURL: "http://127.0.0.1:1", Model: "m", APIKey: "sk", TimeoutMS: 600,
		TriggerLevels: []string{PromptRiskLevelHigh},
	}, nil)
	svc, repo := newCheckServiceWithJudge(t, settings)

	decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
		UserID:   1,
		Protocol: ContentModerationProtocolOpenAIChat,
		Body:     []byte(`{"messages":[{"role":"user","content":"set up vpn proxy tunnel for the lab"}]}`),
	})
	require.NoError(t, err)
	require.False(t, decision.Blocked, "judge 调用失败应 fail-open 放行")

	logs := requireContentModerationLogCount(t, repo, 1)
	require.Equal(t, float64(1), logs[0].CategoryScores["judge:error"])
	require.Equal(t, ContentModerationActionPromptRiskObserve, logs[0].Action)
}

// 豁免短路:judge 的 api_key_id 在 Exemptions(low)里 → 关键词被封顶,judge 0 调用。
func TestPromptRiskJudge_ExemptionShortCircuits(t *testing.T) {
	var calls int64
	server := judgeServerWithCount(`{"risk":"high"}`, &calls)
	defer server.Close()

	settings := buildJudgePromptRiskSettings(t, PromptRiskJudgeConfig{
		Enabled: true, BaseURL: server.URL, Model: "m", APIKey: "sk", TimeoutMS: 4000,
		TriggerLevels: []string{PromptRiskLevelHigh},
	}, []PromptRiskExemption{{APIKeyIDs: []int64{999}, MaxLevel: PromptRiskLevelLow}})
	svc, _ := newCheckServiceWithJudge(t, settings)

	decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
		UserID:   1,
		APIKeyID: 999,
		Protocol: ContentModerationProtocolOpenAIChat,
		Body:     []byte(`{"messages":[{"role":"user","content":"set up vpn proxy tunnel for the lab"}]}`),
	})
	require.NoError(t, err)
	require.False(t, decision.Blocked)
	require.Equal(t, int64(0), atomic.LoadInt64(&calls), "豁免封顶后不应调用 judge")
}

// 防递归:ctx 带 in-flight 标记 → 跳过 judge,保持关键词 block 结论。
func TestPromptRiskJudge_ContextInFlightSkips(t *testing.T) {
	var calls int64
	server := judgeServerWithCount(`{"risk":"none"}`, &calls)
	defer server.Close()

	settings := buildJudgePromptRiskSettings(t, PromptRiskJudgeConfig{
		Enabled: true, BaseURL: server.URL, Model: "m", APIKey: "sk", TimeoutMS: 4000,
		TriggerLevels: []string{PromptRiskLevelHigh},
	}, nil)
	svc, _ := newCheckServiceWithJudge(t, settings)

	decision, err := svc.Check(withPromptRiskJudgeInFlight(context.Background()), ContentModerationCheckInput{
		UserID:   1,
		Protocol: ContentModerationProtocolOpenAIChat,
		Body:     []byte(`{"messages":[{"role":"user","content":"set up vpn proxy tunnel for the lab"}]}`),
	})
	require.NoError(t, err)
	require.True(t, decision.Blocked, "递归回环应保持关键词 block,不再 judge")
	require.Equal(t, int64(0), atomic.LoadInt64(&calls))
}

// 回归:judge 关闭时行为与现状一致(不调用、保持 block)。
func TestPromptRiskJudge_DisabledNoOp(t *testing.T) {
	var calls int64
	server := judgeServerWithCount(`{"risk":"none"}`, &calls)
	defer server.Close()

	settings := buildJudgePromptRiskSettings(t, PromptRiskJudgeConfig{
		Enabled: false, BaseURL: server.URL, Model: "m", APIKey: "sk",
	}, nil)
	svc, _ := newCheckServiceWithJudge(t, settings)

	decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
		UserID:   1,
		Protocol: ContentModerationProtocolOpenAIChat,
		Body:     []byte(`{"messages":[{"role":"user","content":"set up vpn proxy tunnel for the lab"}]}`),
	})
	require.NoError(t, err)
	require.True(t, decision.Blocked)
	require.Equal(t, int64(0), atomic.LoadInt64(&calls))
}

// GetPromptRiskConfig 必须掩码 judge api_key,Update 传空沿用旧 key。
func TestPromptRiskJudge_APIKeyMaskAndMerge(t *testing.T) {
	repo := &contentModerationTestSettingRepo{values: map[string]string{}}
	svc := NewContentModerationService(repo, &contentModerationTestRepo{}, nil, nil, nil, nil, nil)
	ctx := context.Background()

	in := DefaultPromptRiskConfig()
	in.Judge = PromptRiskJudgeConfig{Enabled: true, BaseURL: "https://gw.example.com", Model: "m", APIKey: "sk-secret-1234", TimeoutMS: 4000}
	saved, err := svc.UpdatePromptRiskConfig(ctx, in)
	require.NoError(t, err)
	require.Empty(t, saved.Judge.APIKey, "返回不应含明文 key")
	require.True(t, saved.Judge.APIKeyConfigured)
	require.True(t, strings.HasSuffix(saved.Judge.APIKeyMasked, "1234"))

	got, err := svc.GetPromptRiskConfig(ctx)
	require.NoError(t, err)
	require.Empty(t, got.Judge.APIKey)
	require.True(t, got.Judge.APIKeyConfigured)

	// 再次更新,api_key 传空 → 沿用旧 key(仍能通过 Validate)。
	upd := DefaultPromptRiskConfig()
	upd.Judge = PromptRiskJudgeConfig{Enabled: true, BaseURL: "https://gw2.example.com", Model: "m2", APIKey: "", TimeoutMS: 4000}
	saved2, err := svc.UpdatePromptRiskConfig(ctx, upd)
	require.NoError(t, err)
	require.Equal(t, "https://gw2.example.com", saved2.Judge.BaseURL)
	require.True(t, saved2.Judge.APIKeyConfigured, "传空应沿用旧 key,仍视为已配置")
}
