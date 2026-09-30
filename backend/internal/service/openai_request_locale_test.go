package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

const localeTestEnvironment = "<environment_context><current_date>2026-09-29</current_date><timezone>Asia/Shanghai</timezone></environment_context>"

func localeEnvironment(timezone string) string {
	return strings.Replace(localeTestEnvironment, "Asia/Shanghai", timezone, 1)
}

func localeTestBody(texts []string, kinds []string) []byte {
	content := make([]any, 0, len(texts))
	for _, text := range texts {
		content = append(content, map[string]any{"type": "input_text", "text": text})
	}
	body, _ := json.Marshal(map[string]any{
		"input": []any{map[string]any{
			"role": "user", "content": content,
			"internal_chat_message_metadata_passthrough": map[string]any{"content_item_kinds": kinds},
		}},
		"tools": []any{map[string]any{"type": "web_search", "user_location": map[string]any{
			"country": "US", "city": "New York", "timezone": "America/New_York",
		}}},
	})
	return body
}

// localeAccount builds an OpenAI account; a nil enabled leaves the switch absent.
func localeAccount(id int64, enabled any, timezone string) *Account {
	extra := map[string]any{}
	if enabled != nil {
		extra[openAIRequestTimezoneRewriteEnabledExtraKey] = enabled
	}
	if timezone != "" {
		extra[openAIRequestTimezoneExtraKey] = timezone
	}
	return &Account{ID: id, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: extra}
}

func localeErrorReason(err error) string {
	if err == nil {
		return ""
	}
	return infraerrors.Reason(err)
}

// localeSettingRepo serves the global switch to the gateway forwarding snapshot and
// can simulate a settings outage. WS relay goroutines read it concurrently.
type localeSettingRepo struct {
	*gatewayTTLSettingRepo
	mu     sync.Mutex
	global string
	err    error
	reads  int
}

func (r *localeSettingRepo) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reads++
	if r.err != nil {
		return nil, r.err
	}
	values, _ := r.gatewayTTLSettingRepo.GetMultiple(ctx, keys)
	if r.global != "" {
		values[SettingKeyEnableOpenAIRequestTimezoneRewrite] = r.global
	}
	return values, nil
}

func (r *localeSettingRepo) set(global string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.global, r.err = global, err
}

func (r *localeSettingRepo) readCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.reads
}

// expireLocaleSettingsSnapshot puts the process snapshot in the state a node reaches
// once gatewayForwardingCacheTTL has elapsed: the next read goes to the repository.
func expireLocaleSettingsSnapshot() {
	gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{})
}

func newLocaleSettingService(t *testing.T, global string) (*SettingService, *localeSettingRepo) {
	t.Helper()
	expireLocaleSettingsSnapshot()
	t.Cleanup(expireLocaleSettingsSnapshot)
	repo := &localeSettingRepo{gatewayTTLSettingRepo: &gatewayTTLSettingRepo{data: map[string]string{}}, global: global}
	return NewSettingService(repo, &config.Config{}), repo
}

func TestOpenAIRequestTimezoneCatalogAndValidation(t *testing.T) {
	options := OpenAIRequestTimezoneOptions()
	require.Greater(t, len(options), 30)
	require.True(t, sort.StringsAreSorted(options))
	require.Equal(t, "America/Los_Angeles", DefaultOpenAIRequestTimezone)
	require.Contains(t, options, DefaultOpenAIRequestTimezone)
	seen := make(map[string]struct{}, len(options))
	for _, name := range options {
		require.NotContains(t, seen, name, "duplicate timezone")
		seen[name] = struct{}{}
		_, err := time.LoadLocation(name)
		require.NoError(t, err, "invalid IANA timezone: %s", name)
	}
	for _, allowed := range []string{"Asia/Shanghai", "Asia/Urumqi", "Asia/Hong_Kong", "Asia/Macau", "Asia/Taipei", "Asia/Chongqing", "Europe/Oslo", "Africa/Accra", "Asia/Kathmandu", "Asia/Tokyo", "America/New_York"} {
		require.Contains(t, options, allowed)
		require.NoError(t, ValidateOpenAIRequestTimezoneExtra(PlatformOpenAI, false, map[string]any{openAIRequestTimezoneExtraKey: allowed}))
		require.Equal(t, allowed, (&Account{Platform: PlatformOpenAI, Extra: map[string]any{openAIRequestTimezoneExtraKey: allowed}}).OpenAIRequestTimezone())
	}
	for _, invalid := range []string{"Invalid/Timezone", " Asia/Tokyo", "Asia/Tokyo "} {
		require.NotContains(t, options, invalid)
		require.Equal(t, "INVALID_OPENAI_REQUEST_TIMEZONE", localeErrorReason(ValidateOpenAIRequestTimezoneExtra(PlatformOpenAI, false, map[string]any{openAIRequestTimezoneExtraKey: invalid})))
		require.Equal(t, DefaultOpenAIRequestTimezone, (&Account{Platform: PlatformOpenAI, Extra: map[string]any{openAIRequestTimezoneExtraKey: invalid}}).OpenAIRequestTimezone())
	}
	require.NoError(t, ValidateOpenAIRequestTimezoneExtra(PlatformOpenAI, false, map[string]any{openAIRequestTimezoneExtraKey: ""}))
	require.Error(t, ValidateOpenAIRequestTimezoneExtra(PlatformOpenAI, false, map[string]any{openAIRequestTimezoneExtraKey: 8}))
	require.Error(t, ValidateOpenAIRequestTimezoneExtra(PlatformAnthropic, false, map[string]any{openAIRequestTimezoneExtraKey: "Asia/Singapore"}))
	require.Equal(t, DefaultOpenAIRequestTimezone, (&Account{Platform: PlatformOpenAI}).OpenAIRequestTimezone())
	require.Equal(t, DefaultOpenAIRequestTimezone, (&Account{Platform: PlatformOpenAI, Extra: map[string]any{openAIRequestTimezoneExtraKey: ""}}).OpenAIRequestTimezone())
}

func TestOpenAIRequestTimezoneRewriteAccountContract(t *testing.T) {
	enabledKey, timezoneKey := openAIRequestTimezoneRewriteEnabledExtraKey, openAIRequestTimezoneExtraKey
	validate := func(platform string, shadow bool, extra map[string]any) string {
		return localeErrorReason(ValidateOpenAIRequestTimezoneExtra(platform, shadow, extra))
	}
	for _, value := range []any{true, false, nil} {
		require.Empty(t, validate(PlatformOpenAI, false, map[string]any{enabledKey: value, timezoneKey: "Asia/Tokyo"}))
	}
	for _, value := range []any{"true", 1, map[string]any{}} {
		require.Equal(t, "INVALID_OPENAI_REQUEST_TIMEZONE_REWRITE", validate(PlatformOpenAI, false, map[string]any{enabledKey: value}))
	}
	require.Equal(t, "INVALID_OPENAI_REQUEST_TIMEZONE_REWRITE", validate(PlatformAnthropic, false, map[string]any{enabledKey: true}))
	require.Empty(t, validate(PlatformAnthropic, false, map[string]any{enabledKey: false}), "a disabled switch is not an enabling config")
	for _, extra := range []map[string]any{{enabledKey: true}, {enabledKey: false}, {timezoneKey: "Asia/Tokyo"}, {timezoneKey: ""}} {
		require.Equal(t, "OPENAI_REQUEST_TIMEZONE_MANAGED_BY_PARENT", validate(PlatformOpenAI, true, extra))
	}
	require.Empty(t, validate(PlatformOpenAI, true, map[string]any{enabledKey: nil, timezoneKey: nil}), "null clears a stale shadow value")

	// Only an explicit JSON true on an OpenAI account enables the rewrite.
	for _, value := range []any{false, "true", 1} {
		require.False(t, localeAccount(1, value, "").OpenAIRequestTimezoneRewriteEnabled())
	}
	require.False(t, localeAccount(1, nil, "Asia/Tokyo").OpenAIRequestTimezoneRewriteEnabled())
	require.False(t, (&Account{Platform: PlatformOpenAI, Extra: map[string]any{enabledKey: nil}}).OpenAIRequestTimezoneRewriteEnabled())
	require.False(t, (&Account{Platform: PlatformOpenAI}).OpenAIRequestTimezoneRewriteEnabled())
	require.True(t, localeAccount(1, true, "").OpenAIRequestTimezoneRewriteEnabled())
	anthropic := localeAccount(1, true, "")
	anthropic.Platform = PlatformAnthropic
	require.False(t, anthropic.OpenAIRequestTimezoneRewriteEnabled())
	// Turning the switch off keeps the chosen timezone for re-enabling later.
	require.Equal(t, "Asia/Tokyo", localeAccount(1, false, "Asia/Tokyo").OpenAIRequestTimezone())
}

func TestRewriteOpenAIRequestEnvironmentOnlyTimezone(t *testing.T) {
	for name, text := range map[string]string{
		"historical date":           "<environment_context><current_date>2001-01-01</current_date><timezone>Asia/Shanghai</timezone></environment_context>",
		"unavailable date":          `<environment_context><current_date status="unavailable" /><timezone>Asia/Shanghai</timezone></environment_context>`,
		"closing tag whitespace":    "<environment_context><current_date>2001-01-01</current_date \t><timezone>Asia/Shanghai</timezone \r\n></environment_context  >",
		"timezone only delta":       "<environment_context><timezone>Asia/Shanghai</timezone></environment_context>",
		"opaque unescaped fields":   "<environment_context>\n<subagents>agent & colleague <unnamed</subagents>\n<network enabled=\"true\"><allowed>a&b.example,<example</allowed></network>\n<timezone>Asia/Shanghai</timezone>\n<cwd>/workspace/a&b<c</cwd>\n</environment_context>",
		"attributes and whitespace": "\n <environment_context version=\"test\">\r\n  <timezone source='client > clock'>\n \tAsia/Shanghai\u00a0\n</timezone>\n</environment_context> \t",
		"opaque nested fields":      "<environment_context><filesystem><workspace_roots><root>/workspace</root></workspace_roots></filesystem><timezone>Asia/Shanghai</timezone></environment_context>",
	} {
		t.Run(name, func(t *testing.T) {
			out, previous, reason := rewriteOpenAIRequestEnvironment(text, "America/New_York")
			require.Equal(t, "replaced", reason)
			require.Equal(t, "Asia/Shanghai", previous)
			require.Equal(t, strings.Replace(text, "Asia/Shanghai", "America/New_York", 1), out)
			again, _, reason := rewriteOpenAIRequestEnvironment(out, "America/New_York")
			require.Equal(t, "already_target", reason)
			require.Equal(t, out, again)
		})
	}
}

func TestRewriteOpenAIRequestEnvironmentAmbiguousOrQuoted(t *testing.T) {
	for name, text := range map[string]string{
		"ordinary prose":                    "Timezone: Asia/Shanghai, date: 2001-01-01",
		"inline example":                    "Example: <environment_context><timezone>Asia/Shanghai</timezone></environment_context>",
		"code fence":                        "```xml\n<environment_context><timezone>Asia/Shanghai</timezone></environment_context>\n```",
		"quoted block":                      "> <environment_context><timezone>Asia/Shanghai</timezone></environment_context>",
		"memory history prose":              "Earlier environment:\n<environment_context><timezone>Asia/Shanghai</timezone></environment_context>",
		"missing timezone":                  "<environment_context><current_date>2001-01-01</current_date></environment_context>",
		"empty timezone":                    "<environment_context><timezone> \n </timezone></environment_context>",
		"self closing timezone":             "<environment_context><timezone /></environment_context>",
		"duplicate timezone":                "<environment_context><timezone>Asia/Shanghai</timezone><timezone>Asia/Tokyo</timezone></environment_context>",
		"timezone followed by self closing": "<environment_context><timezone>Asia/Shanghai</timezone><timezone /></environment_context>",
		"timezone with nested tag":          "<environment_context><timezone><value>Asia/Shanghai</value></timezone></environment_context>",
		"nested timezone":                   "<environment_context><example><timezone>Asia/Shanghai</timezone></example></environment_context>",
		"incomplete timezone":               "<environment_context><timezone>Asia/Shanghai</environment_context>",
		"missing root close":                "<environment_context><timezone>Asia/Shanghai</timezone>",
		"self closing root":                 "<environment_context/>",
		"nested root":                       "<environment_context><environment_context><timezone>Asia/Shanghai</timezone></environment_context></environment_context>",
		"two roots":                         "<environment_context><timezone>Asia/Shanghai</timezone></environment_context><environment_context><timezone>Asia/Tokyo</timezone></environment_context>",
		"unclear field boundary":            "<environment_context><timezone>Asia/Shanghai</timezone> some prose </environment_context>",
		"wrong opaque field close":          "<environment_context><timezone>Asia/Shanghai</timezone><network>domain&name</wrong></environment_context>",
	} {
		t.Run(name, func(t *testing.T) {
			out, _, reason := rewriteOpenAIRequestEnvironment(text, "America/New_York")
			require.NotEqual(t, "replaced", reason)
			require.Equal(t, text, out)
		})
	}
}

func TestNormalizeOpenAIRequestLocaleAndDebugLog(t *testing.T) {
	first := "<environment_context>\n  <current_date>2026-09-22</current_date>\n  <timezone>Asia/Shanghai</timezone>\n  <cwd>/app</cwd>\n</environment_context>"
	second := "<environment_context><timezone>America/Los_Angeles</timezone></environment_context>"
	body := localeTestBody([]string{first, second, first}, []string{"environments.environment_context", "environments.environment_context", "user.text"})
	core, observed := observer.New(zap.DebugLevel)
	ctx := logger.IntoContext(context.Background(), zap.New(core))
	out := normalizeOpenAIRequestLocale(ctx, &Account{ID: 42, Platform: PlatformOpenAI}, DefaultOpenAIRequestTimezone, body, "http")
	for _, path := range []string{"input.0.content.0.text", "input.0.content.2.text"} {
		require.Equal(t, strings.Replace(first, "Asia/Shanghai", "America/Los_Angeles", 1), gjson.GetBytes(out, path).String())
	}
	require.Equal(t, second, gjson.GetBytes(out, "input.0.content.1.text").String())
	require.Equal(t, gjson.GetBytes(body, "input.0.internal_chat_message_metadata_passthrough").Raw, gjson.GetBytes(out, "input.0.internal_chat_message_metadata_passthrough").Raw)
	require.Equal(t, "US", gjson.GetBytes(out, "tools.0.user_location.country").String())
	require.Equal(t, "New York", gjson.GetBytes(out, "tools.0.user_location.city").String())
	require.Equal(t, "America/Los_Angeles", gjson.GetBytes(out, "tools.0.user_location.timezone").String())
	require.Len(t, observed.All(), 1)
	fields := observed.All()[0].ContextMap()
	require.Equal(t, int64(42), fields["account_id"])
	require.Equal(t, true, fields["enabled"])
	require.Equal(t, "replaced", fields["reason"])
	require.EqualValues(t, 3, fields["matched_count"])
	require.EqualValues(t, 2, fields["replaced_count"])
	require.EqualValues(t, 1, fields["web_search_replaced_count"])
	encoded, err := json.Marshal(fields)
	require.NoError(t, err)
	for _, clientValue := range []string{"Asia/Shanghai", "America/New_York", "New York", "/app", "2026-09-22"} {
		require.NotContains(t, string(encoded), clientValue, "logs must not carry client values or prompt text")
	}
}

func TestNormalizeOpenAIRequestLocaleNoReplacementStillLogs(t *testing.T) {
	cases := []struct{ name, text, reason string }{
		{"already target", "<environment_context><timezone>America/Los_Angeles</timezone></environment_context>", "already_target"},
		{"no timezone", "<environment_context><current_date>old</current_date></environment_context>", "no_timezone"},
		{"nested value", "<environment_context><timezone><nested/></timezone></environment_context>", "invalid_environment_context"},
		{"self closing", "<environment_context><timezone/></environment_context>", "invalid_environment_context"},
		{"ordinary text", "Today in Asia/Shanghai", "no_environment_context"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			core, observed := observer.New(zap.DebugLevel)
			body := localeTestBody([]string{item.text}, nil)
			body, err := sjson.DeleteBytes(body, "tools")
			require.NoError(t, err)
			out := normalizeOpenAIRequestLocale(logger.IntoContext(context.Background(), zap.New(core)), &Account{Platform: PlatformOpenAI}, DefaultOpenAIRequestTimezone, body, "ws")
			require.Len(t, observed.All(), 1)
			require.Equal(t, item.reason, observed.All()[0].ContextMap()["reason"])
			require.EqualValues(t, 0, observed.All()[0].ContextMap()["replaced_count"])
			require.Equal(t, body, out)
		})
	}
	core, observed := observer.New(zap.InfoLevel)
	normalizeOpenAIRequestLocale(logger.IntoContext(context.Background(), zap.New(core)), &Account{Platform: PlatformOpenAI}, DefaultOpenAIRequestTimezone, localeTestBody(nil, nil), "http")
	require.Empty(t, observed.All())
}

func TestNormalizeOpenAIRequestLocaleWithoutKindPreservesHistory(t *testing.T) {
	env := "<environment_context><current_date>2001-01-01</current_date><timezone>Asia/Shanghai</timezone></environment_context>"
	for name, kinds := range map[string]any{"absent": nil, "empty": []string{}, "unmarked": []string{"user.text"}, "incomplete": []string{"environments.environment_context"}, "invalid type": "user.text"} {
		t.Run(name, func(t *testing.T) {
			body := localeTestBody([]string{env, strings.Replace(env, "2001-01-01", "2002-06-30", 1)}, nil)
			body, _ = sjson.DeleteBytes(body, "tools")
			if kinds == nil {
				body, _ = sjson.DeleteBytes(body, "input.0.internal_chat_message_metadata_passthrough")
			} else {
				body, _ = sjson.SetBytes(body, "input.0.internal_chat_message_metadata_passthrough.content_item_kinds", kinds)
			}
			body, _ = sjson.SetBytes(body, "client_metadata.turn_started_at_unix_ms", 1772938800000)
			account := &Account{Platform: PlatformOpenAI}
			out := normalizeOpenAIRequestLocale(context.Background(), account, "America/New_York", body, "http")
			for i, date := range []string{"2001-01-01", "2002-06-30"} {
				text := gjson.GetBytes(out, "input.0.content").Array()[i].Get("text").String()
				require.Contains(t, text, "<current_date>"+date+"</current_date>")
				require.Contains(t, text, "<timezone>America/New_York</timezone>")
			}
			require.Equal(t, gjson.GetBytes(body, "client_metadata").Raw, gjson.GetBytes(out, "client_metadata").Raw)
			require.Equal(t, gjson.GetBytes(body, "input.0.internal_chat_message_metadata_passthrough").Raw, gjson.GetBytes(out, "input.0.internal_chat_message_metadata_passthrough").Raw)
			// There is no clock input: replaying a historical prefix through either
			// transport, including on a later day, produces exactly the same bytes.
			require.Equal(t, out, normalizeOpenAIRequestLocale(context.Background(), account, "America/New_York", body, "ws"))
			require.Equal(t, out, normalizeOpenAIRequestLocale(context.Background(), account, "America/New_York", out, "http"))
		})
	}
}

func TestNormalizeOpenAIRequestLocaleOnlyUserText(t *testing.T) {
	env := "<environment_context><timezone>Asia/Shanghai</timezone></environment_context>"
	body, err := json.Marshal(map[string]any{"input": []any{
		map[string]any{"role": "user", "content": env},
		map[string]any{"role": "developer", "content": env},
		map[string]any{"role": "assistant", "content": env},
		map[string]any{"role": "tool", "content": env},
		map[string]any{"type": "function_call_output", "output": env},
		map[string]any{"role": "user", "content": []any{map[string]any{"type": "output_text", "text": env}, map[string]any{"type": "input_text", "text": 42}}},
	}})
	require.NoError(t, err)
	out := normalizeOpenAIRequestLocale(context.Background(), &Account{Platform: PlatformOpenAI}, DefaultOpenAIRequestTimezone, body, "http")
	require.Equal(t, strings.Replace(env, "Asia/Shanghai", "America/Los_Angeles", 1), gjson.GetBytes(out, "input.0.content").String())
	original, rewritten := gjson.GetBytes(body, "input").Array(), gjson.GetBytes(out, "input").Array()
	for i := 1; i < len(original); i++ {
		require.Equal(t, original[i].Raw, rewritten[i].Raw)
	}
}

func TestApplyOpenAIRequestLocaleRequiresGlobalAndAccountSwitches(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prose := "Keep Asia/Shanghai and 2026-09-29 in this sentence"
	body := localeTestBody([]string{localeTestEnvironment, prose}, nil)

	for _, tc := range []struct {
		name    string
		global  string
		account *Account
		reason  string
	}{
		{"global missing", "", localeAccount(1, true, ""), "global_disabled"},
		{"global off", "false", localeAccount(1, true, ""), "global_disabled"},
		{"global malformed", "yes", localeAccount(1, true, ""), "global_disabled"},
		{"account switch missing", "true", localeAccount(1, nil, "Asia/Tokyo"), "account_disabled"},
		{"account switch off", "true", localeAccount(1, false, "Asia/Tokyo"), "account_disabled"},
		{"account switch null", "true", &Account{ID: 1, Platform: PlatformOpenAI, Extra: map[string]any{openAIRequestTimezoneRewriteEnabledExtraKey: nil}}, "account_disabled"},
		{"account switch string", "true", localeAccount(1, "true", ""), "account_disabled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settings, repo := newLocaleSettingService(t, tc.global)
			core, observed := observer.New(zap.DebugLevel)
			ctx := logger.IntoContext(context.Background(), zap.New(core))
			out := (&OpenAIGatewayService{settingService: settings}).applyOpenAIRequestLocale(ctx, nil, tc.account, body, "http")
			require.Equal(t, body, out, "a disabled rewrite returns the original bytes")
			require.Len(t, observed.All(), 1)
			fields := observed.All()[0].ContextMap()
			require.Equal(t, tc.reason, fields["reason"])
			require.Equal(t, false, fields["enabled"])
			if tc.reason == "account_disabled" {
				require.Zero(t, repo.readCount(), "a disabled account never touches settings")
			}
		})
	}

	t.Run("both switches on", func(t *testing.T) {
		settings, _ := newLocaleSettingService(t, "true")
		svc := &OpenAIGatewayService{settingService: settings}
		out := svc.applyOpenAIRequestLocale(context.Background(), nil, localeAccount(1, true, ""), body, "http")
		require.Equal(t, localeEnvironment(DefaultOpenAIRequestTimezone), gjson.GetBytes(out, "input.0.content.0.text").String())
		require.Equal(t, prose, gjson.GetBytes(out, "input.0.content.1.text").String())
		require.Equal(t, DefaultOpenAIRequestTimezone, gjson.GetBytes(out, "tools.0.user_location.timezone").String())
		require.Equal(t, "New York", gjson.GetBytes(out, "tools.0.user_location.city").String())
		out = svc.applyOpenAIRequestLocale(context.Background(), nil, localeAccount(1, true, "Asia/Tokyo"), body, "http")
		require.Equal(t, localeEnvironment("Asia/Tokyo"), gjson.GetBytes(out, "input.0.content.0.text").String())
	})

	t.Run("non OpenAI or missing settings service stays untouched", func(t *testing.T) {
		settings, _ := newLocaleSettingService(t, "true")
		anthropic := localeAccount(1, true, "")
		anthropic.Platform = PlatformAnthropic
		svc := &OpenAIGatewayService{settingService: settings}
		require.Equal(t, body, svc.applyOpenAIRequestLocale(context.Background(), nil, anthropic, body, "http"))
		require.Equal(t, body, svc.applyOpenAIRequestLocale(context.Background(), nil, nil, body, "http"))
		require.Equal(t, body, (&OpenAIGatewayService{}).applyOpenAIRequestLocale(context.Background(), nil, localeAccount(1, true, ""), body, "http"))
	})

	t.Run("malformed environment block passes through", func(t *testing.T) {
		settings, _ := newLocaleSettingService(t, "true")
		malformed := localeTestBody([]string{"<environment_context><timezone>Asia/Shanghai</environment_context>"}, nil)
		malformed, _ = sjson.DeleteBytes(malformed, "tools")
		out := (&OpenAIGatewayService{settingService: settings}).applyOpenAIRequestLocale(context.Background(), nil, localeAccount(1, true, ""), malformed, "http")
		require.Equal(t, malformed, out)
	})
}

func TestApplyOpenAIRequestLocaleFailsClosedWhenSettingsUnavailable(t *testing.T) {
	settings, repo := newLocaleSettingService(t, "true")
	repo.set("true", errors.New("settings database unavailable"))
	svc := &OpenAIGatewayService{settingService: settings}
	body := localeTestBody([]string{localeTestEnvironment}, nil)
	core, observed := observer.New(zap.DebugLevel)
	ctx := logger.IntoContext(context.Background(), zap.New(core))

	require.Equal(t, body, svc.applyOpenAIRequestLocale(ctx, nil, localeAccount(1, true, ""), body, "http"))
	require.Equal(t, body, svc.applyOpenAIRequestLocale(ctx, nil, localeAccount(1, true, ""), body, "ws"))
	require.Equal(t, 1, repo.readCount(), "the failure is cached for gatewayForwardingErrorTTL rather than retried per request")
	require.Len(t, observed.All(), 2)
	for _, entry := range observed.All() {
		require.Equal(t, "settings_unavailable", entry.ContextMap()["reason"])
	}

	repo.set("true", nil)
	expireLocaleSettingsSnapshot()
	out := svc.applyOpenAIRequestLocale(ctx, nil, localeAccount(1, true, ""), body, "http")
	require.Equal(t, localeEnvironment(DefaultOpenAIRequestTimezone), gjson.GetBytes(out, "input.0.content.0.text").String())
}

func TestOpenAIRequestTimezoneRewriteGlobalSwitchPropagation(t *testing.T) {
	require.Equal(t, 60*time.Second, gatewayForwardingCacheTTL, "documented worst-case propagation to other nodes")
	require.Equal(t, 5*time.Second, gatewayForwardingErrorTTL)
	settings, repo := newLocaleSettingService(t, "false")
	ctx := context.Background()
	enabled, unavailable := settings.OpenAIRequestTimezoneRewriteState(ctx)
	require.False(t, enabled)
	require.False(t, unavailable)

	// Saving on this node refreshes the snapshot in place, without a read.
	require.NoError(t, settings.UpdateSettings(ctx, &SystemSettings{EnableOpenAIRequestTimezoneRewrite: true}))
	reads := repo.readCount()
	enabled, _ = settings.OpenAIRequestTimezoneRewriteState(ctx)
	require.True(t, enabled)
	require.Equal(t, reads, repo.readCount())

	// A change saved on another node is only visible once the snapshot expires.
	repo.set("false", nil)
	enabled, _ = settings.OpenAIRequestTimezoneRewriteState(ctx)
	require.True(t, enabled)
	expireLocaleSettingsSnapshot()
	enabled, _ = settings.OpenAIRequestTimezoneRewriteState(ctx)
	require.False(t, enabled)
}

// localeForward drives the real HTTP entry point against a fake upstream and returns
// the body the gateway sent upstream.
func localeForward(t *testing.T, settings *SettingService, account *Account, path string, stream bool) []byte {
	t.Helper()
	body := localeTestBody([]string{localeTestEnvironment, "Keep Asia/Shanghai and 2026-09-29 in this sentence"}, nil)
	body, _ = sjson.DeleteBytes(body, "tools")
	body, _ = sjson.SetBytes(body, "model", "gpt-5.4")
	body, _ = sjson.SetBytes(body, "stream", stream)
	body, _ = sjson.SetBytes(body, "instructions", "test")
	body, _ = sjson.SetBytes(body, "input.0.type", "message")
	body, _ = sjson.SetRawBytes(body, "input.-1", []byte(`{"type":"message","role":"assistant","content":[{"type":"output_text","text":"`+localeTestEnvironment+`"}]}`))
	recorder := httptest.NewRecorder()
	client, _ := gin.CreateTestContext(recorder)
	client.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	client.Request.Header.Set("Content-Type", "application/json")
	completed := `{"id":"resp_locale","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(completed)),
	}}
	if (stream || account.IsOpenAIPassthroughEnabled()) && !strings.HasSuffix(path, "/compact") {
		upstream.resp.Header.Set("Content-Type", "text/event-stream")
		upstream.resp.Body = io.NopCloser(strings.NewReader(`data: {"type":"response.completed","response":` + completed + "}\n\n"))
	}
	result, err := (&OpenAIGatewayService{httpUpstream: upstream, settingService: settings}).Forward(context.Background(), client, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "resp_locale")
	return upstream.lastBody
}

func TestForwardOpenAIRequestLocaleGatesEveryHTTPPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name        string
		path        string
		passthrough bool
		stream      bool
	}{
		{"responses", "/v1/responses", false, false},
		{"responses stream", "/v1/responses", false, true},
		{"passthrough", "/v1/responses", true, false},
		{"passthrough stream", "/v1/responses", true, true},
		{"compact", "/v1/responses/compact", false, false},
		{"passthrough compact", "/v1/responses/compact", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account := func(enabled any, timezone string) *Account {
				account := localeAccount(100, enabled, timezone)
				account.Status, account.Schedulable, account.Concurrency = StatusActive, true, 1
				account.Credentials = map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-account"}
				account.Extra["openai_passthrough"] = tc.passthrough
				return account
			}
			settings, repo := newLocaleSettingService(t, "false")
			control := localeForward(t, settings, account(nil, ""), tc.path, tc.stream)
			require.Equal(t, localeTestEnvironment, gjson.GetBytes(control, "input.0.content.0.text").String())

			// Either switch off: the upstream body is byte-identical to a request
			// from an account that never had the feature configured.
			require.Equal(t, control, localeForward(t, settings, account(true, "Asia/Tokyo"), tc.path, tc.stream))
			repo.set("true", nil)
			expireLocaleSettingsSnapshot()
			require.Equal(t, control, localeForward(t, settings, account(nil, "Asia/Tokyo"), tc.path, tc.stream))
			require.Equal(t, control, localeForward(t, settings, account(false, "Asia/Tokyo"), tc.path, tc.stream))

			// Both on: only the user environment timezone changes.
			for timezone, target := range map[string]string{"": DefaultOpenAIRequestTimezone, "Asia/Tokyo": "Asia/Tokyo"} {
				expected, err := sjson.SetBytes(control, "input.0.content.0.text", localeEnvironment(target))
				require.NoError(t, err)
				require.JSONEq(t, string(expected), string(localeForward(t, settings, account(true, timezone), tc.path, tc.stream)))
			}
		})
	}
}

func TestForwardOpenAIRequestLocaleSparkShadowFollowsParent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	settings, _ := newLocaleSettingService(t, "true")
	parentID := int64(800)
	repo := stubOpenAIAccountRepo{accounts: []Account{{
		ID: parentID, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"access_token": "parent-token", "chatgpt_account_id": "chatgpt-parent"},
	}}}
	// Stale shadow values must never win over the parent.
	shadow := localeAccount(801, true, "Asia/Kolkata")
	shadow.Status, shadow.Schedulable, shadow.Concurrency = StatusActive, true, 1
	shadow.ParentAccountID, shadow.QuotaDimension = &parentID, QuotaDimensionSpark
	forward := func() string {
		t.Helper()
		body := localeTestBody([]string{localeTestEnvironment}, nil)
		body, _ = sjson.SetBytes(body, "model", "gpt-5.4")
		body, _ = sjson.SetBytes(body, "stream", false)
		body, _ = sjson.SetBytes(body, "instructions", "test")
		body, _ = sjson.SetBytes(body, "input.0.type", "message")
		body, _ = sjson.DeleteBytes(body, "tools")
		client, _ := gin.CreateTestContext(httptest.NewRecorder())
		client.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
		client.Request.Header.Set("Content-Type", "application/json")
		upstream := &httpUpstreamRecorder{resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp_shadow","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`)),
		}}
		svc := &OpenAIGatewayService{httpUpstream: upstream, accountRepo: repo, settingService: settings}
		_, err := svc.Forward(context.Background(), client, shadow, body)
		require.NoError(t, err)
		require.Equal(t, "Bearer parent-token", upstream.lastReq.Header.Get("Authorization"))
		return gjson.GetBytes(upstream.lastBody, "input.0.content.0.text").String()
	}

	require.Equal(t, localeTestEnvironment, forward(), "parent switch missing: shadow stays untouched")
	repo.accounts[0].Extra = map[string]any{openAIRequestTimezoneRewriteEnabledExtraKey: true, openAIRequestTimezoneExtraKey: "America/New_York"}
	require.Equal(t, localeEnvironment("America/New_York"), forward())
	repo.accounts[0].Extra = map[string]any{openAIRequestTimezoneRewriteEnabledExtraKey: true, openAIRequestTimezoneExtraKey: "Asia/Tokyo"}
	require.Equal(t, localeEnvironment("Asia/Tokyo"), forward(), "a parent change applies on the next attempt")
	repo.accounts[0].Extra = map[string]any{openAIRequestTimezoneRewriteEnabledExtraKey: true}
	require.Equal(t, localeEnvironment(DefaultOpenAIRequestTimezone), forward())
	repo.accounts[0].Extra = map[string]any{openAIRequestTimezoneRewriteEnabledExtraKey: false, openAIRequestTimezoneExtraKey: "Asia/Tokyo"}
	require.Equal(t, localeTestEnvironment, forward())
}

func TestApplyOpenAIRequestLocaleShadowWithoutStagedParentStaysUntouched(t *testing.T) {
	gin.SetMode(gin.TestMode)
	settings, _ := newLocaleSettingService(t, "true")
	parentID := int64(700)
	shadow := localeAccount(701, true, "Asia/Tokyo")
	shadow.ParentAccountID = &parentID
	body := localeTestBody([]string{localeTestEnvironment}, nil)
	svc := &OpenAIGatewayService{settingService: settings}
	for _, staged := range []*Account{nil, localeAccount(999, true, "America/New_York")} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		if staged != nil {
			c.Set(codexAccountIdentitySourceContextKey, staged)
		}
		require.Equal(t, body, svc.applyOpenAIRequestLocale(context.Background(), c, shadow, body, "http"))
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(codexAccountIdentitySourceContextKey, localeAccount(parentID, true, "America/New_York"))
	out := svc.applyOpenAIRequestLocale(context.Background(), c, shadow, body, "http")
	require.Equal(t, localeEnvironment("America/New_York"), gjson.GetBytes(out, "input.0.content.0.text").String())
	// Ordinary accounts ignore whatever another attempt staged.
	ordinary := localeAccount(702, false, "")
	require.Equal(t, body, svc.applyOpenAIRequestLocale(context.Background(), c, ordinary, body, "http"))
}

func localeWSResponseCreate(t *testing.T, fields map[string]any, texts ...string) string {
	t.Helper()
	content := make([]any, 0, len(texts))
	for _, text := range texts {
		content = append(content, map[string]any{"type": "input_text", "text": text})
	}
	frame := map[string]any{
		"type": "response.create", "model": "gpt-5.1", "stream": false,
		"input": []any{map[string]any{"type": "message", "role": "user", "content": content}},
	}
	for key, value := range fields {
		frame[key] = value
	}
	raw, err := json.Marshal(frame)
	require.NoError(t, err)
	return string(raw)
}

func TestOpenAIWSPassthroughRequestLocaleFollowsSwitchesPerResponseCreate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)
	settings, repo := newLocaleSettingService(t, "true")
	cfg := passthroughLifecycleConfig()
	cfg.Gateway.OpenAIFirstOutputTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.IngressInterTurnIdleTimeoutSeconds = 3
	upstream := newStagedPassthroughConn()
	upstream.Send(`{"type":"response.completed","response":{"id":"resp_tz_1","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)
	account := passthroughLifecycleAccount()
	account.Extra[openAIRequestTimezoneRewriteEnabledExtraKey] = true
	account.Extra[openAIRequestTimezoneExtraKey] = "America/New_York"
	svc := newPassthroughLifecycleService(cfg, upstream)
	svc.settingService = settings
	server, serverErr := startPassthroughLifecycleServer(t, controlCtx, svc, account)
	defer server.Close()

	prose := "Answer in <timezone>Asia/Shanghai</timezone> please"
	webSearch := []any{map[string]any{"type": "web_search", "user_location": map[string]any{"type": "approximate", "timezone": "Asia/Shanghai"}}}
	clientConn := dialPassthroughLifecycleClientWithPayload(t, server, localeWSResponseCreate(t, map[string]any{"tools": webSearch}, localeTestEnvironment, prose))
	defer func() { _ = clientConn.CloseNow() }()
	writeClient := func(payload string) {
		writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancelWrite()
		require.NoError(t, clientConn.Write(writeCtx, coderws.MessageText, []byte(payload)))
	}
	completeTurn := func(id string) {
		upstream.Send(`{"type":"response.completed","response":{"id":"` + id + `","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)
		completed, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
		require.NoError(t, err)
		require.Equal(t, "response.completed", gjson.GetBytes(completed, "type").String())
	}

	first := requirePassthroughUpstreamWrite(t, upstream, 3*time.Second)
	require.Equal(t, localeEnvironment("America/New_York"), gjson.GetBytes(first, "input.0.content.0.text").String())
	require.Equal(t, prose, gjson.GetBytes(first, "input.0.content.1.text").String())
	require.Equal(t, "America/New_York", gjson.GetBytes(first, "tools.0.user_location.timezone").String())
	completed, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "response.completed", gjson.GetBytes(completed, "type").String())

	// Only response.create is normalized; other frames are relayed untouched even
	// when they carry the same shapes.
	sessionUpdate := `{"type":"session.update","session":{"model":"gpt-5.1"},"tools":[{"type":"web_search","user_location":{"timezone":"Asia/Shanghai"}}],` +
		`"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"` + localeTestEnvironment + `"}]}]}`
	writeClient(sessionUpdate)
	relayedSession := requirePassthroughUpstreamWrite(t, upstream, 3*time.Second)
	require.Equal(t, "session.update", gjson.GetBytes(relayedSession, "type").String())
	require.Equal(t, localeTestEnvironment, gjson.GetBytes(relayedSession, "input.0.content.0.text").String())
	require.Equal(t, "Asia/Shanghai", gjson.GetBytes(relayedSession, "tools.0.user_location.timezone").String())

	writeClient(localeWSResponseCreate(t, map[string]any{"previous_response_id": "resp_tz_1"}, localeEnvironment("Asia/Tokyo"), prose))
	second := requirePassthroughUpstreamWrite(t, upstream, 3*time.Second)
	require.Equal(t, "response.create", gjson.GetBytes(second, "type").String())
	require.Equal(t, localeEnvironment("America/New_York"), gjson.GetBytes(second, "input.0.content.0.text").String())
	require.Equal(t, prose, gjson.GetBytes(second, "input.0.content.1.text").String())
	completeTurn("resp_tz_2")

	// The global switch is turned off elsewhere; once the snapshot expires the
	// open connection stops rewriting without being closed.
	repo.set("false", nil)
	expireLocaleSettingsSnapshot()
	writeClient(localeWSResponseCreate(t, map[string]any{"previous_response_id": "resp_tz_2"}, localeEnvironment("Asia/Tokyo"), prose))
	third := requirePassthroughUpstreamWrite(t, upstream, 3*time.Second)
	require.Equal(t, localeEnvironment("Asia/Tokyo"), gjson.GetBytes(third, "input.0.content.0.text").String())
	completeTurn("resp_tz_3")

	require.NoError(t, clientConn.Close(coderws.StatusNormalClosure, "done"))
	select {
	case <-serverErr:
	case <-time.After(3 * time.Second):
		t.Fatal("passthrough relay did not exit")
	}
}

func TestOpenAIWSCtxPoolRequestLocaleFollowsGlobalSwitchPerTurn(t *testing.T) {
	gin.SetMode(gin.TestMode)
	settings, repo := newLocaleSettingService(t, "true")
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	cfg.Gateway.OpenAIWS.QueueLimitPerConn = 8
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3
	captureConn := &openAIWSCaptureConn{events: [][]byte{
		[]byte(`{"type":"response.completed","response":{"id":"resp_pool_1","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`),
		[]byte(`{"type":"response.completed","response":{"id":"resp_pool_2","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`),
	}}
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(&openAIWSCaptureDialer{conn: captureConn})
	svc := &OpenAIGatewayService{
		cfg: cfg, httpUpstream: &httpUpstreamRecorder{}, cache: &stubGatewayCache{}, settingService: settings,
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg), toolCorrector: NewCodexToolCorrector(), openaiWSPool: pool,
	}
	account := &Account{
		ID: 116, Name: "openai-ingress-locale", Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test"},
		Extra: map[string]any{
			"responses_websockets_v2_enabled":           true,
			openAIRequestTimezoneRewriteEnabledExtraKey: true,
		},
	}
	serverErrCh := make(chan error, 1)
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeover})
		if err != nil {
			serverErrCh <- err
			return
		}
		defer func() { _ = conn.CloseNow() }()
		ginCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ginCtx.Request = r.Clone(r.Context())
		readCtx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		_, firstMessage, readErr := conn.Read(readCtx)
		cancel()
		if readErr != nil {
			serverErrCh <- readErr
			return
		}
		serverErrCh <- svc.ProxyResponsesWebSocketFromClient(r.Context(), ginCtx, conn, account, "sk-test", firstMessage, nil)
	}))
	defer wsServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http"), nil)
	cancelDial()
	require.NoError(t, err)
	defer func() { _ = clientConn.CloseNow() }()
	turn := func(fields map[string]any, responseID string) {
		writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
		require.NoError(t, clientConn.Write(writeCtx, coderws.MessageText, []byte(localeWSResponseCreate(t, fields, localeTestEnvironment))))
		cancelWrite()
		readCtx, cancelRead := context.WithTimeout(context.Background(), 3*time.Second)
		_, event, readErr := clientConn.Read(readCtx)
		cancelRead()
		require.NoError(t, readErr)
		require.Equal(t, responseID, gjson.GetBytes(event, "response.id").String())
	}

	turn(nil, "resp_pool_1")
	repo.set("false", nil)
	expireLocaleSettingsSnapshot()
	turn(map[string]any{"previous_response_id": "resp_pool_1"}, "resp_pool_2")
	_ = clientConn.Close(coderws.StatusNormalClosure, "done")
	select {
	case serverErr := <-serverErrCh:
		require.NoError(t, serverErr)
	case <-time.After(5 * time.Second):
		t.Fatal("ingress websocket did not exit")
	}

	require.Len(t, captureConn.writes, 2)
	require.Equal(t, localeEnvironment(DefaultOpenAIRequestTimezone), gjson.Get(requestToJSONString(captureConn.writes[0]), "input.0.content.0.text").String())
	require.Equal(t, localeTestEnvironment, gjson.Get(requestToJSONString(captureConn.writes[1]), "input.0.content.0.text").String())
}

type openAIRequestTimezoneAccountRepoStub struct {
	stubOpenAIAccountRepo
	getByIDsCalls   int
	bulkUpdateCalls int
	lastBulkUpdate  AccountBulkUpdate
	extraUpdates    []map[string]any
}

func (r *openAIRequestTimezoneAccountRepoStub) GetByIDs(ctx context.Context, ids []int64) ([]*Account, error) {
	r.getByIDsCalls++
	return r.stubOpenAIAccountRepo.GetByIDs(ctx, ids)
}

func (r *openAIRequestTimezoneAccountRepoStub) BulkUpdate(_ context.Context, ids []int64, updates AccountBulkUpdate) (int64, error) {
	r.bulkUpdateCalls++
	r.lastBulkUpdate = updates
	return int64(len(ids)), nil
}

func (r *openAIRequestTimezoneAccountRepoStub) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.extraUpdates = append(r.extraUpdates, updates)
	return nil
}

func TestBulkUpdateAccountsOpenAIRequestTimezone(t *testing.T) {
	parentID := int64(1)
	openAIOAuth := Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	openAIAPIKey := Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	anthropic := Account{ID: 3, Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	shadow := Account{ID: 4, Platform: PlatformOpenAI, Type: AccountTypeOAuth, ParentAccountID: &parentID}
	bulkUpdate := func(accounts []Account, extra map[string]any) (*openAIRequestTimezoneAccountRepoStub, *BulkUpdateAccountsResult, error) {
		repo := &openAIRequestTimezoneAccountRepoStub{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: accounts}}
		ids := make([]int64, 0, len(accounts))
		for _, account := range accounts {
			ids = append(ids, account.ID)
		}
		result, err := (&adminServiceImpl{accountRepo: repo}).BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{AccountIDs: ids, Extra: extra})
		return repo, result, err
	}
	enabledKey, timezoneKey := openAIRequestTimezoneRewriteEnabledExtraKey, openAIRequestTimezoneExtraKey

	t.Run("valid settings update OpenAI targets", func(t *testing.T) {
		repo, result, err := bulkUpdate([]Account{openAIOAuth, openAIAPIKey}, map[string]any{enabledKey: true, timezoneKey: "America/New_York"})
		require.NoError(t, err)
		require.Equal(t, 2, result.Success)
		require.Equal(t, 1, repo.getByIDsCalls, "targets are loaded once and shared with the other bulk guards")
		require.Equal(t, 1, repo.bulkUpdateCalls)
		require.Equal(t, true, repo.lastBulkUpdate.Extra[enabledKey])
		require.Equal(t, "America/New_York", repo.lastBulkUpdate.Extra[timezoneKey])
	})

	for name, tc := range map[string]struct {
		extra  map[string]any
		reason string
	}{
		"unknown timezone": {map[string]any{timezoneKey: "Invalid/Timezone"}, "INVALID_OPENAI_REQUEST_TIMEZONE"},
		"padded timezone":  {map[string]any{timezoneKey: " Asia/Tokyo"}, "INVALID_OPENAI_REQUEST_TIMEZONE"},
		"number timezone":  {map[string]any{timezoneKey: 8}, "INVALID_OPENAI_REQUEST_TIMEZONE"},
		"string switch":    {map[string]any{enabledKey: "true"}, "INVALID_OPENAI_REQUEST_TIMEZONE_REWRITE"},
		"number switch":    {map[string]any{enabledKey: 1, timezoneKey: "Asia/Tokyo"}, "INVALID_OPENAI_REQUEST_TIMEZONE_REWRITE"},
	} {
		t.Run("rejects "+name+" before write", func(t *testing.T) {
			repo, result, err := bulkUpdate([]Account{openAIOAuth}, tc.extra)
			require.Nil(t, result)
			require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
			require.Equal(t, tc.reason, infraerrors.Reason(err))
			require.Zero(t, repo.bulkUpdateCalls)
		})
	}

	for name, tc := range map[string]struct {
		targets []Account
		extra   map[string]any
	}{
		"non-OpenAI":            {[]Account{anthropic}, map[string]any{enabledKey: false}},
		"mixed platforms":       {[]Account{openAIOAuth, anthropic}, map[string]any{timezoneKey: "Asia/Tokyo"}},
		"shadow switch":         {[]Account{openAIOAuth, shadow}, map[string]any{enabledKey: true}},
		"shadow timezone":       {[]Account{shadow}, map[string]any{timezoneKey: "Asia/Tokyo"}},
		"shadow disabled value": {[]Account{shadow}, map[string]any{enabledKey: false}},
	} {
		t.Run("rejects "+name+" targets before write", func(t *testing.T) {
			repo, result, err := bulkUpdate(tc.targets, tc.extra)
			require.Nil(t, result)
			require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
			require.Equal(t, "OPENAI_BULK_TARGET_INVALID", infraerrors.Reason(err))
			require.Zero(t, repo.bulkUpdateCalls)
		})
	}

	t.Run("null clears stale shadow values like the single-account API", func(t *testing.T) {
		repo, result, err := bulkUpdate([]Account{shadow}, map[string]any{enabledKey: nil, timezoneKey: nil})
		require.NoError(t, err)
		require.Equal(t, 1, result.Success)
		require.Equal(t, 1, repo.bulkUpdateCalls)
	})

	t.Run("updates without the fields keep existing behavior", func(t *testing.T) {
		schedulable := false
		repo := &openAIRequestTimezoneAccountRepoStub{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{openAIOAuth, anthropic}}}
		result, err := (&adminServiceImpl{accountRepo: repo}).BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
			AccountIDs: []int64{1, 3}, Schedulable: &schedulable, Extra: map[string]any{"custom_note": "kept"},
		})
		require.NoError(t, err)
		require.Equal(t, 2, result.Success)
		require.Zero(t, repo.getByIDsCalls, "no OpenAI setting means no target lookup")
		require.Equal(t, 1, repo.bulkUpdateCalls)
		require.Equal(t, "kept", repo.lastBulkUpdate.Extra["custom_note"])
		require.NotContains(t, repo.lastBulkUpdate.Extra, enabledKey)
		require.NotContains(t, repo.lastBulkUpdate.Extra, timezoneKey)
	})
}

func TestUpdateAccountExtraOpenAIRequestTimezoneContract(t *testing.T) {
	parentID := int64(1)
	accounts := []Account{
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth},
		{ID: 2, Platform: PlatformAnthropic, Type: AccountTypeOAuth},
		{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuth, ParentAccountID: &parentID},
	}
	enabledKey, timezoneKey := openAIRequestTimezoneRewriteEnabledExtraKey, openAIRequestTimezoneExtraKey
	for _, tc := range []struct {
		name   string
		id     int64
		extra  map[string]any
		reason string
	}{
		{"OpenAI enable", 1, map[string]any{enabledKey: true, timezoneKey: "Asia/Tokyo"}, ""},
		{"OpenAI disable keeps timezone", 1, map[string]any{enabledKey: false}, ""},
		{"string switch", 1, map[string]any{enabledKey: "true"}, "INVALID_OPENAI_REQUEST_TIMEZONE_REWRITE"},
		{"non-OpenAI enable", 2, map[string]any{enabledKey: true}, "INVALID_OPENAI_REQUEST_TIMEZONE_REWRITE"},
		{"shadow write", 3, map[string]any{enabledKey: true}, "OPENAI_REQUEST_TIMEZONE_MANAGED_BY_PARENT"},
		{"shadow clear", 3, map[string]any{enabledKey: nil, timezoneKey: nil}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &openAIRequestTimezoneAccountRepoStub{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: accounts}}
			err := (&adminServiceImpl{accountRepo: repo}).UpdateAccountExtra(context.Background(), tc.id, tc.extra)
			require.Equal(t, tc.reason, localeErrorReason(err))
			if tc.reason == "" {
				require.Len(t, repo.extraUpdates, 1)
			} else {
				require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
				require.Empty(t, repo.extraUpdates)
			}
		})
	}
}
