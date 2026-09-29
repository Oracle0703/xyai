package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ---- fakes ----

type fakeGPTQuotaRepo struct {
	mu         sync.Mutex
	cfg        GPTQuotaDisplayConfig
	entries    []GPTQuotaDisplayEntry
	candidates []GPTQuotaDisplayEntry
	snapshots  map[int64]*GPTQuotaDisplaySnapshot
	saved      []GPTQuotaDisplaySelection
	listErrs   int
	// onClaimSlot 在领取槽位的数据库往返期间回调，用于模拟并发的手动操作。
	onClaimSlot func()
	claimErr    error
}

func newFakeGPTQuotaRepo(enabled bool, entries ...GPTQuotaDisplayEntry) *fakeGPTQuotaRepo {
	return &fakeGPTQuotaRepo{
		cfg:       GPTQuotaDisplayConfig{Enabled: enabled, IntervalMinutes: 30, StartTime: "09:30", EndTime: "18:00", Version: 1},
		entries:   entries,
		snapshots: map[int64]*GPTQuotaDisplaySnapshot{},
	}
}

func (r *fakeGPTQuotaRepo) GetConfig(context.Context) (*GPTQuotaDisplayConfig, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cfg := r.cfg
	return &cfg, nil
}

func (r *fakeGPTQuotaRepo) SaveConfig(_ context.Context, cfg GPTQuotaDisplayConfig, selections []GPTQuotaDisplaySelection, expectedVersion, updatedBy int64, slotFloor *time.Time) (*GPTQuotaDisplayConfig, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if expectedVersion != r.cfg.Version {
		return nil, ErrGPTQuotaConfigConflict
	}
	cfg.Version = r.cfg.Version + 1
	cfg.UpdatedBy = &updatedBy
	cfg.LastSlotAt = r.cfg.LastSlotAt
	if slotFloor != nil && (cfg.LastSlotAt == nil || cfg.LastSlotAt.Before(*slotFloor)) {
		cfg.LastSlotAt = slotFloor
	}
	r.cfg = cfg
	r.saved = selections
	return &cfg, nil
}

func (r *fakeGPTQuotaRepo) ListSelectedEntries(context.Context) ([]GPTQuotaDisplayEntry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.listErrs > 0 {
		r.listErrs--
		return nil, errors.New("temporary db error")
	}
	return append([]GPTQuotaDisplayEntry(nil), r.entries...), nil
}

func (r *fakeGPTQuotaRepo) ListCandidates(_ context.Context, _ string, limit, offset int) ([]GPTQuotaDisplayEntry, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	total := len(r.candidates)
	if limit <= 0 || offset >= total {
		return []GPTQuotaDisplayEntry{}, total, nil
	}
	return append([]GPTQuotaDisplayEntry(nil), r.candidates[offset:min(offset+limit, total)]...), total, nil
}

func (r *fakeGPTQuotaRepo) ListSnapshots(_ context.Context, ids []int64) (map[int64]*GPTQuotaDisplaySnapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[int64]*GPTQuotaDisplaySnapshot{}
	for _, id := range ids {
		if snap, ok := r.snapshots[id]; ok {
			cp := *snap
			out[id] = &cp
		}
	}
	return out, nil
}

func (r *fakeGPTQuotaRepo) ClaimAttempt(_ context.Context, id int64, at time.Time, cooldown time.Duration) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	snap := r.snapshots[id]
	if snap == nil {
		snap = &GPTQuotaDisplaySnapshot{AccountID: id, Source: gptQuotaDisplaySourceActive}
		r.snapshots[id] = snap
	} else {
		if snap.LastAttemptAt != nil && snap.LastAttemptAt.After(at.Add(-cooldown)) {
			return false, nil
		}
		if snap.RetryAfter != nil && snap.RetryAfter.After(at) {
			return false, nil
		}
	}
	snap.LastAttemptAt = &at
	snap.LastAttemptStatus = GPTQuotaStatusRunning
	return true, nil
}

func (r *fakeGPTQuotaRepo) FinishAttempt(_ context.Context, id int64, at time.Time, status string, retryAfter *time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if snap := r.snapshots[id]; snap != nil && snap.LastAttemptAt != nil && snap.LastAttemptAt.Equal(at) {
		snap.LastAttemptStatus = status
		snap.RetryAfter = retryAfter
	}
	return nil
}

func (r *fakeGPTQuotaRepo) PublishSnapshot(_ context.Context, s *GPTQuotaDisplaySnapshot, at time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	snap := r.snapshots[s.AccountID]
	if snap == nil || (snap.SampledAt != nil && !snap.SampledAt.Before(*s.SampledAt)) {
		return false, nil
	}
	snap.FiveHour, snap.SevenDay, snap.SampledAt, snap.Source = s.FiveHour, s.SevenDay, s.SampledAt, s.Source
	if snap.LastAttemptAt != nil && snap.LastAttemptAt.Equal(at) {
		snap.LastAttemptStatus = GPTQuotaStatusOK
		snap.RetryAfter = nil
	}
	return true, nil
}

func (r *fakeGPTQuotaRepo) ClaimSlot(_ context.Context, slot time.Time) (bool, error) {
	if r.onClaimSlot != nil {
		r.onClaimSlot()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.claimErr != nil {
		return false, r.claimErr
	}
	if r.cfg.LastSlotAt != nil && !r.cfg.LastSlotAt.Before(slot) {
		return false, nil
	}
	r.cfg.LastSlotAt = &slot
	return true, nil
}

type fakeGPTQuotaFetcher struct {
	calls atomic.Int32
	fn    func(ctx context.Context, accountID int64) (*OpenAIQuotaUsage, error)
}

func (f *fakeGPTQuotaFetcher) QueryUsageReadOnly(ctx context.Context, accountID int64) (*OpenAIQuotaUsage, error) {
	f.calls.Add(1)
	if f.fn == nil {
		return gptQuotaTestUsage(25, 40), nil
	}
	return f.fn(ctx, accountID)
}

type fakeGPTQuotaAccountRepo struct {
	AccountRepository
	accounts map[int64]*Account
}

func (r *fakeGPTQuotaAccountRepo) GetByIDs(_ context.Context, ids []int64) ([]*Account, error) {
	var out []*Account
	for _, id := range ids {
		if a, ok := r.accounts[id]; ok {
			out = append(out, a)
		}
	}
	return out, nil
}

func gptQuotaTestUsage(fiveUsed, sevenUsed float64) *OpenAIQuotaUsage {
	return &OpenAIQuotaUsage{RateLimit: &OpenAIRateLimit{
		PrimaryWindow:   &OpenAIRateLimitWindow{UsedPercent: fiveUsed, LimitWindowSeconds: 18000, ResetAfterSeconds: 3600},
		SecondaryWindow: &OpenAIRateLimitWindow{UsedPercent: sevenUsed, LimitWindowSeconds: 604800, ResetAfterSeconds: 86400},
	}}
}

func gptQuotaEntry(id, accountID int64, name string) GPTQuotaDisplayEntry {
	return GPTQuotaDisplayEntry{ID: id, AccountID: accountID, AccountExists: true, AccountName: name, Platform: PlatformOpenAI, AccountType: AccountTypeOAuth}
}

func gptQuotaAt(hour, minute int) time.Time {
	return time.Date(2026, 9, 24, hour, minute, 0, 0, gptQuotaDisplayLocation)
}

func newGPTQuotaTestService(repo *fakeGPTQuotaRepo, fetcher *fakeGPTQuotaFetcher, accounts *fakeGPTQuotaAccountRepo, now time.Time) *GPTQuotaDisplayService {
	var accountRepo AccountRepository
	if accounts != nil {
		accountRepo = accounts
	}
	svc := NewGPTQuotaDisplayService(repo, accountRepo, fetcher, nil, nil)
	current := now
	var mu sync.Mutex
	svc.now = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return current
	}
	return svc
}

// ---- normalization ----

func TestNormalizeGPTQuotaUsage(t *testing.T) {
	sampled := time.Date(2026, 9, 24, 2, 0, 0, 0, time.UTC)
	w := func(used float64, seconds, resetAfter, resetAt int64) *OpenAIRateLimitWindow {
		return &OpenAIRateLimitWindow{UsedPercent: used, LimitWindowSeconds: seconds, ResetAfterSeconds: resetAfter, ResetAt: resetAt}
	}
	tests := []struct {
		name                string
		primary, secondary  *OpenAIRateLimitWindow
		wantFive, wantSeven *float64
	}{
		{"normal", w(25, 18000, 60, 0), w(40, 604800, 60, 0), ptrQuotaFloat(75), ptrQuotaFloat(60)},
		{"swapped positions", w(40, 604800, 60, 0), w(25, 18000, 60, 0), ptrQuotaFloat(75), ptrQuotaFloat(60)},
		{"zero used is full", w(0, 18000, 60, 0), nil, ptrQuotaFloat(100), nil},
		{"hundred used", w(100, 18000, 60, 0), nil, ptrQuotaFloat(0), nil},
		{"over hundred clamps", w(130, 18000, 60, 0), nil, ptrQuotaFloat(0), nil},
		{"negative used invalid", w(-1, 18000, 60, 0), w(10, 604800, 60, 0), nil, ptrQuotaFloat(90)},
		{"nan invalid", w(math.NaN(), 18000, 60, 0), nil, nil, nil},
		{"inf invalid", w(math.Inf(1), 18000, 60, 0), nil, nil, nil},
		{"same category both unavailable", w(10, 18000, 60, 0), w(20, 3600, 60, 0), nil, nil},
		{"unknown length ignored", w(10, 0, 60, 0), w(20, 604800, 60, 0), nil, ptrQuotaFloat(80)},
		{"six hour boundary", w(10, 21600, 60, 0), w(20, 21601, 60, 0), ptrQuotaFloat(90), ptrQuotaFloat(80)},
		{"one decimal", w(33.333, 18000, 60, 0), nil, ptrQuotaFloat(66.7), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			five, seven := NormalizeGPTQuotaUsage(&OpenAIQuotaUsage{RateLimit: &OpenAIRateLimit{PrimaryWindow: tt.primary, SecondaryWindow: tt.secondary}}, sampled)
			assertGPTQuotaRemaining(t, "five_hour", tt.wantFive, five)
			assertGPTQuotaRemaining(t, "seven_day", tt.wantSeven, seven)
		})
	}
}

func TestNormalizeGPTQuotaUsageIgnoresAdditionalRateLimits(t *testing.T) {
	usage := gptQuotaTestUsage(25, 40)
	usage.AdditionalRateLimits = []OpenAIAdditionalRateLimit{{RateLimit: &OpenAIRateLimit{PrimaryWindow: &OpenAIRateLimitWindow{UsedPercent: 99, LimitWindowSeconds: 18000}}}}
	five, seven := NormalizeGPTQuotaUsage(usage, time.Now())
	require.Equal(t, 75.0, five.RemainingPercent)
	require.Equal(t, 60.0, seven.RemainingPercent)
}

func TestNormalizeGPTQuotaUsageResetTime(t *testing.T) {
	sampled := time.Date(2026, 9, 24, 2, 0, 0, 0, time.UTC)
	norm := func(w *OpenAIRateLimitWindow) *GPTQuotaWindow {
		five, _ := NormalizeGPTQuotaUsage(&OpenAIQuotaUsage{RateLimit: &OpenAIRateLimit{PrimaryWindow: w}}, sampled)
		require.NotNil(t, five)
		return five
	}

	missing := norm(&OpenAIRateLimitWindow{UsedPercent: 10, LimitWindowSeconds: 18000})
	require.Nil(t, missing.ResetAt, "missing reset time must not become 'now'")
	require.Empty(t, missing.ResetTimeSource)

	after := norm(&OpenAIRateLimitWindow{UsedPercent: 10, LimitWindowSeconds: 18000, ResetAfterSeconds: 90})
	require.Equal(t, sampled.Add(90*time.Second), *after.ResetAt)
	require.Equal(t, "reset_after_seconds", after.ResetTimeSource)

	resetAt := sampled.Add(time.Hour).Unix()
	abs := norm(&OpenAIRateLimitWindow{UsedPercent: 10, LimitWindowSeconds: 18000, ResetAfterSeconds: 90, ResetAt: resetAt})
	require.Equal(t, time.Unix(resetAt, 0).UTC(), *abs.ResetAt)
	require.Equal(t, "reset_at", abs.ResetTimeSource)

	millis := norm(&OpenAIRateLimitWindow{UsedPercent: 10, LimitWindowSeconds: 18000, ResetAt: resetAt * 1000})
	require.Nil(t, millis.ResetAt, "millisecond timestamps are out of range")
}

func ptrQuotaFloat(v float64) *float64 { return &v }

func assertGPTQuotaRemaining(t *testing.T, label string, want *float64, got *GPTQuotaWindow) {
	t.Helper()
	if want == nil {
		require.Nil(t, got, label)
		return
	}
	require.NotNil(t, got, label)
	require.InDelta(t, *want, got.RemainingPercent, 1e-9, label)
}

// ---- schedule ----

func TestGPTQuotaDaySlots(t *testing.T) {
	slots30 := gptQuotaDaySlots(gptQuotaAt(12, 0), 30)
	require.Len(t, slots30, 18)
	require.Equal(t, gptQuotaAt(9, 30), slots30[0])
	require.Equal(t, gptQuotaAt(18, 0), slots30[len(slots30)-1])

	slots60 := gptQuotaDaySlots(gptQuotaAt(12, 0), 60)
	require.Len(t, slots60, 10)
	require.Equal(t, gptQuotaAt(17, 30), slots60[8])
	require.Equal(t, gptQuotaAt(18, 0), slots60[9], "60-minute mode closes at 18:00")
}

func TestGPTQuotaDueSlot(t *testing.T) {
	tests := []struct {
		name     string
		now      time.Time
		interval int
		wantSlot time.Time
		wantDue  bool
	}{
		{"before window", gptQuotaAt(9, 29), 30, time.Time{}, false},
		{"first slot", gptQuotaAt(9, 30), 30, gptQuotaAt(9, 30), true},
		{"restart catches latest slot", gptQuotaAt(10, 10), 30, gptQuotaAt(10, 0), true},
		{"60 minute catch-up", gptQuotaAt(10, 10), 60, gptQuotaAt(9, 30), true},
		{"closing slot", gptQuotaAt(18, 0), 30, gptQuotaAt(18, 0), true},
		{"closing grace", gptQuotaAt(18, 4), 60, gptQuotaAt(18, 0), true},
		{"after window", gptQuotaAt(18, 5), 30, gptQuotaAt(18, 0), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slot, due := gptQuotaDueSlot(tt.now, tt.interval)
			require.Equal(t, tt.wantDue, due)
			if tt.wantDue {
				require.Equal(t, tt.wantSlot, slot)
			}
		})
	}
}

func TestGPTQuotaNextSlot(t *testing.T) {
	require.Equal(t, gptQuotaAt(9, 30), gptQuotaNextSlot(gptQuotaAt(9, 29), 30))
	require.Equal(t, gptQuotaAt(18, 0), gptQuotaNextSlot(gptQuotaAt(17, 45), 60))
	require.Equal(t, gptQuotaAt(9, 30).AddDate(0, 0, 1), gptQuotaNextSlot(gptQuotaAt(18, 0), 30))
	require.Equal(t, gptQuotaAt(9, 30).AddDate(0, 0, 1), gptQuotaNextSlot(gptQuotaAt(23, 0), 60))
}

func TestGPTQuotaIsStale(t *testing.T) {
	yesterdayClose := gptQuotaAt(18, 0).AddDate(0, 0, -1).Add(5 * time.Second)
	at := func(ts time.Time) *time.Time { return &ts }
	require.False(t, gptQuotaIsStale(nil, gptQuotaAt(12, 0), 30), "no sample is 'no data', not stale")
	require.False(t, gptQuotaIsStale(at(yesterdayClose), gptQuotaAt(9, 31), 30), "grace after 09:30 slot")
	require.True(t, gptQuotaIsStale(at(yesterdayClose), gptQuotaAt(9, 36), 30))
	require.False(t, gptQuotaIsStale(at(gptQuotaAt(9, 29).Add(30*time.Second)), gptQuotaAt(9, 40), 30), "sample within cooldown before slot counts")
	require.True(t, gptQuotaIsStale(at(gptQuotaAt(9, 28)), gptQuotaAt(9, 40), 30))
	require.False(t, gptQuotaIsStale(at(gptQuotaAt(18, 0).Add(10*time.Second)), gptQuotaAt(23, 0), 30), "overnight is not stale")
	require.False(t, gptQuotaIsStale(at(gptQuotaAt(9, 30).Add(10*time.Second)), gptQuotaAt(10, 20), 60), "60-minute mode waits for 10:30")
}

// ---- naming, sorting, aliases, eligibility ----

func TestCompareGPTQuotaNamesNaturalOrder(t *testing.T) {
	entries := []GPTQuotaDisplayEntry{
		gptQuotaEntry(1, 11, "c-10"),
		gptQuotaEntry(2, 12, "c-backup"),
		gptQuotaEntry(3, 13, "C-2 team"),
		gptQuotaEntry(4, 14, "c-002"),
		gptQuotaEntry(5, 15, " c-1"),
		gptQuotaEntry(6, 16, "c-alpha"),
	}
	sortGPTQuotaEntries(entries)
	var names []string
	for _, e := range entries {
		names = append(names, strings.TrimSpace(e.AccountName))
	}
	require.Equal(t, []string{"c-1", "c-002", "C-2 team", "c-10", "c-alpha", "c-backup"}, names)
	require.Greater(t, compareGPTQuotaNames("c-2", 1, "c-02", 2), 0, "equal numbers fall back to normalized name")
	require.Less(t, compareGPTQuotaNames("c-99", 9, "d-1", 1), 0, "xunyou always sorts before wsdashi")
	require.Less(t, compareGPTQuotaNames("d-99", 9, "team", 1), 0, "unprefixed names sort last")
	require.Less(t, compareGPTQuotaNames("c-2", 1, "c-2", 2), 0, "then account id")
}

func TestGPTQuotaClassAndSafeDisplayName(t *testing.T) {
	require.Equal(t, GPTQuotaGroupXunyou, GPTQuotaClass("  C-01 foo@bar.com"))
	require.Equal(t, GPTQuotaGroupWsdashi, GPTQuotaClass("d-3"))
	require.Empty(t, GPTQuotaClass("e-3"))
	require.Equal(t, "c-01", SafeGPTQuotaDisplayName("  C-01 foo@bar.com", 9))
	require.Equal(t, "d-#9", SafeGPTQuotaDisplayName("d-ops@example.com", 9))
}

func TestNormalizeGPTQuotaAlias(t *testing.T) {
	for _, ok := range []string{"迅游 1号", "c-01 backup", "  速宝_A  "} {
		_, err := NormalizeGPTQuotaAlias(ok)
		require.NoError(t, err, ok)
	}
	got, err := NormalizeGPTQuotaAlias("  速宝_A  ")
	require.NoError(t, err)
	require.Equal(t, "速宝_A", got)
	for _, bad := range []string{"a@b.com", "sk-proj-abc", "0f3b1c2d-1234-4abc-9def-001122334455", "line\nbreak", strings.Repeat("a", 41), "abcdefghijklmnopqrstuvwxyz"} {
		_, err := NormalizeGPTQuotaAlias(bad)
		require.Error(t, err, bad)
	}
}

func TestGPTQuotaSelectable(t *testing.T) {
	parent := int64(1)
	base := func() *Account {
		return &Account{ID: 2, Name: "c-1", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{}}
	}
	ok, _ := GPTQuotaSelectable(base())
	require.True(t, ok)

	cases := map[string]func(*Account){
		GPTQuotaReasonNotOpenAI:       func(a *Account) { a.Platform = PlatformAnthropic },
		GPTQuotaReasonNotOAuth:        func(a *Account) { a.Type = AccountTypeSetupToken },
		GPTQuotaReasonShadow:          func(a *Account) { a.ParentAccountID = &parent },
		GPTQuotaReasonPAT:             func(a *Account) { a.Credentials["auth_mode"] = "personal_access_token" },
		GPTQuotaReasonAgentIdentity:   func(a *Account) { a.Credentials["auth_mode"] = OpenAIAuthModeAgentIdentity },
		GPTQuotaReasonNoDisplayPrefix: func(a *Account) { a.Name = "team-1" },
	}
	for want, mutate := range cases {
		a := base()
		mutate(a)
		ok, reason := GPTQuotaSelectable(a)
		require.False(t, ok, want)
		require.Equal(t, want, reason)
	}
}

func TestGPTQuotaAdminRoutesDeniedForSubAdmins(t *testing.T) {
	var permissions []string
	for permission := range adminPermissionRouteRules {
		permissions = append(permissions, permission)
	}
	subAdmin := &User{Role: RoleSubAdmin, AdminPermissions: permissions}
	for _, route := range []struct{ method, path string }{
		{"GET", "/api/v1/admin/gpt-quota"},
		{"GET", "/api/v1/admin/gpt-quota/candidates"},
		{"PUT", "/api/v1/admin/gpt-quota/config"},
		{"POST", "/api/v1/admin/gpt-quota/refresh"},
	} {
		require.False(t, CanAccessAdminRoute(subAdmin, route.method, route.path), route.path)
	}
}

// ---- read isolation ----

func TestGPTQuotaReadPathsNeverCallUpstream(t *testing.T) {
	repo := newFakeGPTQuotaRepo(true, gptQuotaEntry(1, 11, "c-1"), gptQuotaEntry(2, 12, "d-1"))
	repo.candidates = []GPTQuotaDisplayEntry{gptQuotaEntry(0, 11, "c-1")}
	fetcher := &fakeGPTQuotaFetcher{}
	accounts := &fakeGPTQuotaAccountRepo{accounts: map[int64]*Account{11: {ID: 11, Name: "c-1", Platform: PlatformOpenAI, Type: AccountTypeOAuth}}}
	svc := newGPTQuotaTestService(repo, fetcher, accounts, gptQuotaAt(12, 0))
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_, err := svc.UserView(ctx)
		require.NoError(t, err)
		_, err = svc.AdminView(ctx)
		require.NoError(t, err)
		_, err = svc.Candidates(ctx, "", 1, 20)
		require.NoError(t, err)
		_, err = svc.Enabled(ctx)
		require.NoError(t, err)
	}
	repo.cfg.Enabled = false
	_, err := svc.UserView(ctx)
	require.NoError(t, err)
	require.Zero(t, fetcher.calls.Load(), "reads must never reach upstream, even with missing snapshots")
}

func TestGPTQuotaUserViewWhitelistAndFiltering(t *testing.T) {
	parent := int64(99)
	shadow := gptQuotaEntry(4, 14, "c-3")
	shadow.ParentAccountID = &parent
	pat := gptQuotaEntry(5, 15, "c-4")
	pat.AuthMode = "personal_access_token"
	deleted := gptQuotaEntry(6, 16, "c-5")
	deleted.AccountExists = false
	renamed := gptQuotaEntry(7, 17, "x-1")
	aliased := gptQuotaEntry(8, 18, "d-2 ops@example.com")
	aliased.DisplayName = "速宝二号"
	repo := newFakeGPTQuotaRepo(true, gptQuotaEntry(1, 11, "c-10 a@b.com"), gptQuotaEntry(2, 12, "c-2"), gptQuotaEntry(3, 13, "d-1"), shadow, pat, deleted, renamed, aliased)
	sampled := gptQuotaAt(11, 30).UTC()
	repo.snapshots[11] = &GPTQuotaDisplaySnapshot{AccountID: 11, FiveHour: &GPTQuotaWindow{RemainingPercent: 50}, SampledAt: &sampled, LastAttemptStatus: GPTQuotaStatusRateLimited, Source: "active"}
	repo.snapshots[14] = &GPTQuotaDisplaySnapshot{AccountID: 14, FiveHour: &GPTQuotaWindow{RemainingPercent: 1}, SampledAt: &sampled}
	svc := newGPTQuotaTestService(repo, &fakeGPTQuotaFetcher{}, nil, gptQuotaAt(12, 0))

	view, err := svc.UserView(context.Background())
	require.NoError(t, err)
	require.True(t, view.Enabled)
	require.NotNil(t, view.NextScheduledAt)
	require.Equal(t, []string{"c-2", "c-10"}, gptQuotaCardNames(view.Groups.Xunyou))
	require.Equal(t, []string{"d-1", "速宝二号"}, gptQuotaCardNames(view.Groups.Wsdashi))
	require.Nil(t, view.Groups.Xunyou[0].SampledAt, "no snapshot means no data")
	require.Equal(t, 50.0, view.Groups.Xunyou[1].FiveHour.RemainingPercent)

	raw, err := json.Marshal(view)
	require.NoError(t, err)
	body := string(raw)
	for _, leaked := range []string{"account_id", "last_attempt", "retry_after", "rate_limited", "a@b.com", "ops@example.com", `"source"`} {
		require.NotContains(t, body, leaked)
	}
	var decoded struct {
		Groups struct {
			Xunyou []map[string]json.RawMessage `json:"xunyou"`
		} `json:"groups"`
	}
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Len(t, decoded.Groups.Xunyou, 2)
	var keys []string
	for k := range decoded.Groups.Xunyou[1] {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	require.Equal(t, []string{"display_name", "five_hour", "id", "sampled_at", "seven_day", "stale"}, keys)
}

func TestGPTQuotaUserViewDisabledHidesEverything(t *testing.T) {
	repo := newFakeGPTQuotaRepo(false, gptQuotaEntry(1, 11, "c-1"))
	sampled := time.Now()
	repo.snapshots[11] = &GPTQuotaDisplaySnapshot{AccountID: 11, SampledAt: &sampled, FiveHour: &GPTQuotaWindow{RemainingPercent: 10}}
	view, err := newGPTQuotaTestService(repo, &fakeGPTQuotaFetcher{}, nil, gptQuotaAt(12, 0)).UserView(context.Background())
	require.NoError(t, err)
	require.False(t, view.Enabled)
	require.Nil(t, view.NextScheduledAt)
	require.Empty(t, view.Groups.Xunyou)
	require.Empty(t, view.Groups.Wsdashi)
}

func TestGPTQuotaAdminViewShowsIneligibleEntriesWhenDisabled(t *testing.T) {
	deleted := gptQuotaEntry(2, 12, "c-2")
	deleted.AccountExists = false
	repo := newFakeGPTQuotaRepo(false, gptQuotaEntry(1, 11, "c-1"), deleted, gptQuotaEntry(3, 13, "c-3"))
	accounts := &fakeGPTQuotaAccountRepo{accounts: map[int64]*Account{
		11: {ID: 11, Credentials: map[string]any{"chatgpt_account_id": "shared"}},
		13: {ID: 13, Credentials: map[string]any{"chatgpt_account_id": "shared"}},
	}}
	view, err := newGPTQuotaTestService(repo, &fakeGPTQuotaFetcher{}, accounts, gptQuotaAt(12, 0)).AdminView(context.Background())
	require.NoError(t, err)
	require.Len(t, view.Entries, 3)
	require.False(t, view.Entries[1].Eligible)
	require.Equal(t, GPTQuotaReasonAccountNotFound, view.Entries[1].Reason)
	require.Equal(t, GPTQuotaStatusNever, view.Entries[0].LastAttemptStatus)
	require.Contains(t, view.Entries[0].Warnings, GPTQuotaWarningDuplicateChatGPT)
	require.Contains(t, view.Entries[2].Warnings, GPTQuotaWarningDuplicateChatGPT)
}

func gptQuotaCardNames(cards []GPTQuotaUserCard) []string {
	out := []string{}
	for _, c := range cards {
		out = append(out, c.DisplayName)
	}
	return out
}

// ---- refresh ----

func TestGPTQuotaRefreshEntryPublishesSnapshot(t *testing.T) {
	repo := newFakeGPTQuotaRepo(true, gptQuotaEntry(1, 11, "c-1"))
	fetcher := &fakeGPTQuotaFetcher{}
	svc := newGPTQuotaTestService(repo, fetcher, nil, gptQuotaAt(12, 0))

	result, err := svc.RefreshEntry(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, GPTQuotaStatusOK, result.Status)
	require.NotNil(t, result.Entry)
	require.Equal(t, 75.0, result.Entry.FiveHour.RemainingPercent)
	require.Equal(t, 60.0, result.Entry.SevenDay.RemainingPercent)
	require.NotNil(t, result.Entry.SampledAt)

	// 冷却期内再次刷新直接跳过，不再访问上游。
	result, err = svc.RefreshEntry(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, GPTQuotaStatusSkippedCooldown, result.Status)
	require.EqualValues(t, 1, fetcher.calls.Load())
}

func TestGPTQuotaRefreshFailureKeepsSnapshotAndBacksOff(t *testing.T) {
	repo := newFakeGPTQuotaRepo(true, gptQuotaEntry(1, 11, "c-1"))
	oldSample := gptQuotaAt(10, 0).UTC()
	oldAttempt := oldSample.Add(-time.Hour)
	repo.snapshots[11] = &GPTQuotaDisplaySnapshot{AccountID: 11, FiveHour: &GPTQuotaWindow{RemainingPercent: 42}, SampledAt: &oldSample, LastAttemptAt: &oldAttempt, LastAttemptStatus: GPTQuotaStatusOK}
	fetcher := &fakeGPTQuotaFetcher{fn: func(context.Context, int64) (*OpenAIQuotaUsage, error) {
		return nil, &OpenAIQuotaReadOnlyError{Category: GPTQuotaStatusRateLimited, StatusCode: http.StatusTooManyRequests}
	}}
	now := gptQuotaAt(12, 0)
	svc := newGPTQuotaTestService(repo, fetcher, nil, now)

	result, err := svc.RefreshEntry(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, GPTQuotaStatusRateLimited, result.Status)
	snap := repo.snapshots[11]
	require.Equal(t, 42.0, snap.FiveHour.RemainingPercent, "failure must not clear the last good quota")
	require.Equal(t, oldSample, *snap.SampledAt, "failure must not move sampled_at")
	require.NotNil(t, snap.RetryAfter)
	require.Equal(t, now.UTC().Add(gptQuotaDisplayDefaultBackoff), *snap.RetryAfter, "429 without Retry-After backs off 5 minutes")

	// 冷却过后仍在 Retry-After 窗口内：跳过并标记退避。
	svc.now = func() time.Time { return now.Add(2 * time.Minute) }
	result, err = svc.RefreshEntry(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, GPTQuotaStatusSkippedBackoff, result.Status)
	require.EqualValues(t, 1, fetcher.calls.Load())
}

func TestGPTQuotaRefreshOnlyKeepsWindowsFromThisSample(t *testing.T) {
	repo := newFakeGPTQuotaRepo(true, gptQuotaEntry(1, 11, "c-1"))
	oldSample := gptQuotaAt(10, 0).UTC()
	repo.snapshots[11] = &GPTQuotaDisplaySnapshot{AccountID: 11, FiveHour: &GPTQuotaWindow{RemainingPercent: 10}, SevenDay: &GPTQuotaWindow{RemainingPercent: 20}, SampledAt: &oldSample}
	fetcher := &fakeGPTQuotaFetcher{fn: func(context.Context, int64) (*OpenAIQuotaUsage, error) {
		return &OpenAIQuotaUsage{RateLimit: &OpenAIRateLimit{PrimaryWindow: &OpenAIRateLimitWindow{UsedPercent: 5, LimitWindowSeconds: 18000}}}, nil
	}}
	_, err := newGPTQuotaTestService(repo, fetcher, nil, gptQuotaAt(12, 0)).RefreshEntry(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, 95.0, repo.snapshots[11].FiveHour.RemainingPercent)
	require.Nil(t, repo.snapshots[11].SevenDay, "missing window must not reuse the old sample")
}

func TestGPTQuotaRefreshNoSupportedWindowsKeepsOldSnapshot(t *testing.T) {
	repo := newFakeGPTQuotaRepo(true, gptQuotaEntry(1, 11, "c-1"))
	oldSample := gptQuotaAt(10, 0).UTC()
	repo.snapshots[11] = &GPTQuotaDisplaySnapshot{AccountID: 11, FiveHour: &GPTQuotaWindow{RemainingPercent: 10}, SampledAt: &oldSample}
	fetcher := &fakeGPTQuotaFetcher{fn: func(context.Context, int64) (*OpenAIQuotaUsage, error) {
		return &OpenAIQuotaUsage{RateLimit: &OpenAIRateLimit{}}, nil
	}}
	result, err := newGPTQuotaTestService(repo, fetcher, nil, gptQuotaAt(12, 0)).RefreshEntry(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, GPTQuotaStatusNoSupportedWindows, result.Status)
	require.Equal(t, oldSample, *repo.snapshots[11].SampledAt)
	require.Equal(t, 10.0, repo.snapshots[11].FiveHour.RemainingPercent)
}

func TestGPTQuotaRefreshGuards(t *testing.T) {
	ctx := context.Background()
	fetcher := &fakeGPTQuotaFetcher{}

	disabled := newGPTQuotaTestService(newFakeGPTQuotaRepo(false, gptQuotaEntry(1, 11, "c-1")), fetcher, nil, gptQuotaAt(12, 0))
	_, err := disabled.RefreshEntry(ctx, 1)
	require.ErrorIs(t, err, ErrGPTQuotaDisplayDisabled)
	_, err = disabled.RefreshAll(ctx)
	require.ErrorIs(t, err, ErrGPTQuotaDisplayDisabled)

	ineligible := gptQuotaEntry(2, 12, "c-2")
	ineligible.AuthMode = OpenAIAuthModeAgentIdentity
	svc := newGPTQuotaTestService(newFakeGPTQuotaRepo(true, gptQuotaEntry(1, 11, "c-1"), ineligible), fetcher, nil, gptQuotaAt(12, 0))
	_, err = svc.RefreshEntry(ctx, 99)
	require.ErrorIs(t, err, ErrGPTQuotaEntryNotFound)
	_, err = svc.RefreshEntry(ctx, 2)
	require.Error(t, err)
	require.Zero(t, fetcher.calls.Load())
}

func TestGPTQuotaRefreshAllCountsAndSkipsIneligible(t *testing.T) {
	ineligible := gptQuotaEntry(3, 13, "c-3")
	ineligible.AuthMode = "personal_access_token"
	repo := newFakeGPTQuotaRepo(true, gptQuotaEntry(1, 11, "c-1"), gptQuotaEntry(2, 12, "d-1"), ineligible)
	fetcher := &fakeGPTQuotaFetcher{fn: func(_ context.Context, id int64) (*OpenAIQuotaUsage, error) {
		if id == 12 {
			return nil, &OpenAIQuotaReadOnlyError{Category: GPTQuotaStatusUnauthorized, StatusCode: http.StatusUnauthorized}
		}
		return gptQuotaTestUsage(10, 10), nil
	}}
	svc := newGPTQuotaTestService(repo, fetcher, nil, gptQuotaAt(12, 0))
	_, err := svc.RefreshAll(context.Background())
	require.NoError(t, err)
	require.Eventually(t, func() bool { return !svc.batchStatus().Running }, 5*time.Second, 10*time.Millisecond)
	status := svc.batchStatus()
	require.False(t, status.Running)
	require.Equal(t, 2, status.Total)
	require.Equal(t, 1, status.Succeeded)
	require.Equal(t, 1, status.Failed)
	require.Equal(t, map[string]int{GPTQuotaStatusUnauthorized: 1}, status.FailureCategories)
	require.EqualValues(t, 2, fetcher.calls.Load())
}

func TestGPTQuotaStopCancelsInFlightRefresh(t *testing.T) {
	repo := newFakeGPTQuotaRepo(true, gptQuotaEntry(1, 11, "c-1"))
	started := make(chan struct{})
	fetcher := &fakeGPTQuotaFetcher{fn: func(ctx context.Context, _ int64) (*OpenAIQuotaUsage, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	svc := newGPTQuotaTestService(repo, fetcher, nil, gptQuotaAt(12, 0))
	_, err := svc.RefreshAll(context.Background())
	require.NoError(t, err)
	<-started

	done := make(chan struct{})
	go func() {
		svc.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop must cancel in-flight upstream requests instead of waiting for them")
	}
	require.Equal(t, GPTQuotaStatusCancelled, repo.snapshots[11].LastAttemptStatus)
	_, err = svc.RefreshAll(context.Background())
	require.Error(t, err, "no new batches after stop")
}

// ---- scheduling ----

func TestGPTQuotaRunScheduledRunsEachSlotOnce(t *testing.T) {
	repo := newFakeGPTQuotaRepo(true, gptQuotaEntry(1, 11, "c-1"), gptQuotaEntry(2, 12, "d-1"))
	fetcher := &fakeGPTQuotaFetcher{}
	svc := newGPTQuotaTestService(repo, fetcher, nil, gptQuotaAt(9, 30).Add(10*time.Second))
	svc.runScheduled(context.Background())
	svc.runScheduled(context.Background())
	require.EqualValues(t, 2, fetcher.calls.Load())
	require.Equal(t, gptQuotaAt(9, 30), *repo.cfg.LastSlotAt)
}

func TestGPTQuotaRunScheduledRespectsWindowAndSwitch(t *testing.T) {
	fetcher := &fakeGPTQuotaFetcher{}
	outside := newGPTQuotaTestService(newFakeGPTQuotaRepo(true, gptQuotaEntry(1, 11, "c-1")), fetcher, nil, gptQuotaAt(8, 0))
	outside.runScheduled(context.Background())
	disabled := newGPTQuotaTestService(newFakeGPTQuotaRepo(false, gptQuotaEntry(1, 11, "c-1")), fetcher, nil, gptQuotaAt(10, 0))
	disabled.runScheduled(context.Background())
	require.Zero(t, fetcher.calls.Load())
}

func TestGPTQuotaRunScheduledSkipsSlotCrossedByLongBatch(t *testing.T) {
	repo := newFakeGPTQuotaRepo(true, gptQuotaEntry(1, 11, "c-1"))
	var svc *GPTQuotaDisplayService
	current := gptQuotaAt(9, 30)
	var mu sync.Mutex
	fetcher := &fakeGPTQuotaFetcher{fn: func(context.Context, int64) (*OpenAIQuotaUsage, error) {
		mu.Lock()
		current = gptQuotaAt(10, 1) // 批次跨过 10:00 槽位
		mu.Unlock()
		return gptQuotaTestUsage(1, 1), nil
	}}
	svc = newGPTQuotaTestService(repo, fetcher, nil, current)
	svc.now = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return current
	}
	svc.runScheduled(context.Background())
	require.Equal(t, gptQuotaAt(10, 0), *repo.cfg.LastSlotAt, "crossed slot is claimed as skipped")
	svc.runScheduled(context.Background())
	require.EqualValues(t, 1, fetcher.calls.Load(), "the crossed slot must not be replayed")
}

func TestGPTQuotaEnablingMidSlotWaitsForNextSlot(t *testing.T) {
	repo := newFakeGPTQuotaRepo(false, gptQuotaEntry(1, 11, "c-1"))
	accounts := &fakeGPTQuotaAccountRepo{accounts: map[int64]*Account{11: {ID: 11, Name: "c-1", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{}}}}
	fetcher := &fakeGPTQuotaFetcher{}
	current := gptQuotaAt(14, 47)
	svc := newGPTQuotaTestService(repo, fetcher, accounts, current)
	svc.now = func() time.Time { return current }

	_, err := svc.SaveConfig(context.Background(), GPTQuotaSaveRequest{Enabled: true, IntervalMinutes: 30, ExpectedVersion: 1, Entries: []GPTQuotaDisplaySelection{{AccountID: 11}}}, 1)
	require.NoError(t, err)
	current = gptQuotaAt(14, 47).Add(30 * time.Second)
	svc.runScheduled(context.Background())
	require.Zero(t, fetcher.calls.Load(), "the 14:30 slot must not run just because the display was enabled at 14:47")

	current = gptQuotaAt(15, 0).Add(10 * time.Second)
	svc.runScheduled(context.Background())
	require.EqualValues(t, 1, fetcher.calls.Load(), "the next planned slot runs normally")
}

func TestGPTQuotaBatchStopsWhenDisplayDisabled(t *testing.T) {
	repo := newFakeGPTQuotaRepo(true, gptQuotaEntry(1, 11, "c-1"), gptQuotaEntry(2, 12, "c-2"), gptQuotaEntry(3, 13, "c-3"), gptQuotaEntry(4, 14, "c-4"))
	fetcher := &fakeGPTQuotaFetcher{fn: func(context.Context, int64) (*OpenAIQuotaUsage, error) {
		repo.mu.Lock()
		repo.cfg.Enabled = false
		repo.mu.Unlock()
		return gptQuotaTestUsage(1, 1), nil
	}}
	svc := newGPTQuotaTestService(repo, fetcher, nil, gptQuotaAt(12, 0))
	_, begun := svc.beginBatch("manual")
	require.True(t, begun)
	svc.runBatch(context.Background(), repo.entries)
	require.LessOrEqual(t, fetcher.calls.Load(), int32(gptQuotaDisplayConcurrency), "no new upstream calls are dispatched after the display is disabled")
}

func TestGPTQuotaRetryAfterIsCapped(t *testing.T) {
	now := time.Now()
	status, retryAt := classifyGPTQuotaFetchError(context.Background(), &OpenAIQuotaReadOnlyError{Category: GPTQuotaStatusRateLimited, RetryAfter: 1000 * time.Hour}, now)
	require.Equal(t, GPTQuotaStatusRateLimited, status)
	require.Equal(t, now.Add(gptQuotaDisplayMaxBackoff), *retryAt)
}

func TestGPTQuotaCandidatesPaginationAndEligibility(t *testing.T) {
	repo := newFakeGPTQuotaRepo(true, gptQuotaEntry(1, 11, "c-1"))
	pat := gptQuotaEntry(0, 13, "c-3")
	pat.AuthMode = "personal_access_token"
	repo.candidates = []GPTQuotaDisplayEntry{gptQuotaEntry(0, 11, "c-1"), gptQuotaEntry(0, 12, "d-2"), pat, gptQuotaEntry(0, 14, "team")}
	svc := newGPTQuotaTestService(repo, &fakeGPTQuotaFetcher{}, nil, gptQuotaAt(12, 0))

	page, err := svc.Candidates(context.Background(), " c ", 1, 3)
	require.NoError(t, err)
	require.Equal(t, 4, page.Total)
	require.Len(t, page.Items, 3)
	require.True(t, page.Items[0].Selected)
	require.True(t, page.Items[1].Eligible)
	require.False(t, page.Items[2].Eligible)
	require.Equal(t, GPTQuotaReasonPAT, page.Items[2].Reason)

	page, err = svc.Candidates(context.Background(), "", 2, 3)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.Equal(t, GPTQuotaReasonNoDisplayPrefix, page.Items[0].Reason)

	page, err = svc.Candidates(context.Background(), "", math.MaxInt/2, 50)
	require.NoError(t, err, "huge page must not overflow into a negative offset")
	require.Empty(t, page.Items)
	require.Equal(t, 4, page.Total)
}

func TestGPTQuotaScheduleUsesBeijingTimeRegardlessOfGlobalTimezone(t *testing.T) {
	fetcher := &fakeGPTQuotaFetcher{}
	// 01:30 UTC = 09:30 北京时间：到期。
	svc := newGPTQuotaTestService(newFakeGPTQuotaRepo(true, gptQuotaEntry(1, 11, "c-1")), fetcher, nil, time.Date(2026, 9, 24, 1, 30, 10, 0, time.UTC))
	svc.runScheduled(context.Background())
	require.EqualValues(t, 1, fetcher.calls.Load())

	// 09:30 UTC = 17:30 北京时间：到期；10:30 UTC = 18:30 北京时间：不在时段内。
	late := newGPTQuotaTestService(newFakeGPTQuotaRepo(true, gptQuotaEntry(1, 11, "c-1")), fetcher, nil, time.Date(2026, 9, 24, 10, 30, 0, 0, time.UTC))
	late.runScheduled(context.Background())
	require.EqualValues(t, 1, fetcher.calls.Load())

	view, err := svc.UserView(context.Background())
	require.NoError(t, err)
	require.Equal(t, "Asia/Shanghai", view.Schedule.Timezone)
	require.Equal(t, time.Date(2026, 9, 24, 2, 0, 0, 0, time.UTC), view.NextScheduledAt.UTC(), "next slot is 10:00 Beijing time")
}

func TestGPTQuotaRunScheduledDoesNotConsumeSlotWhenBatchCannotStart(t *testing.T) {
	repo := newFakeGPTQuotaRepo(true, gptQuotaEntry(1, 11, "c-1"))
	repo.listErrs = 1
	fetcher := &fakeGPTQuotaFetcher{}
	svc := newGPTQuotaTestService(repo, fetcher, nil, gptQuotaAt(9, 30).Add(10*time.Second))

	svc.runScheduled(context.Background())
	require.Nil(t, repo.cfg.LastSlotAt, "a transient read failure must not consume the slot")
	require.Zero(t, fetcher.calls.Load())

	_, begun := svc.beginBatch("manual")
	require.True(t, begun)
	svc.runScheduled(context.Background())
	require.Nil(t, repo.cfg.LastSlotAt, "a running manual batch must not consume the slot")
	svc.batchMu.Lock()
	svc.batch.Running = false
	svc.batchMu.Unlock()

	svc.runScheduled(context.Background())
	require.Equal(t, gptQuotaAt(9, 30), *repo.cfg.LastSlotAt)
	require.EqualValues(t, 1, fetcher.calls.Load(), "the same slot is retried on the next tick")
}

func TestGPTQuotaManualBatchDuringSlotClaimDoesNotLoseSlot(t *testing.T) {
	repo := newFakeGPTQuotaRepo(true, gptQuotaEntry(1, 11, "c-1"), gptQuotaEntry(2, 12, "d-1"))
	fetcher := &fakeGPTQuotaFetcher{}
	svc := newGPTQuotaTestService(repo, fetcher, nil, gptQuotaAt(9, 30).Add(10*time.Second))
	manualStarted := true
	// 管理员恰好在定时任务领取槽位的数据库往返期间点击"立即刷新全部"。
	repo.onClaimSlot = func() {
		_, err := svc.RefreshAll(context.Background())
		manualStarted = err == nil
	}

	svc.runScheduled(context.Background())
	require.Eventually(t, func() bool { return !svc.batchStatus().Running }, 5*time.Second, 10*time.Millisecond)

	require.Equal(t, gptQuotaAt(9, 30), *repo.cfg.LastSlotAt)
	require.False(t, manualStarted, "the scheduled batch owns this instance once it starts claiming the slot")
	require.Equal(t, "scheduled", svc.batchStatus().Trigger, "a claimed slot must actually run")
	require.EqualValues(t, 2, fetcher.calls.Load())
}

func TestGPTQuotaConcurrentRefreshAllStartsOneBatch(t *testing.T) {
	repo := newFakeGPTQuotaRepo(true, gptQuotaEntry(1, 11, "c-1"))
	release := make(chan struct{})
	fetcher := &fakeGPTQuotaFetcher{fn: func(context.Context, int64) (*OpenAIQuotaUsage, error) {
		<-release
		return gptQuotaTestUsage(1, 1), nil
	}}
	svc := newGPTQuotaTestService(repo, fetcher, nil, gptQuotaAt(12, 0))
	var started, rejected atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := svc.RefreshAll(context.Background()); err == nil {
				started.Add(1)
			} else if errors.Is(err, ErrGPTQuotaBatchRunning) {
				rejected.Add(1)
			}
		}()
	}
	wg.Wait()
	close(release)
	require.Eventually(t, func() bool { return !svc.batchStatus().Running }, 5*time.Second, 10*time.Millisecond)
	require.EqualValues(t, 1, started.Load())
	require.EqualValues(t, 19, rejected.Load())
	require.EqualValues(t, 1, fetcher.calls.Load())
}

func TestGPTQuotaFailedSlotClaimReleasesBatchAndKeepsLastStatus(t *testing.T) {
	repo := newFakeGPTQuotaRepo(true, gptQuotaEntry(1, 11, "c-1"))
	fetcher := &fakeGPTQuotaFetcher{}
	svc := newGPTQuotaTestService(repo, fetcher, nil, gptQuotaAt(9, 30).Add(10*time.Second))
	finished := gptQuotaAt(9, 0)
	svc.batch = GPTQuotaBatchStatus{Trigger: "manual", FinishedAt: &finished, Total: 5, Succeeded: 4, Failed: 1, FailureCategories: map[string]int{GPTQuotaStatusTimeout: 1}}

	repo.claimErr = errors.New("temporary db error")
	svc.runScheduled(context.Background())
	status := svc.batchStatus()
	require.False(t, status.Running, "a failed claim must release the batch reservation")
	require.Equal(t, 5, status.Total, "the previous batch summary stays visible")
	require.Nil(t, repo.cfg.LastSlotAt)

	repo.claimErr = nil
	svc.runScheduled(context.Background())
	require.EqualValues(t, 1, fetcher.calls.Load(), "the slot is retried after the transient failure")
}

// ---- save ----

func TestGPTQuotaSaveConfigValidation(t *testing.T) {
	ctx := context.Background()
	accounts := &fakeGPTQuotaAccountRepo{accounts: map[int64]*Account{
		11: {ID: 11, Name: "c-1", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{}},
		12: {ID: 12, Name: "team", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{}},
		13: {ID: 13, Name: "c-3", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"auth_mode": "personal_access_token"}},
	}}
	svc := newGPTQuotaTestService(newFakeGPTQuotaRepo(false), &fakeGPTQuotaFetcher{}, accounts, gptQuotaAt(12, 0))
	req := func(entries ...GPTQuotaDisplaySelection) GPTQuotaSaveRequest {
		return GPTQuotaSaveRequest{Enabled: true, IntervalMinutes: 30, ExpectedVersion: 1, Entries: entries}
	}

	_, err := svc.SaveConfig(ctx, GPTQuotaSaveRequest{IntervalMinutes: 45, ExpectedVersion: 1}, 7)
	require.Error(t, err)
	_, err = svc.SaveConfig(ctx, GPTQuotaSaveRequest{IntervalMinutes: 30}, 7)
	require.Error(t, err, "expected_version is required")
	_, err = svc.SaveConfig(ctx, req(GPTQuotaDisplaySelection{AccountID: 12}), 7)
	require.Error(t, err, "missing c-/d- prefix")
	_, err = svc.SaveConfig(ctx, req(GPTQuotaDisplaySelection{AccountID: 13}), 7)
	require.Error(t, err, "PAT is not eligible")
	_, err = svc.SaveConfig(ctx, req(GPTQuotaDisplaySelection{AccountID: 11, DisplayName: "a@b.com"}), 7)
	require.Error(t, err, "alias must not look like an email")
	_, err = svc.SaveConfig(ctx, req(GPTQuotaDisplaySelection{AccountID: 11}, GPTQuotaDisplaySelection{AccountID: 11}), 7)
	require.Error(t, err, "duplicate selection")

	result, err := svc.SaveConfig(ctx, req(GPTQuotaDisplaySelection{AccountID: 11, DisplayName: "  迅游一号 "}), 7)
	require.NoError(t, err)
	require.EqualValues(t, 2, result.Config.Version)
	require.EqualValues(t, 7, *result.Config.UpdatedBy)

	_, err = svc.SaveConfig(ctx, req(GPTQuotaDisplaySelection{AccountID: 11}), 7)
	require.ErrorIs(t, err, ErrGPTQuotaConfigConflict, "stale expected_version")
}

func TestGPTQuotaSaveConfigKeepsPreviouslySelectedIneligibleEntries(t *testing.T) {
	deleted := gptQuotaEntry(1, 11, "c-1")
	deleted.AccountExists = false
	repo := newFakeGPTQuotaRepo(true, deleted)
	accounts := &fakeGPTQuotaAccountRepo{accounts: map[int64]*Account{
		12: {ID: 12, Name: "c-2", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{}},
	}}
	svc := newGPTQuotaTestService(repo, &fakeGPTQuotaFetcher{}, accounts, gptQuotaAt(12, 0))
	_, err := svc.SaveConfig(context.Background(), GPTQuotaSaveRequest{Enabled: true, IntervalMinutes: 60, ExpectedVersion: 1, Entries: []GPTQuotaDisplaySelection{{AccountID: 11}, {AccountID: 12}}}, 1)
	require.NoError(t, err, "an entry that lost eligibility after being saved must not block saving")
	require.Len(t, repo.saved, 2)
}

// ---- read-only upstream query ----

func TestQueryUsageReadOnlySkipsResetCreditsAndExtraWrites(t *testing.T) {
	account := &Account{ID: 100, Name: "c-1", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Credentials: map[string]any{"chatgpt_account_id": "org-1"}}
	repo := &stubQuotaAccountRepo{accounts: map[int64]*Account{100: account}}
	tokenProvider := NewOpenAITokenProvider(repo, &stubQuotaTokenCache{tokens: map[string]string{OpenAITokenCacheKey(account): "fake-token"}}, nil)
	var usageCalls, otherCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/wham/usage" {
			otherCalls.Add(1)
			http.NotFound(w, r)
			return
		}
		usageCalls.Add(1)
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"rate_limit":{"primary_window":{"used_percent":25,"limit_window_seconds":18000,"reset_after_seconds":60}}}`))
	}))
	defer srv.Close()

	svc := NewOpenAIQuotaService(repo, nil, tokenProvider, newQuotaRedirectingFactory(srv), nil)
	usage, err := svc.QueryUsageReadOnly(context.Background(), 100)
	require.NoError(t, err)
	require.Equal(t, 25.0, usage.RateLimit.PrimaryWindow.UsedPercent)
	require.EqualValues(t, 1, usageCalls.Load())
	require.Zero(t, otherCalls.Load(), "no reset-credit or other upstream calls")
	require.Zero(t, repo.extraUpdateCalls, "never writes account extra")
}

func TestQueryUsageReadOnlyClassifiesFailures(t *testing.T) {
	newService := func(t *testing.T, account *Account, handler http.HandlerFunc) *OpenAIQuotaService {
		repo := &stubQuotaAccountRepo{accounts: map[int64]*Account{account.ID: account}}
		tokenProvider := NewOpenAITokenProvider(repo, &stubQuotaTokenCache{tokens: map[string]string{OpenAITokenCacheKey(account): "fake-token"}}, nil)
		srv := httptest.NewServer(handler)
		t.Cleanup(srv.Close)
		return NewOpenAIQuotaService(repo, nil, tokenProvider, newQuotaRedirectingFactory(srv), nil)
	}
	oauth := func() *Account {
		return &Account{ID: 100, Name: "c-1", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Credentials: map[string]any{"chatgpt_account_id": "org-1"}}
	}
	category := func(err error) *OpenAIQuotaReadOnlyError {
		var readOnlyErr *OpenAIQuotaReadOnlyError
		require.True(t, errors.As(err, &readOnlyErr), "%v", err)
		return readOnlyErr
	}

	t.Run("429 with Retry-After", func(t *testing.T) {
		svc := newService(t, oauth(), func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Retry-After", "600")
			w.WriteHeader(http.StatusTooManyRequests)
		})
		_, err := svc.QueryUsageReadOnly(context.Background(), 100)
		got := category(err)
		require.Equal(t, GPTQuotaStatusRateLimited, got.Category)
		require.InDelta(t, 600, got.RetryAfter.Seconds(), 2)
	})
	t.Run("401", func(t *testing.T) {
		svc := newService(t, oauth(), func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) })
		_, err := svc.QueryUsageReadOnly(context.Background(), 100)
		require.Equal(t, GPTQuotaStatusUnauthorized, category(err).Category)
	})
	t.Run("parse failure", func(t *testing.T) {
		svc := newService(t, oauth(), func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("content-type", "application/json")
			_, _ = w.Write([]byte(`{not json`))
		})
		_, err := svc.QueryUsageReadOnly(context.Background(), 100)
		require.Equal(t, GPTQuotaStatusParseFailed, category(err).Category)
	})
	t.Run("window without used_percent is treated as missing", func(t *testing.T) {
		svc := newService(t, oauth(), func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("content-type", "application/json")
			_, _ = w.Write([]byte(`{"rate_limit":{"primary_window":{"limit_window_seconds":18000,"reset_after_seconds":60},"secondary_window":{"used_percent":null,"limit_window_seconds":604800},"allowed":true}}`))
		})
		usage, err := svc.QueryUsageReadOnly(context.Background(), 100)
		require.NoError(t, err)
		require.Nil(t, usage.RateLimit.PrimaryWindow, "missing used_percent must not become 0% used")
		require.Nil(t, usage.RateLimit.SecondaryWindow, "null used_percent must not become 0% used")
	})
	t.Run("cancelled context wins over other categories", func(t *testing.T) {
		svc := newService(t, oauth(), func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := svc.QueryUsageReadOnly(ctx, 100)
		require.Equal(t, GPTQuotaStatusCancelled, category(err).Category)
	})
	t.Run("expired token without refresh token is skipped before the provider", func(t *testing.T) {
		account := oauth()
		account.Credentials["expires_at"] = time.Now().Add(-time.Hour).Format(time.RFC3339)
		var calls atomic.Int32
		svc := newService(t, account, func(w http.ResponseWriter, _ *http.Request) { calls.Add(1) })
		// stubQuotaAccountRepo 未实现 SetError/BlockAccountScheduling：若走到禁用路径会直接 panic。
		_, err := svc.QueryUsageReadOnly(context.Background(), 100)
		require.Equal(t, GPTQuotaStatusTokenUnavailable, category(err).Category)
		require.Zero(t, calls.Load())
	})
	t.Run("agent identity and shadow are not supported", func(t *testing.T) {
		agent := oauth()
		agent.Credentials["auth_mode"] = OpenAIAuthModeAgentIdentity
		svc := newService(t, agent, func(http.ResponseWriter, *http.Request) {})
		_, err := svc.QueryUsageReadOnly(context.Background(), 100)
		require.Equal(t, GPTQuotaStatusNotSupported, category(err).Category)

		parent := int64(1)
		shadow := oauth()
		shadow.ParentAccountID = &parent
		svc = newService(t, shadow, func(http.ResponseWriter, *http.Request) {})
		_, err = svc.QueryUsageReadOnly(context.Background(), 100)
		require.Equal(t, GPTQuotaStatusNotSupported, category(err).Category)
	})
}
