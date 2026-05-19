package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/width"
)

const (
	RequestInterceptMatchExact    = "exact"
	RequestInterceptMatchContains = "contains"
	RequestInterceptMatchRegex    = "regex"

	RequestInterceptScopeAll             = "all"
	RequestInterceptScopeMessages        = "messages"
	RequestInterceptScopeResponses       = "responses"
	RequestInterceptScopeChatCompletions = "chat_completions"
	RequestInterceptScopeGemini          = "gemini"
	RequestInterceptScopeImages          = "images"
)

type RequestInterceptNormalization struct {
	TrimSpace         bool `json:"trim_space"`
	CaseInsensitive   bool `json:"case_insensitive"`
	FullWidthToHalf   bool `json:"full_width_to_half"`
	CollapseSpace     bool `json:"collapse_space"`
	RemovePunctuation bool `json:"remove_punctuation"`
}

type RequestInterceptRuleConfig struct {
	ID              string                        `json:"id"`
	Name            string                        `json:"name"`
	Enabled         bool                          `json:"enabled"`
	Priority        int                           `json:"priority"`
	MatchMode       string                        `json:"match_mode"`
	Keywords        []string                      `json:"keywords"`
	Reply           string                        `json:"reply"`
	Scopes          []string                      `json:"scopes"`
	Normalize       RequestInterceptNormalization `json:"normalize"`
	CaseInsensitive bool                          `json:"case_insensitive"`
	Description     string                        `json:"description,omitempty"`
	CreatedAt       string                        `json:"created_at,omitempty"`
	UpdatedAt       string                        `json:"updated_at,omitempty"`
}

type RequestInterceptRulesDocument struct {
	Version   int                          `json:"version"`
	Rules     []RequestInterceptRuleConfig `json:"rules"`
	UpdatedAt string                       `json:"updated_at"`
}

type RequestInterceptConfig struct {
	Enabled bool `json:"enabled"`
}

type RequestInterceptMatchInput struct {
	Text     string
	Endpoint string
}

type RequestInterceptDecision struct {
	RuleID    string `json:"rule_id"`
	RuleName  string `json:"rule_name"`
	Keyword   string `json:"keyword"`
	MatchMode string `json:"match_mode"`
	Reply     string `json:"reply"`
	Endpoint  string `json:"endpoint"`
}

type RequestInterceptRulesService struct {
	settingRepo SettingRepository
}

func NewRequestInterceptRulesService(settingRepo SettingRepository) *RequestInterceptRulesService {
	return &RequestInterceptRulesService{settingRepo: settingRepo}
}

func NewRequestInterceptRulesServiceFromSettingService(settingService *SettingService) *RequestInterceptRulesService {
	if settingService == nil {
		return &RequestInterceptRulesService{}
	}
	return &RequestInterceptRulesService{settingRepo: settingService.settingRepo}
}

func DefaultRequestInterceptNormalization() RequestInterceptNormalization {
	return RequestInterceptNormalization{
		TrimSpace:         true,
		CaseInsensitive:   true,
		FullWidthToHalf:   true,
		CollapseSpace:     true,
		RemovePunctuation: true,
	}
}

func (s *RequestInterceptRulesService) GetConfig(ctx context.Context) (RequestInterceptConfig, error) {
	if s == nil || s.settingRepo == nil {
		return RequestInterceptConfig{}, fmt.Errorf("request intercept rules service is not configured")
	}
	value, err := s.settingRepo.GetValue(ctx, SettingKeyRequestInterceptEnabled)
	if err != nil {
		if err == ErrSettingNotFound {
			return RequestInterceptConfig{Enabled: true}, nil
		}
		return RequestInterceptConfig{}, fmt.Errorf("get request intercept config: %w", err)
	}
	return RequestInterceptConfig{Enabled: !isFalseSettingValue(value)}, nil
}

func (s *RequestInterceptRulesService) SetConfig(ctx context.Context, cfg RequestInterceptConfig) (RequestInterceptConfig, error) {
	if s == nil || s.settingRepo == nil {
		return RequestInterceptConfig{}, fmt.Errorf("request intercept rules service is not configured")
	}
	if err := s.settingRepo.Set(ctx, SettingKeyRequestInterceptEnabled, strconv.FormatBool(cfg.Enabled)); err != nil {
		return RequestInterceptConfig{}, fmt.Errorf("save request intercept config: %w", err)
	}
	return cfg, nil
}

func (s *RequestInterceptRulesService) ListRules(ctx context.Context) ([]RequestInterceptRuleConfig, error) {
	if s == nil || s.settingRepo == nil {
		return nil, fmt.Errorf("request intercept rules service is not configured")
	}
	value, err := s.settingRepo.GetValue(ctx, SettingKeyRequestInterceptRules)
	if err != nil {
		if err == ErrSettingNotFound {
			return []RequestInterceptRuleConfig{}, nil
		}
		return nil, fmt.Errorf("get request intercept rules: %w", err)
	}
	if strings.TrimSpace(value) == "" {
		return []RequestInterceptRuleConfig{}, nil
	}

	var doc RequestInterceptRulesDocument
	if err := json.Unmarshal([]byte(value), &doc); err != nil {
		return nil, fmt.Errorf("parse request intercept rules: %w", err)
	}
	return normalizeRequestInterceptRules(doc.Rules)
}

func (s *RequestInterceptRulesService) SaveRules(ctx context.Context, rules []RequestInterceptRuleConfig) ([]RequestInterceptRuleConfig, error) {
	if s == nil || s.settingRepo == nil {
		return nil, fmt.Errorf("request intercept rules service is not configured")
	}
	normalized, err := normalizeAndValidateRequestInterceptRules(rules)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for i := range normalized {
		if strings.TrimSpace(normalized[i].CreatedAt) == "" {
			normalized[i].CreatedAt = now
		}
		normalized[i].UpdatedAt = now
	}
	doc := RequestInterceptRulesDocument{
		Version:   1,
		Rules:     normalized,
		UpdatedAt: now,
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("marshal request intercept rules: %w", err)
	}
	if err := s.settingRepo.Set(ctx, SettingKeyRequestInterceptRules, string(raw)); err != nil {
		return nil, fmt.Errorf("save request intercept rules: %w", err)
	}
	return normalized, nil
}

func (s *RequestInterceptRulesService) UpsertRule(ctx context.Context, rule RequestInterceptRuleConfig) (RequestInterceptRuleConfig, error) {
	rules, err := s.ListRules(ctx)
	if err != nil {
		return RequestInterceptRuleConfig{}, err
	}
	found := false
	for i := range rules {
		if rules[i].ID == strings.TrimSpace(rule.ID) {
			if strings.TrimSpace(rule.CreatedAt) == "" {
				rule.CreatedAt = rules[i].CreatedAt
			}
			rules[i] = rule
			found = true
			break
		}
	}
	if !found {
		rules = append(rules, rule)
	}
	saved, err := s.SaveRules(ctx, rules)
	if err != nil {
		return RequestInterceptRuleConfig{}, err
	}
	for _, item := range saved {
		if item.ID == strings.TrimSpace(rule.ID) {
			return item, nil
		}
	}
	return RequestInterceptRuleConfig{}, fmt.Errorf("saved rule not found")
}

func (s *RequestInterceptRulesService) DeleteRule(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("id cannot be empty")
	}
	rules, err := s.ListRules(ctx)
	if err != nil {
		return err
	}
	next := make([]RequestInterceptRuleConfig, 0, len(rules))
	found := false
	for _, rule := range rules {
		if rule.ID == id {
			found = true
			continue
		}
		next = append(next, rule)
	}
	if !found {
		return ErrSettingNotFound
	}
	_, err = s.SaveRules(ctx, next)
	return err
}

func (s *RequestInterceptRulesService) TestRules(ctx context.Context, input RequestInterceptMatchInput) (*RequestInterceptDecision, bool, error) {
	rules, err := s.ListRules(ctx)
	if err != nil {
		return nil, false, err
	}
	decision, ok := MatchRequestInterceptRules(rules, input)
	if !ok {
		return nil, false, nil
	}
	return &decision, true, nil
}

func MatchRequestInterceptRules(rules []RequestInterceptRuleConfig, input RequestInterceptMatchInput) (RequestInterceptDecision, bool) {
	normalizedRules, err := normalizeRequestInterceptRules(rules)
	if err != nil {
		normalizedRules = rules
	}
	for _, rule := range normalizedRules {
		if !rule.Enabled || strings.TrimSpace(rule.Reply) == "" || !requestInterceptScopeMatches(rule.Scopes, input.Endpoint) {
			continue
		}
		text := NormalizeRequestInterceptText(input.Text, rule.Normalize)
		if text == "" {
			continue
		}
		for _, keyword := range rule.Keywords {
			normalizedKeyword := NormalizeRequestInterceptText(keyword, rule.Normalize)
			if normalizedKeyword == "" {
				continue
			}
			if requestInterceptKeywordMatches(rule.MatchMode, text, normalizedKeyword) {
				return RequestInterceptDecision{
					RuleID:    rule.ID,
					RuleName:  rule.Name,
					Keyword:   normalizedKeyword,
					MatchMode: rule.MatchMode,
					Reply:     rule.Reply,
					Endpoint:  input.Endpoint,
				}, true
			}
		}
	}
	return RequestInterceptDecision{}, false
}

func NormalizeRequestInterceptText(text string, normalization RequestInterceptNormalization) string {
	if normalization == (RequestInterceptNormalization{}) {
		normalization = DefaultRequestInterceptNormalization()
	}
	if normalization.FullWidthToHalf {
		text = width.Narrow.String(text)
	}
	if normalization.TrimSpace {
		text = strings.TrimSpace(text)
	}
	if normalization.CaseInsensitive {
		text = strings.ToLower(text)
	}
	if normalization.RemovePunctuation {
		var b strings.Builder
		for _, r := range text {
			if unicode.IsPunct(r) || unicode.IsSymbol(r) {
				continue
			}
			b.WriteRune(r)
		}
		text = b.String()
	}
	if normalization.CollapseSpace {
		text = strings.Join(strings.Fields(text), " ")
	}
	if normalization.TrimSpace {
		text = strings.TrimSpace(text)
	}
	return text
}

func normalizeRequestInterceptRules(rules []RequestInterceptRuleConfig) ([]RequestInterceptRuleConfig, error) {
	normalized := make([]RequestInterceptRuleConfig, 0, len(rules))
	for _, rule := range rules {
		if strings.TrimSpace(rule.ID) == "" && strings.TrimSpace(rule.Name) == "" && strings.TrimSpace(rule.Reply) == "" && len(rule.Keywords) == 0 {
			continue
		}
		rule.ID = strings.TrimSpace(rule.ID)
		rule.Name = strings.TrimSpace(rule.Name)
		rule.MatchMode = normalizeRequestInterceptMatchMode(rule.MatchMode)
		rule.Reply = strings.TrimSpace(rule.Reply)
		if len(rule.Scopes) == 0 {
			rule.Scopes = []string{RequestInterceptScopeAll}
		}
		rule.Scopes = normalizeRequestInterceptScopes(rule.Scopes)
		if rule.Normalize == (RequestInterceptNormalization{}) {
			rule.Normalize = DefaultRequestInterceptNormalization()
		}
		if rule.CaseInsensitive {
			rule.Normalize.CaseInsensitive = true
		}
		rule.Keywords = compactTrimmedStrings(rule.Keywords)
		normalized = append(normalized, rule)
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		if normalized[i].Priority == normalized[j].Priority {
			return normalized[i].ID < normalized[j].ID
		}
		return normalized[i].Priority < normalized[j].Priority
	})
	return normalized, nil
}

func normalizeAndValidateRequestInterceptRules(rules []RequestInterceptRuleConfig) ([]RequestInterceptRuleConfig, error) {
	normalized, err := normalizeRequestInterceptRules(rules)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for i, rule := range normalized {
		if rule.ID == "" {
			return nil, fmt.Errorf("rule[%d]: id cannot be empty", i)
		}
		if seen[rule.ID] {
			return nil, fmt.Errorf("rule[%d]: duplicate id %q", i, rule.ID)
		}
		seen[rule.ID] = true
		if rule.Name == "" {
			return nil, fmt.Errorf("rule[%d]: name cannot be empty", i)
		}
		if len(rule.Keywords) == 0 {
			return nil, fmt.Errorf("rule[%d]: keywords cannot be empty", i)
		}
		if rule.Reply == "" {
			return nil, fmt.Errorf("rule[%d]: reply cannot be empty", i)
		}
		if rule.MatchMode == RequestInterceptMatchRegex {
			for _, keyword := range rule.Keywords {
				if _, err := regexp.Compile(keyword); err != nil {
					return nil, fmt.Errorf("rule[%d]: invalid regex %q: %w", i, keyword, err)
				}
			}
		}
	}
	return normalized, nil
}

func normalizeRequestInterceptMatchMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case RequestInterceptMatchExact:
		return RequestInterceptMatchExact
	case RequestInterceptMatchRegex:
		return RequestInterceptMatchRegex
	default:
		return RequestInterceptMatchContains
	}
}

func normalizeRequestInterceptScopes(scopes []string) []string {
	result := make([]string, 0, len(scopes))
	seen := map[string]bool{}
	for _, scope := range scopes {
		scope = strings.ToLower(strings.TrimSpace(scope))
		switch scope {
		case RequestInterceptScopeMessages, RequestInterceptScopeResponses, RequestInterceptScopeChatCompletions, RequestInterceptScopeGemini, RequestInterceptScopeImages:
		default:
			scope = RequestInterceptScopeAll
		}
		if !seen[scope] {
			seen[scope] = true
			result = append(result, scope)
		}
	}
	if len(result) == 0 {
		return []string{RequestInterceptScopeAll}
	}
	return result
}

func compactTrimmedStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func requestInterceptKeywordMatches(mode string, text string, keyword string) bool {
	switch normalizeRequestInterceptMatchMode(mode) {
	case RequestInterceptMatchExact:
		return text == keyword
	case RequestInterceptMatchRegex:
		matched, err := regexp.MatchString(keyword, text)
		return err == nil && matched
	default:
		return strings.Contains(text, keyword)
	}
}

func requestInterceptScopeMatches(scopes []string, endpoint string) bool {
	endpointScope := requestInterceptScopeFromEndpoint(endpoint)
	for _, scope := range normalizeRequestInterceptScopes(scopes) {
		if scope == RequestInterceptScopeAll || scope == endpointScope {
			return true
		}
	}
	return false
}

func requestInterceptScopeFromEndpoint(endpoint string) string {
	endpoint = strings.ToLower(strings.TrimSpace(endpoint))
	switch {
	case strings.Contains(endpoint, "/chat/completions"):
		return RequestInterceptScopeChatCompletions
	case strings.Contains(endpoint, "/responses"):
		return RequestInterceptScopeResponses
	case strings.Contains(endpoint, "/v1beta/models/"), strings.Contains(endpoint, "/antigravity/v1beta/models/"):
		return RequestInterceptScopeGemini
	case strings.Contains(endpoint, "/images/"):
		return RequestInterceptScopeImages
	default:
		return RequestInterceptScopeMessages
	}
}
