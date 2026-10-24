package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// AccountOperatorNotifyRecipient 子管理员账号运维操作的固定通知收件人。
const AccountOperatorNotifyRecipient = "huangyu@xunyou.com"

const accountOperatorNotifyTimeout = 30 * time.Second

type subAdminAccountRestrictionsKey struct{}

// WithSubAdminAccountRestrictions 标记请求来自子管理员，只由 AdminAuth 设置：
// 账号读取不得顺带恢复账号状态（清错误、清限流），新增类导入不得改写已有账号。
func WithSubAdminAccountRestrictions(ctx context.Context) context.Context {
	return context.WithValue(ctx, subAdminAccountRestrictionsKey{}, true)
}

func HasSubAdminAccountRestrictions(ctx context.Context) bool {
	restricted, _ := ctx.Value(subAdminAccountRestrictionsKey{}).(bool)
	return restricted
}

// AccountOperatorActivityKind 描述子管理员账号写操作的通知方式。
type AccountOperatorActivityKind string

const (
	// AccountOperatorActivityNone 无持久副作用：OAuth 链接/换码、预览、只读批量查询。
	AccountOperatorActivityNone AccountOperatorActivityKind = "none"
	// AccountOperatorActivityCreate 响应体携带新增或被导入更新的账号。
	AccountOperatorActivityCreate AccountOperatorActivityKind = "create"
	// AccountOperatorActivityTest 测试连接，账号状态有变化时通知。
	AccountOperatorActivityTest AccountOperatorActivityKind = "test"
	// AccountOperatorActivityReauth 重新授权，始终通知。
	AccountOperatorActivityReauth AccountOperatorActivityKind = "reauth"
)

// accountOperatorActivityRoutes 必须覆盖 AdminPermissionAccountsOperator 白名单中的全部非 GET 路由。
var accountOperatorActivityRoutes = map[adminRouteRule]AccountOperatorActivityKind{
	{"POST", "/api/v1/admin/accounts"}:                              AccountOperatorActivityCreate,
	{"POST", "/api/v1/admin/accounts/import/codex-session"}:         AccountOperatorActivityCreate,
	{"POST", "/api/v1/admin/openai/create-from-codex-pat"}:          AccountOperatorActivityCreate,
	{"POST", "/api/v1/admin/grok/sso-to-oauth"}:                     AccountOperatorActivityCreate,
	{"POST", "/api/v1/admin/accounts/:id/test"}:                     AccountOperatorActivityTest,
	{"POST", "/api/v1/admin/accounts/:id/apply-oauth-credentials"}:  AccountOperatorActivityReauth,
	{"POST", "/api/v1/admin/accounts/usage/batch"}:                  AccountOperatorActivityNone,
	{"POST", "/api/v1/admin/accounts/today-stats/batch"}:            AccountOperatorActivityNone,
	{"POST", "/api/v1/admin/accounts/check-mixed-channel"}:          AccountOperatorActivityNone,
	{"POST", "/api/v1/admin/accounts/models/sync-upstream-preview"}: AccountOperatorActivityNone,
	{"POST", "/api/v1/admin/accounts/generate-auth-url"}:            AccountOperatorActivityNone,
	{"POST", "/api/v1/admin/accounts/generate-setup-token-url"}:     AccountOperatorActivityNone,
	{"POST", "/api/v1/admin/accounts/exchange-code"}:                AccountOperatorActivityNone,
	{"POST", "/api/v1/admin/accounts/exchange-setup-token-code"}:    AccountOperatorActivityNone,
	{"POST", "/api/v1/admin/accounts/cookie-auth"}:                  AccountOperatorActivityNone,
	{"POST", "/api/v1/admin/accounts/setup-token-cookie-auth"}:      AccountOperatorActivityNone,
	{"POST", "/api/v1/admin/openai/generate-auth-url"}:              AccountOperatorActivityNone,
	{"POST", "/api/v1/admin/openai/exchange-code"}:                  AccountOperatorActivityNone,
	{"POST", "/api/v1/admin/openai/refresh-token"}:                  AccountOperatorActivityNone,
	{"POST", "/api/v1/admin/gemini/oauth/auth-url"}:                 AccountOperatorActivityNone,
	{"POST", "/api/v1/admin/gemini/oauth/exchange-code"}:            AccountOperatorActivityNone,
	{"POST", "/api/v1/admin/antigravity/oauth/auth-url"}:            AccountOperatorActivityNone,
	{"POST", "/api/v1/admin/antigravity/oauth/exchange-code"}:       AccountOperatorActivityNone,
	{"POST", "/api/v1/admin/antigravity/oauth/refresh-token"}:       AccountOperatorActivityNone,
	{"POST", "/api/v1/admin/grok/oauth/auth-url"}:                   AccountOperatorActivityNone,
	{"POST", "/api/v1/admin/grok/oauth/exchange-code"}:              AccountOperatorActivityNone,
	{"POST", "/api/v1/admin/grok/oauth/refresh-token"}:              AccountOperatorActivityNone,
	{"POST", "/api/v1/admin/grok/oauth/sso-token"}:                  AccountOperatorActivityNone,
	{"POST", "/api/v1/admin/grok/oauth/password"}:                   AccountOperatorActivityNone,
}

// AccountOperatorActivityKindForRoute 返回子管理员账号写路由的通知方式；未归类返回 false。
func AccountOperatorActivityKindForRoute(method, route string) (AccountOperatorActivityKind, bool) {
	kind, ok := accountOperatorActivityRoutes[adminRouteRule{method: strings.ToUpper(strings.TrimSpace(method)), route: route}]
	return kind, ok
}

// AccountOperatorActor 是触发通知的子管理员请求信息。
type AccountOperatorActor struct {
	UserID   int64
	Email    string
	ClientIP string
	Method   string
	Path     string
}

// AccountActivityRef 是新增类接口响应中识别出的账号。
type AccountActivityRef struct {
	AccountID int64
	Action    string
}

// AccountActivitySnapshot 是账号在操作前后的非敏感视图；凭据和 extra 只保留逐 key 摘要。
type AccountActivitySnapshot struct {
	ID             int64
	Name           string
	Platform       string
	Type           string
	Status         string
	ErrorMessage   string
	Schedulable    bool
	Priority       int
	Concurrency    int
	LoadFactor     string
	RateMultiplier string
	Proxy          string
	Groups         string
	BaseURL        string
	Identity       string
	ExpiresAt      string
	RuntimeState   string

	credentialDigests map[string]string
	extraDigests      map[string]string
}

type accountOperatorAccountReader interface {
	GetByID(ctx context.Context, id int64) (*Account, error)
	GetByIDs(ctx context.Context, ids []int64) ([]*Account, error)
}

type accountOperatorMailer interface {
	SendEmail(ctx context.Context, to, subject, body string) error
}

type accountOperatorSiteInfo interface {
	GetSiteName(ctx context.Context) string
	GetFrontendURL(ctx context.Context) string
}

// AccountOperatorActivityService 把子管理员造成的账号变动发邮件给固定收件人。
type AccountOperatorActivityService struct {
	accounts  accountOperatorAccountReader
	mailer    accountOperatorMailer
	site      accountOperatorSiteInfo
	recipient string
	now       func() time.Time
	dispatch  func(func())
}

func NewAccountOperatorActivityService(accountRepo AccountRepository, emailService *EmailService, settingService *SettingService) *AccountOperatorActivityService {
	s := &AccountOperatorActivityService{
		recipient: AccountOperatorNotifyRecipient,
		now:       time.Now,
		dispatch:  func(fn func()) { go fn() },
	}
	if accountRepo != nil {
		s.accounts = accountRepo
	}
	if emailService != nil {
		s.mailer = emailService
	}
	if settingService != nil {
		s.site = settingService
	}
	return s
}

// Snapshot 读取账号当前状态；读取失败返回 nil，后续邮件按"操作前未知"处理。
func (s *AccountOperatorActivityService) Snapshot(ctx context.Context, accountID int64) *AccountActivitySnapshot {
	if s == nil || s.accounts == nil || accountID <= 0 {
		return nil
	}
	account, err := s.accounts.GetByID(ctx, accountID)
	if err != nil || account == nil {
		return nil
	}
	return newAccountActivitySnapshot(account, s.now())
}

// RecordChange 异步读取操作后的账号状态并通知。重新授权始终通知；测试连接仅在状态变化时通知。
func (s *AccountOperatorActivityService) RecordChange(actor AccountOperatorActor, kind AccountOperatorActivityKind, accountID int64, before *AccountActivitySnapshot) {
	if s == nil || (kind != AccountOperatorActivityTest && kind != AccountOperatorActivityReauth) {
		return
	}
	s.run(func(ctx context.Context) {
		after := s.Snapshot(ctx, accountID)
		changes := diffAccountActivitySnapshots(before, after)
		if kind == AccountOperatorActivityTest && !hasSignificantAccountActivityChange(changes) {
			return
		}
		name := fmt.Sprintf("#%d", accountID)
		if after != nil {
			name = fmt.Sprintf("%s (#%d)", after.Name, accountID)
		}
		title := "重新授权账号"
		if kind == AccountOperatorActivityTest {
			title = "测试连接后账号状态变化"
		}
		section := accountActivitySectionHTML(title+" "+name, after, changes)
		s.send(ctx, actor, title+"："+name, section)
	})
}

// RecordCreated 异步通知新增类接口创建或导入更新的账号，一次请求合并为一封邮件。
func (s *AccountOperatorActivityService) RecordCreated(actor AccountOperatorActor, refs []AccountActivityRef) {
	if s == nil || len(refs) == 0 {
		return
	}
	s.run(func(ctx context.Context) {
		ids := make([]int64, 0, len(refs))
		for _, ref := range refs {
			ids = append(ids, ref.AccountID)
		}
		byID := make(map[int64]*Account, len(ids))
		if s.accounts != nil {
			accounts, err := s.accounts.GetByIDs(ctx, ids)
			if err != nil {
				slog.Warn("account_operator_notify_load_failed", "error", err, "account_ids", ids)
			}
			for _, account := range accounts {
				if account != nil {
					byID[account.ID] = account
				}
			}
		}
		var sections strings.Builder
		names := make([]string, 0, len(refs))
		now := s.now()
		for _, ref := range refs {
			var snapshot *AccountActivitySnapshot
			name := fmt.Sprintf("#%d", ref.AccountID)
			if account := byID[ref.AccountID]; account != nil {
				snapshot = newAccountActivitySnapshot(account, now)
				name = fmt.Sprintf("%s (#%d)", snapshot.Name, ref.AccountID)
			}
			names = append(names, name)
			title := "新增账号 "
			if ref.Action == "updated" {
				title = "导入更新已有账号 "
			}
			sections.WriteString(accountActivitySectionHTML(title+name, snapshot, nil))
		}
		subject := "新增账号：" + names[0]
		if len(names) > 1 {
			subject = fmt.Sprintf("新增/导入账号：%s 等 %d 个", names[0], len(names))
		}
		s.send(ctx, actor, subject, sections.String())
	})
}

func (s *AccountOperatorActivityService) run(fn func(ctx context.Context)) {
	if s.mailer == nil {
		return
	}
	s.dispatch(func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.Error("account_operator_notify_panic", "panic", recovered)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), accountOperatorNotifyTimeout)
		defer cancel()
		fn(ctx)
	})
}

func (s *AccountOperatorActivityService) send(ctx context.Context, actor AccountOperatorActor, subject, sections string) {
	siteName := defaultSiteName
	link := ""
	if s.site != nil {
		if name := strings.TrimSpace(s.site.GetSiteName(ctx)); name != "" {
			siteName = name
		}
		if base := strings.TrimRight(strings.TrimSpace(s.site.GetFrontendURL(ctx)), "/"); base != "" {
			link = base + "/admin/accounts"
		}
	}
	body := buildAccountOperatorEmailBody(siteName, actor, s.now(), sections, link)
	if err := s.mailer.SendEmail(ctx, s.recipient, fmt.Sprintf("[%s] 子管理员%s", siteName, subject), body); err != nil {
		slog.Warn("account_operator_notify_send_failed", "error", err, "operator_id", actor.UserID, "route", actor.Path)
	}
}

// ExtractAccountActivityRefs 从新增类接口的成功响应中找出账号：
// 账号 DTO（含 id/platform/type）或导入结果项（含 account_id/action）。
func ExtractAccountActivityRefs(body []byte) []AccountActivityRef {
	var envelope struct {
		Data any `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil
	}
	var refs []AccountActivityRef
	seen := make(map[int64]struct{})
	add := func(id int64, action string) {
		if id <= 0 {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		refs = append(refs, AccountActivityRef{AccountID: id, Action: action})
	}
	var walk func(value any)
	walk = func(value any) {
		switch v := value.(type) {
		case []any:
			for _, item := range v {
				walk(item)
			}
		case map[string]any:
			if id, ok := jsonPositiveInt(v["id"]); ok && isAccountActivityText(v["platform"]) && isAccountActivityText(v["type"]) {
				add(id, "created")
				return
			}
			if id, ok := jsonPositiveInt(v["account_id"]); ok {
				if action, _ := v["action"].(string); action == "created" || action == "updated" {
					add(id, action)
				}
				return
			}
			keys := make([]string, 0, len(v))
			for key := range v {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				walk(v[key])
			}
		}
	}
	walk(envelope.Data)
	return refs
}

func jsonPositiveInt(value any) (int64, bool) {
	number, ok := value.(float64)
	if !ok || number <= 0 || number != float64(int64(number)) {
		return 0, false
	}
	return int64(number), true
}

func isAccountActivityText(value any) bool {
	text, ok := value.(string)
	return ok && text != ""
}

var accountActivityIdentityKeys = []string{"email_address", "email", "account_email", "chatgpt_account_id", "account_uuid", "org_uuid", "project_id"}

func newAccountActivitySnapshot(account *Account, now time.Time) *AccountActivitySnapshot {
	snapshot := &AccountActivitySnapshot{
		ID:                account.ID,
		Name:              account.Name,
		Platform:          account.Platform,
		Type:              account.Type,
		Status:            account.Status,
		ErrorMessage:      account.ErrorMessage,
		Schedulable:       account.Schedulable,
		Priority:          account.Priority,
		Concurrency:       account.Concurrency,
		LoadFactor:        "-",
		RateMultiplier:    "1",
		Proxy:             "直连",
		Groups:            "-",
		BaseURL:           "-",
		ExpiresAt:         formatAccountActivityTime(account.ExpiresAt),
		credentialDigests: accountActivityDigests(account.Credentials),
		extraDigests:      accountActivityDigests(account.Extra),
	}
	if account.LoadFactor != nil {
		snapshot.LoadFactor = strconv.Itoa(*account.LoadFactor)
	}
	if account.RateMultiplier != nil {
		snapshot.RateMultiplier = strconv.FormatFloat(*account.RateMultiplier, 'f', -1, 64)
	}
	if account.Proxy != nil {
		snapshot.Proxy = fmt.Sprintf("%s (#%d)", account.Proxy.Name, account.Proxy.ID)
	} else if account.ProxyID != nil {
		snapshot.Proxy = fmt.Sprintf("#%d", *account.ProxyID)
	}
	groups := make([]string, 0, len(account.Groups))
	for _, group := range account.Groups {
		if group != nil {
			groups = append(groups, fmt.Sprintf("%s (#%d)", group.Name, group.ID))
		}
	}
	if len(groups) == 0 {
		for _, id := range account.GroupIDs {
			groups = append(groups, fmt.Sprintf("#%d", id))
		}
	}
	if len(groups) > 0 {
		sort.Strings(groups)
		snapshot.Groups = strings.Join(groups, ", ")
	}
	if baseURL, _ := account.Credentials["base_url"].(string); strings.TrimSpace(baseURL) != "" {
		snapshot.BaseURL = strings.TrimSpace(baseURL)
	}
	identity := make([]string, 0, 2)
	for _, key := range accountActivityIdentityKeys {
		for _, source := range []map[string]any{account.Credentials, account.Extra} {
			if value, ok := source[key].(string); ok && strings.TrimSpace(value) != "" {
				identity = append(identity, key+"="+strings.TrimSpace(value))
				break
			}
		}
	}
	if len(identity) > 0 {
		snapshot.Identity = strings.Join(identity, ", ")
	} else {
		snapshot.Identity = "-"
	}
	runtime := make([]string, 0, 3)
	if account.RateLimitResetAt != nil && account.RateLimitResetAt.After(now) {
		runtime = append(runtime, "限流至 "+formatAccountActivityTime(account.RateLimitResetAt))
	}
	if account.OverloadUntil != nil && account.OverloadUntil.After(now) {
		runtime = append(runtime, "过载至 "+formatAccountActivityTime(account.OverloadUntil))
	}
	if account.TempUnschedulableUntil != nil && account.TempUnschedulableUntil.After(now) {
		state := "临时不可调度至 " + formatAccountActivityTime(account.TempUnschedulableUntil)
		if reason := strings.TrimSpace(account.TempUnschedulableReason); reason != "" {
			state += "（" + reason + "）"
		}
		runtime = append(runtime, state)
	}
	if len(runtime) > 0 {
		snapshot.RuntimeState = strings.Join(runtime, "；")
	} else {
		snapshot.RuntimeState = "正常"
	}
	if snapshot.ErrorMessage == "" {
		snapshot.ErrorMessage = "-"
	}
	return snapshot
}

func accountActivityDigests(values map[string]any) map[string]string {
	digests := make(map[string]string, len(values))
	for key, value := range values {
		raw, err := json.Marshal(value)
		if err != nil {
			raw = []byte(fmt.Sprint(value))
		}
		sum := sha256.Sum256(raw)
		digests[key] = hex.EncodeToString(sum[:])
	}
	return digests
}

func formatAccountActivityTime(value *time.Time) string {
	if value == nil || value.IsZero() {
		return "-"
	}
	return value.Local().Format("2006-01-02 15:04:05")
}

type accountActivityChange struct {
	Label       string
	Before      string
	After       string
	Significant bool
}

func accountActivityFields(snapshot *AccountActivitySnapshot) [][2]string {
	if snapshot == nil {
		return nil
	}
	return [][2]string{
		{"名称", snapshot.Name},
		{"平台 / 类型", snapshot.Platform + " / " + snapshot.Type},
		{"状态", snapshot.Status},
		{"错误信息", snapshot.ErrorMessage},
		{"可调度", strconv.FormatBool(snapshot.Schedulable)},
		{"运行状态", snapshot.RuntimeState},
		{"上游身份", snapshot.Identity},
		{"上游地址", snapshot.BaseURL},
		{"分组", snapshot.Groups},
		{"代理", snapshot.Proxy},
		{"优先级", strconv.Itoa(snapshot.Priority)},
		{"并发", strconv.Itoa(snapshot.Concurrency)},
		{"负载因子", snapshot.LoadFactor},
		{"计费倍率", snapshot.RateMultiplier},
		{"过期时间", snapshot.ExpiresAt},
	}
}

func diffAccountActivitySnapshots(before, after *AccountActivitySnapshot) []accountActivityChange {
	if before == nil || after == nil {
		label := "账号快照"
		state := "操作前未能读取"
		if after == nil {
			state = "操作后未能读取（可能已不存在）"
		}
		return []accountActivityChange{{Label: label, Before: "-", After: state, Significant: true}}
	}
	beforeFields := accountActivityFields(before)
	afterFields := accountActivityFields(after)
	var changes []accountActivityChange
	for i := range afterFields {
		if beforeFields[i][1] != afterFields[i][1] {
			changes = append(changes, accountActivityChange{Label: afterFields[i][0], Before: beforeFields[i][1], After: afterFields[i][1], Significant: true})
		}
	}
	if keys := changedAccountActivityKeys(before.credentialDigests, after.credentialDigests); keys != "" {
		changes = append(changes, accountActivityChange{Label: "凭据字段", Before: "-", After: keys + " 已变更", Significant: true})
	}
	if keys := changedAccountActivityKeys(before.extraDigests, after.extraDigests); keys != "" {
		changes = append(changes, accountActivityChange{Label: "附加配置字段", Before: "-", After: keys + " 已变更"})
	}
	return changes
}

func hasSignificantAccountActivityChange(changes []accountActivityChange) bool {
	for _, change := range changes {
		if change.Significant {
			return true
		}
	}
	return false
}

func changedAccountActivityKeys(before, after map[string]string) string {
	keys := make([]string, 0)
	for key, digest := range after {
		if previous, ok := before[key]; !ok {
			keys = append(keys, "+"+key)
		} else if previous != digest {
			keys = append(keys, key)
		}
	}
	for key := range before {
		if _, ok := after[key]; !ok {
			keys = append(keys, "-"+key)
		}
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}

var accountActivityOfficialHosts = map[string]struct{}{
	"api.anthropic.com":                 {},
	"api.openai.com":                    {},
	"chatgpt.com":                       {},
	"generativelanguage.googleapis.com": {},
	"api.x.ai":                          {},
	"api.deepseek.com":                  {},
	"api.moonshot.cn":                   {},
	"api.moonshot.ai":                   {},
	"open.bigmodel.cn":                  {},
}

func isOfficialAccountBaseURL(raw string) bool {
	if raw == "" || raw == "-" {
		return true
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	_, ok := accountActivityOfficialHosts[strings.ToLower(parsed.Hostname())]
	return ok
}

func accountActivitySectionHTML(title string, snapshot *AccountActivitySnapshot, changes []accountActivityChange) string {
	const cell = `padding:8px 10px;border-bottom:1px solid #eef0f4;vertical-align:top;`
	var b strings.Builder
	b.WriteString(`<h2 style="margin:28px 0 12px;font-size:18px;">` + html.EscapeString(title) + `</h2>`)
	if len(changes) > 0 {
		b.WriteString(`<table style="width:100%;border-collapse:collapse;font-size:14px;margin-bottom:12px;background:#fff7ed;">`)
		b.WriteString(`<tr><th style="` + cell + `text-align:left;color:#9a3412;">变更项</th><th style="` + cell + `text-align:left;color:#9a3412;">操作前</th><th style="` + cell + `text-align:left;color:#9a3412;">操作后</th></tr>`)
		for _, change := range changes {
			b.WriteString(`<tr><td style="` + cell + `color:#666;">` + html.EscapeString(change.Label) + `</td><td style="` + cell + `">` + html.EscapeString(change.Before) + `</td><td style="` + cell + `font-weight:600;color:#c2410c;">` + html.EscapeString(change.After) + `</td></tr>`)
		}
		b.WriteString(`</table>`)
	}
	if snapshot == nil {
		b.WriteString(`<p style="color:#b91c1c;">未能读取账号当前状态，请到后台核对。</p>`)
		return b.String()
	}
	b.WriteString(`<table style="width:100%;border-collapse:collapse;font-size:14px;">`)
	for _, field := range accountActivityFields(snapshot) {
		value := html.EscapeString(field[1])
		if field[0] == "上游地址" && !isOfficialAccountBaseURL(field[1]) {
			value = `<span style="color:#dc2626;font-weight:700;">` + value + `（非官方地址，用户请求会转发到这里）</span>`
		}
		b.WriteString(`<tr><td style="` + cell + `width:120px;color:#666;">` + html.EscapeString(field[0]) + `</td><td style="` + cell + `">` + value + `</td></tr>`)
	}
	b.WriteString(`</table>`)
	return b.String()
}

func buildAccountOperatorEmailBody(siteName string, actor AccountOperatorActor, at time.Time, sections, link string) string {
	operator := strings.TrimSpace(actor.Email)
	if operator == "" {
		operator = "-"
	}
	clientIP := strings.TrimSpace(actor.ClientIP)
	if clientIP == "" {
		clientIP = "-"
	}
	linkBlock := ""
	if link != "" {
		linkBlock = `<p style="margin-top:24px;"><a href="` + html.EscapeString(link) + `" style="color:#2563eb;">打开账号管理</a></p>`
	}
	return fmt.Sprintf(`<!doctype html>
<html>
<body style="margin:0;padding:0;background:#f5f6fb;color:#222;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Arial,sans-serif;">
  <div style="max-width:760px;margin:0 auto;padding:32px 20px;">
    <div style="background:#fff;border-radius:14px;padding:32px 36px;box-shadow:0 8px 28px rgba(15,23,42,.08);">
      <div style="letter-spacing:3px;color:#999;font-size:13px;">SUB-ADMIN ACCOUNT ACTIVITY / 子管理员账号操作</div>
      <table style="width:100%%;border-collapse:collapse;font-size:14px;margin-top:18px;">
        <tr><td style="padding:6px 0;width:120px;color:#666;">操作人</td><td style="padding:6px 0;">%s（用户 #%d）</td></tr>
        <tr><td style="padding:6px 0;color:#666;">时间</td><td style="padding:6px 0;">%s</td></tr>
        <tr><td style="padding:6px 0;color:#666;">接口</td><td style="padding:6px 0;">%s %s</td></tr>
        <tr><td style="padding:6px 0;color:#666;">来源 IP</td><td style="padding:6px 0;">%s</td></tr>
      </table>
      %s
      %s
      <p style="font-size:13px;line-height:1.8;color:#888;margin-top:28px;">此邮件由 %s 自动发送，不包含任何凭据内容。</p>
    </div>
  </div>
</body>
</html>`,
		html.EscapeString(operator),
		actor.UserID,
		html.EscapeString(at.Local().Format("2006-01-02 15:04:05")),
		html.EscapeString(actor.Method),
		html.EscapeString(actor.Path),
		html.EscapeString(clientIP),
		sections,
		linkBlock,
		html.EscapeString(siteName),
	)
}
