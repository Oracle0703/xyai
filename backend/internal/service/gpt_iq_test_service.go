package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/google/uuid"
)

// GPT 降智测试：每天定时（或在配置页手动）对额度展示中的账号发固定题目，存储每次回答、
// token 与耗时，读取时计算"正常 / 疑似降智 / 失败"等判定并在 /gpt-quota 下方公开展示。
// 账号集合、展示名、分组与排序复用 GPT 额度展示条目。设计见 docs/features/gpt-iq-test-design-cn.md。

const (
	GPTIQTestLeaderKey           = "jobs:gpt-iq-test"
	GPTIQTestPollIntervalSeconds = 15 * 60

	GPTIQTriggerSchedule = "schedule"
	GPTIQTriggerManual   = "manual"

	GPTIQBatchRunning   = "running"
	GPTIQBatchFinished  = "finished"
	GPTIQBatchCancelled = "cancelled"
	GPTIQBatchAborted   = "aborted"
	GPTIQBatchAbandoned = "abandoned"

	GPTIQResultRunning   = "running"
	GPTIQResultOK        = "ok"
	GPTIQResultError     = "error"
	GPTIQResultSkipped   = "skipped"
	GPTIQResultCancelled = "cancelled"
	GPTIQResultAbandoned = "abandoned"

	// 跳过原因（reason，status=skipped）。
	GPTIQSkipAccountUnavailable = "account_unavailable"
	GPTIQSkipNotSupported       = "not_supported"
	GPTIQSkipAccountError       = "account_error"
	GPTIQSkipAccountRateLimited = "account_rate_limited"
	GPTIQSkipBackoff            = "backoff"
	GPTIQSkipTokenUnavailable   = "token_unavailable"
	GPTIQSkipLowQuota           = "low_quota"
	// GPTIQSkipQuotaUnknown：额度读取失败、无快照或快照过旧。额度未知时不消耗真实账号额度。
	GPTIQSkipQuotaUnknown     = "quota_unknown"
	GPTIQSkipCooldown         = "cooldown"
	GPTIQSkipBudget           = "budget"
	GPTIQSkipAfterAuthFailure = "after_auth_failure"
	GPTIQSkipAfterRateLimit   = "after_rate_limit"

	gptIQTickInterval      = 30 * time.Second
	gptIQHeartbeatInterval = 10 * time.Second
	gptIQAbandonAfter      = 10 * time.Minute
	gptIQBatchTimeout      = 90 * time.Minute
	gptIQLockTTL           = 5 * time.Minute
	gptIQScheduleWindow    = 3 * time.Hour
	gptIQConcurrency       = 3
	gptIQRetryDelay        = 30 * time.Second
	gptIQManualCooldown    = 5 * time.Minute
	gptIQDefaultBackoff    = 5 * time.Minute
	gptIQMaxBackoff        = 24 * time.Hour
	gptIQQuotaMaxAge       = 15 * time.Minute
	gptIQAnswerMaxBytes    = 64 << 10
	gptIQFinishTimeout     = 5 * time.Second
	gptIQRunTimeMinMinute  = 5 * 60
	gptIQRunTimeMaxMinute  = 11 * 60
	gptIQDateLayout        = "2006-01-02"

	// 批次未正常完成的原因（failure_reason）。
	GPTIQBatchFailureTimeout       = "timeout"
	GPTIQBatchFailureStopping      = "stopping"
	GPTIQBatchFailurePersistFailed = "persist_failed"
)

var (
	ErrGPTIQConfigConflict   = infraerrors.Conflict("GPT_IQ_TEST_CONFIG_CONFLICT", "GPT IQ test config changed; reload and retry")
	ErrGPTIQCaseConflict     = infraerrors.Conflict("GPT_IQ_TEST_CASE_CONFLICT", "test case changed; reload and retry")
	ErrGPTIQCaseNotFound     = infraerrors.NotFound("GPT_IQ_TEST_CASE_NOT_FOUND", "test case not found")
	ErrGPTIQEntryNotFound    = infraerrors.NotFound("GPT_IQ_TEST_ENTRY_NOT_FOUND", "account entry not found")
	ErrGPTIQBatchRunning     = infraerrors.Conflict("GPT_IQ_TEST_BATCH_RUNNING", "a GPT IQ test batch is already running")
	ErrGPTIQBatchNotFound    = infraerrors.NotFound("GPT_IQ_TEST_BATCH_NOT_FOUND", "test batch not found")
	ErrGPTIQBatchNotRunning  = infraerrors.Conflict("GPT_IQ_TEST_BATCH_NOT_RUNNING", "test batch is not running")
	ErrGPTIQBudgetExceeded   = infraerrors.Conflict("GPT_IQ_TEST_DAILY_BUDGET_EXCEEDED", "daily call budget is used up")
	ErrGPTIQScopeDenied      = infraerrors.Forbidden("GPT_IQ_TEST_SCOPE_DENIED", "operation is outside your organization scope")
	ErrGPTIQNothingToRun     = infraerrors.BadRequest("GPT_IQ_TEST_NOTHING_TO_RUN", "no account or test case to run")
	ErrGPTIQRejudgeNotGraded = infraerrors.BadRequest("GPT_IQ_TEST_REJUDGE_NOT_GRADED", "only graded test cases can be re-scored")
	ErrGPTIQNotConfigured    = infraerrors.ServiceUnavailable("GPT_IQ_TEST_NOT_CONFIGURED", "GPT IQ test service is not configured")
	ErrGPTIQServiceStopping  = infraerrors.ServiceUnavailable("GPT_IQ_TEST_STOPPING", "GPT IQ test service is stopping")
	// ErrGPTIQLeaseLost 表示批次已被回收（abandoned）或结束，旧执行者不得继续回写。
	ErrGPTIQLeaseLost = errors.New("gpt iq test batch lease lost")
	// ErrGPTIQBatchCancelRequested 表示批次已请求取消，派发前预占名额时拒绝继续派发。
	ErrGPTIQBatchCancelRequested = errors.New("gpt iq test batch cancel requested")
)

func gptIQInvalid(msg string) error {
	return infraerrors.BadRequest("GPT_IQ_TEST_INVALID", msg)
}

type GPTIQTestConfig struct {
	Enabled               bool      `json:"enabled"`
	ScheduleEnabled       bool      `json:"schedule_enabled"`
	RunTime               string    `json:"run_time"`
	LastRunDate           string    `json:"last_run_date,omitempty"`
	MinAccuracyPercent    int       `json:"min_accuracy_percent"`
	MinGradedRuns         int       `json:"min_graded_runs"`
	ReasoningRatioPercent int       `json:"reasoning_ratio_percent"`
	MinFiveHourPercent    int       `json:"min_five_hour_percent"`
	MinSevenDayPercent    int       `json:"min_seven_day_percent"`
	DailyCallBudget       int       `json:"daily_call_budget"`
	RetentionDays         int       `json:"retention_days"`
	Version               int64     `json:"version"`
	UpdatedBy             *int64    `json:"updated_by,omitempty"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type GPTIQTestConfigInput struct {
	Enabled               bool   `json:"enabled"`
	ScheduleEnabled       bool   `json:"schedule_enabled"`
	RunTime               string `json:"run_time"`
	MinAccuracyPercent    int    `json:"min_accuracy_percent"`
	MinGradedRuns         int    `json:"min_graded_runs"`
	ReasoningRatioPercent int    `json:"reasoning_ratio_percent"`
	MinFiveHourPercent    int    `json:"min_five_hour_percent"`
	MinSevenDayPercent    int    `json:"min_seven_day_percent"`
	DailyCallBudget       int    `json:"daily_call_budget"`
	RetentionDays         int    `json:"retention_days"`
	ExpectedVersion       int64  `json:"expected_version"`
}

// GPTIQTestCase 是测试项当前状态（可变属性 + 当前版本）。
type GPTIQTestCase struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Enabled      bool   `json:"enabled"`
	SortOrder    int    `json:"sort_order"`
	PublicPrompt bool   `json:"public_prompt"`
	PublicAnswer bool   `json:"public_answer"`
	Version      int64  `json:"version"`
	RevisionID   int64  `json:"revision_id"`
	Revision     int    `json:"revision"`
	GPTIQRevisionSpec
	UpdatedAt time.Time `json:"updated_at"`
}

type GPTIQTestRevision struct {
	ID       int64 `json:"id"`
	CaseID   int64 `json:"case_id"`
	Revision int   `json:"revision"`
	GPTIQRevisionSpec
}

type GPTIQTestBatch struct {
	ID              int64     `json:"id"`
	Trigger         string    `json:"trigger"`
	RunDate         string    `json:"run_date"`
	TriggeredBy     *int64    `json:"triggered_by,omitempty"`
	ScopeGroups     []string  `json:"scope_groups"`
	EntryIDs        []int64   `json:"entry_ids"`
	CaseIDs         []int64   `json:"case_ids"`
	Status          string    `json:"status"`
	LeaseToken      string    `json:"-"`
	CancelRequested bool      `json:"cancel_requested"`
	HeartbeatAt     time.Time `json:"heartbeat_at"`
	TotalCalls      int       `json:"total_calls"`
	// FailureReason 为空表示正常结束或仍在运行；否则见 GPTIQBatchFailure*。
	FailureReason string     `json:"failure_reason"`
	StartedAt     time.Time  `json:"started_at"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
}

// GPTIQTestBatchSummary 带批次内结果计数（SQL 聚合）。
type GPTIQTestBatchSummary struct {
	GPTIQTestBatch
	Completed int `json:"completed"`
	OK        int `json:"ok"`
	Errors    int `json:"errors"`
	Skipped   int `json:"skipped"`
	// Interrupted 为取消与执行中断（cancelled、abandoned）的调用，不算账号失败。
	Interrupted int `json:"interrupted"`
	Graded      int `json:"graded"`
	Correct     int `json:"correct"`
}

type GPTIQTestResult struct {
	ID               int64      `json:"id"`
	BatchID          int64      `json:"batch_id"`
	RunDate          string     `json:"run_date"`
	Trigger          string     `json:"trigger"`
	EntryID          int64      `json:"entry_id"`
	AccountID        int64      `json:"-"`
	CaseID           int64      `json:"case_id"`
	RevisionID       int64      `json:"revision_id"`
	ScoredRevisionID *int64     `json:"scored_revision_id,omitempty"`
	RunIndex         int        `json:"run_index"`
	Attempts         int        `json:"attempts"`
	Status           string     `json:"status"`
	Reason           string     `json:"reason,omitempty"`
	ErrorSummary     string     `json:"error_summary,omitempty"`
	HTTPStatus       *int       `json:"http_status,omitempty"`
	Correct          *bool      `json:"correct"`
	ModelRequested   string     `json:"model_requested"`
	ModelResponded   string     `json:"model_responded,omitempty"`
	ReasoningEffort  string     `json:"reasoning_effort,omitempty"`
	AnswerText       string     `json:"answer_text"`
	AnswerTruncated  bool       `json:"answer_truncated"`
	InputTokens      *int       `json:"input_tokens"`
	CachedTokens     *int       `json:"cached_tokens"`
	OutputTokens     *int       `json:"output_tokens"`
	ReasoningTokens  *int       `json:"reasoning_tokens"`
	TTFTMs           *int       `json:"ttft_ms"`
	DurationMs       *int       `json:"duration_ms"`
	StartedAt        time.Time  `json:"started_at"`
	FinishedAt       *time.Time `json:"finished_at,omitempty"`
}

type GPTIQCorrectUpdate struct {
	ResultID         int64
	Correct          *bool
	ScoredRevisionID int64
}

// GPTIQTestActor 是管理接口的操作者。两项能力分开表达：
//   - CanConfigure：可改全站配置、增改删测试项、重新判分（完整管理员、完整 API Key、持 admin.gpt_iq_test 的子管理员）；
//   - FullScope：运行、查看批次详情、取消的范围为全站。组织负责人（持 admin.organization_manage）即使另持
//     admin.gpt_iq_test，运行与详情仍限其授权组织（Groups），只能取消本人发起的手动批次。
type GPTIQTestActor struct {
	UserID       int64
	CanConfigure bool
	FullScope    bool
	Groups       []string
}

func (a *GPTIQTestActor) allows(group string) bool {
	if a == nil {
		return false
	}
	if a.FullScope {
		return true
	}
	for _, g := range a.Groups {
		if g == group {
			return true
		}
	}
	return false
}

func (a *GPTIQTestActor) scopeGroups() []string {
	if a == nil {
		return []string{}
	}
	if a.FullScope {
		return []string{GPTQuotaGroupXunyou, GPTQuotaGroupWsdashi}
	}
	return append([]string{}, a.Groups...)
}

type GPTIQTestRepository interface {
	GetConfig(ctx context.Context) (*GPTIQTestConfig, error)
	// SaveConfig 按 expectedVersion 条件更新；markRunDate 非空时把 last_run_date 抬到该日期，避免隐式补跑。
	SaveConfig(ctx context.Context, cfg GPTIQTestConfig, expectedVersion, updatedBy int64, markRunDate string) (*GPTIQTestConfig, error)
	ListCases(ctx context.Context) ([]GPTIQTestCase, error)
	GetCase(ctx context.Context, id int64) (*GPTIQTestCase, error)
	CreateCase(ctx context.Context, in GPTIQCaseInput, spec GPTIQRevisionSpec, actorID int64) (*GPTIQTestCase, error)
	// UpdateCase 按 expectedVersion 条件更新；版本字段变化时新建不可变版本。
	UpdateCase(ctx context.Context, id, expectedVersion int64, in GPTIQCaseInput, spec GPTIQRevisionSpec, actorID int64) (*GPTIQTestCase, error)
	DeleteCase(ctx context.Context, id, expectedVersion, actorID int64) error
	ListRevisions(ctx context.Context, ids []int64) (map[int64]*GPTIQTestRevision, error)

	// CreateBatch 插入运行中批次；已有运行中批次时返回 ErrGPTIQBatchRunning。
	CreateBatch(ctx context.Context, batch *GPTIQTestBatch) (*GPTIQTestBatch, error)
	ClaimRunDate(ctx context.Context, date string) (bool, error)
	FinishBatch(ctx context.Context, id int64, token, status string) error
	// CloseBatch 同 FinishBatch，并记录未正常完成的原因（failure_reason）。
	CloseBatch(ctx context.Context, id int64, token, status, failureReason string) error
	HeartbeatBatch(ctx context.Context, id int64, token string) (alive, cancelRequested bool, err error)
	RequestCancel(ctx context.Context, id int64) (bool, error)
	// ReapAbandoned 把心跳超过 staleAfter（按数据库时钟）的运行中批次标为 abandoned 并释放全站占用。
	ReapAbandoned(ctx context.Context, staleAfter time.Duration) (int, error)
	GetBatch(ctx context.Context, id int64) (*GPTIQTestBatch, error)
	ListBatchStatuses(ctx context.Context, ids []int64) (map[int64]string, error)
	// ListBatchSummaries 列出最近批次；entryIDs 非 nil 时只返回包含这些条目的批次，计数也只统计这些条目。
	ListBatchSummaries(ctx context.Context, limit int, entryIDs []int64) ([]GPTIQTestBatchSummary, error)

	// ReserveDispatch 在同一事务内校验批次执行权（运行中、令牌一致、未请求取消）并原子预占一次当日派发名额；
	// 执行权失效返回 ErrGPTIQLeaseLost，已请求取消返回 ErrGPTIQBatchCancelRequested。ReleaseDispatch 归还未实际发出的名额。
	ReserveDispatch(ctx context.Context, batchID int64, token, date string, budget int) (bool, error)
	ReleaseDispatch(ctx context.Context, date string) error
	DispatchedOn(ctx context.Context, date string) (int, error)

	// InsertResult / FinishResult 校验批次执行权令牌，失效时返回 ErrGPTIQLeaseLost。
	InsertResult(ctx context.Context, token string, result *GPTIQTestResult) (int64, error)
	FinishResult(ctx context.Context, token string, result *GPTIQTestResult) error
	ListBatchResults(ctx context.Context, batchID int64, answerLimit int) ([]GPTIQTestResult, error)
	ListResultsSince(ctx context.Context, since string, entryIDs []int64, answerLimit int) ([]GPTIQTestResult, error)
	ListEntryResults(ctx context.Context, entryID, caseID int64, since string) ([]GPTIQTestResult, error)
	RecentRunExists(ctx context.Context, entryID, caseID int64, since time.Time) (bool, error)

	ListBackoff(ctx context.Context, accountIDs []int64) (map[int64]time.Time, error)
	SetBackoff(ctx context.Context, accountID int64, until time.Time, reason string) error

	ListRejudgeCandidates(ctx context.Context, caseID int64, promptSHA256 string) ([]GPTIQTestResult, error)
	// UpdateCorrect 只在结果当前的 scored_revision_id 为空或不大于目标版本时写入，较早的重评不能覆盖较新版本的分数。
	UpdateCorrect(ctx context.Context, updates []GPTIQCorrectUpdate) error
	// CountScoredWith 返回 ids 中当前以 revisionID 判分的结果数（重评回报实际写入数）。
	CountScoredWith(ctx context.Context, ids []int64, revisionID int64) (int, error)
	Cleanup(ctx context.Context, before string) (int64, error)

	// LoadActor 按认证中间件写入的操作者 ID 重读角色、权限与组织授权；无权时返回 ErrGPTIQScopeDenied。
	LoadActor(ctx context.Context) (*GPTIQTestActor, error)
}

type GPTIQTestService struct {
	repo     GPTIQTestRepository
	quota    *GPTQuotaDisplayService
	accounts AccountRepository
	prober   GPTIQProber
	locks    LeaderLockCache
	db       *sql.DB
	now      func() time.Time

	retryDelay        time.Duration
	heartbeatInterval time.Duration
	batchTimeout      time.Duration

	lifecycleMu sync.Mutex
	started     bool
	stopped     bool
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup

	cleanupMu   sync.Mutex
	lastCleanup string
}

func NewGPTIQTestService(repo GPTIQTestRepository, quota *GPTQuotaDisplayService, accounts AccountRepository, prober GPTIQProber, locks LeaderLockCache, db *sql.DB) *GPTIQTestService {
	ctx, cancel := context.WithCancel(context.Background())
	return &GPTIQTestService{
		repo:              repo,
		quota:             quota,
		accounts:          accounts,
		prober:            prober,
		locks:             locks,
		db:                db,
		now:               time.Now,
		retryDelay:        gptIQRetryDelay,
		heartbeatInterval: gptIQHeartbeatInterval,
		batchTimeout:      gptIQBatchTimeout,
		ctx:               ctx,
		cancel:            cancel,
	}
}

func (s *GPTIQTestService) Start() {
	if s == nil || s.repo == nil {
		return
	}
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	if s.started || s.stopped {
		return
	}
	s.started = true
	s.wg.Add(1)
	go s.runLoop()
}

// Stop 取消在途批次（含上游请求）并等待后台 goroutine 退出。
func (s *GPTIQTestService) Stop() {
	if s == nil {
		return
	}
	s.lifecycleMu.Lock()
	if s.stopped {
		s.lifecycleMu.Unlock()
		return
	}
	s.stopped = true
	s.cancel()
	s.lifecycleMu.Unlock()
	s.wg.Wait()
}

func (s *GPTIQTestService) goTracked(fn func()) bool {
	s.lifecycleMu.Lock()
	if s.stopped {
		s.lifecycleMu.Unlock()
		return false
	}
	s.wg.Add(1)
	s.lifecycleMu.Unlock()
	go func() {
		defer s.wg.Done()
		fn()
	}()
	return true
}

func (s *GPTIQTestService) localNow() time.Time {
	return s.now().In(gptQuotaDisplayLocation)
}

func (s *GPTIQTestService) localDate() string {
	return s.localNow().Format(gptIQDateLayout)
}

func (s *GPTIQTestService) ready() error {
	if s == nil || s.repo == nil {
		return ErrGPTIQNotConfigured
	}
	return nil
}

func detachedGPTIQContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), gptIQFinishTimeout)
}

// ---- 排程 ----

func (s *GPTIQTestService) runLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(gptIQTickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.tick(s.ctx)
		}
	}
}

func (s *GPTIQTestService) tick(ctx context.Context) {
	if reaped, err := s.repo.ReapAbandoned(ctx, gptIQAbandonAfter); err != nil {
		if ctx.Err() == nil {
			slog.Warn("gpt_iq_test_reap_failed", "error", err)
		}
	} else if reaped > 0 {
		slog.Warn("gpt_iq_test_batches_abandoned", "count", reaped)
	}
	cfg, err := s.repo.GetConfig(ctx)
	if err != nil {
		if ctx.Err() == nil {
			slog.Warn("gpt_iq_test_config_load_failed", "error", err)
		}
		return
	}
	s.maybeCleanup(ctx, cfg)
	s.runScheduled(ctx, cfg)
}

// gptIQRunWindow 返回某日的计划时刻与窗口结束时刻（Asia/Shanghai）。
func gptIQRunWindow(now time.Time, runTime string) (start, end time.Time, ok bool) {
	minute, ok := parseGPTIQRunTime(runTime)
	if !ok {
		return time.Time{}, time.Time{}, false
	}
	local := now.In(gptQuotaDisplayLocation)
	start = time.Date(local.Year(), local.Month(), local.Day(), minute/60, minute%60, 0, 0, gptQuotaDisplayLocation)
	return start, start.Add(gptIQScheduleWindow), true
}

func parseGPTIQRunTime(raw string) (int, bool) {
	t, err := time.Parse("15:04", strings.TrimSpace(raw))
	if err != nil {
		return 0, false
	}
	return t.Hour()*60 + t.Minute(), true
}

// runScheduled 先占用全站唯一的运行中批次，再条件领取当日；领取失败时中止批次，不留下
// "已领取但未执行"的日期（同额度展示 §12.9）。手动批次在途时不消耗当日，窗口内下次检查重试。
func (s *GPTIQTestService) runScheduled(ctx context.Context, cfg *GPTIQTestConfig) {
	if !cfg.ScheduleEnabled {
		return
	}
	now := s.localNow()
	date := now.Format(gptIQDateLayout)
	start, end, ok := gptIQRunWindow(now, cfg.RunTime)
	if !ok || now.Before(start) || !now.Before(end) || cfg.LastRunDate == date {
		return
	}
	owner := fmt.Sprintf("gpt-iq-test-%d", time.Now().UnixNano())
	release, acquired := tryAcquireSingletonLeaderLock(ctx, s.locks, s.db, GPTIQTestLeaderKey, owner, gptIQLockTTL)
	if !acquired {
		return
	}
	defer release()

	entries, err := s.visibleEntries(ctx)
	if err != nil {
		slog.Warn("gpt_iq_test_list_entries_failed", "error", err)
		return
	}
	allCases, err := s.repo.ListCases(ctx)
	if err != nil {
		slog.Warn("gpt_iq_test_list_cases_failed", "error", err)
		return
	}
	cases := make([]GPTIQTestCase, 0, len(allCases))
	for _, c := range allCases {
		if c.Enabled {
			cases = append(cases, c)
		}
	}
	if len(entries) == 0 || len(cases) == 0 {
		if claimed, err := s.repo.ClaimRunDate(ctx, date); err == nil && claimed {
			slog.Info("gpt_iq_test_schedule_nothing_to_run", "date", date, "entries", len(entries), "cases", len(cases))
		}
		return
	}
	batch, err := s.repo.CreateBatch(ctx, newGPTIQBatch(GPTIQTriggerSchedule, date, nil, (&GPTIQTestActor{FullScope: true}).scopeGroups(), entries, cases))
	if err != nil {
		if !errors.Is(err, ErrGPTIQBatchRunning) {
			slog.Warn("gpt_iq_test_create_batch_failed", "error", err)
		}
		return
	}
	claimed, err := s.repo.ClaimRunDate(ctx, date)
	if err != nil || !claimed {
		finishCtx, cancel := detachedGPTIQContext(ctx)
		_ = s.repo.FinishBatch(finishCtx, batch.ID, batch.LeaseToken, GPTIQBatchAborted)
		cancel()
		if err != nil {
			slog.Warn("gpt_iq_test_claim_date_failed", "date", date, "error", err)
		}
		return
	}
	if !s.goTracked(func() { s.executeBatch(s.ctx, batch, entries, cases, cfg) }) {
		finishCtx, cancel := detachedGPTIQContext(ctx)
		_ = s.repo.CloseBatch(finishCtx, batch.ID, batch.LeaseToken, GPTIQBatchAborted, GPTIQBatchFailureStopping)
		cancel()
	}
}

// maybeCleanup 每个实例每天最多清理一次超出保留期的结果与批次，与是否跑批次无关。
func (s *GPTIQTestService) maybeCleanup(ctx context.Context, cfg *GPTIQTestConfig) {
	today := s.localDate()
	s.cleanupMu.Lock()
	if s.lastCleanup == today {
		s.cleanupMu.Unlock()
		return
	}
	s.lastCleanup = today
	s.cleanupMu.Unlock()
	retention := cfg.RetentionDays
	if retention <= 0 {
		retention = 90
	}
	before := s.localNow().AddDate(0, 0, -retention).Format(gptIQDateLayout)
	if removed, err := s.repo.Cleanup(ctx, before); err != nil {
		slog.Warn("gpt_iq_test_cleanup_failed", "error", err)
		s.cleanupMu.Lock()
		s.lastCleanup = ""
		s.cleanupMu.Unlock()
	} else if removed > 0 {
		slog.Info("gpt_iq_test_cleanup_done", "before", before, "removed", removed)
	}
}

func newGPTIQBatch(trigger, date string, triggeredBy *int64, groups []string, entries []gptIQEntry, cases []GPTIQTestCase) *GPTIQTestBatch {
	batch := &GPTIQTestBatch{
		Trigger:     trigger,
		RunDate:     date,
		TriggeredBy: triggeredBy,
		ScopeGroups: groups,
		Status:      GPTIQBatchRunning,
		LeaseToken:  uuid.NewString(),
		EntryIDs:    make([]int64, 0, len(entries)),
		CaseIDs:     make([]int64, 0, len(cases)),
	}
	runs := 0
	for _, c := range cases {
		batch.CaseIDs = append(batch.CaseIDs, c.ID)
		runs += c.Runs
	}
	for _, e := range entries {
		batch.EntryIDs = append(batch.EntryIDs, e.ID)
	}
	batch.TotalCalls = runs * len(entries)
	return batch
}

// ---- 账号集合（复用额度展示条目） ----

type gptIQEntry struct {
	ID          int64
	AccountID   int64
	DisplayName string
	Group       string
	Eligible    bool
}

// selectedEntries 读取额度展示的全部已选条目（含失去资格的），已按额度展示的自然顺序排序。
func (s *GPTIQTestService) selectedEntries(ctx context.Context) ([]gptIQEntry, error) {
	if s.quota == nil || s.quota.repo == nil {
		return []gptIQEntry{}, nil
	}
	raw, err := s.quota.repo.ListSelectedEntries(ctx)
	if err != nil {
		return nil, err
	}
	sortGPTQuotaEntries(raw)
	out := make([]gptIQEntry, 0, len(raw))
	for _, e := range raw {
		ok, _ := e.eligibility()
		out = append(out, gptIQEntry{ID: e.ID, AccountID: e.AccountID, DisplayName: e.effectiveDisplayName(), Group: GPTQuotaClass(e.AccountName), Eligible: ok})
	}
	return out, nil
}

// visibleEntries 是用户页与测试的账号集合：已选且仍有资格的额度展示条目。
func (s *GPTIQTestService) visibleEntries(ctx context.Context) ([]gptIQEntry, error) {
	all, err := s.selectedEntries(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]gptIQEntry, 0, len(all))
	for _, e := range all {
		if e.Eligible {
			out = append(out, e)
		}
	}
	return out, nil
}

// ---- 批次执行 ----

type gptIQBatchState struct {
	budgetExhausted atomic.Bool
	leaseLost       atomic.Bool
	cancelled       atomic.Bool
	// persistFailed：结果写入出现非执行权错误，批次不能标为完成。
	persistFailed atomic.Bool
}

// executeBatch 跨账号并发 3、账号内测试项与次数串行执行；心跳同时检查取消与执行权。
func (s *GPTIQTestService) executeBatch(root context.Context, batch *GPTIQTestBatch, entries []gptIQEntry, cases []GPTIQTestCase, cfg *GPTIQTestConfig) {
	ctx, cancel := context.WithTimeout(root, s.batchTimeout)
	defer cancel()
	state := &gptIQBatchState{}

	hbDone := make(chan struct{})
	hbStopped := make(chan struct{})
	go func() {
		defer close(hbStopped)
		ticker := time.NewTicker(s.heartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-hbDone:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				alive, cancelRequested, err := s.repo.HeartbeatBatch(ctx, batch.ID, batch.LeaseToken)
				if err != nil {
					continue
				}
				if !alive {
					state.leaseLost.Store(true)
					cancel()
					return
				}
				if cancelRequested {
					state.cancelled.Store(true)
					cancel()
					return
				}
			}
		}
	}()

	backoffs := map[int64]time.Time{}
	if len(entries) > 0 {
		ids := make([]int64, 0, len(entries))
		for _, e := range entries {
			ids = append(ids, e.AccountID)
		}
		if m, err := s.repo.ListBackoff(ctx, ids); err == nil {
			backoffs = m
		}
	}

	sem := make(chan struct{}, gptIQConcurrency)
	var wg sync.WaitGroup
	for _, entry := range entries {
		sem <- struct{}{}
		if ctx.Err() != nil {
			<-sem
			break
		}
		wg.Add(1)
		go func(entry gptIQEntry) {
			defer wg.Done()
			defer func() { <-sem }()
			s.runEntry(ctx, cancel, batch, entry, cases, cfg, backoffs[entry.AccountID], state)
		}(entry)
	}
	wg.Wait()
	close(hbDone)
	<-hbStopped

	if state.leaseLost.Load() {
		slog.Warn("gpt_iq_test_batch_lease_lost", "batch_id", batch.ID)
		return
	}
	status, reason := GPTIQBatchFinished, ""
	switch {
	case state.cancelled.Load():
		status = GPTIQBatchCancelled
	case root.Err() != nil:
		status, reason = GPTIQBatchAborted, GPTIQBatchFailureStopping
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		// 批次超过 90 分钟上限：未跑完的账号不应显示为"完成"。
		status, reason = GPTIQBatchAborted, GPTIQBatchFailureTimeout
	case state.persistFailed.Load():
		// 结果写入失败：计划的调用没有全部落库，保留部分完成状态与原因。
		status, reason = GPTIQBatchAborted, GPTIQBatchFailurePersistFailed
	}
	finishCtx, finishCancel := detachedGPTIQContext(root)
	defer finishCancel()
	if err := s.repo.CloseBatch(finishCtx, batch.ID, batch.LeaseToken, status, reason); err != nil {
		slog.Warn("gpt_iq_test_finish_batch_failed", "batch_id", batch.ID, "error", err)
	}
	slog.Info("gpt_iq_test_batch_done", "batch_id", batch.ID, "trigger", batch.Trigger, "status", status)
}

func (s *GPTIQTestService) runEntry(ctx context.Context, cancelBatch context.CancelFunc, batch *GPTIQTestBatch, entry gptIQEntry, cases []GPTIQTestCase, cfg *GPTIQTestConfig, backoffUntil time.Time, state *gptIQBatchState) {
	account, err := s.accounts.GetByID(ctx, entry.AccountID)
	if err != nil {
		account = nil
	}
	if ctx.Err() != nil {
		return
	}
	entrySkip := s.entrySkipReason(ctx, account, cfg, backoffUntil)
	for _, c := range cases {
		caseSkip := ""
		if entrySkip == "" && batch.Trigger == GPTIQTriggerManual {
			if recent, err := s.repo.RecentRunExists(ctx, entry.ID, c.ID, s.now().Add(-gptIQManualCooldown)); err == nil && recent {
				caseSkip = GPTIQSkipCooldown
			}
		}
		for run := 1; run <= c.Runs; run++ {
			if ctx.Err() != nil {
				return
			}
			result := &GPTIQTestResult{
				BatchID: batch.ID, RunDate: batch.RunDate, Trigger: batch.Trigger,
				EntryID: entry.ID, AccountID: entry.AccountID, CaseID: c.ID, RevisionID: c.RevisionID,
				RunIndex: run, ModelRequested: c.Model, ReasoningEffort: c.ReasoningEffort,
				StartedAt: s.now().UTC(),
			}
			skip := entrySkip
			if skip == "" {
				skip = caseSkip
			}
			if skip == "" && state.budgetExhausted.Load() {
				skip = GPTIQSkipBudget
			}
			if skip != "" {
				finished := result.StartedAt
				result.Status, result.Reason, result.FinishedAt = GPTIQResultSkipped, skip, &finished
				if _, err := s.repo.InsertResult(ctx, batch.LeaseToken, result); err != nil {
					s.handleWriteError(err, batch, state, cancelBatch)
					return
				}
				continue
			}
			result.Status = GPTIQResultRunning
			id, err := s.repo.InsertResult(ctx, batch.LeaseToken, result)
			if err != nil {
				s.handleWriteError(err, batch, state, cancelBatch)
				return
			}
			result.ID = id
			probe := s.probeWithRetry(ctx, cancelBatch, batch, account, c, state)
			if ctx.Err() != nil && probe.Reason != "" {
				// 批次被取消、停机或到达批次上限时，在途调用的失败归因于批次中断，不算账号失败。
				probe.Reason, probe.Transient = GPTQuotaStatusCancelled, false
			}
			s.applyProbe(result, c, probe)
			if err := s.finishResult(ctx, batch.LeaseToken, result); err != nil {
				s.handleWriteError(err, batch, state, cancelBatch)
				return
			}
			switch probe.Reason {
			case GPTQuotaStatusUnauthorized, GPTQuotaStatusForbidden, GPTQuotaStatusTokenUnavailable, GPTQuotaStatusNotSupported:
				entrySkip = GPTIQSkipAfterAuthFailure
			case GPTQuotaStatusRateLimited:
				entrySkip = GPTIQSkipAfterRateLimit
				backoff := probe.RetryAfter
				if backoff <= 0 {
					backoff = gptIQDefaultBackoff
				}
				backoff = min(backoff, gptIQMaxBackoff)
				backoffCtx, backoffCancel := detachedGPTIQContext(ctx)
				if err := s.repo.SetBackoff(backoffCtx, entry.AccountID, s.now().Add(backoff), GPTQuotaStatusRateLimited); err != nil {
					slog.Warn("gpt_iq_test_set_backoff_failed", "account_id", entry.AccountID, "error", err)
				}
				backoffCancel()
			}
		}
	}
}

// probeWithRetry 每次实际派发前原子预占当日名额（首发与重试各占一次）；未发出的请求归还名额。
// 超时、5xx、流中断重试一次。
func (s *GPTIQTestService) probeWithRetry(ctx context.Context, cancelBatch context.CancelFunc, batch *GPTIQTestBatch, account *Account, c GPTIQTestCase, state *gptIQBatchState) GPTIQProbeResult {
	req := GPTIQProbeRequest{Model: c.Model, ReasoningEffort: c.ReasoningEffort, Prompt: c.Prompt, Timeout: time.Duration(c.TimeoutSeconds) * time.Second}
	var last GPTIQProbeResult
	for attempt := 1; attempt <= 2; attempt++ {
		date := s.localDate()
		cfg, err := s.repo.GetConfig(ctx)
		if err != nil {
			if attempt == 1 {
				last = GPTIQProbeResult{Reason: GPTQuotaStatusInternalError}
				if ctx.Err() != nil {
					last.Reason = GPTQuotaStatusCancelled
				}
			}
			return last
		}
		reserved, err := s.repo.ReserveDispatch(ctx, batch.ID, batch.LeaseToken, date, cfg.DailyCallBudget)
		switch {
		case errors.Is(err, ErrGPTIQLeaseLost):
			// 批次已被回收或结束：旧执行者不得再派发（心跳尚未发现时也在这里拦住）。
			state.leaseLost.Store(true)
			cancelBatch()
			return GPTIQProbeResult{Reason: GPTQuotaStatusCancelled, attempts: max(attempt-1, 1)}
		case errors.Is(err, ErrGPTIQBatchCancelRequested):
			state.cancelled.Store(true)
			cancelBatch()
			return GPTIQProbeResult{Reason: GPTQuotaStatusCancelled, attempts: max(attempt-1, 1)}
		}
		if err != nil || !reserved {
			if !reserved && err == nil {
				state.budgetExhausted.Store(true)
			}
			if attempt == 1 {
				last = GPTIQProbeResult{Reason: GPTIQSkipBudget}
				if err != nil {
					last.Reason = GPTQuotaStatusInternalError
					if ctx.Err() != nil {
						last.Reason = GPTQuotaStatusCancelled
					}
				}
			}
			return last
		}
		res := s.prober.Probe(ctx, account, req)
		if !res.Dispatched {
			releaseCtx, releaseCancel := detachedGPTIQContext(ctx)
			_ = s.repo.ReleaseDispatch(releaseCtx, date)
			releaseCancel()
		}
		res.attempts = attempt
		last = res
		if !res.Transient || attempt == 2 || ctx.Err() != nil {
			return last
		}
		timer := time.NewTimer(s.retryDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			last.Reason, last.Transient = GPTQuotaStatusCancelled, false
			return last
		case <-timer.C:
		}
	}
	return last
}

// handleWriteError 处理结果写入失败：执行权丢失则停止整批；其余错误记为持久化失败，批次收尾时不标完成。
func (s *GPTIQTestService) handleWriteError(err error, batch *GPTIQTestBatch, state *gptIQBatchState, cancelBatch context.CancelFunc) {
	if errors.Is(err, ErrGPTIQLeaseLost) {
		state.leaseLost.Store(true)
		cancelBatch()
		return
	}
	state.persistFailed.Store(true)
	slog.Warn("gpt_iq_test_result_write_failed", "batch_id", batch.ID, "error", err)
}

// finishResult 写入调用结果；非执行权错误记录日志并用新的短超时重试一次，避免结果行停在 running
// 后被批次收尾改写成 cancelled 而丢失真实失败。
func (s *GPTIQTestService) finishResult(ctx context.Context, token string, result *GPTIQTestResult) error {
	var err error
	for attempt := 1; attempt <= 2; attempt++ {
		finishCtx, cancel := detachedGPTIQContext(ctx)
		err = s.repo.FinishResult(finishCtx, token, result)
		cancel()
		if err == nil || errors.Is(err, ErrGPTIQLeaseLost) {
			return err
		}
		slog.Warn("gpt_iq_test_finish_result_failed", "result_id", result.ID, "attempt", attempt, "error", err)
	}
	return err
}

// applyProbe 把上游结果写入结果行：成功时按当前版本判分；回答超过上限截断并标记（判分用全文）。
func (s *GPTIQTestService) applyProbe(result *GPTIQTestResult, c GPTIQTestCase, probe GPTIQProbeResult) {
	finished := s.now().UTC()
	result.FinishedAt = &finished
	result.Attempts = max(probe.attempts, 1)
	if probe.ModelRequested != "" {
		result.ModelRequested = probe.ModelRequested
	}
	result.ModelResponded = truncateGPTIQString(gptIQCleanText(probe.ModelResponded), 100)
	result.InputTokens, result.CachedTokens = probe.InputTokens, probe.CachedTokens
	result.OutputTokens, result.ReasoningTokens = probe.OutputTokens, probe.ReasoningTokens
	if probe.HTTPStatus > 0 {
		status := probe.HTTPStatus
		result.HTTPStatus = &status
	}
	if probe.Duration > 0 {
		ms := int(probe.Duration.Milliseconds())
		result.DurationMs = &ms
	}
	if probe.TTFT > 0 {
		ms := int(probe.TTFT.Milliseconds())
		result.TTFTMs = &ms
	}
	answer := gptIQCleanText(probe.Answer)
	if len(answer) > gptIQAnswerMaxBytes {
		result.AnswerText = strings.ToValidUTF8(answer[:gptIQAnswerMaxBytes], "")
		result.AnswerTruncated = true
	} else {
		result.AnswerText = answer
	}
	switch probe.Reason {
	case "":
		result.Status = GPTIQResultOK
		result.Correct = ScoreGPTIQAnswer(c.GPTIQRevisionSpec, answer)
		if result.Correct != nil {
			rev := c.RevisionID
			result.ScoredRevisionID = &rev
		}
	case GPTIQSkipBudget:
		result.Status, result.Reason = GPTIQResultSkipped, GPTIQSkipBudget
	case GPTQuotaStatusCancelled:
		result.Status, result.Reason = GPTIQResultCancelled, GPTQuotaStatusCancelled
	default:
		result.Status, result.Reason = GPTIQResultError, probe.Reason
		result.ErrorSummary = gptIQCleanText(probe.ErrorSummary)
	}
}

// gptIQCleanText 去掉 PostgreSQL text 不接受的 NUL 与非法 UTF-8。上游回答、错误体与模型名都可能带。
func gptIQCleanText(s string) string {
	return strings.ToValidUTF8(strings.ReplaceAll(s, "\x00", ""), "")
}

func truncateGPTIQString(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	return string([]rune(s)[:maxRunes])
}

// entrySkipReason 判定账号本批次是否整体跳过：不可用、错误、已知限流、本功能退避、token 不可用、
// 额度不足。暂停调度（schedulable=false）的账号照常测试，以便观察恢复情况。
func (s *GPTIQTestService) entrySkipReason(ctx context.Context, account *Account, cfg *GPTIQTestConfig, backoffUntil time.Time) string {
	now := s.now()
	switch {
	case account == nil:
		return GPTIQSkipAccountUnavailable
	case func() bool { ok, _ := GPTQuotaAccountEligibility(account); return !ok }():
		return GPTIQSkipNotSupported
	case !account.IsActive():
		return GPTIQSkipAccountError
	case (account.RateLimitResetAt != nil && now.Before(*account.RateLimitResetAt)) ||
		(account.OverloadUntil != nil && now.Before(*account.OverloadUntil)) ||
		(account.TempUnschedulableUntil != nil && now.Before(*account.TempUnschedulableUntil)):
		return GPTIQSkipAccountRateLimited
	case !backoffUntil.IsZero() && now.Before(backoffUntil):
		return GPTIQSkipBackoff
	case !gptIQTokenUsable(account, now):
		return GPTIQSkipTokenUnavailable
	}
	switch s.quotaCheck(ctx, account.ID, cfg) {
	case gptIQQuotaLow:
		return GPTIQSkipLowQuota
	case gptIQQuotaUnknown:
		return GPTIQSkipQuotaUnknown
	}
	return ""
}

type gptIQQuotaState int

const (
	gptIQQuotaOK gptIQQuotaState = iota
	gptIQQuotaLow
	gptIQQuotaUnknown
)

// quotaCheck 执行前按需只读刷新一次额度（复用额度展示的冷却、退避与快照发布），再读快照：
//   - 读取失败、无快照、采样超过 15 分钟：未知，跳过本账号，不消耗真实额度；
//   - 窗口重置时间已过而采样早于重置：旧窗口已失效，等同未知（要靠新采样决定是否放行）；
//   - 任一配置了阈值的窗口剩余低于阈值：额度不足。上游未提供的窗口不参与判断。
func (s *GPTIQTestService) quotaCheck(ctx context.Context, accountID int64, cfg *GPTIQTestConfig) gptIQQuotaState {
	if cfg.MinFiveHourPercent <= 0 && cfg.MinSevenDayPercent <= 0 {
		return gptIQQuotaOK
	}
	if s.quota == nil || s.quota.repo == nil {
		return gptIQQuotaUnknown
	}
	// 额度展示失败时保留上次成功快照，所以不能只看快照：本次刷新失败或处于 429 退避都按未知处理；
	// 只有正常 60 秒冷却（他人刚采过）且最近一次尝试成功时，才复用那次的快照。
	refresh := s.quota.refreshAccount(ctx, accountID)
	if refresh != GPTQuotaStatusOK && refresh != GPTQuotaStatusSkippedCooldown {
		return gptIQQuotaUnknown
	}
	snaps, err := s.quota.repo.ListSnapshots(ctx, []int64{accountID})
	if err != nil {
		return gptIQQuotaUnknown
	}
	snap := snaps[accountID]
	now := s.now()
	if snap == nil || snap.SampledAt == nil || now.Sub(*snap.SampledAt) > gptIQQuotaMaxAge {
		return gptIQQuotaUnknown
	}
	if refresh == GPTQuotaStatusSkippedCooldown && snap.LastAttemptStatus != GPTQuotaStatusOK {
		return gptIQQuotaUnknown
	}
	worst := gptIQQuotaOK
	for _, check := range []struct {
		w   *GPTQuotaWindow
		min int
	}{{snap.FiveHour, cfg.MinFiveHourPercent}, {snap.SevenDay, cfg.MinSevenDayPercent}} {
		switch gptIQWindowState(check.w, check.min, *snap.SampledAt, now) {
		case gptIQQuotaLow:
			return gptIQQuotaLow
		case gptIQQuotaUnknown:
			worst = gptIQQuotaUnknown
		}
	}
	return worst
}

func gptIQWindowState(w *GPTQuotaWindow, minPercent int, sampledAt, now time.Time) gptIQQuotaState {
	if w == nil || minPercent <= 0 {
		return gptIQQuotaOK
	}
	if w.ResetAt != nil && !w.ResetAt.After(now) && sampledAt.Before(*w.ResetAt) {
		return gptIQQuotaUnknown
	}
	if w.RemainingPercent < float64(minPercent) {
		return gptIQQuotaLow
	}
	return gptIQQuotaOK
}

// ---- 管理操作 ----

// Actor 读取当前操作者范围。
func (s *GPTIQTestService) Actor(ctx context.Context) (*GPTIQTestActor, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	return s.repo.LoadActor(ctx)
}

// requireConfigure 拒绝没有全站配置能力的操作者（只持组织权限的组织负责人）。
func requireGPTIQConfigure(actor *GPTIQTestActor) error {
	if actor == nil || !actor.CanConfigure {
		return ErrGPTIQScopeDenied
	}
	return nil
}

func (s *GPTIQTestService) SaveConfig(ctx context.Context, actor *GPTIQTestActor, in GPTIQTestConfigInput) (*GPTIQTestConfig, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if err := requireGPTIQConfigure(actor); err != nil {
		return nil, err
	}
	in.RunTime = strings.TrimSpace(in.RunTime)
	minute, ok := parseGPTIQRunTime(in.RunTime)
	if !ok || minute < gptIQRunTimeMinMinute || minute > gptIQRunTimeMaxMinute {
		return nil, gptIQInvalid("run_time must be HH:MM between 05:00 and 11:00")
	}
	switch {
	case in.MinAccuracyPercent < 0 || in.MinAccuracyPercent > 100,
		in.ReasoningRatioPercent < 0 || in.ReasoningRatioPercent > 100,
		in.MinFiveHourPercent < 0 || in.MinFiveHourPercent > 100,
		in.MinSevenDayPercent < 0 || in.MinSevenDayPercent > 100:
		return nil, gptIQInvalid("percent thresholds must be 0-100")
	case in.MinGradedRuns < 1 || in.MinGradedRuns > gptIQMaxRuns:
		return nil, gptIQInvalid("min_graded_runs must be 1-5")
	case in.DailyCallBudget < 0 || in.DailyCallBudget > 5000:
		return nil, gptIQInvalid("daily_call_budget must be 0-5000")
	case in.RetentionDays < 7 || in.RetentionDays > 365:
		return nil, gptIQInvalid("retention_days must be 7-365")
	}
	cur, err := s.repo.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	runTime := fmt.Sprintf("%02d:%02d", minute/60, minute%60)
	markDate := ""
	if in.ScheduleEnabled && (!cur.ScheduleEnabled || runTime != cur.RunTime) {
		now := s.localNow()
		if start, _, ok := gptIQRunWindow(now, runTime); ok && !now.Before(start) {
			markDate = now.Format(gptIQDateLayout)
		}
	}
	cfg := GPTIQTestConfig{
		Enabled: in.Enabled, ScheduleEnabled: in.ScheduleEnabled, RunTime: runTime,
		MinAccuracyPercent: in.MinAccuracyPercent, MinGradedRuns: in.MinGradedRuns, ReasoningRatioPercent: in.ReasoningRatioPercent,
		MinFiveHourPercent: in.MinFiveHourPercent, MinSevenDayPercent: in.MinSevenDayPercent,
		DailyCallBudget: in.DailyCallBudget, RetentionDays: in.RetentionDays,
	}
	return s.repo.SaveConfig(ctx, cfg, in.ExpectedVersion, actor.UserID, markDate)
}

func (s *GPTIQTestService) CreateCase(ctx context.Context, actor *GPTIQTestActor, in GPTIQCaseInput) (*GPTIQTestCase, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if err := requireGPTIQConfigure(actor); err != nil {
		return nil, err
	}
	normalized, spec, err := NormalizeGPTIQCaseInput(in)
	if err != nil {
		return nil, gptIQInvalid(err.Error())
	}
	return s.repo.CreateCase(ctx, normalized, spec, actor.UserID)
}

func (s *GPTIQTestService) UpdateCase(ctx context.Context, actor *GPTIQTestActor, id, expectedVersion int64, in GPTIQCaseInput) (*GPTIQTestCase, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if err := requireGPTIQConfigure(actor); err != nil {
		return nil, err
	}
	normalized, spec, err := NormalizeGPTIQCaseInput(in)
	if err != nil {
		return nil, gptIQInvalid(err.Error())
	}
	return s.repo.UpdateCase(ctx, id, expectedVersion, normalized, spec, actor.UserID)
}

func (s *GPTIQTestService) DeleteCase(ctx context.Context, actor *GPTIQTestActor, id, expectedVersion int64) error {
	if err := s.ready(); err != nil {
		return err
	}
	if err := requireGPTIQConfigure(actor); err != nil {
		return err
	}
	return s.repo.DeleteCase(ctx, id, expectedVersion, actor.UserID)
}

type GPTIQRunRequest struct {
	EntryIDs []int64 `json:"entry_ids"`
	CaseIDs  []int64 `json:"case_ids"`
}

// StartRun 发起手动批次。"全部"在服务端解析为操作者范围内的账号；组织负责人指定范围外的条目返回 403。
// 停用的测试项可以被显式指定（试运行），未指定时只跑启用项。
func (s *GPTIQTestService) StartRun(ctx context.Context, actor *GPTIQTestActor, req GPTIQRunRequest) (*GPTIQTestBatch, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	cfg, err := s.repo.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	visible, err := s.visibleEntries(ctx)
	if err != nil {
		return nil, err
	}
	entries := make([]gptIQEntry, 0, len(visible))
	if len(req.EntryIDs) == 0 {
		for _, e := range visible {
			if actor.allows(e.Group) {
				entries = append(entries, e)
			}
		}
	} else {
		byID := make(map[int64]gptIQEntry, len(visible))
		for _, e := range visible {
			byID[e.ID] = e
		}
		seen := map[int64]bool{}
		for _, id := range req.EntryIDs {
			e, ok := byID[id]
			if !ok {
				return nil, ErrGPTIQEntryNotFound
			}
			if !actor.allows(e.Group) {
				return nil, ErrGPTIQScopeDenied
			}
			if !seen[id] {
				seen[id] = true
				entries = append(entries, e)
			}
		}
		// 保持额度展示的自然顺序。
		ordered := make([]gptIQEntry, 0, len(entries))
		for _, e := range visible {
			if seen[e.ID] {
				ordered = append(ordered, e)
			}
		}
		entries = ordered
	}
	allCases, err := s.repo.ListCases(ctx)
	if err != nil {
		return nil, err
	}
	cases := make([]GPTIQTestCase, 0, len(allCases))
	if len(req.CaseIDs) == 0 {
		for _, c := range allCases {
			if c.Enabled {
				cases = append(cases, c)
			}
		}
	} else {
		wanted := map[int64]bool{}
		for _, id := range req.CaseIDs {
			wanted[id] = true
		}
		for _, c := range allCases {
			if wanted[c.ID] {
				cases = append(cases, c)
				delete(wanted, c.ID)
			}
		}
		if len(wanted) > 0 {
			return nil, ErrGPTIQCaseNotFound
		}
	}
	if len(entries) == 0 || len(cases) == 0 {
		return nil, ErrGPTIQNothingToRun
	}
	date := s.localDate()
	dispatched, err := s.repo.DispatchedOn(ctx, date)
	if err != nil {
		return nil, err
	}
	if dispatched >= cfg.DailyCallBudget {
		return nil, ErrGPTIQBudgetExceeded
	}
	userID := actor.UserID
	var triggeredBy *int64
	if userID > 0 {
		triggeredBy = &userID
	}
	batch, err := s.repo.CreateBatch(ctx, newGPTIQBatch(GPTIQTriggerManual, date, triggeredBy, actor.scopeGroups(), entries, cases))
	if err != nil {
		return nil, err
	}
	if !s.goTracked(func() { s.executeBatch(s.ctx, batch, entries, cases, cfg) }) {
		finishCtx, cancel := detachedGPTIQContext(ctx)
		_ = s.repo.CloseBatch(finishCtx, batch.ID, batch.LeaseToken, GPTIQBatchAborted, GPTIQBatchFailureStopping)
		cancel()
		return nil, ErrGPTIQServiceStopping
	}
	return batch, nil
}

// CancelRun 请求取消运行中批次。组织负责人只能取消自己发起的手动批次。
func (s *GPTIQTestService) CancelRun(ctx context.Context, actor *GPTIQTestActor, batchID int64) error {
	if err := s.ready(); err != nil {
		return err
	}
	batch, err := s.repo.GetBatch(ctx, batchID)
	if err != nil {
		return err
	}
	if !actor.FullScope && (batch.Trigger != GPTIQTriggerManual || batch.TriggeredBy == nil || *batch.TriggeredBy != actor.UserID) {
		return ErrGPTIQScopeDenied
	}
	if batch.Status != GPTIQBatchRunning {
		return ErrGPTIQBatchNotRunning
	}
	ok, err := s.repo.RequestCancel(ctx, batchID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrGPTIQBatchNotRunning
	}
	return nil
}

type GPTIQRejudgeResult struct {
	Checked          int `json:"checked"`
	Updated          int `json:"updated"`
	SkippedTruncated int `json:"skipped_truncated"`
}

// Rejudge 用测试项当前版本的判分规则重算同一题面（prompt_sha256 相同）的历史回答，不访问上游。
// 截断过的回答证据不足，保持原分数并计入 skipped_truncated。
func (s *GPTIQTestService) Rejudge(ctx context.Context, actor *GPTIQTestActor, caseID int64) (*GPTIQRejudgeResult, error) {
	return s.RejudgeAt(ctx, actor, caseID, 0)
}

// RejudgeAt 同 Rejudge；expectedRevisionID 非 0 时要求测试项当前版本仍是它（页面加载后被改过返回 409），
// 写分由仓储按版本单调保护，并发重评不会让旧版本覆盖新版本的分数。
func (s *GPTIQTestService) RejudgeAt(ctx context.Context, actor *GPTIQTestActor, caseID, expectedRevisionID int64) (*GPTIQRejudgeResult, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if err := requireGPTIQConfigure(actor); err != nil {
		return nil, err
	}
	c, err := s.repo.GetCase(ctx, caseID)
	if err != nil {
		return nil, err
	}
	if expectedRevisionID > 0 && c.RevisionID != expectedRevisionID {
		return nil, ErrGPTIQCaseConflict
	}
	if c.Kind != GPTIQKindGraded {
		return nil, ErrGPTIQRejudgeNotGraded
	}
	candidates, err := s.repo.ListRejudgeCandidates(ctx, caseID, c.PromptSHA256)
	if err != nil {
		return nil, err
	}
	out := &GPTIQRejudgeResult{Checked: len(candidates)}
	updates := make([]GPTIQCorrectUpdate, 0, len(candidates))
	for _, r := range candidates {
		if r.AnswerTruncated {
			out.SkippedTruncated++
			continue
		}
		correct := ScoreGPTIQAnswer(c.GPTIQRevisionSpec, r.AnswerText)
		same := r.ScoredRevisionID != nil && *r.ScoredRevisionID == c.RevisionID && gptIQBoolPtrEqual(r.Correct, correct)
		if !same {
			updates = append(updates, GPTIQCorrectUpdate{ResultID: r.ID, Correct: correct, ScoredRevisionID: c.RevisionID})
		}
	}
	if err := s.repo.UpdateCorrect(ctx, updates); err != nil {
		return nil, err
	}
	// 写分受版本单调保护，被更新版本挡下的行不算"已更新"：按实际以本版本判分的行数回报。
	ids := make([]int64, 0, len(updates))
	for _, u := range updates {
		ids = append(ids, u.ResultID)
	}
	applied, err := s.repo.CountScoredWith(ctx, ids, c.RevisionID)
	if err != nil {
		return nil, err
	}
	out.Updated = applied
	return out, nil
}

func gptIQBoolPtrEqual(a, b *bool) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
