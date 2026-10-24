//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeAccountOperatorReader struct {
	byID  map[int64][]*Account
	calls map[int64]int
}

func (f *fakeAccountOperatorReader) GetByID(_ context.Context, id int64) (*Account, error) {
	if f.calls == nil {
		f.calls = map[int64]int{}
	}
	versions := f.byID[id]
	if len(versions) == 0 {
		return nil, ErrAccountNotFound
	}
	index := f.calls[id]
	if index >= len(versions) {
		index = len(versions) - 1
	}
	f.calls[id]++
	return versions[index], nil
}

func (f *fakeAccountOperatorReader) GetByIDs(ctx context.Context, ids []int64) ([]*Account, error) {
	out := make([]*Account, 0, len(ids))
	for _, id := range ids {
		if account, err := f.GetByID(ctx, id); err == nil {
			out = append(out, account)
		}
	}
	return out, nil
}

type sentAccountOperatorEmail struct {
	to, subject, body string
}

type fakeAccountOperatorMailer struct {
	sent []sentAccountOperatorEmail
}

func (f *fakeAccountOperatorMailer) SendEmail(_ context.Context, to, subject, body string) error {
	f.sent = append(f.sent, sentAccountOperatorEmail{to: to, subject: subject, body: body})
	return nil
}

type fakeAccountOperatorSite struct{}

func (fakeAccountOperatorSite) GetSiteName(context.Context) string { return "XYAI" }
func (fakeAccountOperatorSite) GetFrontendURL(context.Context) string {
	return "https://ai.example.com/"
}

func newTestAccountOperatorActivityService(reader *fakeAccountOperatorReader, mailer *fakeAccountOperatorMailer, now time.Time) *AccountOperatorActivityService {
	return &AccountOperatorActivityService{
		accounts:  reader,
		mailer:    mailer,
		site:      fakeAccountOperatorSite{},
		recipient: AccountOperatorNotifyRecipient,
		now:       func() time.Time { return now },
		dispatch:  func(fn func()) { fn() },
	}
}

var testAccountOperatorActor = AccountOperatorActor{UserID: 7, Email: "ops@xunyou.com", ClientIP: "10.0.0.8", Method: "POST", Path: "/api/v1/admin/accounts/:id/test"}

func TestExtractAccountActivityRefs(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []AccountActivityRef
	}{
		{
			name: "single account response ignores nested group and proxy objects",
			body: `{"code":0,"data":{"id":12,"platform":"openai","type":"apikey","groups":[{"id":3,"platform":"openai","type":"x"}],"proxy":{"id":5}}}`,
			want: []AccountActivityRef{{AccountID: 12, Action: "created"}},
		},
		{
			name: "grok sso batch only reports created accounts",
			body: `{"code":0,"data":{"created":[{"index":1,"account":{"id":21,"platform":"grok","type":"oauth"}},{"index":2,"account":{"id":22,"platform":"grok","type":"oauth"}}],"failed":[{"index":3,"error":"bad"}]}}`,
			want: []AccountActivityRef{{AccountID: 21, Action: "created"}, {AccountID: 22, Action: "created"}},
		},
		{
			name: "codex session import reports created and updated items",
			body: `{"code":0,"data":{"total":3,"items":[{"index":1,"action":"created","account_id":31},{"index":2,"action":"updated","account_id":32},{"index":3,"action":"skipped"}]}}`,
			want: []AccountActivityRef{{AccountID: 31, Action: "created"}, {AccountID: 32, Action: "updated"}},
		},
		{name: "invalid json", body: `not-json`, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ExtractAccountActivityRefs([]byte(tt.body)))
		})
	}
}

func TestAccountOperatorTestWithoutStateChangeSendsNothing(t *testing.T) {
	now := time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC)
	account := &Account{ID: 9, Name: "claude-a", Platform: PlatformAnthropic, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{"access_token": "secret-a"}, Extra: map[string]any{"usage_snapshot": 1}}
	changedExtra := *account
	changedExtra.Extra = map[string]any{"usage_snapshot": 2}
	reader := &fakeAccountOperatorReader{byID: map[int64][]*Account{9: {account, &changedExtra}}}
	mailer := &fakeAccountOperatorMailer{}
	svc := newTestAccountOperatorActivityService(reader, mailer, now)

	before := svc.Snapshot(context.Background(), 9)
	svc.RecordChange(testAccountOperatorActor, AccountOperatorActivityTest, 9, before)

	require.Empty(t, mailer.sent, "runtime extra churn alone must not trigger a test notification")
}

func TestAccountOperatorTestRecoveringAccountSendsDiff(t *testing.T) {
	now := time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC)
	resetAt := now.Add(30 * time.Minute)
	limited := &Account{ID: 9, Name: "claude-a", Platform: PlatformAnthropic, Type: AccountTypeOAuth, Status: StatusError, ErrorMessage: "401 unauthorized",
		Schedulable: true, RateLimitResetAt: &resetAt, Credentials: map[string]any{"access_token": "secret-a"}}
	recovered := *limited
	recovered.Status = StatusActive
	recovered.ErrorMessage = ""
	recovered.RateLimitResetAt = nil
	reader := &fakeAccountOperatorReader{byID: map[int64][]*Account{9: {limited, &recovered}}}
	mailer := &fakeAccountOperatorMailer{}
	svc := newTestAccountOperatorActivityService(reader, mailer, now)

	svc.RecordChange(testAccountOperatorActor, AccountOperatorActivityTest, 9, svc.Snapshot(context.Background(), 9))

	require.Len(t, mailer.sent, 1)
	email := mailer.sent[0]
	require.Equal(t, AccountOperatorNotifyRecipient, email.to)
	require.Contains(t, email.subject, "测试连接后账号状态变化")
	require.Contains(t, email.body, "401 unauthorized")
	require.Contains(t, email.body, "限流至")
	require.Contains(t, email.body, "ops@xunyou.com")
	require.Contains(t, email.body, "https://ai.example.com/admin/accounts")
	require.NotContains(t, email.body, "secret-a")
}

func TestAccountOperatorReauthAlwaysNotifiesWithoutLeakingCredentials(t *testing.T) {
	now := time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC)
	before := &Account{ID: 5, Name: "gpt-main", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{"access_token": "old-token", "refresh_token": "old-refresh", "email": "a@corp.com"}}
	after := *before
	after.Credentials = map[string]any{"access_token": "new-token", "refresh_token": "new-refresh", "email": "b@other.com"}
	reader := &fakeAccountOperatorReader{byID: map[int64][]*Account{5: {before, &after}}}
	mailer := &fakeAccountOperatorMailer{}
	svc := newTestAccountOperatorActivityService(reader, mailer, now)

	svc.RecordChange(testAccountOperatorActor, AccountOperatorActivityReauth, 5, svc.Snapshot(context.Background(), 5))

	require.Len(t, mailer.sent, 1)
	body := mailer.sent[0].body
	require.Contains(t, mailer.sent[0].subject, "重新授权账号")
	require.Contains(t, body, "email=a@corp.com")
	require.Contains(t, body, "email=b@other.com")
	require.Contains(t, body, "access_token, email, refresh_token 已变更")
	for _, secret := range []string{"old-token", "new-token", "old-refresh", "new-refresh"} {
		require.NotContains(t, body, secret)
	}
}

func TestAccountOperatorRecordCreatedAggregatesAndHighlightsCustomBaseURL(t *testing.T) {
	now := time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC)
	proxyID := int64(4)
	official := &Account{ID: 1, Name: "official", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive,
		Credentials: map[string]any{"api_key": "sk-official", "base_url": "https://api.openai.com"}}
	custom := &Account{ID: 2, Name: "<b>relay</b>", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Priority: 1,
		ProxyID: &proxyID, Proxy: &Proxy{ID: 4, Name: "hk"}, Groups: []*Group{{ID: 3, Name: "prod"}},
		Credentials: map[string]any{"api_key": "sk-relay", "base_url": "https://relay.example.net/v1"}}
	reader := &fakeAccountOperatorReader{byID: map[int64][]*Account{1: {official}, 2: {custom}}}
	mailer := &fakeAccountOperatorMailer{}
	svc := newTestAccountOperatorActivityService(reader, mailer, now)

	svc.RecordCreated(testAccountOperatorActor, []AccountActivityRef{{AccountID: 1, Action: "created"}, {AccountID: 2, Action: "updated"}})

	require.Len(t, mailer.sent, 1)
	email := mailer.sent[0]
	require.Equal(t, "[XYAI] 子管理员新增/导入账号：official (#1) 等 2 个", email.subject)
	require.Contains(t, email.body, "导入更新已有账号")
	require.Contains(t, email.body, "&lt;b&gt;relay&lt;/b&gt;")
	require.NotContains(t, email.body, "<b>relay</b>")
	require.Contains(t, email.body, "prod (#3)")
	require.Contains(t, email.body, "hk (#4)")
	require.Equal(t, 1, strings.Count(email.body, "非官方地址"))
	require.NotContains(t, email.body, "sk-relay")
}

func TestAccountOperatorWithoutMailerIsNoop(t *testing.T) {
	svc := NewAccountOperatorActivityService(nil, nil, nil)
	svc.dispatch = func(func()) { t.Fatal("must not dispatch without mailer") }
	svc.RecordCreated(testAccountOperatorActor, []AccountActivityRef{{AccountID: 1, Action: "created"}})
	svc.RecordChange(testAccountOperatorActor, AccountOperatorActivityReauth, 1, nil)
	require.Nil(t, svc.Snapshot(context.Background(), 1))
}

func TestSubAdminGrokQuotaProbeDoesNotClearRateLimit(t *testing.T) {
	for _, tc := range []struct {
		name       string
		ctx        context.Context
		wantClears int
	}{
		{"sub admin read keeps rate limit", WithSubAdminAccountRestrictions(context.Background()), 0},
		{"full admin read keeps existing recovery", context.Background(), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account := healthyGrokQuotaOAuthAccount(45)
			observedResetAt := time.Now().Add(-time.Second).UTC().Truncate(time.Second)
			observedLimitedAt := observedResetAt.Add(-grokRateLimitRepeatCooldown)
			account.RateLimitedAt = &observedLimitedAt
			account.RateLimitResetAt = &observedResetAt
			repo := &grokQuotaAccountRepo{
				mockAccountRepoForPlatform: &mockAccountRepoForPlatform{accountsByID: map[int64]*Account{45: account}},
				recoveryClearResult:        true,
			}
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader(`{"id":"resp_probe"}`)),
			}}
			svc := NewGrokQuotaService(repo, nil, NewGrokTokenProvider(repo, nil), upstream, nil)

			_, err := svc.ProbeUsage(tc.ctx, 45)
			require.NoError(t, err)
			require.Equal(t, tc.wantClears, repo.recoveryClearCalls)
			require.Contains(t, repo.updates[45], grokQuotaSnapshotExtraKey, "the quota snapshot itself is still cached")
		})
	}
}
