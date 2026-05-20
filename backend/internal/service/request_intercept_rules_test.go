package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type requestInterceptSettingRepoStub struct {
	values map[string]string
}

func (r *requestInterceptSettingRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	value, err := r.GetValue(ctx, key)
	if err != nil {
		return nil, err
	}
	return &Setting{Key: key, Value: value, UpdatedAt: time.Now()}, nil
}

func (r *requestInterceptSettingRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	if r.values == nil {
		r.values = map[string]string{}
	}
	value, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}

func (r *requestInterceptSettingRepoStub) Set(ctx context.Context, key, value string) error {
	if r.values == nil {
		r.values = map[string]string{}
	}
	r.values[key] = value
	return nil
}

func (r *requestInterceptSettingRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	result := map[string]string{}
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			result[key] = value
		}
	}
	return result, nil
}

func (r *requestInterceptSettingRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	if r.values == nil {
		r.values = map[string]string{}
	}
	for key, value := range settings {
		r.values[key] = value
	}
	return nil
}

func (r *requestInterceptSettingRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	result := map[string]string{}
	for key, value := range r.values {
		result[key] = value
	}
	return result, nil
}

func (r *requestInterceptSettingRepoStub) Delete(ctx context.Context, key string) error {
	delete(r.values, key)
	return nil
}

func TestRequestInterceptRulesServiceSaveListAndMatch(t *testing.T) {
	repo := &requestInterceptSettingRepoStub{}
	svc := NewRequestInterceptRulesService(repo)

	saved, err := svc.SaveRules(context.Background(), []RequestInterceptRuleConfig{
		{
			ID:              "greeting",
			Name:            "问候语",
			Enabled:         true,
			Priority:        20,
			MatchMode:       RequestInterceptMatchExact,
			Keywords:        []string{"hi", "你好"},
			Reply:           "你好，我是迅游AI，有什么可以帮助你？",
			Scopes:          []string{RequestInterceptScopeAll},
			Normalize:       DefaultRequestInterceptNormalization(),
			CaseInsensitive: true,
		},
		{
			ID:              "policy",
			Name:            "政策拦截",
			Enabled:         true,
			Priority:        10,
			MatchMode:       RequestInterceptMatchContains,
			Keywords:        []string{"示例敏感词"},
			Reply:           "你的问题超出法律规定，请问一些其他的。",
			Scopes:          []string{RequestInterceptScopeAll},
			Normalize:       DefaultRequestInterceptNormalization(),
			CaseInsensitive: true,
		},
	})
	require.NoError(t, err)
	require.Len(t, saved, 2)

	rules, err := svc.ListRules(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"policy", "greeting"}, []string{rules[0].ID, rules[1].ID})

	decision, ok := MatchRequestInterceptRules(rules, RequestInterceptMatchInput{
		Text:     "  HI  ",
		Endpoint: "/v1/responses",
	})
	require.True(t, ok)
	require.Equal(t, "greeting", decision.RuleID)
	require.Equal(t, "hi", decision.Keyword)
	require.Equal(t, "你好，我是迅游AI，有什么可以帮助你？", decision.Reply)

	decision, ok = MatchRequestInterceptRules(rules, RequestInterceptMatchInput{
		Text:     "这里有，示例敏感词。",
		Endpoint: "/v1/chat/completions",
	})
	require.True(t, ok)
	require.Equal(t, "policy", decision.RuleID)
}

func TestRequestInterceptRulesServiceRegexAndNormalization(t *testing.T) {
	rules := []RequestInterceptRuleConfig{{
		ID:        "regex",
		Name:      "正则",
		Enabled:   true,
		Priority:  1,
		MatchMode: RequestInterceptMatchRegex,
		Keywords:  []string{`h\s*i`},
		Reply:     "你好，我是迅游AI，有什么可以帮助你？",
		Scopes:    []string{RequestInterceptScopeAll},
		Normalize: RequestInterceptNormalization{
			TrimSpace:         true,
			CaseInsensitive:   true,
			FullWidthToHalf:   true,
			CollapseSpace:     true,
			RemovePunctuation: false,
		},
	}}

	decision, ok := MatchRequestInterceptRules(rules, RequestInterceptMatchInput{
		Text:     "Ｈ   Ｉ",
		Endpoint: "/v1/messages",
	})
	require.True(t, ok)
	require.Equal(t, "regex", decision.RuleID)
}

func TestRequestInterceptRulesServiceFullContextScope(t *testing.T) {
	rules := []RequestInterceptRuleConfig{
		{
			ID:         "latest-user",
			Name:       "本轮用户输入",
			Enabled:    true,
			Priority:   1,
			MatchMode:  RequestInterceptMatchContains,
			MatchScope: RequestInterceptMatchScopeLatestUser,
			Keywords:   []string{"历史敏感词"},
			Reply:      "latest reply",
			Scopes:     []string{RequestInterceptScopeAll},
			Normalize:  DefaultRequestInterceptNormalization(),
		},
		{
			ID:         "full-context",
			Name:       "完整上下文",
			Enabled:    true,
			Priority:   2,
			MatchMode:  RequestInterceptMatchContains,
			MatchScope: RequestInterceptMatchScopeFullContext,
			Keywords:   []string{"历史敏感词"},
			Reply:      "full context reply",
			Scopes:     []string{RequestInterceptScopeAll},
			Normalize:  DefaultRequestInterceptNormalization(),
		},
	}

	decision, ok := MatchRequestInterceptRules(rules, RequestInterceptMatchInput{
		Text:            "hi",
		FullContextText: "上一轮出现历史敏感词\nhi",
		Endpoint:        "/v1/responses",
	})

	require.True(t, ok)
	require.Equal(t, "full-context", decision.RuleID)
	require.Equal(t, RequestInterceptMatchScopeFullContext, decision.MatchScope)
}

func TestRequestInterceptRulesServiceValidatesRules(t *testing.T) {
	svc := NewRequestInterceptRulesService(&requestInterceptSettingRepoStub{})

	_, err := svc.SaveRules(context.Background(), []RequestInterceptRuleConfig{{
		ID:        "bad",
		Name:      "无回复",
		Enabled:   true,
		MatchMode: RequestInterceptMatchExact,
		Keywords:  []string{"hi"},
		Scopes:    []string{RequestInterceptScopeAll},
	}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "reply")
}
