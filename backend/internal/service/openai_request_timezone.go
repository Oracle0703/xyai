package service

import (
	_ "embed"
	"net/http"
	"sort"
	"strings"
	"time"
	_ "time/tzdata"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/gin-gonic/gin"
)

const DefaultOpenAIRequestTimezone = "America/Los_Angeles"
const openAIRequestTimezoneExtraKey = "openai_request_timezone"
const openAIRequestTimezoneRewriteEnabledExtraKey = "openai_request_timezone_rewrite_enabled"

//go:embed openai_request_timezones.txt
var openAIRequestTimezoneNames string

var openAIRequestTimezoneOptions = strings.Fields(openAIRequestTimezoneNames)
var openAIRequestTimezoneAllowed = func() map[string]struct{} {
	allowed := make(map[string]struct{}, len(openAIRequestTimezoneOptions))
	for _, name := range openAIRequestTimezoneOptions {
		allowed[name] = struct{}{}
	}
	return allowed
}()

func OpenAIRequestTimezoneOptions() []string {
	options := append([]string(nil), openAIRequestTimezoneOptions...)
	sort.Strings(options)
	return options
}

// ValidateOpenAIRequestTimezoneExtra validates the per-account request timezone
// settings present in extra for every admin write path. The switch must be a JSON
// boolean and can only be enabled on OpenAI accounts; a retained timezone must be
// allowlisted. Spark shadows follow their parent, so any non-null value is
// rejected instead of being stored and silently ignored. A null value clears the key.
func ValidateOpenAIRequestTimezoneExtra(platform string, shadow bool, extra map[string]any) error {
	if extra == nil {
		return nil
	}
	enabledRaw := extra[openAIRequestTimezoneRewriteEnabledExtraKey]
	timezoneRaw := extra[openAIRequestTimezoneExtraKey]
	if shadow && (enabledRaw != nil || timezoneRaw != nil) {
		return infraerrors.New(http.StatusBadRequest, "OPENAI_REQUEST_TIMEZONE_MANAGED_BY_PARENT", "Spark shadow accounts use the parent account's request timezone settings")
	}
	if enabledRaw != nil {
		enabled, ok := enabledRaw.(bool)
		if !ok || (enabled && platform != PlatformOpenAI) {
			return infraerrors.New(http.StatusBadRequest, "INVALID_OPENAI_REQUEST_TIMEZONE_REWRITE", "openai_request_timezone_rewrite_enabled must be a boolean and can only be enabled for an OpenAI account")
		}
	}
	if timezoneRaw != nil {
		name, ok := timezoneRaw.(string)
		if !ok || (name != "" && (platform != PlatformOpenAI || !isAllowedOpenAIRequestTimezone(name))) {
			return infraerrors.New(http.StatusBadRequest, "INVALID_OPENAI_REQUEST_TIMEZONE", "openai_request_timezone must be an allowed IANA timezone for an OpenAI account")
		}
	}
	return nil
}

func isAllowedOpenAIRequestTimezone(name string) bool {
	if _, ok := openAIRequestTimezoneAllowed[name]; !ok {
		return false
	}
	_, err := time.LoadLocation(name)
	return err == nil
}

// openAIRequestTimezoneSource returns the account whose settings govern a request
// routed to account. Spark shadows have no settings of their own and follow the
// parent that prepareCodexAccountIdentitySource staged for the current attempt, so
// a parent change applies from the next attempt without syncing shadow rows. It
// returns nil when that parent is not staged, which leaves the request untouched.
func openAIRequestTimezoneSource(c *gin.Context, account *Account) *Account {
	if !account.IsShadow() {
		return account
	}
	if parent := codexAccountIdentitySource(c, nil); parent != nil && parent.ID == *account.ParentAccountID {
		return parent
	}
	return nil
}

// OpenAIRequestTimezoneRewriteEnabled reports the account switch. Only an explicit
// JSON true on an OpenAI account enables it; missing, null or any other value is off.
func (account *Account) OpenAIRequestTimezoneRewriteEnabled() bool {
	if account == nil || !account.IsOpenAI() || account.Extra == nil {
		return false
	}
	enabled, ok := account.Extra[openAIRequestTimezoneRewriteEnabledExtraKey].(bool)
	return ok && enabled
}

// OpenAIRequestTimezone returns the target timezone, independent of whether the
// rewrite is enabled: a retained allowlisted value, otherwise the default.
func (account *Account) OpenAIRequestTimezone() string {
	if account != nil && account.IsOpenAI() {
		if name := account.getExtraString(openAIRequestTimezoneExtraKey); isAllowedOpenAIRequestTimezone(name) {
			return name
		}
	}
	return DefaultOpenAIRequestTimezone
}
