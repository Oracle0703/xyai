//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ---- in-memory repository ----

type fakeGPTIQRepo struct {
	mu        sync.Mutex
	cfg       GPTIQTestConfig
	cases     map[int64]*GPTIQTestCase
	deleted   map[int64]bool
	revisions map[int64]*GPTIQTestRevision
	batches   map[int64]*GPTIQTestBatch
	results   map[int64]*GPTIQTestResult
	usage     map[string]int
	backoff   map[int64]time.Time
	actor     *GPTIQTestActor
	nextID    int64
	// onInsert 在插入结果时回调（测试中用于模拟取消或执行权丢失）。
	onInsert func(r *GPTIQTestResult)
	cfgErr   error
	// finishErrs 让前 N 次 FinishResult 返回普通错误（模拟数据库瞬时失败）。
	finishErrs int
}

func newFakeGPTIQRepo() *fakeGPTIQRepo {
	return &fakeGPTIQRepo{
		cfg: GPTIQTestConfig{
			Enabled: true, RunTime: "08:30", MinAccuracyPercent: 60, MinGradedRuns: 2, ReasoningRatioPercent: 30,
			MinFiveHourPercent: 10, MinSevenDayPercent: 5, DailyCallBudget: 300, RetentionDays: 90, Version: 1,
		},
		cases: map[int64]*GPTIQTestCase{}, deleted: map[int64]bool{}, revisions: map[int64]*GPTIQTestRevision{},
		batches: map[int64]*GPTIQTestBatch{}, results: map[int64]*GPTIQTestResult{},
		usage: map[string]int{}, backoff: map[int64]time.Time{},
		actor: &GPTIQTestActor{UserID: 1, FullScope: true},
	}
}

func (r *fakeGPTIQRepo) id() int64 { r.nextID++; return r.nextID }

func (r *fakeGPTIQRepo) GetConfig(context.Context) (*GPTIQTestConfig, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cfgErr != nil {
		return nil, r.cfgErr
	}
	cfg := r.cfg
	return &cfg, nil
}

func (r *fakeGPTIQRepo) SaveConfig(_ context.Context, cfg GPTIQTestConfig, expectedVersion, updatedBy int64, markRunDate string) (*GPTIQTestConfig, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if expectedVersion != r.cfg.Version {
		return nil, ErrGPTIQConfigConflict
	}
	cfg.Version = r.cfg.Version + 1
	cfg.LastRunDate = r.cfg.LastRunDate
	if markRunDate != "" && cfg.LastRunDate < markRunDate {
		cfg.LastRunDate = markRunDate
	}
	cfg.UpdatedBy = &updatedBy
	r.cfg = cfg
	out := cfg
	return &out, nil
}

func (r *fakeGPTIQRepo) addCase(in GPTIQCaseInput) *GPTIQTestCase {
	norm, spec, err := NormalizeGPTIQCaseInput(in)
	if err != nil {
		panic(err)
	}
	c, err := r.CreateCase(context.Background(), norm, spec, 1)
	if err != nil {
		panic(err)
	}
	return c
}

func (r *fakeGPTIQRepo) ListCases(context.Context) ([]GPTIQTestCase, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []GPTIQTestCase{}
	for id, c := range r.cases {
		if !r.deleted[id] {
			out = append(out, *c)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (r *fakeGPTIQRepo) GetCase(_ context.Context, id int64) (*GPTIQTestCase, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.cases[id]
	if !ok || r.deleted[id] {
		return nil, ErrGPTIQCaseNotFound
	}
	out := *c
	return &out, nil
}

func (r *fakeGPTIQRepo) CreateCase(_ context.Context, in GPTIQCaseInput, spec GPTIQRevisionSpec, _ int64) (*GPTIQTestCase, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id := r.id()
	rev := &GPTIQTestRevision{ID: r.id(), CaseID: id, Revision: 1, GPTIQRevisionSpec: spec}
	r.revisions[rev.ID] = rev
	c := &GPTIQTestCase{ID: id, Name: in.Name, Enabled: in.Enabled, SortOrder: in.SortOrder, PublicPrompt: in.PublicPrompt, PublicAnswer: in.PublicAnswer, Version: 1, RevisionID: rev.ID, Revision: 1, GPTIQRevisionSpec: spec}
	r.cases[id] = c
	out := *c
	return &out, nil
}

func (r *fakeGPTIQRepo) UpdateCase(_ context.Context, id, expectedVersion int64, in GPTIQCaseInput, spec GPTIQRevisionSpec, _ int64) (*GPTIQTestCase, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.cases[id]
	if !ok || r.deleted[id] {
		return nil, ErrGPTIQCaseNotFound
	}
	if c.Version != expectedVersion {
		return nil, ErrGPTIQCaseConflict
	}
	if !spec.SameAs(c.GPTIQRevisionSpec) {
		rev := &GPTIQTestRevision{ID: r.id(), CaseID: id, Revision: c.Revision + 1, GPTIQRevisionSpec: spec}
		r.revisions[rev.ID] = rev
		c.RevisionID, c.Revision, c.GPTIQRevisionSpec = rev.ID, rev.Revision, spec
	}
	c.Name, c.Enabled, c.SortOrder, c.PublicPrompt, c.PublicAnswer = in.Name, in.Enabled, in.SortOrder, in.PublicPrompt, in.PublicAnswer
	c.Version++
	out := *c
	return &out, nil
}

func (r *fakeGPTIQRepo) DeleteCase(_ context.Context, id, expectedVersion, _ int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.cases[id]
	if !ok || r.deleted[id] {
		return ErrGPTIQCaseNotFound
	}
	if c.Version != expectedVersion {
		return ErrGPTIQCaseConflict
	}
	r.deleted[id] = true
	return nil
}

func (r *fakeGPTIQRepo) ListRevisions(_ context.Context, ids []int64) (map[int64]*GPTIQTestRevision, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[int64]*GPTIQTestRevision{}
	for _, id := range ids {
		if v, ok := r.revisions[id]; ok {
			out[id] = v
		}
	}
	return out, nil
}

func (r *fakeGPTIQRepo) CreateBatch(_ context.Context, b *GPTIQTestBatch) (*GPTIQTestBatch, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.batches {
		if existing.Status == GPTIQBatchRunning {
			return nil, ErrGPTIQBatchRunning
		}
	}
	out := *b
	out.ID = r.id()
	out.Status = GPTIQBatchRunning
	r.batches[out.ID] = &out
	copyOut := out
	return &copyOut, nil
}

func (r *fakeGPTIQRepo) ClaimRunDate(_ context.Context, date string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cfg.LastRunDate >= date {
		return false, nil
	}
	r.cfg.LastRunDate = date
	return true, nil
}

func (r *fakeGPTIQRepo) FinishBatch(ctx context.Context, id int64, token, status string) error {
	return r.CloseBatch(ctx, id, token, status, "")
}

func (r *fakeGPTIQRepo) CloseBatch(_ context.Context, id int64, token, status, failureReason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	b := r.batches[id]
	if b == nil || b.LeaseToken != token || b.Status != GPTIQBatchRunning {
		return ErrGPTIQLeaseLost
	}
	b.Status = status
	b.FailureReason = failureReason
	now := time.Now()
	b.FinishedAt = &now
	return nil
}

func (r *fakeGPTIQRepo) HeartbeatBatch(_ context.Context, id int64, token string) (bool, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b := r.batches[id]
	if b == nil || b.LeaseToken != token || b.Status != GPTIQBatchRunning {
		return false, false, nil
	}
	return true, b.CancelRequested, nil
}

func (r *fakeGPTIQRepo) RequestCancel(_ context.Context, id int64) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b := r.batches[id]
	if b == nil || b.Status != GPTIQBatchRunning {
		return false, nil
	}
	b.CancelRequested = true
	return true, nil
}

func (r *fakeGPTIQRepo) ReapAbandoned(context.Context, time.Duration) (int, error) { return 0, nil }

func (r *fakeGPTIQRepo) GetBatch(_ context.Context, id int64) (*GPTIQTestBatch, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b := r.batches[id]
	if b == nil {
		return nil, ErrGPTIQBatchNotFound
	}
	out := *b
	return &out, nil
}

func (r *fakeGPTIQRepo) ListBatchStatuses(_ context.Context, ids []int64) (map[int64]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[int64]string{}
	for _, id := range ids {
		if b := r.batches[id]; b != nil {
			out[id] = b.Status
		}
	}
	return out, nil
}

func (r *fakeGPTIQRepo) ListBatchSummaries(_ context.Context, _ int, entryIDs []int64) ([]GPTIQTestBatchSummary, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	scope := map[int64]bool{}
	for _, id := range entryIDs {
		scope[id] = true
	}
	inScope := func(id int64) bool { return entryIDs == nil || scope[id] }
	out := []GPTIQTestBatchSummary{}
	for _, b := range r.batches {
		touches := entryIDs == nil
		for _, id := range b.EntryIDs {
			touches = touches || scope[id]
		}
		if !touches {
			continue
		}
		sum := GPTIQTestBatchSummary{GPTIQTestBatch: *b}
		for _, res := range r.results {
			if res.BatchID != b.ID || !inScope(res.EntryID) {
				continue
			}
			switch res.Status {
			case GPTIQResultOK:
				sum.OK++
			case GPTIQResultError:
				sum.Errors++
			case GPTIQResultSkipped:
				sum.Skipped++
			case GPTIQResultCancelled, GPTIQResultAbandoned:
				sum.Interrupted++
			}
		}
		out = append(out, sum)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, nil
}

func (r *fakeGPTIQRepo) ReserveDispatch(_ context.Context, batchID int64, token, date string, budget int) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.leaseOK(batchID, token) {
		return false, ErrGPTIQLeaseLost
	}
	if r.batches[batchID].CancelRequested {
		return false, ErrGPTIQBatchCancelRequested
	}
	if r.usage[date] >= budget {
		return false, nil
	}
	r.usage[date]++
	return true, nil
}

func (r *fakeGPTIQRepo) ReleaseDispatch(_ context.Context, date string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.usage[date] > 0 {
		r.usage[date]--
	}
	return nil
}

func (r *fakeGPTIQRepo) DispatchedOn(_ context.Context, date string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.usage[date], nil
}

func (r *fakeGPTIQRepo) leaseOK(batchID int64, token string) bool {
	b := r.batches[batchID]
	return b != nil && b.LeaseToken == token && b.Status == GPTIQBatchRunning
}

func (r *fakeGPTIQRepo) InsertResult(_ context.Context, token string, res *GPTIQTestResult) (int64, error) {
	r.mu.Lock()
	if !r.leaseOK(res.BatchID, token) {
		r.mu.Unlock()
		return 0, ErrGPTIQLeaseLost
	}
	out := *res
	out.ID = r.id()
	r.results[out.ID] = &out
	hook := r.onInsert
	r.mu.Unlock()
	if hook != nil {
		hook(&out)
	}
	return out.ID, nil
}

func (r *fakeGPTIQRepo) FinishResult(_ context.Context, token string, res *GPTIQTestResult) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finishErrs > 0 {
		r.finishErrs--
		return errors.New("transient db error")
	}
	cur := r.results[res.ID]
	if cur == nil || cur.Status != GPTIQResultRunning || !r.leaseOK(cur.BatchID, token) {
		return ErrGPTIQLeaseLost
	}
	out := *res
	r.results[res.ID] = &out
	return nil
}

func (r *fakeGPTIQRepo) filterResults(keep func(*GPTIQTestResult) bool) []GPTIQTestResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []GPTIQTestResult{}
	for _, res := range r.results {
		if keep(res) {
			out = append(out, *res)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (r *fakeGPTIQRepo) ListBatchResults(_ context.Context, batchID int64, _ int) ([]GPTIQTestResult, error) {
	return r.filterResults(func(res *GPTIQTestResult) bool { return res.BatchID == batchID }), nil
}

func (r *fakeGPTIQRepo) ListResultsSince(_ context.Context, since string, entryIDs []int64, _ int) ([]GPTIQTestResult, error) {
	want := map[int64]bool{}
	for _, id := range entryIDs {
		want[id] = true
	}
	return r.filterResults(func(res *GPTIQTestResult) bool { return res.RunDate >= since && want[res.EntryID] }), nil
}

func (r *fakeGPTIQRepo) ListEntryResults(_ context.Context, entryID, caseID int64, since string) ([]GPTIQTestResult, error) {
	return r.filterResults(func(res *GPTIQTestResult) bool {
		return res.EntryID == entryID && (caseID == 0 || res.CaseID == caseID) && res.RunDate >= since
	}), nil
}

func (r *fakeGPTIQRepo) RecentRunExists(_ context.Context, entryID, caseID int64, since time.Time) (bool, error) {
	found := r.filterResults(func(res *GPTIQTestResult) bool {
		return res.EntryID == entryID && res.CaseID == caseID && !res.StartedAt.Before(since) && res.Status != GPTIQResultSkipped && res.Status != GPTIQResultCancelled
	})
	return len(found) > 0, nil
}

func (r *fakeGPTIQRepo) ListBackoff(_ context.Context, ids []int64) (map[int64]time.Time, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[int64]time.Time{}
	for _, id := range ids {
		if until, ok := r.backoff[id]; ok {
			out[id] = until
		}
	}
	return out, nil
}

func (r *fakeGPTIQRepo) SetBackoff(_ context.Context, accountID int64, until time.Time, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.backoff[accountID] = until
	return nil
}

func (r *fakeGPTIQRepo) ListRejudgeCandidates(_ context.Context, caseID int64, sha string) ([]GPTIQTestResult, error) {
	r.mu.Lock()
	revs := r.revisions
	r.mu.Unlock()
	return r.filterResults(func(res *GPTIQTestResult) bool {
		rev := revs[res.RevisionID]
		return res.CaseID == caseID && res.Status == GPTIQResultOK && rev != nil && rev.PromptSHA256 == sha && rev.Kind == GPTIQKindGraded
	}), nil
}

func (r *fakeGPTIQRepo) UpdateCorrect(_ context.Context, updates []GPTIQCorrectUpdate) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, u := range updates {
		res := r.results[u.ResultID]
		if res == nil || res.Status != GPTIQResultOK || (res.ScoredRevisionID != nil && *res.ScoredRevisionID > u.ScoredRevisionID) {
			continue
		}
		{
			res.Correct = u.Correct
			rev := u.ScoredRevisionID
			res.ScoredRevisionID = &rev
		}
	}
	return nil
}

func (r *fakeGPTIQRepo) CountScoredWith(_ context.Context, ids []int64, revisionID int64) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, id := range ids {
		if res := r.results[id]; res != nil && res.ScoredRevisionID != nil && *res.ScoredRevisionID == revisionID {
			n++
		}
	}
	return n, nil
}

func (r *fakeGPTIQRepo) Cleanup(context.Context, string) (int64, error) { return 0, nil }

func (r *fakeGPTIQRepo) LoadActor(context.Context) (*GPTIQTestActor, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.actor == nil {
		return nil, ErrGPTIQScopeDenied
	}
	out := *r.actor
	return &out, nil
}

// ---- prober / accounts ----

type fakeGPTIQProber struct {
	calls atomic.Int32
	mu    sync.Mutex
	fn    func(account *Account, req GPTIQProbeRequest, call int) GPTIQProbeResult
	// block 非空时调用阻塞到 block 关闭或 ctx 取消（模拟在途请求）。
	block chan struct{}
}

func (p *fakeGPTIQProber) Probe(ctx context.Context, account *Account, req GPTIQProbeRequest) GPTIQProbeResult {
	if p.block != nil {
		select {
		case <-p.block:
		case <-ctx.Done():
			p.calls.Add(1)
			return GPTIQProbeResult{Reason: GPTQuotaStatusCancelled, Dispatched: true}
		}
	}
	call := int(p.calls.Add(1))
	if p.fn == nil {
		return gptIQOK("最终答案＝21", 500)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.fn(account, req, call)
}

func gptIQOK(answer string, reasoning int) GPTIQProbeResult {
	in, out := 100, reasoning+50
	r := reasoning
	return GPTIQProbeResult{Answer: answer, Dispatched: true, InputTokens: &in, OutputTokens: &out, ReasoningTokens: &r, Duration: 2 * time.Second, TTFT: time.Second}
}

type fakeGPTIQAccountRepo struct {
	AccountRepository
	mu       sync.Mutex
	accounts map[int64]*Account
}

func (r *fakeGPTIQAccountRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.accounts[id]
	if !ok {
		return nil, errors.New("not found")
	}
	cp := *a
	return &cp, nil
}

func gptIQTestAccount(id int64, name string) *Account {
	return &Account{ID: id, Name: name, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{"access_token": "t", "refresh_token": "rt"}}
}

type gptIQHarness struct {
	svc      *GPTIQTestService
	repo     *fakeGPTIQRepo
	quota    *fakeGPTQuotaRepo
	fetcher  *fakeGPTQuotaFetcher
	prober   *fakeGPTIQProber
	accounts *fakeGPTIQAccountRepo
	now      time.Time
}

// newGPTIQHarness 建两个迅游账号（c-01、c-02）与一个速宝账号（d-01）。
func newGPTIQHarness(t *testing.T, now time.Time) *gptIQHarness {
	t.Helper()
	quotaRepo := newFakeGPTQuotaRepo(true,
		gptQuotaEntry(11, 101, "c-01"), gptQuotaEntry(12, 102, "c-02"), gptQuotaEntry(21, 201, "d-01"))
	fetcher := &fakeGPTQuotaFetcher{}
	quotaSvc := newGPTQuotaTestService(quotaRepo, fetcher, nil, now)
	accounts := &fakeGPTIQAccountRepo{accounts: map[int64]*Account{
		101: gptIQTestAccount(101, "c-01"), 102: gptIQTestAccount(102, "c-02"), 201: gptIQTestAccount(201, "d-01"),
	}}
	repo := newFakeGPTIQRepo()
	prober := &fakeGPTIQProber{}
	svc := NewGPTIQTestService(repo, quotaSvc, accounts, prober, nil, nil)
	svc.now = func() time.Time { return now }
	svc.retryDelay = time.Millisecond
	svc.heartbeatInterval = 5 * time.Millisecond
	t.Cleanup(svc.Stop)
	return &gptIQHarness{svc: svc, repo: repo, quota: quotaRepo, fetcher: fetcher, prober: prober, accounts: accounts, now: now}
}

func gptIQCandyInput() GPTIQCaseInput {
	tpl := GPTIQCaseTemplates()[1].Input
	return tpl
}

func gptIQHiInput() GPTIQCaseInput {
	return GPTIQCaseTemplates()[0].Input
}

func gptIQAt(day, hour, minute int) time.Time {
	return time.Date(2026, 11, day, hour, minute, 0, 0, gptQuotaDisplayLocation)
}

// runManual 发起手动批次并等待结束。
func (h *gptIQHarness) runManual(t *testing.T, actor *GPTIQTestActor, req GPTIQRunRequest) *GPTIQTestBatch {
	t.Helper()
	batch, err := h.svc.StartRun(context.Background(), actor, req)
	require.NoError(t, err)
	h.waitBatch(t, batch.ID)
	return batch
}

func (h *gptIQHarness) waitBatch(t *testing.T, id int64) *GPTIQTestBatch {
	t.Helper()
	var b *GPTIQTestBatch
	require.Eventually(t, func() bool {
		var err error
		b, err = h.repo.GetBatch(context.Background(), id)
		return err == nil && b.Status != GPTIQBatchRunning
	}, 5*time.Second, 5*time.Millisecond)
	return b
}

func (h *gptIQHarness) results(batchID int64) []GPTIQTestResult {
	out, _ := h.repo.ListBatchResults(context.Background(), batchID, 0)
	return out
}

var gptIQFullActor = &GPTIQTestActor{UserID: 1, CanConfigure: true, FullScope: true}

// ---- tests ----

func TestGPTIQBatchStoresScoredResultsAndReservesBudget(t *testing.T) {
	h := newGPTIQHarness(t, gptIQAt(23, 10, 0))
	hi := h.repo.addCase(gptIQHiInput())
	candy := h.repo.addCase(gptIQCandyInput())
	h.prober.fn = func(_ *Account, req GPTIQProbeRequest, call int) GPTIQProbeResult {
		if req.Prompt == "hi" {
			return gptIQOK("Hello!", 0)
		}
		if call%3 == 0 {
			return gptIQOK("最终答案＝29", 400)
		}
		return gptIQOK("最终答案＝21", 500)
	}
	batch := h.runManual(t, gptIQFullActor, GPTIQRunRequest{})
	require.Equal(t, GPTIQBatchFinished, h.waitBatch(t, batch.ID).Status)
	results := h.results(batch.ID)
	require.Len(t, results, 3*(1+3))
	for _, r := range results {
		require.Equal(t, GPTIQResultOK, r.Status)
		require.Equal(t, 1, r.Attempts)
		if r.CaseID == hi.ID {
			require.Nil(t, r.Correct)
			require.Nil(t, r.ScoredRevisionID)
		} else {
			require.Equal(t, candy.ID, r.CaseID)
			require.NotNil(t, r.Correct)
			require.Equal(t, candy.RevisionID, *r.ScoredRevisionID)
			require.Equal(t, candy.RevisionID, r.RevisionID)
		}
		require.Equal(t, "2026-11-23", r.RunDate)
	}
	require.Equal(t, 12, h.repo.usage["2026-11-23"])
	require.EqualValues(t, 12, h.prober.calls.Load())
}

func TestGPTIQRetryAndBudgetCountEveryDispatch(t *testing.T) {
	h := newGPTIQHarness(t, gptIQAt(23, 10, 0))
	h.repo.addCase(gptIQHiInput())
	h.repo.cfg.DailyCallBudget = 1
	h.prober.fn = func(_ *Account, _ GPTIQProbeRequest, call int) GPTIQProbeResult {
		return GPTIQProbeResult{Reason: GPTQuotaStatusTimeout, Transient: true, Dispatched: true}
	}
	batch := h.runManual(t, gptIQFullActor, GPTIQRunRequest{EntryIDs: []int64{11, 21}})
	results := h.results(batch.ID)
	require.Len(t, results, 2)
	// 名额只剩 1：先拿到名额的账号重试时拿不到名额，保留首发的超时；另一个账号跳过。两账号并发，谁先拿到不确定。
	outcomes := map[string]int{}
	for _, r := range results {
		outcomes[r.Status+":"+r.Reason]++
	}
	require.Equal(t, map[string]int{"error:timeout": 1, "skipped:budget": 1}, outcomes)
	require.EqualValues(t, 1, h.prober.calls.Load())
	require.Equal(t, 1, h.repo.usage["2026-11-23"])

	_, err := h.svc.StartRun(context.Background(), gptIQFullActor, GPTIQRunRequest{})
	require.ErrorIs(t, err, ErrGPTIQBudgetExceeded)
}

func TestGPTIQTransientRetrySucceeds(t *testing.T) {
	h := newGPTIQHarness(t, gptIQAt(23, 10, 0))
	h.repo.addCase(gptIQHiInput())
	h.prober.fn = func(_ *Account, _ GPTIQProbeRequest, call int) GPTIQProbeResult {
		if call == 1 {
			return GPTIQProbeResult{Reason: GPTQuotaStatusUpstreamError, Transient: true, Dispatched: true}
		}
		return gptIQOK("hello", 0)
	}
	batch := h.runManual(t, gptIQFullActor, GPTIQRunRequest{EntryIDs: []int64{11}})
	results := h.results(batch.ID)
	require.Len(t, results, 1)
	require.Equal(t, GPTIQResultOK, results[0].Status)
	require.Equal(t, 2, results[0].Attempts)
	require.Equal(t, 2, h.repo.usage["2026-11-23"])
}

func TestGPTIQUndispatchedProbeReleasesBudget(t *testing.T) {
	h := newGPTIQHarness(t, gptIQAt(23, 10, 0))
	h.repo.addCase(gptIQHiInput())
	h.prober.fn = func(*Account, GPTIQProbeRequest, int) GPTIQProbeResult {
		return GPTIQProbeResult{Reason: GPTQuotaStatusTokenUnavailable}
	}
	h.runManual(t, gptIQFullActor, GPTIQRunRequest{EntryIDs: []int64{11}})
	require.Equal(t, 0, h.repo.usage["2026-11-23"])
}

func TestGPTIQAuthAndRateLimitSkipRemainingRuns(t *testing.T) {
	h := newGPTIQHarness(t, gptIQAt(23, 10, 0))
	h.repo.addCase(gptIQCandyInput())
	h.repo.addCase(gptIQHiInput())
	h.prober.fn = func(account *Account, _ GPTIQProbeRequest, _ int) GPTIQProbeResult {
		switch account.ID {
		case 101:
			return GPTIQProbeResult{Reason: GPTQuotaStatusUnauthorized, HTTPStatus: 401, Dispatched: true}
		case 102:
			return GPTIQProbeResult{Reason: GPTQuotaStatusRateLimited, HTTPStatus: 429, RetryAfter: time.Hour, Dispatched: true}
		}
		return gptIQOK("最终答案＝21", 500)
	}
	batch := h.runManual(t, gptIQFullActor, GPTIQRunRequest{})
	counts := map[int64]map[string]int{}
	for _, r := range h.results(batch.ID) {
		if counts[r.EntryID] == nil {
			counts[r.EntryID] = map[string]int{}
		}
		counts[r.EntryID][r.Status+":"+r.Reason]++
	}
	require.Equal(t, map[string]int{"error:unauthorized": 1, "skipped:after_auth_failure": 3}, counts[11])
	require.Equal(t, map[string]int{"error:rate_limited": 1, "skipped:after_rate_limit": 3}, counts[12])
	require.Equal(t, map[string]int{"ok:": 4}, counts[21])
	require.WithinDuration(t, h.now.Add(time.Hour), h.repo.backoff[102], time.Second)
	_, has101 := h.repo.backoff[101]
	require.False(t, has101)

	// 退避期内下一批次整账号跳过，不访问上游。
	before := h.prober.calls.Load()
	h.svc.now = func() time.Time { return h.now.Add(10 * time.Minute) }
	next := h.runManual(t, gptIQFullActor, GPTIQRunRequest{EntryIDs: []int64{12}})
	for _, r := range h.results(next.ID) {
		require.Equal(t, GPTIQSkipBackoff, r.Reason)
	}
	require.Equal(t, before, h.prober.calls.Load())
}

func TestGPTIQEntrySkipReasons(t *testing.T) {
	now := gptIQAt(23, 10, 0)
	future := now.Add(time.Hour)
	expired := now.Add(-time.Hour).Format(time.RFC3339)
	cases := map[string]struct {
		mutate func(a *Account)
		want   string
	}{
		"error status":           {func(a *Account) { a.Status = StatusError }, GPTIQSkipAccountError},
		"rate limited":           {func(a *Account) { a.RateLimitResetAt = &future }, GPTIQSkipAccountRateLimited},
		"overloaded":             {func(a *Account) { a.OverloadUntil = &future }, GPTIQSkipAccountRateLimited},
		"token unusable":         {func(a *Account) { a.Credentials = map[string]any{"access_token": "t", "expires_at": expired} }, GPTIQSkipTokenUnavailable},
		"paused is still tested": {func(a *Account) { a.Schedulable = false }, ""},
	}
	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			h := newGPTIQHarness(t, now)
			h.repo.addCase(gptIQHiInput())
			tt.mutate(h.accounts.accounts[101])
			batch := h.runManual(t, gptIQFullActor, GPTIQRunRequest{EntryIDs: []int64{11}})
			results := h.results(batch.ID)
			require.Len(t, results, 1)
			if tt.want == "" {
				require.Equal(t, GPTIQResultOK, results[0].Status)
				return
			}
			require.Equal(t, GPTIQResultSkipped, results[0].Status)
			require.Equal(t, tt.want, results[0].Reason)
			require.Zero(t, h.prober.calls.Load())
		})
	}
}

func TestGPTIQLowQuotaUsesFreshReadOnlySample(t *testing.T) {
	now := gptIQAt(23, 8, 30)
	h := newGPTIQHarness(t, now)
	h.repo.addCase(gptIQHiInput())
	// 上游返回 5 小时已用 95%（剩余 5% < 10%）。
	h.fetcher.fn = func(context.Context, int64) (*OpenAIQuotaUsage, error) { return gptQuotaTestUsage(95, 10), nil }
	batch := h.runManual(t, gptIQFullActor, GPTIQRunRequest{EntryIDs: []int64{11}})
	results := h.results(batch.ID)
	require.Equal(t, GPTIQSkipLowQuota, results[0].Reason)
	require.EqualValues(t, 1, h.fetcher.calls.Load(), "fresh wham/usage read before testing")
	require.Zero(t, h.prober.calls.Load())

	// 窗口重置时间已过而采样早于重置：旧窗口失效，等同未知；采样晚于重置则按新值判断。
	reset := now.Add(-time.Minute)
	before, after := reset.Add(-time.Hour), reset.Add(time.Second)
	require.Equal(t, gptIQQuotaUnknown, gptIQWindowState(&GPTQuotaWindow{RemainingPercent: 90, ResetAt: &reset}, 10, before, now))
	require.Equal(t, gptIQQuotaLow, gptIQWindowState(&GPTQuotaWindow{RemainingPercent: 1, ResetAt: &reset}, 10, after, now))
	require.Equal(t, gptIQQuotaLow, gptIQWindowState(&GPTQuotaWindow{RemainingPercent: 1}, 10, now, now))
	require.Equal(t, gptIQQuotaOK, gptIQWindowState(nil, 10, now, now), "a window the upstream does not provide does not block")
	require.Equal(t, gptIQQuotaOK, gptIQWindowState(&GPTQuotaWindow{RemainingPercent: 50}, 10, now, now))
}

func TestGPTIQUnknownQuotaSkipsWithoutSpending(t *testing.T) {
	h := newGPTIQHarness(t, gptIQAt(23, 8, 30))
	h.repo.addCase(gptIQHiInput())
	h.fetcher.fn = func(context.Context, int64) (*OpenAIQuotaUsage, error) { return nil, errors.New("boom") }
	batch := h.runManual(t, gptIQFullActor, GPTIQRunRequest{EntryIDs: []int64{11}})
	results := h.results(batch.ID)
	require.Equal(t, GPTIQResultSkipped, results[0].Status)
	require.Equal(t, GPTIQSkipQuotaUnknown, results[0].Reason)
	require.Zero(t, h.prober.calls.Load())
	require.Zero(t, h.repo.usage["2026-11-23"])

	// 额度阈值都为 0 时不做额度保护，也就不需要额度读数。
	h.repo.cfg.MinFiveHourPercent, h.repo.cfg.MinSevenDayPercent = 0, 0
	h.svc.now = func() time.Time { return gptIQAt(23, 9, 0) }
	next := h.runManual(t, gptIQFullActor, GPTIQRunRequest{EntryIDs: []int64{11}})
	require.Equal(t, GPTIQResultOK, h.results(next.ID)[0].Status)
}

func TestGPTIQOrganizationLeaderScope(t *testing.T) {
	h := newGPTIQHarness(t, gptIQAt(23, 10, 0))
	h.repo.addCase(gptIQHiInput())
	leader := &GPTIQTestActor{UserID: 7, Groups: []string{GPTQuotaGroupXunyou}}

	_, err := h.svc.StartRun(context.Background(), leader, GPTIQRunRequest{EntryIDs: []int64{21}})
	require.ErrorIs(t, err, ErrGPTIQScopeDenied)

	// 只持组织权限：全站配置、测试项写入、重评都在服务端拒绝。
	_, err = h.svc.SaveConfig(context.Background(), leader, GPTIQTestConfigInput{RunTime: "08:30", MinGradedRuns: 2, RetentionDays: 90, ExpectedVersion: 1})
	require.ErrorIs(t, err, ErrGPTIQScopeDenied)
	_, err = h.svc.CreateCase(context.Background(), leader, gptIQHiInput())
	require.ErrorIs(t, err, ErrGPTIQScopeDenied)
	cases, _ := h.repo.ListCases(context.Background())
	_, err = h.svc.UpdateCase(context.Background(), leader, cases[0].ID, cases[0].Version, gptIQHiInput())
	require.ErrorIs(t, err, ErrGPTIQScopeDenied)
	require.ErrorIs(t, h.svc.DeleteCase(context.Background(), leader, cases[0].ID, cases[0].Version), ErrGPTIQScopeDenied)
	_, err = h.svc.Rejudge(context.Background(), leader, cases[0].ID)
	require.ErrorIs(t, err, ErrGPTIQScopeDenied)

	batch := h.runManual(t, leader, GPTIQRunRequest{})
	entries := map[int64]bool{}
	for _, r := range h.results(batch.ID) {
		entries[r.EntryID] = true
	}
	require.Equal(t, map[int64]bool{11: true, 12: true}, entries, "\"all\" resolves to the leader's organization only")
	stored, _ := h.repo.GetBatch(context.Background(), batch.ID)
	require.Equal(t, []string{GPTQuotaGroupXunyou}, stored.ScopeGroups)

	// 批次详情只返回本组织条目。
	full := h.runManual(t, gptIQFullActor, GPTIQRunRequest{})
	detail, err := h.svc.BatchDetail(context.Background(), leader, full.ID)
	require.NoError(t, err)
	for _, r := range detail.Results {
		require.Equal(t, GPTQuotaGroupXunyou, r.Group)
	}
	require.Len(t, detail.Results, 2)
	require.Equal(t, 2, detail.VisibleTotalCalls, "progress denominator only counts the leader's entries")
	fullDetail, err := h.svc.BatchDetail(context.Background(), gptIQFullActor, full.ID)
	require.NoError(t, err)
	require.Equal(t, 3, fullDetail.VisibleTotalCalls)

	view, err := h.svc.AdminView(context.Background(), leader)
	require.NoError(t, err)
	runnable := map[int64]bool{}
	for _, e := range view.Entries {
		runnable[e.ID] = e.Runnable
	}
	require.Equal(t, map[int64]bool{11: true, 12: true}, runnable, "other organizations' entries are not listed in the admin view")
}

func TestGPTIQCancelScope(t *testing.T) {
	h := newGPTIQHarness(t, gptIQAt(23, 10, 0))
	h.repo.addCase(gptIQCandyInput())
	h.prober.block = make(chan struct{})
	leaderA := &GPTIQTestActor{UserID: 7, Groups: []string{GPTQuotaGroupXunyou}}
	leaderB := &GPTIQTestActor{UserID: 8, Groups: []string{GPTQuotaGroupXunyou}}
	batch, err := h.svc.StartRun(context.Background(), leaderA, GPTIQRunRequest{})
	require.NoError(t, err)

	_, err = h.svc.StartRun(context.Background(), gptIQFullActor, GPTIQRunRequest{})
	require.ErrorIs(t, err, ErrGPTIQBatchRunning)
	require.ErrorIs(t, h.svc.CancelRun(context.Background(), leaderB, batch.ID), ErrGPTIQScopeDenied)
	require.NoError(t, h.svc.CancelRun(context.Background(), leaderA, batch.ID))
	require.Equal(t, GPTIQBatchCancelled, h.waitBatch(t, batch.ID).Status)
	for _, r := range h.results(batch.ID) {
		require.Equal(t, GPTIQResultCancelled, r.Status, "in-flight calls are cancelled, no further runs are started")
	}
	require.ErrorIs(t, h.svc.CancelRun(context.Background(), gptIQFullActor, batch.ID), ErrGPTIQBatchNotRunning)
}

func TestGPTIQLeaseLostStopsWriting(t *testing.T) {
	h := newGPTIQHarness(t, gptIQAt(23, 10, 0))
	h.repo.addCase(gptIQCandyInput())
	var once sync.Once
	h.repo.onInsert = func(r *GPTIQTestResult) {
		once.Do(func() {
			h.repo.mu.Lock()
			h.repo.batches[r.BatchID].Status = GPTIQBatchAbandoned
			h.repo.mu.Unlock()
		})
	}
	batch, err := h.svc.StartRun(context.Background(), gptIQFullActor, GPTIQRunRequest{EntryIDs: []int64{11}})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return h.prober.calls.Load() <= 1 }, time.Second, 5*time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	results := h.results(batch.ID)
	require.Len(t, results, 1, "no further rows after the batch was reclaimed")
	require.Equal(t, GPTIQResultRunning, results[0].Status, "stale executor cannot finish the row")
	b, _ := h.repo.GetBatch(context.Background(), batch.ID)
	require.Equal(t, GPTIQBatchAbandoned, b.Status)
}

func TestGPTIQManualCooldown(t *testing.T) {
	h := newGPTIQHarness(t, gptIQAt(23, 10, 0))
	h.repo.addCase(gptIQHiInput())
	h.runManual(t, gptIQFullActor, GPTIQRunRequest{EntryIDs: []int64{11}})
	second := h.runManual(t, gptIQFullActor, GPTIQRunRequest{EntryIDs: []int64{11}})
	require.Equal(t, GPTIQSkipCooldown, h.results(second.ID)[0].Reason)
	require.EqualValues(t, 1, h.prober.calls.Load())
}

func TestGPTIQScheduledRunWindowAndDateClaim(t *testing.T) {
	h := newGPTIQHarness(t, gptIQAt(23, 8, 29))
	h.repo.addCase(gptIQHiInput())
	h.repo.cfg.ScheduleEnabled = true
	cfg, _ := h.repo.GetConfig(context.Background())

	h.svc.runScheduled(context.Background(), cfg)
	require.Empty(t, h.repo.batches, "before run_time")

	h.svc.now = func() time.Time { return gptIQAt(23, 8, 30) }
	h.svc.runScheduled(context.Background(), cfg)
	require.Len(t, h.repo.batches, 1)
	require.Equal(t, "2026-11-23", h.repo.cfg.LastRunDate)
	var id int64
	for k := range h.repo.batches {
		id = k
	}
	b := h.waitBatch(t, id)
	require.Equal(t, GPTIQTriggerSchedule, b.Trigger)
	require.Len(t, h.results(id), 3)

	cfg, _ = h.repo.GetConfig(context.Background())
	h.svc.runScheduled(context.Background(), cfg)
	require.Len(t, h.repo.batches, 1, "date already claimed")

	h.svc.now = func() time.Time { return gptIQAt(24, 11, 31) }
	cfg, _ = h.repo.GetConfig(context.Background())
	h.svc.runScheduled(context.Background(), cfg)
	require.Len(t, h.repo.batches, 1, "past the 3 hour window")
}

func TestGPTIQScheduledRunDoesNotClaimDateWhileManualRuns(t *testing.T) {
	h := newGPTIQHarness(t, gptIQAt(23, 9, 0))
	h.repo.addCase(gptIQHiInput())
	h.repo.cfg.ScheduleEnabled = true
	_, err := h.repo.CreateBatch(context.Background(), &GPTIQTestBatch{Trigger: GPTIQTriggerManual, RunDate: "2026-11-23", LeaseToken: "x"})
	require.NoError(t, err)
	cfg, _ := h.repo.GetConfig(context.Background())
	h.svc.runScheduled(context.Background(), cfg)
	require.Empty(t, h.repo.cfg.LastRunDate)
	require.Len(t, h.repo.batches, 1)
}

func TestGPTIQSaveConfigDoesNotBackfillPassedRunTime(t *testing.T) {
	h := newGPTIQHarness(t, gptIQAt(23, 9, 0))
	in := GPTIQTestConfigInput{Enabled: true, ScheduleEnabled: true, RunTime: "08:30", MinAccuracyPercent: 60, MinGradedRuns: 2, ReasoningRatioPercent: 30, MinFiveHourPercent: 10, MinSevenDayPercent: 5, DailyCallBudget: 300, RetentionDays: 90, ExpectedVersion: 1}
	cfg, err := h.svc.SaveConfig(context.Background(), gptIQFullActor, in)
	require.NoError(t, err)
	require.Equal(t, "2026-11-23", cfg.LastRunDate)
	require.Equal(t, gptIQAt(24, 8, 30).UTC(), *h.svc.nextRunAt(cfg))

	in.ExpectedVersion = cfg.Version
	in.RunTime = "10:00"
	cfg, err = h.svc.SaveConfig(context.Background(), gptIQFullActor, in)
	require.NoError(t, err)
	require.Equal(t, "10:00", cfg.RunTime)

	in.ExpectedVersion = 1
	_, err = h.svc.SaveConfig(context.Background(), gptIQFullActor, in)
	require.ErrorIs(t, err, ErrGPTIQConfigConflict)
	in.ExpectedVersion = cfg.Version
	in.RunTime = "12:00"
	_, err = h.svc.SaveConfig(context.Background(), gptIQFullActor, in)
	require.Error(t, err)
}

// ---- 判定与读取 ----

func gptIQSetOf(batchID, revisionID int64, trigger, date string, statuses ...string) *gptIQSet {
	results := []GPTIQTestResult{}
	for i, s := range statuses {
		r := GPTIQTestResult{BatchID: batchID, EntryID: 1, CaseID: 1, RevisionID: revisionID, RunIndex: i + 1, Trigger: trigger, RunDate: date}
		switch s {
		case "y", "n":
			r.Status = GPTIQResultOK
			v := s == "y"
			r.Correct = &v
		case "ok":
			r.Status = GPTIQResultOK
		case "e":
			r.Status, r.Reason = GPTIQResultError, GPTQuotaStatusTimeout
		case "s":
			r.Status, r.Reason = GPTIQResultSkipped, GPTIQSkipLowQuota
		}
		results = append(results, r)
	}
	return buildGPTIQSets(results)[gptIQSetKey{1, 1, batchID}]
}

func TestGPTIQVerdict(t *testing.T) {
	cfg := &GPTIQTestConfig{MinAccuracyPercent: 60, MinGradedRuns: 2, ReasoningRatioPercent: 30}
	revs := map[int64]*GPTIQTestRevision{
		1: {ID: 1, GPTIQRevisionSpec: GPTIQRevisionSpec{Kind: GPTIQKindGraded}},
		2: {ID: 2, GPTIQRevisionSpec: GPTIQRevisionSpec{Kind: GPTIQKindChat}},
	}
	verdict := func(sets ...*gptIQSet) string {
		v, _ := gptIQVerdict(sets, revs, map[gptIQBaselineKey]int{}, cfg)
		return v
	}
	require.Equal(t, GPTIQVerdictUntested, verdict())
	require.Equal(t, GPTIQVerdictNormal, verdict(gptIQSetOf(1, 1, "schedule", "d", "y", "y", "n")))
	require.Equal(t, GPTIQVerdictSuspect, verdict(gptIQSetOf(1, 1, "schedule", "d", "y", "n", "n")))
	require.Equal(t, GPTIQVerdictInsufficient, verdict(gptIQSetOf(1, 1, "schedule", "d", "y", "e", "e")), "1 correct + 2 timeouts is not normal")
	require.Equal(t, GPTIQVerdictFailed, verdict(gptIQSetOf(1, 1, "schedule", "d", "e", "e", "e")))
	require.Equal(t, GPTIQVerdictSkipped, verdict(gptIQSetOf(1, 1, "schedule", "d", "s", "s", "s")))
	require.Equal(t, GPTIQVerdictConnected, verdict(gptIQSetOf(1, 2, "schedule", "d", "ok")))

	_, reasons := gptIQVerdict([]*gptIQSet{gptIQSetOf(1, 1, "schedule", "d", "y", "n", "n")}, revs, nil, cfg)
	require.Equal(t, []GPTIQReason{{Code: GPTIQReasonLowAccuracy, CaseID: 1, Correct: 1, Graded: 3}}, reasons)
}

func TestGPTIQVerdictLowReasoningNeedsPeers(t *testing.T) {
	cfg := &GPTIQTestConfig{MinAccuracyPercent: 60, MinGradedRuns: 2, ReasoningRatioPercent: 30}
	revs := map[int64]*GPTIQTestRevision{1: {ID: 1, GPTIQRevisionSpec: GPTIQRevisionSpec{Kind: GPTIQKindGraded}}}
	var results []GPTIQTestResult
	add := func(entry int64, reasoning int) {
		for run := 1; run <= 3; run++ {
			v, rt := true, reasoning
			results = append(results, GPTIQTestResult{BatchID: 9, EntryID: entry, CaseID: 1, RevisionID: 1, RunIndex: run, Status: GPTIQResultOK, Correct: &v, ReasoningTokens: &rt, ModelRequested: "gpt-6.1-sol"})
		}
	}
	add(1, 5000)
	add(2, 6000)
	sets := buildGPTIQSets(results)
	require.Empty(t, gptIQBaselines(sets), "fewer than 3 peers with data in the same batch: no baseline")
	add(3, 900)
	add(4, 5500)
	sets = buildGPTIQSets(results)
	baselines := gptIQBaselines(sets)
	v, reasons := gptIQVerdict([]*gptIQSet{sets[gptIQSetKey{3, 1, 9}]}, revs, baselines, cfg)
	require.Equal(t, GPTIQVerdictSuspect, v)
	require.Equal(t, GPTIQReasonLowReasoning, reasons[0].Code)
	require.Equal(t, 17, reasons[0].Percent)
	v, _ = gptIQVerdict([]*gptIQSet{sets[gptIQSetKey{1, 1, 9}]}, revs, baselines, cfg)
	require.Equal(t, GPTIQVerdictNormal, v)
}

func TestGPTIQTrendUsesOnlyScheduledBatches(t *testing.T) {
	sched := gptIQSetOf(1, 1, GPTIQTriggerSchedule, "2026-11-23", "y", "y", "y")
	manual := gptIQSetOf(2, 1, GPTIQTriggerManual, "2026-11-23", "n", "n", "n")
	manualOnly := gptIQSetOf(3, 1, GPTIQTriggerManual, "2026-11-24", "y", "y", "y")
	sets := map[gptIQSetKey]*gptIQSet{{1, 1, 1}: sched, {1, 1, 2}: manual, {1, 1, 3}: manualOnly}
	require.Same(t, sched, latestGPTIQSet(sets, 1, 1, "2026-11-23"))
	require.Nil(t, latestGPTIQSet(sets, 1, 1, "2026-11-24"), "a manual-only day has no official result")
	require.Same(t, manualOnly, latestGPTIQSet(sets, 1, 1, ""), "card shows the latest retest")
	revs := map[int64]*GPTIQTestRevision{1: {ID: 1, GPTIQRevisionSpec: GPTIQRevisionSpec{Kind: GPTIQKindGraded}}}
	trend := gptIQTrend(sets, 1, []GPTIQTestCase{{ID: 1}}, revs, nil, &GPTIQTestConfig{MinAccuracyPercent: 60, MinGradedRuns: 2})
	require.Equal(t, []GPTIQTrendPoint{{Date: "2026-11-23", Verdict: GPTIQVerdictNormal, Correct: 3, Graded: 3}}, trend)
}

func TestGPTIQUserViewVisibilityAndNoUpstreamCalls(t *testing.T) {
	h := newGPTIQHarness(t, gptIQAt(23, 10, 0))
	candy := h.repo.addCase(gptIQCandyInput())
	h.runManual(t, gptIQFullActor, GPTIQRunRequest{})
	probes, fetches := h.prober.calls.Load(), h.fetcher.calls.Load()

	view, err := h.svc.UserView(context.Background())
	require.NoError(t, err)
	require.Len(t, view.Groups.Xunyou, 2)
	require.Len(t, view.Groups.Wsdashi, 1)
	require.Equal(t, "c-01", view.Groups.Xunyou[0].DisplayName)
	card := view.Groups.Xunyou[0]
	require.Equal(t, int64(11), card.ID)
	require.Equal(t, GPTIQVerdictNormal, card.Verdict)
	require.Equal(t, 3, card.Cases[0].Correct)
	require.NotEmpty(t, card.Cases[0].Runs[0].AnswerPreview)
	require.Empty(t, card.Cases[0].Runs[0].Answer, "list never carries full answers")
	require.NotEmpty(t, view.Cases[0].Prompt)

	detail, err := h.svc.EntryDetail(context.Background(), 11, 0, 0)
	require.NoError(t, err)
	require.Equal(t, "最终答案＝21", detail.Sets[0].Runs[0].Answer)

	// 关闭公开回答：列表预览与详情全文都不返回；关闭题面公开：不返回题面。
	in := gptIQCandyInput()
	in.PublicAnswer, in.PublicPrompt = false, false
	_, err = h.svc.UpdateCase(context.Background(), gptIQFullActor, candy.ID, candy.Version, in)
	require.NoError(t, err)
	view, _ = h.svc.UserView(context.Background())
	require.Empty(t, view.Groups.Xunyou[0].Cases[0].Runs[0].AnswerPreview)
	require.Empty(t, view.Cases[0].Prompt)
	detail, _ = h.svc.EntryDetail(context.Background(), 11, 0, 0)
	require.Empty(t, detail.Sets[0].Runs[0].Answer)
	require.Empty(t, detail.Sets[0].Runs[0].AnswerPreview)

	// 条目不再展示：详情 404；功能关闭：空分组。
	h.quota.mu.Lock()
	h.quota.entries = h.quota.entries[1:]
	h.quota.mu.Unlock()
	_, err = h.svc.EntryDetail(context.Background(), 11, 0, 0)
	require.ErrorIs(t, err, ErrGPTIQEntryNotFound)
	h.repo.cfg.Enabled = false
	view, _ = h.svc.UserView(context.Background())
	require.Empty(t, view.Groups.Xunyou)
	require.Empty(t, view.Groups.Wsdashi)

	_, err = h.svc.AdminView(context.Background(), gptIQFullActor)
	require.NoError(t, err)
	require.Equal(t, probes, h.prober.calls.Load(), "read paths never probe upstream")
	require.Equal(t, fetches, h.fetcher.calls.Load(), "read paths never sample quota")
}

func TestGPTIQRejudgeUsesCurrentRuleForSamePrompt(t *testing.T) {
	h := newGPTIQHarness(t, gptIQAt(23, 10, 0))
	in := gptIQCandyInput()
	in.ScoringExpected = "29"
	c := h.repo.addCase(in)
	h.runManual(t, gptIQFullActor, GPTIQRunRequest{EntryIDs: []int64{11}})
	for _, r := range h.repo.results {
		require.False(t, *r.Correct)
	}
	// 截断回答不重评。
	var truncatedID int64
	for id, r := range h.repo.results {
		r.AnswerTruncated = true
		truncatedID = id
		break
	}
	in.ScoringExpected = "21"
	updated, err := h.svc.UpdateCase(context.Background(), gptIQFullActor, c.ID, c.Version, in)
	require.NoError(t, err)
	require.NotEqual(t, c.RevisionID, updated.RevisionID, "scoring change creates a new immutable revision")
	out, err := h.svc.Rejudge(context.Background(), gptIQFullActor, c.ID)
	require.NoError(t, err)
	require.Equal(t, &GPTIQRejudgeResult{Checked: 3, Updated: 2, SkippedTruncated: 1}, out)
	for id, r := range h.repo.results {
		if id == truncatedID {
			require.False(t, *r.Correct)
			continue
		}
		require.True(t, *r.Correct)
		require.Equal(t, updated.RevisionID, *r.ScoredRevisionID)
	}

	// 改题面后旧回答不属于新题，不再参与重评。
	in.Prompt = "另一道题\n最终答案＝<整数>"
	changed, err := h.svc.UpdateCase(context.Background(), gptIQFullActor, c.ID, updated.Version, in)
	require.NoError(t, err)
	out, err = h.svc.Rejudge(context.Background(), gptIQFullActor, changed.ID)
	require.NoError(t, err)
	require.Zero(t, out.Checked)

	hi := h.repo.addCase(gptIQHiInput())
	_, err = h.svc.Rejudge(context.Background(), gptIQFullActor, hi.ID)
	require.ErrorIs(t, err, ErrGPTIQRejudgeNotGraded)
}

func TestGPTIQStatusCombinesBothSwitches(t *testing.T) {
	h := newGPTIQHarness(t, gptIQAt(23, 10, 0))
	for _, tt := range []struct{ quota, iq bool }{{false, false}, {true, false}, {false, true}, {true, true}} {
		h.quota.mu.Lock()
		h.quota.cfg.Enabled = tt.quota
		h.quota.mu.Unlock()
		h.repo.cfg.Enabled = tt.iq
		q, iq, err := h.svc.Status(context.Background())
		require.NoError(t, err)
		require.Equal(t, tt.quota, q)
		require.Equal(t, tt.iq, iq)
	}
}

// ---- 自审修复的回归用例 ----

func TestGPTIQBatchTimeoutAbortsWithoutBlamingAccounts(t *testing.T) {
	h := newGPTIQHarness(t, gptIQAt(23, 10, 0))
	h.repo.addCase(gptIQCandyInput())
	h.svc.batchTimeout = 30 * time.Millisecond
	h.prober.block = make(chan struct{})
	batch, err := h.svc.StartRun(context.Background(), gptIQFullActor, GPTIQRunRequest{EntryIDs: []int64{11}})
	require.NoError(t, err)
	stored := h.waitBatch(t, batch.ID)
	require.Equal(t, GPTIQBatchAborted, stored.Status, "hitting the batch limit is not a normal finish")
	require.Equal(t, GPTIQBatchFailureTimeout, stored.FailureReason)
	results := h.results(batch.ID)
	require.Len(t, results, 1, "no new runs after the limit")
	require.Equal(t, GPTIQResultCancelled, results[0].Status)
	require.Equal(t, GPTQuotaStatusCancelled, results[0].Reason)
}

func TestGPTIQFinishResultRetriesAndCleansText(t *testing.T) {
	h := newGPTIQHarness(t, gptIQAt(23, 10, 0))
	h.repo.addCase(gptIQHiInput())
	h.repo.finishErrs = 1
	h.prober.fn = func(*Account, GPTIQProbeRequest, int) GPTIQProbeResult {
		return GPTIQProbeResult{Reason: GPTQuotaStatusUpstreamError, HTTPStatus: 400, ErrorSummary: "bad\x00body", ModelResponded: "gpt\x00-x", Dispatched: true}
	}
	batch := h.runManual(t, gptIQFullActor, GPTIQRunRequest{EntryIDs: []int64{11}})
	results := h.results(batch.ID)
	require.Len(t, results, 1)
	require.Equal(t, GPTIQResultError, results[0].Status, "a transient finish failure is retried, not lost")
	require.Equal(t, "badbody", results[0].ErrorSummary)
	require.Equal(t, "gpt-x", results[0].ModelResponded)
}

func TestGPTIQCardIgnoresRunningAndAllSkippedBatches(t *testing.T) {
	h := newGPTIQHarness(t, gptIQAt(23, 10, 0))
	h.repo.addCase(gptIQCandyInput())
	h.runManual(t, gptIQFullActor, GPTIQRunRequest{EntryIDs: []int64{11}})
	// 冷却内复测：全部跳过，卡片仍显示之前的结论。
	retest := h.runManual(t, gptIQFullActor, GPTIQRunRequest{EntryIDs: []int64{11}})
	for _, r := range h.results(retest.ID) {
		require.Equal(t, GPTIQSkipCooldown, r.Reason)
	}
	view, err := h.svc.UserView(context.Background())
	require.NoError(t, err)
	card := view.Groups.Xunyou[0]
	require.Equal(t, GPTIQVerdictNormal, card.Verdict)
	require.NotEqual(t, retest.ID, card.Cases[0].BatchID)

	// 运行中批次（只跑了 1 次）不参与卡片。
	// 额度展示与降智两个服务的时钟一起前进，否则额度快照看起来是一小时前的，账号会因额度未知被跳过。
	later := func() time.Time { return h.now.Add(time.Hour) }
	h.svc.now, h.svc.quota.now = later, later
	h.prober.block = make(chan struct{})
	running, err := h.svc.StartRun(context.Background(), gptIQFullActor, GPTIQRunRequest{EntryIDs: []int64{11}})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return len(h.results(running.ID)) == 1 }, time.Second, 5*time.Millisecond)
	view, err = h.svc.UserView(context.Background())
	require.NoError(t, err)
	require.Equal(t, GPTIQVerdictNormal, view.Groups.Xunyou[0].Verdict)
	require.NotEqual(t, running.ID, view.Groups.Xunyou[0].Cases[0].BatchID)
	close(h.prober.block)
	h.waitBatch(t, running.ID)
}

func TestGPTIQStatusFailsIndependently(t *testing.T) {
	h := newGPTIQHarness(t, gptIQAt(23, 10, 0))
	h.repo.cfgErr = errors.New("relation gpt_iq_test_config does not exist")
	q, iq, err := h.svc.Status(context.Background())
	require.NoError(t, err, "an IQ config failure must not hide the quota menu")
	require.True(t, q)
	require.False(t, iq)
}

func TestGPTIQNextRunAtInsideOpenWindow(t *testing.T) {
	h := newGPTIQHarness(t, gptIQAt(23, 8, 45))
	cfg := &GPTIQTestConfig{ScheduleEnabled: true, RunTime: "08:30"}
	require.Equal(t, gptIQAt(23, 8, 30).UTC(), *h.svc.nextRunAt(cfg), "today's run is still pending inside the window")
	cfg.LastRunDate = "2026-11-23"
	require.Equal(t, gptIQAt(24, 8, 30).UTC(), *h.svc.nextRunAt(cfg))
	h.svc.now = func() time.Time { return gptIQAt(23, 11, 31) }
	cfg.LastRunDate = ""
	require.Equal(t, gptIQAt(24, 8, 30).UTC(), *h.svc.nextRunAt(cfg), "window closed")
	h.svc.now = func() time.Time { return gptIQAt(23, 7, 0) }
	require.Equal(t, gptIQAt(23, 8, 30).UTC(), *h.svc.nextRunAt(cfg))
}

// ---- 第二轮外部审核的回归用例 ----

func TestGPTIQFinalLineStrictLastLine(t *testing.T) {
	spec := gptIQGradedSpec(GPTIQScoringFinalLine, "21")
	for answer, want := range map[string]bool{
		"推理……\n最终答案＝21":         true,
		"最终答案＝21\n更正为 29":       false,
		"最终答案＝21.5":             false,
		"最终答案＝不是21，是29":         false,
		"**最终答案＝21**\n\n":       true,
		"> 最终答案：21。":            true,
		"最终答案＝21 颗":             false,
		"先说结论：最终答案＝21":          false,
		"最终答案＝29\n更正：\n最终答案＝21": true,
	} {
		got := ScoreGPTIQAnswer(spec, answer)
		require.NotNil(t, got, answer)
		require.Equal(t, want, *got, answer)
	}
}

func TestGPTIQDualPermissionLeaderKeepsOrganizationScope(t *testing.T) {
	h := newGPTIQHarness(t, gptIQAt(23, 10, 0))
	c := h.repo.addCase(gptIQHiInput())
	leader := &GPTIQTestActor{UserID: 7, CanConfigure: true, Groups: []string{GPTQuotaGroupXunyou}}
	// 另持 admin.gpt_iq_test：可编辑共享题目……
	_, err := h.svc.UpdateCase(context.Background(), leader, c.ID, c.Version, gptIQHiInput())
	require.NoError(t, err)
	// ……但运行、详情与取消仍限本组织。
	_, err = h.svc.StartRun(context.Background(), leader, GPTIQRunRequest{EntryIDs: []int64{21}})
	require.ErrorIs(t, err, ErrGPTIQScopeDenied)
	h.prober.block = make(chan struct{})
	other, err := h.svc.StartRun(context.Background(), gptIQFullActor, GPTIQRunRequest{})
	require.NoError(t, err)
	require.ErrorIs(t, h.svc.CancelRun(context.Background(), leader, other.ID), ErrGPTIQScopeDenied)
	close(h.prober.block)
	h.waitBatch(t, other.ID)
	detail, err := h.svc.BatchDetail(context.Background(), leader, other.ID)
	require.NoError(t, err)
	require.Equal(t, []int64{11, 12}, detail.Batch.EntryIDs, "batch metadata is projected to the leader's entries")
	require.Equal(t, 2, detail.Batch.TotalCalls)
	require.Nil(t, detail.Batch.TriggeredBy, "the other initiator is not disclosed")
	for _, r := range detail.Results {
		require.Equal(t, GPTQuotaGroupXunyou, r.Group)
	}

	view, err := h.svc.AdminView(context.Background(), leader)
	require.NoError(t, err)
	require.True(t, view.Scope.CanConfigure)
	require.False(t, view.Scope.FullScope)
	for _, b := range view.Batches {
		require.NotContains(t, b.EntryIDs, int64(21))
	}
}

func TestGPTIQLeaderAdminViewHidesOtherOrganizations(t *testing.T) {
	h := newGPTIQHarness(t, gptIQAt(23, 10, 0))
	h.repo.addCase(gptIQHiInput())
	h.repo.backoff[201] = h.now.Add(time.Hour)
	wsdashiOnly := h.runManual(t, gptIQFullActor, GPTIQRunRequest{EntryIDs: []int64{21}})
	both := h.runManual(t, gptIQFullActor, GPTIQRunRequest{EntryIDs: []int64{11, 21}})
	leader := &GPTIQTestActor{UserID: 7, Groups: []string{GPTQuotaGroupXunyou}}
	view, err := h.svc.AdminView(context.Background(), leader)
	require.NoError(t, err)
	require.False(t, view.Scope.CanConfigure)
	require.Empty(t, view.Templates)
	ids := []int64{}
	for _, b := range view.Batches {
		ids = append(ids, b.ID)
		require.Equal(t, []int64{11}, b.EntryIDs)
		require.Equal(t, 1, b.TotalCalls)
		require.Equal(t, 1, b.OK+b.Skipped+b.Errors, "counts only cover the leader's entries")
	}
	require.Equal(t, []int64{both.ID}, ids, "batches without the leader's entries are not listed")
	for _, e := range view.Entries {
		require.NotEqual(t, int64(21), e.ID, "other organizations' entries (and their backoff) are not disclosed")
	}
	_, err = h.svc.BatchDetail(context.Background(), leader, wsdashiOnly.ID)
	require.ErrorIs(t, err, ErrGPTIQBatchNotFound)
}

func TestGPTIQReclaimedBatchNeverDispatches(t *testing.T) {
	ctx := context.Background()
	h := newGPTIQHarness(t, gptIQAt(23, 8, 30))
	cfg, _ := h.repo.GetConfig(ctx)
	entries, _ := h.svc.visibleEntries(ctx)
	cases := []GPTIQTestCase{*h.repo.addCase(gptIQHiInput())}
	batch, err := h.repo.CreateBatch(ctx, newGPTIQBatch(GPTIQTriggerSchedule, h.svc.localDate(), nil, nil, entries[:1], cases))
	require.NoError(t, err)
	h.svc.heartbeatInterval = time.Hour
	// 另一实例在本执行者插入结果行之后、下次心跳之前回收了批次。
	h.repo.onInsert = func(row *GPTIQTestResult) {
		require.NoError(t, h.repo.FinishBatch(ctx, row.BatchID, batch.LeaseToken, GPTIQBatchAbandoned))
	}
	h.svc.executeBatch(ctx, batch, entries[:1], cases, cfg)
	require.Zero(t, h.prober.calls.Load())
	require.Zero(t, h.repo.usage[h.svc.localDate()])
}

func TestGPTIQCancelRequestStopsDispatchBeforeHeartbeat(t *testing.T) {
	ctx := context.Background()
	h := newGPTIQHarness(t, gptIQAt(23, 8, 30))
	cfg, _ := h.repo.GetConfig(ctx)
	entries, _ := h.svc.visibleEntries(ctx)
	cases := []GPTIQTestCase{*h.repo.addCase(gptIQHiInput())}
	batch, err := h.repo.CreateBatch(ctx, newGPTIQBatch(GPTIQTriggerManual, h.svc.localDate(), nil, nil, entries[:1], cases))
	require.NoError(t, err)
	h.svc.heartbeatInterval = time.Hour
	h.repo.onInsert = func(row *GPTIQTestResult) {
		_, _ = h.repo.RequestCancel(ctx, row.BatchID)
	}
	h.svc.executeBatch(ctx, batch, entries[:1], cases, cfg)
	require.Zero(t, h.prober.calls.Load())
	b, _ := h.repo.GetBatch(ctx, batch.ID)
	require.Equal(t, GPTIQBatchCancelled, b.Status)
}

type gptIQFailingInserts struct{ GPTIQTestRepository }

func (gptIQFailingInserts) InsertResult(context.Context, string, *GPTIQTestResult) (int64, error) {
	return 0, errors.New("result storage temporarily unavailable")
}

func TestGPTIQStorageFailureIsNotAFinishedBatch(t *testing.T) {
	h := newGPTIQHarness(t, gptIQAt(23, 8, 30))
	h.repo.addCase(gptIQHiInput())
	h.svc.repo = gptIQFailingInserts{h.repo}
	batch := h.runManual(t, gptIQFullActor, GPTIQRunRequest{EntryIDs: []int64{11}})
	stored, err := h.repo.GetBatch(context.Background(), batch.ID)
	require.NoError(t, err)
	require.Equal(t, GPTIQBatchAborted, stored.Status)
	require.Equal(t, GPTIQBatchFailurePersistFailed, stored.FailureReason)
	require.Zero(t, h.prober.calls.Load())
}

func TestGPTIQInterruptionIsNotAnAccountFailure(t *testing.T) {
	cfg := &GPTIQTestConfig{MinGradedRuns: 2, MinAccuracyPercent: 60}
	for _, status := range []string{GPTIQResultAbandoned, GPTIQResultCancelled} {
		sets := buildGPTIQSets([]GPTIQTestResult{{BatchID: 7, EntryID: 1, CaseID: 1, RevisionID: 1, Status: status}})
		verdict, reasons := gptIQVerdict([]*gptIQSet{sets[gptIQSetKey{1, 1, 7}]}, nil, nil, cfg)
		require.Equal(t, GPTIQVerdictInsufficient, verdict, status)
		require.Equal(t, GPTIQReasonInterrupted, reasons[0].Code)
	}
	// 一次答对 + 两次中断：样本不足，不是失败也不是正常。
	revs := map[int64]*GPTIQTestRevision{1: {ID: 1, GPTIQRevisionSpec: GPTIQRevisionSpec{Kind: GPTIQKindGraded}}}
	v, _ := gptIQVerdict([]*gptIQSet{gptIQSetOfInterrupted()}, revs, nil, cfg)
	require.Equal(t, GPTIQVerdictInsufficient, v)
}

func gptIQSetOfInterrupted() *gptIQSet {
	yes := true
	results := []GPTIQTestResult{
		{BatchID: 8, EntryID: 1, CaseID: 1, RevisionID: 1, RunIndex: 1, Status: GPTIQResultOK, Correct: &yes},
		{BatchID: 8, EntryID: 1, CaseID: 1, RevisionID: 1, RunIndex: 2, Status: GPTIQResultAbandoned},
		{BatchID: 8, EntryID: 1, CaseID: 1, RevisionID: 1, RunIndex: 3, Status: GPTIQResultCancelled},
	}
	set := buildGPTIQSets(results)[gptIQSetKey{1, 1, 8}]
	if set.errors != 0 || set.interrupted != 2 {
		panic("interrupted runs must not count as errors")
	}
	return set
}

func TestGPTIQHistoryCarriesExecutedRevision(t *testing.T) {
	ctx := context.Background()
	h := newGPTIQHarness(t, gptIQAt(23, 8, 30))
	in := gptIQHiInput()
	in.Prompt = "original question"
	original := h.repo.addCase(in)
	h.runManual(t, gptIQFullActor, GPTIQRunRequest{EntryIDs: []int64{11}})
	in.Prompt, in.Model = "replacement question", "gpt-6.1-sol"
	updated, err := h.svc.UpdateCase(ctx, gptIQFullActor, original.ID, original.Version, in)
	require.NoError(t, err)
	detail, err := h.svc.EntryDetail(ctx, 11, original.ID, 14)
	require.NoError(t, err)
	require.Len(t, detail.Sets, 1)
	rev := detail.Sets[0].Revision
	require.NotNil(t, rev)
	require.Equal(t, original.RevisionID, rev.ID)
	require.Equal(t, "original question", rev.Prompt)
	require.Equal(t, original.Model, rev.Model)
	require.Equal(t, "replacement question", detail.Cases[0].Prompt)

	// 关闭题面公开后，历史版本的题面也不返回。
	in.PublicPrompt = false
	_, err = h.svc.UpdateCase(ctx, gptIQFullActor, original.ID, updated.Version, in)
	require.NoError(t, err)
	detail, _ = h.svc.EntryDetail(ctx, 11, original.ID, 14)
	require.Empty(t, detail.Sets[0].Revision.Prompt)
}

type gptIQPausedRejudge struct {
	GPTIQTestRepository
	pauseRevision int64
	waiting       chan struct{}
	proceed       chan struct{}
}

func (r *gptIQPausedRejudge) UpdateCorrect(ctx context.Context, updates []GPTIQCorrectUpdate) error {
	if len(updates) > 0 && updates[0].ScoredRevisionID == r.pauseRevision {
		close(r.waiting)
		<-r.proceed
	}
	return r.GPTIQTestRepository.UpdateCorrect(ctx, updates)
}

func TestGPTIQSlowRejudgeCannotUndoNewerScore(t *testing.T) {
	ctx := context.Background()
	h := newGPTIQHarness(t, gptIQAt(23, 8, 30))
	in := gptIQCandyInput()
	in.Runs = 1
	original := h.repo.addCase(in)
	batch := h.runManual(t, gptIQFullActor, GPTIQRunRequest{EntryIDs: []int64{11}})
	in.ScoringExpected = "29"
	older, err := h.svc.UpdateCase(ctx, gptIQFullActor, original.ID, original.Version, in)
	require.NoError(t, err)
	paused := &gptIQPausedRejudge{GPTIQTestRepository: h.repo, pauseRevision: older.RevisionID, waiting: make(chan struct{}), proceed: make(chan struct{})}
	h.svc.repo = paused
	done := make(chan error, 1)
	go func() { _, err := h.svc.Rejudge(ctx, gptIQFullActor, original.ID); done <- err }()
	<-paused.waiting
	in.ScoringExpected = "21"
	newer, err := h.svc.UpdateCase(ctx, gptIQFullActor, original.ID, older.Version, in)
	require.NoError(t, err)
	_, err = h.svc.Rejudge(ctx, gptIQFullActor, original.ID)
	require.NoError(t, err)
	close(paused.proceed)
	require.NoError(t, <-done)
	out := h.results(batch.ID)[0]
	require.Equal(t, newer.RevisionID, *out.ScoredRevisionID)
	require.True(t, *out.Correct)

	// 页面加载后测试项又被改过：带旧版本 ID 的重评返回冲突。
	_, err = h.svc.RejudgeAt(ctx, gptIQFullActor, original.ID, older.RevisionID)
	require.ErrorIs(t, err, ErrGPTIQCaseConflict)
}

func TestGPTIQReasoningBaselineComparesSameEffectiveModel(t *testing.T) {
	var results []GPTIQTestResult
	for i, model := range []string{"model-a", "model-b", "model-b", "model-b"} {
		tokens := 1000
		if i == 0 {
			tokens = 30
		}
		for run := 1; run <= 3; run++ {
			yes, rt := true, tokens
			results = append(results, GPTIQTestResult{BatchID: 1, EntryID: int64(i + 1), CaseID: 1, RevisionID: 1, RunIndex: run, Status: GPTIQResultOK, Correct: &yes, ReasoningTokens: &rt, ModelRequested: model})
		}
	}
	sets := buildGPTIQSets(results)
	baselines := gptIQBaselines(sets)
	require.NotContains(t, baselines, gptIQBaselineKey{1, 1, "model-a"}, "a single account on its model has no peers")
	require.Equal(t, 1000, baselines[gptIQBaselineKey{1, 1, "model-b"}])
	revs := map[int64]*GPTIQTestRevision{1: {ID: 1, GPTIQRevisionSpec: GPTIQRevisionSpec{Kind: GPTIQKindGraded}}}
	v, _ := gptIQVerdict([]*gptIQSet{sets[gptIQSetKey{1, 1, 1}]}, revs, baselines, &GPTIQTestConfig{MinGradedRuns: 2, MinAccuracyPercent: 60, ReasoningRatioPercent: 30})
	require.Equal(t, GPTIQVerdictNormal, v)
}

func TestGPTIQAuditPromptDigest(t *testing.T) {
	raw := []byte(`{"name":"糖果题","prompt":"line1\r\nline2 sk-abcdefghijklmnop","model":"gpt-6.1-sol"}`)
	out := string(GPTIQAuditPromptDigest(raw))
	require.NotContains(t, out, "line1")
	require.NotContains(t, out, "sk-abc")
	require.Contains(t, out, "sha256:"+gptIQPromptHash("line1\nline2 sk-abcdefghijklmnop"))
	require.Contains(t, out, "gpt-6.1-sol")
	require.JSONEq(t, `{"_body":"<omitted>"}`, string(GPTIQAuditPromptDigest([]byte("not json secret-canary"))))

	// encoding/json 绑定键名大小写不敏感："Prompt" 一样是题面；非字符串题面只留省略标记；未知字段丢弃。
	for _, body := range []string{
		`{"Prompt":"secret-canary","name":"x"}`,
		`{"PROMPT":"secret-canary"}`,
		`{"prompt":["secret-canary"]}`,
		`{"prompt":{"text":"secret-canary"}}`,
		`{"name":{"nested":"secret-canary"}}`,
		`{"extra_note":"secret-canary","prompt":"ok"}`,
		`["secret-canary"]`,
	} {
		got := string(GPTIQAuditPromptDigest([]byte(body)))
		require.NotContains(t, got, "secret-canary", body)
		require.True(t, json.Valid([]byte(got)), body)
	}
	got := string(GPTIQAuditPromptDigest([]byte(`{"Prompt":"secret-canary"}`)))
	require.Contains(t, got, "sha256:"+gptIQPromptHash("secret-canary"))
}

// ---- 第二轮复审（recheck）的回归用例 ----

func TestGPTIQQuotaRefreshFailureIgnoresEarlierSuccess(t *testing.T) {
	for _, tc := range []struct {
		name           string
		lastAttemptAgo time.Duration
		lastStatus     string
		backoff        bool
		wantFetch      int32
		wantModel      int32
		wantReason     string
	}{
		{"refresh failed after an earlier success", 5 * time.Minute, GPTQuotaStatusOK, false, 1, 0, GPTIQSkipQuotaUnknown},
		{"quota probe in 429 backoff", 2 * time.Minute, GPTQuotaStatusRateLimited, true, 0, 0, GPTIQSkipQuotaUnknown},
		{"latest attempt failed inside cooldown", 20 * time.Second, GPTQuotaStatusTimeout, false, 0, 0, GPTIQSkipQuotaUnknown},
		{"fresh success inside normal cooldown", 20 * time.Second, GPTQuotaStatusOK, false, 0, 1, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := gptIQAt(24, 8, 30)
			h := newGPTIQHarness(t, now)
			h.repo.addCase(gptIQHiInput())
			last := now.Add(-tc.lastAttemptAgo)
			sample := now.Add(-5 * time.Minute)
			if tc.lastStatus == GPTQuotaStatusOK && tc.lastAttemptAgo < time.Minute {
				sample = last
			}
			snap := &GPTQuotaDisplaySnapshot{AccountID: 101, SampledAt: &sample, LastAttemptAt: &last, LastAttemptStatus: tc.lastStatus,
				FiveHour: &GPTQuotaWindow{RemainingPercent: 80}, SevenDay: &GPTQuotaWindow{RemainingPercent: 80}}
			if tc.backoff {
				retry := now.Add(3 * time.Minute)
				snap.RetryAfter = &retry
			}
			h.quota.snapshots[101] = snap
			h.fetcher.fn = func(context.Context, int64) (*OpenAIQuotaUsage, error) {
				return nil, errors.New("quota temporarily unavailable")
			}
			batch := h.runManual(t, gptIQFullActor, GPTIQRunRequest{EntryIDs: []int64{11}})
			r := h.results(batch.ID)[0]
			require.Equal(t, tc.wantFetch, h.fetcher.calls.Load())
			require.Equal(t, tc.wantModel, h.prober.calls.Load())
			require.Equal(t, int(tc.wantModel), h.repo.usage[h.svc.localDate()])
			require.Equal(t, tc.wantReason, r.Reason)
		})
	}
}

func TestGPTIQRejudgeLeavesHistoricalChatUnscored(t *testing.T) {
	ctx := context.Background()
	h := newGPTIQHarness(t, gptIQAt(24, 8, 30))
	in := gptIQHiInput()
	first := h.repo.addCase(in)
	batch := h.runManual(t, gptIQFullActor, GPTIQRunRequest{EntryIDs: []int64{11}})
	in.Kind, in.ScoringMode, in.ScoringExpected = GPTIQKindGraded, GPTIQScoringContains, "hello"
	second, err := h.svc.UpdateCase(ctx, gptIQFullActor, first.ID, first.Version, in)
	require.NoError(t, err)
	out, err := h.svc.RejudgeAt(ctx, gptIQFullActor, first.ID, second.RevisionID)
	require.NoError(t, err)
	require.Zero(t, out.Checked, "executions that ran as chat are not re-scoring candidates")
	row := h.results(batch.ID)[0]
	require.Nil(t, row.Correct)
	require.Nil(t, row.ScoredRevisionID)
}

func TestGPTIQObsoleteRejudgeReportsZeroUpdated(t *testing.T) {
	ctx := context.Background()
	h := newGPTIQHarness(t, gptIQAt(24, 8, 30))
	in := gptIQCandyInput()
	in.Runs = 1
	first := h.repo.addCase(in)
	batch := h.runManual(t, gptIQFullActor, GPTIQRunRequest{EntryIDs: []int64{11}})
	in.ScoringExpected = "29"
	older, err := h.svc.UpdateCase(ctx, gptIQFullActor, first.ID, first.Version, in)
	require.NoError(t, err)
	paused := &gptIQPausedRejudge{GPTIQTestRepository: h.repo, pauseRevision: older.RevisionID, waiting: make(chan struct{}), proceed: make(chan struct{})}
	h.svc.repo = paused
	type ret struct {
		out *GPTIQRejudgeResult
		err error
	}
	done := make(chan ret, 1)
	go func() {
		out, err := h.svc.RejudgeAt(ctx, gptIQFullActor, first.ID, older.RevisionID)
		done <- ret{out, err}
	}()
	<-paused.waiting
	in.ScoringExpected = "21"
	newer, err := h.svc.UpdateCase(ctx, gptIQFullActor, first.ID, older.Version, in)
	require.NoError(t, err)
	fresh, err := h.svc.RejudgeAt(ctx, gptIQFullActor, first.ID, newer.RevisionID)
	require.NoError(t, err)
	require.Equal(t, 1, fresh.Updated)
	close(paused.proceed)
	got := <-done
	require.NoError(t, got.err)
	require.Zero(t, got.out.Updated, "the guarded write committed nothing, so nothing is reported as updated")
	row := h.results(batch.ID)[0]
	require.Equal(t, newer.RevisionID, *row.ScoredRevisionID)
	require.True(t, *row.Correct)
}

func TestGPTIQAdminEntriesFollowScope(t *testing.T) {
	h := newGPTIQHarness(t, gptIQAt(24, 10, 0))
	ids := func(actor *GPTIQTestActor) []int64 {
		view, err := h.svc.AdminView(context.Background(), actor)
		require.NoError(t, err)
		out := []int64{}
		for _, e := range view.Entries {
			require.True(t, e.Runnable)
			out = append(out, e.ID)
		}
		return out
	}
	require.Equal(t, []int64{11, 12}, ids(&GPTIQTestActor{UserID: 7, Groups: []string{GPTQuotaGroupXunyou}}), "organization leader")
	require.Equal(t, []int64{21}, ids(&GPTIQTestActor{UserID: 8, CanConfigure: true, Groups: []string{GPTQuotaGroupWsdashi}}), "leader who may also configure")
	require.Equal(t, []int64{11, 12, 21}, ids(gptIQFullActor), "full admin")
	require.Equal(t, []int64{11, 12, 21}, ids(&GPTIQTestActor{UserID: 9, CanConfigure: true, FullScope: true}), "sub-admin with the site-wide permission")
}
