package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"golang.org/x/sync/singleflight"
)

// GPT 账号额度共享展示：管理员勾选 OpenAI OAuth 普通主账号，后台在固定时段只读采集
// wham/usage 写入独立快照表；用户读取只读快照，任何读取路径都不访问上游。
// 设计见 docs/features/gpt-account-quota-display-design-cn.md。

const (
	GPTQuotaDisplayLeaderKey           = "jobs:gpt-quota-display"
	GPTQuotaDisplayPollIntervalSeconds = 15 * 60

	GPTQuotaGroupXunyou  = "xunyou"
	GPTQuotaGroupWsdashi = "wsdashi"

	gptQuotaDisplayLockTTL        = 30 * time.Minute
	gptQuotaDisplayBatchTimeout   = 25 * time.Minute
	gptQuotaDisplaySingleTimeout  = 45 * time.Second
	gptQuotaDisplayTickInterval   = 30 * time.Second
	gptQuotaDisplayConcurrency    = 3
	gptQuotaDisplayCooldown       = 60 * time.Second
	gptQuotaDisplayStaleGrace     = 5 * time.Minute
	gptQuotaDisplayDefaultBackoff = 5 * time.Minute
	gptQuotaDisplayMaxBackoff     = 24 * time.Hour
	gptQuotaDisplayFinishTimeout  = 5 * time.Second

	gptQuotaDisplayStartMinute  = 9*60 + 30
	gptQuotaDisplayEndMinute    = 18 * 60
	gptQuotaDisplayStartTime    = "09:30"
	gptQuotaDisplayTimezone     = "Asia/Shanghai"
	gptQuotaDisplayEndTime      = "18:00"
	gptQuotaFiveHourMaxSeconds  = 6 * 60 * 60
	gptQuotaMaxResetHorizon     = 8 * 24 * time.Hour
	gptQuotaDisplaySourceActive = "active"
	gptQuotaCandidateMaxPage    = 200
	gptQuotaCandidateMaxOffset  = 1_000_000
)

// 采集尝试状态与失败类别。管理端展示这些稳定类别，不返回上游原文。
const (
	GPTQuotaStatusNever              = "never"
	GPTQuotaStatusRunning            = "running"
	GPTQuotaStatusOK                 = "ok"
	GPTQuotaStatusSkippedCooldown    = "skipped_cooldown"
	GPTQuotaStatusSkippedBackoff     = "skipped_backoff"
	GPTQuotaStatusNoSupportedWindows = "no_supported_windows"
	GPTQuotaStatusTokenUnavailable   = "token_unavailable"
	GPTQuotaStatusAccountUnavailable = "account_unavailable"
	GPTQuotaStatusNotSupported       = "not_supported"
	GPTQuotaStatusUnauthorized       = "unauthorized"
	GPTQuotaStatusForbidden          = "forbidden"
	GPTQuotaStatusRateLimited        = "rate_limited"
	GPTQuotaStatusTimeout            = "timeout"
	GPTQuotaStatusUpstreamError      = "upstream_error"
	GPTQuotaStatusRequestFailed      = "request_failed"
	GPTQuotaStatusParseFailed        = "parse_failed"
	GPTQuotaStatusCancelled          = "cancelled"
	GPTQuotaStatusInternalError      = "internal_error"
)

// 展示条目失去资格的原因。
const (
	GPTQuotaReasonAccountNotFound   = "account_not_found"
	GPTQuotaReasonNotOpenAI         = "not_openai"
	GPTQuotaReasonNotOAuth          = "not_oauth"
	GPTQuotaReasonShadow            = "shadow_account"
	GPTQuotaReasonPAT               = "personal_access_token"
	GPTQuotaReasonAgentIdentity     = "agent_identity"
	GPTQuotaReasonNoDisplayPrefix   = "no_display_prefix"
	GPTQuotaWarningDuplicateChatGPT = "duplicate_chatgpt_account"
)

var (
	ErrGPTQuotaConfigConflict  = infraerrors.Conflict("GPT_QUOTA_CONFIG_CONFLICT", "GPT quota display config changed; reload and retry")
	ErrGPTQuotaDisplayDisabled = infraerrors.Conflict("GPT_QUOTA_DISPLAY_DISABLED", "GPT quota display is disabled")
	ErrGPTQuotaBatchRunning    = infraerrors.Conflict("GPT_QUOTA_BATCH_RUNNING", "a GPT quota refresh batch is already running")
	ErrGPTQuotaEntryNotFound   = infraerrors.NotFound("GPT_QUOTA_ENTRY_NOT_FOUND", "GPT quota display entry not found")
	ErrGPTQuotaNotConfigured   = infraerrors.ServiceUnavailable("GPT_QUOTA_NOT_CONFIGURED", "GPT quota display service is not configured")
	errGPTQuotaServiceStopping = errors.New("gpt quota display service is stopping")
	// gptQuotaDisplayLocation 本功能固定北京时间，不随全局 timezone 配置变化；
	// 缺少 tzdata 时回落到 UTC+8（中国无夏令时，结果相同）。
	gptQuotaDisplayLocation     = loadGPTQuotaDisplayLocation()
	gptQuotaNameNumberPattern   = regexp.MustCompile(`^[cCdD]-([0-9]+)`)
	gptQuotaAliasPattern        = regexp.MustCompile(`^[\p{Han}A-Za-z0-9_\- ]{1,40}$`)
	gptQuotaAliasSecretPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)sk-`),
		regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}`),
		regexp.MustCompile(`[A-Za-z0-9]{20,}`),
	}
)

type GPTQuotaDisplayConfig struct {
	Enabled         bool       `json:"enabled"`
	IntervalMinutes int        `json:"interval_minutes"`
	StartTime       string     `json:"start_time"`
	EndTime         string     `json:"end_time"`
	Version         int64      `json:"version"`
	LastSlotAt      *time.Time `json:"last_slot_at,omitempty"`
	UpdatedBy       *int64     `json:"updated_by,omitempty"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// GPTQuotaDisplayEntry 是已选展示条目与账号当前元数据的合并视图。
// 账号凭据只取资格判断所需的 auth_mode，不加载 token。
type GPTQuotaDisplayEntry struct {
	ID              int64
	AccountID       int64
	DisplayName     string
	AccountExists   bool
	AccountName     string
	Platform        string
	AccountType     string
	ParentAccountID *int64
	AuthMode        string
	LegacyAuthMode  string
}

// GPTQuotaDisplaySelection 是管理员保存的一条展示选择。
type GPTQuotaDisplaySelection struct {
	AccountID   int64  `json:"account_id"`
	DisplayName string `json:"display_name"`
}

type GPTQuotaWindow struct {
	RemainingPercent float64    `json:"remaining_percent"`
	ResetAt          *time.Time `json:"reset_at,omitempty"`
	ResetTimeSource  string     `json:"reset_time_source,omitempty"`
}

type GPTQuotaDisplaySnapshot struct {
	AccountID         int64
	FiveHour          *GPTQuotaWindow
	SevenDay          *GPTQuotaWindow
	SampledAt         *time.Time
	LastAttemptAt     *time.Time
	LastAttemptStatus string
	RetryAfter        *time.Time
	Source            string
	UpdatedAt         time.Time
}

type GPTQuotaDisplayRepository interface {
	GetConfig(ctx context.Context) (*GPTQuotaDisplayConfig, error)
	// SaveConfig 在同一事务内按 expectedVersion 条件更新配置并替换展示选择；版本不符返回 ErrGPTQuotaConfigConflict。
	// slotFloor 非空时把 last_slot_at 抬到不早于该时间，使开启展示或改间隔后不补跑当前已过的槽位。
	SaveConfig(ctx context.Context, cfg GPTQuotaDisplayConfig, selections []GPTQuotaDisplaySelection, expectedVersion, updatedBy int64, slotFloor *time.Time) (*GPTQuotaDisplayConfig, error)
	ListSelectedEntries(ctx context.Context) ([]GPTQuotaDisplayEntry, error)
	// ListCandidates 在数据库分页列出 OpenAI OAuth 未删除账号（c-/d- 前缀与编号自然排序在前），并返回总数。
	ListCandidates(ctx context.Context, search string, limit, offset int) ([]GPTQuotaDisplayEntry, int, error)
	ListSnapshots(ctx context.Context, accountIDs []int64) (map[int64]*GPTQuotaDisplaySnapshot, error)
	// ClaimAttempt 跨实例领取一次采集：冷却期内或 Retry-After 未到返回 false。
	ClaimAttempt(ctx context.Context, accountID int64, attemptAt time.Time, cooldown time.Duration) (bool, error)
	// FinishAttempt 仅当 last_attempt_at 仍等于本次尝试时写入结果，晚返回的旧尝试不覆盖新状态。
	FinishAttempt(ctx context.Context, accountID int64, attemptAt time.Time, status string, retryAfter *time.Time) error
	// PublishSnapshot 按 sampled_at 条件发布成功快照；已有更新的快照时返回 false。
	PublishSnapshot(ctx context.Context, snap *GPTQuotaDisplaySnapshot, attemptAt time.Time) (bool, error)
	// ClaimSlot 以单行条件更新领取计划槽位，多实例与重启都只执行一次。
	ClaimSlot(ctx context.Context, slot time.Time) (bool, error)
}

type gptQuotaUsageFetcher interface {
	QueryUsageReadOnly(ctx context.Context, accountID int64) (*OpenAIQuotaUsage, error)
}

type GPTQuotaSchedule struct {
	Start           string `json:"start"`
	End             string `json:"end"`
	IntervalMinutes int    `json:"interval_minutes"`
	Timezone        string `json:"timezone"`
}

// GPTQuotaUserCard 是用户侧卡片白名单：不含账号 ID、原名或采集尝试状态。
type GPTQuotaUserCard struct {
	ID          int64           `json:"id"`
	DisplayName string          `json:"display_name"`
	FiveHour    *GPTQuotaWindow `json:"five_hour"`
	SevenDay    *GPTQuotaWindow `json:"seven_day"`
	SampledAt   *time.Time      `json:"sampled_at"`
	Stale       bool            `json:"stale"`
}

type GPTQuotaUserGroups struct {
	Xunyou  []GPTQuotaUserCard `json:"xunyou"`
	Wsdashi []GPTQuotaUserCard `json:"wsdashi"`
}

type GPTQuotaUserView struct {
	Enabled             bool               `json:"enabled"`
	ServerTime          time.Time          `json:"server_time"`
	PollIntervalSeconds int                `json:"poll_interval_seconds"`
	Schedule            GPTQuotaSchedule   `json:"schedule"`
	NextScheduledAt     *time.Time         `json:"next_scheduled_at"`
	InScheduleWindow    bool               `json:"in_schedule_window"`
	Groups              GPTQuotaUserGroups `json:"groups"`
}

type GPTQuotaAdminEntry struct {
	ID                   int64           `json:"id"`
	AccountID            int64           `json:"account_id"`
	AccountName          string          `json:"account_name"`
	Group                string          `json:"group"`
	DisplayName          string          `json:"display_name"`
	EffectiveDisplayName string          `json:"effective_display_name"`
	Eligible             bool            `json:"eligible"`
	Reason               string          `json:"reason,omitempty"`
	Warnings             []string        `json:"warnings,omitempty"`
	FiveHour             *GPTQuotaWindow `json:"five_hour"`
	SevenDay             *GPTQuotaWindow `json:"seven_day"`
	SampledAt            *time.Time      `json:"sampled_at"`
	Stale                bool            `json:"stale"`
	LastAttemptAt        *time.Time      `json:"last_attempt_at"`
	LastAttemptStatus    string          `json:"last_attempt_status"`
	RetryAfter           *time.Time      `json:"retry_after"`
}

type GPTQuotaBatchStatus struct {
	Running           bool           `json:"running"`
	Trigger           string         `json:"trigger,omitempty"`
	StartedAt         *time.Time     `json:"started_at,omitempty"`
	FinishedAt        *time.Time     `json:"finished_at,omitempty"`
	Total             int            `json:"total"`
	Succeeded         int            `json:"succeeded"`
	Failed            int            `json:"failed"`
	Skipped           int            `json:"skipped"`
	FailureCategories map[string]int `json:"failure_categories,omitempty"`
}

type GPTQuotaAdminView struct {
	Config           *GPTQuotaDisplayConfig `json:"config"`
	ServerTime       time.Time              `json:"server_time"`
	Schedule         GPTQuotaSchedule       `json:"schedule"`
	NextScheduledAt  *time.Time             `json:"next_scheduled_at"`
	InScheduleWindow bool                   `json:"in_schedule_window"`
	Entries          []GPTQuotaAdminEntry   `json:"entries"`
	Batch            GPTQuotaBatchStatus    `json:"batch"`
}

type GPTQuotaCandidate struct {
	AccountID   int64  `json:"account_id"`
	AccountName string `json:"account_name"`
	Group       string `json:"group"`
	Eligible    bool   `json:"eligible"`
	Reason      string `json:"reason,omitempty"`
	Selected    bool   `json:"selected"`
}

type GPTQuotaCandidatePage struct {
	Items    []GPTQuotaCandidate `json:"items"`
	Total    int                 `json:"total"`
	Page     int                 `json:"page"`
	PageSize int                 `json:"page_size"`
}

type GPTQuotaSaveRequest struct {
	Enabled         bool                       `json:"enabled"`
	IntervalMinutes int                        `json:"interval_minutes"`
	ExpectedVersion int64                      `json:"expected_version"`
	Entries         []GPTQuotaDisplaySelection `json:"entries"`
}

type GPTQuotaSaveResult struct {
	Config   *GPTQuotaDisplayConfig `json:"config"`
	Warnings []string               `json:"warnings,omitempty"`
}

type GPTQuotaRefreshResult struct {
	EntryID int64               `json:"entry_id"`
	Status  string              `json:"status"`
	Entry   *GPTQuotaAdminEntry `json:"entry,omitempty"`
}

type GPTQuotaDisplayService struct {
	repo     GPTQuotaDisplayRepository
	accounts AccountRepository
	fetcher  gptQuotaUsageFetcher
	locks    LeaderLockCache
	db       *sql.DB
	now      func() time.Time

	refreshGroup singleflight.Group

	lifecycleMu sync.Mutex
	started     bool
	stopped     bool
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup

	batchMu sync.Mutex
	batch   GPTQuotaBatchStatus
}

func NewGPTQuotaDisplayService(repo GPTQuotaDisplayRepository, accounts AccountRepository, fetcher gptQuotaUsageFetcher, locks LeaderLockCache, db *sql.DB) *GPTQuotaDisplayService {
	ctx, cancel := context.WithCancel(context.Background())
	return &GPTQuotaDisplayService{
		repo:     repo,
		accounts: accounts,
		fetcher:  fetcher,
		locks:    locks,
		db:       db,
		now:      time.Now,
		ctx:      ctx,
		cancel:   cancel,
	}
}

func (s *GPTQuotaDisplayService) Start() {
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

// Stop 取消在途采集（含上游请求）并等待后台 goroutine 退出。
func (s *GPTQuotaDisplayService) Stop() {
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

// track 在服务未停止时登记一项受 wg 跟踪的工作，Stop 会等待其结束。
func (s *GPTQuotaDisplayService) track() (func(), bool) {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	if s.stopped {
		return nil, false
	}
	s.wg.Add(1)
	return s.wg.Done, true
}

// goTracked 在服务未停止时启动受 wg 跟踪的 goroutine。
func (s *GPTQuotaDisplayService) goTracked(fn func()) bool {
	done, ok := s.track()
	if !ok {
		return false
	}
	go func() {
		defer done()
		fn()
	}()
	return true
}

func loadGPTQuotaDisplayLocation() *time.Location {
	if loc, err := time.LoadLocation(gptQuotaDisplayTimezone); err == nil {
		return loc
	}
	return time.FixedZone(gptQuotaDisplayTimezone, 8*60*60)
}

func (s *GPTQuotaDisplayService) localNow() time.Time {
	return s.now().In(gptQuotaDisplayLocation)
}

func (s *GPTQuotaDisplayService) runLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(gptQuotaDisplayTickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.runScheduled(s.ctx)
		}
	}
}

func (s *GPTQuotaDisplayService) runScheduled(ctx context.Context) {
	cfg, err := s.repo.GetConfig(ctx)
	if err != nil {
		if ctx.Err() == nil {
			slog.Warn("gpt_quota_display_config_load_failed", "error", err)
		}
		return
	}
	if !cfg.Enabled {
		return
	}
	slot, due := gptQuotaDueSlot(s.localNow(), cfg.IntervalMinutes)
	if !due || (cfg.LastSlotAt != nil && !cfg.LastSlotAt.Before(slot)) {
		return
	}
	owner := fmt.Sprintf("gpt-quota-%d", time.Now().UnixNano())
	release, acquired := tryAcquireSingletonLeaderLock(ctx, s.locks, s.db, GPTQuotaDisplayLeaderKey, owner, gptQuotaDisplayLockTTL)
	if !acquired {
		return
	}
	defer release()
	// 先读取条目并原子占用本实例批次，再领取槽位：读取失败、手动批次执行中或领取失败
	// 都不消耗槽位，下一次检查（30 秒后）在槽位有效期内重试；占用后手动全量刷新会收到 409。
	entries, err := s.repo.ListSelectedEntries(ctx)
	if err != nil {
		slog.Warn("gpt_quota_display_list_entries_failed", "slot", slot, "error", err)
		return
	}
	abortBatch, ok := s.beginBatch("scheduled")
	if !ok {
		return
	}
	claimed, err := s.repo.ClaimSlot(ctx, slot)
	if err != nil || !claimed {
		abortBatch()
		if err != nil {
			slog.Warn("gpt_quota_display_claim_slot_failed", "slot", slot, "error", err)
		}
		return
	}
	batchCtx, cancel := context.WithTimeout(ctx, gptQuotaDisplayBatchTimeout)
	status := s.runBatch(batchCtx, entries)
	cancel()
	slog.Info("gpt_quota_display_slot_done", "slot", slot, "total", status.Total, "succeeded", status.Succeeded, "failed", status.Failed, "skipped", status.Skipped)

	// 批次跨过下一计划时点时直接跳过该槽位，不排队追补。
	if next, ok := gptQuotaDueSlot(s.localNow(), cfg.IntervalMinutes); ok && next.After(slot) {
		if skipped, err := s.repo.ClaimSlot(ctx, next); err == nil && skipped {
			slog.Warn("gpt_quota_display_slot_skipped", "slot", next, "reason", "previous_batch_overran")
		}
	}
}

// beginBatch 以检查并设置的方式原子占用本实例批次；返回的 abort 在批次最终没有启动时
// 释放占用并恢复上一批次的统计，供管理页继续展示。
func (s *GPTQuotaDisplayService) beginBatch(trigger string) (abort func(), ok bool) {
	s.batchMu.Lock()
	defer s.batchMu.Unlock()
	if s.batch.Running {
		return nil, false
	}
	previous := s.batch
	started := s.now().UTC()
	s.batch = GPTQuotaBatchStatus{Running: true, Trigger: trigger, StartedAt: &started}
	return func() {
		s.batchMu.Lock()
		defer s.batchMu.Unlock()
		s.batch = previous
	}, true
}

func (s *GPTQuotaDisplayService) batchStatus() GPTQuotaBatchStatus {
	s.batchMu.Lock()
	defer s.batchMu.Unlock()
	out := s.batch
	if len(s.batch.FailureCategories) > 0 {
		out.FailureCategories = make(map[string]int, len(s.batch.FailureCategories))
		for k, v := range s.batch.FailureCategories {
			out.FailureCategories[k] = v
		}
	}
	return out
}

// runBatch 以固定并发采集已选且仍有资格的条目，单账号失败不阻断其他账号。调用方须先 beginBatch。
func (s *GPTQuotaDisplayService) runBatch(ctx context.Context, entries []GPTQuotaDisplayEntry) GPTQuotaBatchStatus {
	targets := make([]GPTQuotaDisplayEntry, 0, len(entries))
	for _, entry := range entries {
		if ok, _ := entry.eligibility(); ok {
			targets = append(targets, entry)
		}
	}
	s.batchMu.Lock()
	s.batch.Total = len(targets)
	s.batchMu.Unlock()

	sem := make(chan struct{}, gptQuotaDisplayConcurrency)
	var wg sync.WaitGroup
	for _, entry := range targets {
		sem <- struct{}{}
		// 拿到并发槽位后再检查，避免关闭展示后仍派发排队中的请求。
		if ctx.Err() != nil || !s.stillEnabled(ctx) {
			<-sem
			break
		}
		wg.Add(1)
		go func(accountID int64) {
			defer wg.Done()
			defer func() { <-sem }()
			s.recordBatchResult(s.refreshAccount(ctx, accountID))
		}(entry.AccountID)
	}
	wg.Wait()

	s.batchMu.Lock()
	defer s.batchMu.Unlock()
	finished := s.now().UTC()
	s.batch.Running = false
	s.batch.FinishedAt = &finished
	return s.batch
}

// stillEnabled 让批次在管理员关闭展示后不再派发新的上游请求。
func (s *GPTQuotaDisplayService) stillEnabled(ctx context.Context) bool {
	cfg, err := s.repo.GetConfig(ctx)
	return err == nil && cfg.Enabled
}

func (s *GPTQuotaDisplayService) recordBatchResult(status string) {
	s.batchMu.Lock()
	defer s.batchMu.Unlock()
	switch {
	case status == GPTQuotaStatusOK:
		s.batch.Succeeded++
	case isGPTQuotaSkippedStatus(status):
		s.batch.Skipped++
	default:
		s.batch.Failed++
		if s.batch.FailureCategories == nil {
			s.batch.FailureCategories = map[string]int{}
		}
		s.batch.FailureCategories[status]++
	}
}

func isGPTQuotaSkippedStatus(status string) bool {
	return status == GPTQuotaStatusSkippedCooldown || status == GPTQuotaStatusSkippedBackoff
}

// refreshAccount 对单个账号做一次只读采集；同账号并发请求经 singleflight 合并，
// 冷却与 Retry-After 退避由数据库条件领取保证跨实例生效。
func (s *GPTQuotaDisplayService) refreshAccount(ctx context.Context, accountID int64) string {
	v, _, _ := s.refreshGroup.Do(strconv.FormatInt(accountID, 10), func() (any, error) {
		return s.doRefresh(ctx, accountID), nil
	})
	status, _ := v.(string)
	return status
}

func (s *GPTQuotaDisplayService) doRefresh(ctx context.Context, accountID int64) string {
	if s.fetcher == nil {
		return GPTQuotaStatusInternalError
	}
	attemptAt := s.now().UTC().Truncate(time.Microsecond)
	claimed, err := s.repo.ClaimAttempt(ctx, accountID, attemptAt, gptQuotaDisplayCooldown)
	if err != nil {
		if ctx.Err() != nil {
			return GPTQuotaStatusCancelled
		}
		slog.Warn("gpt_quota_display_claim_attempt_failed", "account_id", accountID, "error", err)
		return GPTQuotaStatusInternalError
	}
	if !claimed {
		return s.skippedStatus(ctx, accountID, attemptAt)
	}

	usage, err := s.fetcher.QueryUsageReadOnly(ctx, accountID)
	if err != nil {
		status, retryAfter := classifyGPTQuotaFetchError(ctx, err, s.now().UTC())
		s.finishAttempt(ctx, accountID, attemptAt, status, retryAfter)
		slog.Warn("gpt_quota_display_fetch_failed", "account_id", accountID, "status", status)
		return status
	}
	sampledAt := s.now().UTC().Truncate(time.Microsecond)
	fiveHour, sevenDay := NormalizeGPTQuotaUsage(usage, sampledAt)
	if fiveHour == nil && sevenDay == nil {
		s.finishAttempt(ctx, accountID, attemptAt, GPTQuotaStatusNoSupportedWindows, nil)
		slog.Warn("gpt_quota_display_no_supported_windows", "account_id", accountID)
		return GPTQuotaStatusNoSupportedWindows
	}
	snap := &GPTQuotaDisplaySnapshot{AccountID: accountID, FiveHour: fiveHour, SevenDay: sevenDay, SampledAt: &sampledAt, Source: gptQuotaDisplaySourceActive}
	publishCtx, cancel := detachedGPTQuotaContext(ctx)
	published, err := s.repo.PublishSnapshot(publishCtx, snap, attemptAt)
	cancel()
	if err != nil {
		s.finishAttempt(ctx, accountID, attemptAt, GPTQuotaStatusInternalError, nil)
		slog.Warn("gpt_quota_display_publish_failed", "account_id", accountID, "error", err)
		return GPTQuotaStatusInternalError
	}
	if !published {
		// 已有更新的成功快照，本次结果不回写额度，只记录尝试成功。
		s.finishAttempt(ctx, accountID, attemptAt, GPTQuotaStatusOK, nil)
	}
	return GPTQuotaStatusOK
}

func (s *GPTQuotaDisplayService) skippedStatus(ctx context.Context, accountID int64, now time.Time) string {
	snaps, err := s.repo.ListSnapshots(ctx, []int64{accountID})
	if err == nil {
		if snap := snaps[accountID]; snap != nil && snap.RetryAfter != nil && snap.RetryAfter.After(now) {
			return GPTQuotaStatusSkippedBackoff
		}
	}
	return GPTQuotaStatusSkippedCooldown
}

func (s *GPTQuotaDisplayService) finishAttempt(ctx context.Context, accountID int64, attemptAt time.Time, status string, retryAfter *time.Time) {
	finishCtx, cancel := detachedGPTQuotaContext(ctx)
	defer cancel()
	if err := s.repo.FinishAttempt(finishCtx, accountID, attemptAt, status, retryAfter); err != nil {
		slog.Warn("gpt_quota_display_finish_attempt_failed", "account_id", accountID, "status", status, "error", err)
	}
}

// detachedGPTQuotaContext 让停机取消后仍能在短超时内落下尝试结果。
func detachedGPTQuotaContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), gptQuotaDisplayFinishTimeout)
}

func classifyGPTQuotaFetchError(ctx context.Context, err error, now time.Time) (string, *time.Time) {
	var readOnlyErr *OpenAIQuotaReadOnlyError
	if errors.As(err, &readOnlyErr) {
		if readOnlyErr.Category == GPTQuotaStatusRateLimited {
			backoff := readOnlyErr.RetryAfter
			if backoff <= 0 {
				backoff = gptQuotaDisplayDefaultBackoff
			}
			backoff = min(backoff, gptQuotaDisplayMaxBackoff)
			retryAt := now.Add(backoff)
			return GPTQuotaStatusRateLimited, &retryAt
		}
		return readOnlyErr.Category, nil
	}
	if ctx.Err() != nil {
		return GPTQuotaStatusCancelled, nil
	}
	return GPTQuotaStatusRequestFailed, nil
}

// ---- 用户与管理端读取（只读数据库，不访问上游） ----

func (s *GPTQuotaDisplayService) scheduleView(cfg *GPTQuotaDisplayConfig) GPTQuotaSchedule {
	return GPTQuotaSchedule{Start: gptQuotaDisplayStartTime, End: gptQuotaDisplayEndTime, IntervalMinutes: normalizeGPTQuotaInterval(cfg.IntervalMinutes), Timezone: gptQuotaDisplayTimezone}
}

func (s *GPTQuotaDisplayService) Enabled(ctx context.Context) (bool, error) {
	cfg, err := s.repo.GetConfig(ctx)
	if err != nil {
		return false, err
	}
	return cfg.Enabled, nil
}

func (s *GPTQuotaDisplayService) UserView(ctx context.Context) (*GPTQuotaUserView, error) {
	cfg, err := s.repo.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	now := s.localNow()
	view := &GPTQuotaUserView{
		Enabled:             cfg.Enabled,
		ServerTime:          now.UTC(),
		PollIntervalSeconds: GPTQuotaDisplayPollIntervalSeconds,
		Schedule:            s.scheduleView(cfg),
		InScheduleWindow:    gptQuotaInWindow(now),
		Groups:              GPTQuotaUserGroups{Xunyou: []GPTQuotaUserCard{}, Wsdashi: []GPTQuotaUserCard{}},
	}
	if !cfg.Enabled {
		return view, nil
	}
	next := gptQuotaNextSlot(now, cfg.IntervalMinutes).UTC()
	view.NextScheduledAt = &next

	entries, err := s.repo.ListSelectedEntries(ctx)
	if err != nil {
		return nil, err
	}
	visible := make([]GPTQuotaDisplayEntry, 0, len(entries))
	for _, entry := range entries {
		if ok, _ := entry.eligibility(); ok {
			visible = append(visible, entry)
		}
	}
	sortGPTQuotaEntries(visible)
	snaps, err := s.repo.ListSnapshots(ctx, gptQuotaEntryAccountIDs(visible))
	if err != nil {
		return nil, err
	}
	for _, entry := range visible {
		card := GPTQuotaUserCard{ID: entry.ID, DisplayName: entry.effectiveDisplayName()}
		if snap := snaps[entry.AccountID]; snap != nil && snap.SampledAt != nil {
			card.FiveHour = snap.FiveHour
			card.SevenDay = snap.SevenDay
			card.SampledAt = snap.SampledAt
			card.Stale = gptQuotaIsStale(snap.SampledAt, now, cfg.IntervalMinutes)
		}
		if GPTQuotaClass(entry.AccountName) == GPTQuotaGroupXunyou {
			view.Groups.Xunyou = append(view.Groups.Xunyou, card)
		} else {
			view.Groups.Wsdashi = append(view.Groups.Wsdashi, card)
		}
	}
	return view, nil
}

// AdminView 展示全部已选条目（含失去资格的条目及原因），不受公开开关影响。
func (s *GPTQuotaDisplayService) AdminView(ctx context.Context) (*GPTQuotaAdminView, error) {
	cfg, err := s.repo.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	entries, err := s.repo.ListSelectedEntries(ctx)
	if err != nil {
		return nil, err
	}
	sortGPTQuotaEntries(entries)
	ids := gptQuotaEntryAccountIDs(entries)
	snaps, err := s.repo.ListSnapshots(ctx, ids)
	if err != nil {
		return nil, err
	}
	duplicates := map[int64]bool{}
	if s.accounts != nil && len(ids) > 0 {
		if accounts, err := s.accounts.GetByIDs(ctx, ids); err == nil {
			for _, id := range duplicateChatGPTAccountIDs(accounts) {
				duplicates[id] = true
			}
		} else {
			slog.Warn("gpt_quota_display_load_accounts_failed", "error", err)
		}
	}
	now := s.localNow()
	view := &GPTQuotaAdminView{
		Config:           cfg,
		ServerTime:       now.UTC(),
		Schedule:         s.scheduleView(cfg),
		InScheduleWindow: gptQuotaInWindow(now),
		Entries:          make([]GPTQuotaAdminEntry, 0, len(entries)),
		Batch:            s.batchStatus(),
	}
	if cfg.Enabled {
		next := gptQuotaNextSlot(now, cfg.IntervalMinutes).UTC()
		view.NextScheduledAt = &next
	}
	for _, entry := range entries {
		item := s.adminEntry(entry, snaps[entry.AccountID], now, cfg.IntervalMinutes)
		if duplicates[entry.AccountID] {
			item.Warnings = append(item.Warnings, GPTQuotaWarningDuplicateChatGPT)
		}
		view.Entries = append(view.Entries, item)
	}
	return view, nil
}

func (s *GPTQuotaDisplayService) adminEntry(entry GPTQuotaDisplayEntry, snap *GPTQuotaDisplaySnapshot, now time.Time, interval int) GPTQuotaAdminEntry {
	ok, reason := entry.eligibility()
	item := GPTQuotaAdminEntry{
		ID:                   entry.ID,
		AccountID:            entry.AccountID,
		AccountName:          entry.AccountName,
		Group:                GPTQuotaClass(entry.AccountName),
		DisplayName:          entry.DisplayName,
		EffectiveDisplayName: entry.effectiveDisplayName(),
		Eligible:             ok,
		Reason:               reason,
		LastAttemptStatus:    GPTQuotaStatusNever,
	}
	if snap != nil {
		item.FiveHour = snap.FiveHour
		item.SevenDay = snap.SevenDay
		item.SampledAt = snap.SampledAt
		item.Stale = gptQuotaIsStale(snap.SampledAt, now, interval)
		item.LastAttemptAt = snap.LastAttemptAt
		item.LastAttemptStatus = snap.LastAttemptStatus
		item.RetryAfter = snap.RetryAfter
	}
	return item
}

func (s *GPTQuotaDisplayService) Candidates(ctx context.Context, search string, page, pageSize int) (*GPTQuotaCandidatePage, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > gptQuotaCandidateMaxPage {
		pageSize = 50
	}
	search = strings.TrimSpace(search)
	out := &GPTQuotaCandidatePage{Items: []GPTQuotaCandidate{}, Page: page, PageSize: pageSize}
	// 超大页码先截断，避免 (page-1)*pageSize 溢出；只返回总数。
	if page-1 > gptQuotaCandidateMaxOffset/pageSize {
		_, total, err := s.repo.ListCandidates(ctx, search, 0, 0)
		if err != nil {
			return nil, err
		}
		out.Total = total
		return out, nil
	}
	candidates, total, err := s.repo.ListCandidates(ctx, search, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, err
	}
	out.Total = total
	entries, err := s.repo.ListSelectedEntries(ctx)
	if err != nil {
		return nil, err
	}
	selected := make(map[int64]bool, len(entries))
	for _, entry := range entries {
		selected[entry.AccountID] = true
	}
	for _, candidate := range candidates {
		ok, reason := candidate.eligibility()
		out.Items = append(out.Items, GPTQuotaCandidate{AccountID: candidate.AccountID, AccountName: candidate.AccountName, Group: GPTQuotaClass(candidate.AccountName), Eligible: ok, Reason: reason, Selected: selected[candidate.AccountID]})
	}
	return out, nil
}

// SaveConfig 校验并保存配置与展示选择。新增条目必须符合资格与前缀；
// 已保存但后来失去资格的条目允许保留（用户侧与采集都会排除），避免管理员无法保存。
func (s *GPTQuotaDisplayService) SaveConfig(ctx context.Context, req GPTQuotaSaveRequest, updatedBy int64) (*GPTQuotaSaveResult, error) {
	if s.accounts == nil {
		return nil, ErrGPTQuotaNotConfigured
	}
	if req.IntervalMinutes != 30 && req.IntervalMinutes != 60 {
		return nil, infraerrors.BadRequest("GPT_QUOTA_INVALID_INTERVAL", "interval_minutes must be 30 or 60")
	}
	if req.ExpectedVersion <= 0 {
		return nil, infraerrors.BadRequest("GPT_QUOTA_VERSION_REQUIRED", "expected_version is required")
	}
	current, err := s.repo.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	existing, err := s.repo.ListSelectedEntries(ctx)
	if err != nil {
		return nil, err
	}
	previouslySelected := make(map[int64]bool, len(existing))
	for _, entry := range existing {
		previouslySelected[entry.AccountID] = true
	}

	ids := make([]int64, 0, len(req.Entries))
	seen := make(map[int64]bool, len(req.Entries))
	selections := make([]GPTQuotaDisplaySelection, 0, len(req.Entries))
	for _, item := range req.Entries {
		if item.AccountID <= 0 {
			return nil, infraerrors.BadRequest("GPT_QUOTA_INVALID_ACCOUNT", "account_id is required")
		}
		if seen[item.AccountID] {
			return nil, infraerrors.BadRequest("GPT_QUOTA_DUPLICATE_ACCOUNT", fmt.Sprintf("account %d is selected more than once", item.AccountID))
		}
		seen[item.AccountID] = true
		alias, err := NormalizeGPTQuotaAlias(item.DisplayName)
		if err != nil {
			return nil, infraerrors.BadRequest("GPT_QUOTA_INVALID_DISPLAY_NAME", fmt.Sprintf("invalid display name for account %d", item.AccountID))
		}
		ids = append(ids, item.AccountID)
		selections = append(selections, GPTQuotaDisplaySelection{AccountID: item.AccountID, DisplayName: alias})
	}

	accounts, err := s.accounts.GetByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]*Account, len(accounts))
	for _, account := range accounts {
		if account != nil {
			byID[account.ID] = account
		}
	}
	for _, id := range ids {
		if previouslySelected[id] {
			continue
		}
		if ok, reason := GPTQuotaSelectable(byID[id]); !ok {
			return nil, infraerrors.BadRequest("GPT_QUOTA_ACCOUNT_NOT_ELIGIBLE", fmt.Sprintf("account %d is not eligible: %s", id, reason))
		}
	}

	// 开启展示或修改间隔后不隐式请求上游：当前已过的槽位视为已处理，等待下一计划时点。
	// current 与 expected_version 不一致时事务内的版本检查会返回冲突，不会按过期判断写入。
	var slotFloor *time.Time
	if (req.Enabled && !current.Enabled) || req.IntervalMinutes != current.IntervalMinutes {
		floor := s.now().UTC()
		slotFloor = &floor
	}
	cfg := GPTQuotaDisplayConfig{Enabled: req.Enabled, IntervalMinutes: req.IntervalMinutes, StartTime: gptQuotaDisplayStartTime, EndTime: gptQuotaDisplayEndTime}
	saved, err := s.repo.SaveConfig(ctx, cfg, selections, req.ExpectedVersion, updatedBy, slotFloor)
	if err != nil {
		return nil, err
	}
	result := &GPTQuotaSaveResult{Config: saved}
	if len(duplicateChatGPTAccountIDs(accounts)) > 0 {
		result.Warnings = append(result.Warnings, GPTQuotaWarningDuplicateChatGPT)
	}
	return result, nil
}

// RefreshEntry 同步采集单个展示条目；展示关闭时拒绝，冷却或退避中返回跳过状态和最近快照。
func (s *GPTQuotaDisplayService) RefreshEntry(ctx context.Context, entryID int64) (*GPTQuotaRefreshResult, error) {
	cfg, err := s.repo.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return nil, ErrGPTQuotaDisplayDisabled
	}
	entries, err := s.repo.ListSelectedEntries(ctx)
	if err != nil {
		return nil, err
	}
	var target *GPTQuotaDisplayEntry
	for i := range entries {
		if entries[i].ID == entryID {
			target = &entries[i]
			break
		}
	}
	if target == nil {
		return nil, ErrGPTQuotaEntryNotFound
	}
	if ok, reason := target.eligibility(); !ok {
		return nil, infraerrors.BadRequest("GPT_QUOTA_ENTRY_NOT_ELIGIBLE", "entry is not eligible: "+reason)
	}
	done, ok := s.track()
	if !ok {
		return nil, errGPTQuotaServiceStopping
	}
	defer done()
	// 采集脱离请求上下文，随服务生命周期取消，避免管理员关闭页面打断合并中的定时采集。
	refreshCtx, cancel := context.WithTimeout(s.ctx, gptQuotaDisplaySingleTimeout)
	defer cancel()
	status := s.refreshAccount(refreshCtx, target.AccountID)

	result := &GPTQuotaRefreshResult{EntryID: entryID, Status: status}
	snaps, err := s.repo.ListSnapshots(ctx, []int64{target.AccountID})
	if err == nil {
		item := s.adminEntry(*target, snaps[target.AccountID], s.localNow(), cfg.IntervalMinutes)
		result.Entry = &item
	}
	return result, nil
}

// RefreshAll 异步分批采集全部展示条目，管理页通过 AdminView 读取批次计数与各条目状态。
func (s *GPTQuotaDisplayService) RefreshAll(ctx context.Context) (GPTQuotaBatchStatus, error) {
	cfg, err := s.repo.GetConfig(ctx)
	if err != nil {
		return GPTQuotaBatchStatus{}, err
	}
	if !cfg.Enabled {
		return GPTQuotaBatchStatus{}, ErrGPTQuotaDisplayDisabled
	}
	entries, err := s.repo.ListSelectedEntries(ctx)
	if err != nil {
		return GPTQuotaBatchStatus{}, err
	}
	abortBatch, ok := s.beginBatch("manual")
	if !ok {
		return s.batchStatus(), ErrGPTQuotaBatchRunning
	}
	started := s.goTracked(func() {
		batchCtx, cancel := context.WithTimeout(s.ctx, gptQuotaDisplayBatchTimeout)
		defer cancel()
		status := s.runBatch(batchCtx, entries)
		slog.Info("gpt_quota_display_manual_batch_done", "total", status.Total, "succeeded", status.Succeeded, "failed", status.Failed, "skipped", status.Skipped)
	})
	if !started {
		abortBatch()
		return GPTQuotaBatchStatus{}, errGPTQuotaServiceStopping
	}
	return s.batchStatus(), nil
}

// ---- 资格、归类、展示名与排序 ----

// GPTQuotaAccountEligibility 判断账号是否属于首版可展示范围：OpenAI OAuth 普通主账号，
// 排除 shadow、PAT、Agent Identity（Setup Token 因类型不是 OAuth 天然排除）。
func GPTQuotaAccountEligibility(account *Account) (bool, string) {
	if account == nil {
		return false, GPTQuotaReasonAccountNotFound
	}
	if account.Platform != PlatformOpenAI {
		return false, GPTQuotaReasonNotOpenAI
	}
	if account.Type != AccountTypeOAuth {
		return false, GPTQuotaReasonNotOAuth
	}
	if account.IsShadow() {
		return false, GPTQuotaReasonShadow
	}
	if account.IsOpenAIPersonalAccessToken() {
		return false, GPTQuotaReasonPAT
	}
	if account.IsOpenAIAgentIdentity() {
		return false, GPTQuotaReasonAgentIdentity
	}
	return true, ""
}

// GPTQuotaSelectable 在资格之外要求账号名带 c-/d- 前缀。
func GPTQuotaSelectable(account *Account) (bool, string) {
	if ok, reason := GPTQuotaAccountEligibility(account); !ok {
		return false, reason
	}
	if GPTQuotaClass(account.Name) == "" {
		return false, GPTQuotaReasonNoDisplayPrefix
	}
	return true, ""
}

func (e GPTQuotaDisplayEntry) eligibility() (bool, string) {
	if !e.AccountExists {
		return false, GPTQuotaReasonAccountNotFound
	}
	credentials := map[string]any{}
	if e.AuthMode != "" {
		credentials[openAIAuthModeCredentialKey] = e.AuthMode
	}
	if e.LegacyAuthMode != "" {
		credentials[openAIAuthModeLegacyCredentialKey] = e.LegacyAuthMode
	}
	return GPTQuotaSelectable(&Account{ID: e.AccountID, Name: e.AccountName, Platform: e.Platform, Type: e.AccountType, ParentAccountID: e.ParentAccountID, Credentials: credentials})
}

func (e GPTQuotaDisplayEntry) effectiveDisplayName() string {
	if alias := strings.TrimSpace(e.DisplayName); alias != "" {
		return alias
	}
	return SafeGPTQuotaDisplayName(e.AccountName, e.ID)
}

// GPTQuotaClass 按账号原名 TrimSpace 后大小写不敏感的前缀归类：c- 为迅游，d- 为速宝。
func GPTQuotaClass(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.HasPrefix(n, "c-"):
		return GPTQuotaGroupXunyou
	case strings.HasPrefix(n, "d-"):
		return GPTQuotaGroupWsdashi
	default:
		return ""
	}
}

// SafeGPTQuotaDisplayName 默认展示名只保留前缀与编号（如 c-01），不暴露原名中的邮箱或备注；
// 无编号时用展示条目 ID 区分。
func SafeGPTQuotaDisplayName(name string, entryID int64) string {
	n := strings.TrimSpace(name)
	prefix := "gpt-"
	if GPTQuotaClass(n) != "" {
		prefix = strings.ToLower(n[:2])
	}
	if m := gptQuotaNameNumberPattern.FindStringSubmatch(n); len(m) == 2 {
		return prefix + m[1]
	}
	return prefix + "#" + strconv.FormatInt(entryID, 10)
}

// NormalizeGPTQuotaAlias 去除首尾空白后校验别名：限定字符集与长度，拒绝疑似密钥或账号 ID 的内容。
func NormalizeGPTQuotaAlias(raw string) (string, error) {
	alias := strings.TrimSpace(raw)
	if alias == "" {
		return "", nil
	}
	if !gptQuotaAliasPattern.MatchString(alias) {
		return "", errors.New("invalid characters or length")
	}
	for _, pattern := range gptQuotaAliasSecretPatterns {
		if pattern.MatchString(alias) {
			return "", errors.New("looks like a credential or account id")
		}
	}
	return alias, nil
}

func gptQuotaNameNumber(name string) (string, bool) {
	m := gptQuotaNameNumberPattern.FindStringSubmatch(strings.TrimSpace(name))
	if len(m) != 2 {
		return "", false
	}
	n := strings.TrimLeft(m[1], "0")
	if n == "" {
		n = "0"
	}
	return n, true
}

// compareGPTQuotaNames 实现自然排序：同组内有编号的按数值升序排在前，
// 无编号的在后；再按规范化名称和账号 ID 保证稳定。
func compareGPTQuotaNames(nameA string, idA int64, nameB string, idB int64) int {
	if ra, rb := gptQuotaGroupRank(GPTQuotaClass(nameA)), gptQuotaGroupRank(GPTQuotaClass(nameB)); ra != rb {
		return ra - rb
	}
	na, okA := gptQuotaNameNumber(nameA)
	nb, okB := gptQuotaNameNumber(nameB)
	switch {
	case okA && !okB:
		return -1
	case !okA && okB:
		return 1
	case okA && okB && na != nb:
		if len(na) != len(nb) {
			return len(na) - len(nb)
		}
		return strings.Compare(na, nb)
	}
	if c := strings.Compare(strings.ToLower(strings.TrimSpace(nameA)), strings.ToLower(strings.TrimSpace(nameB))); c != 0 {
		return c
	}
	switch {
	case idA < idB:
		return -1
	case idA > idB:
		return 1
	default:
		return 0
	}
}

// gptQuotaGroupRank 固定迅游在前、速宝在后，与用户页左右列和候选列表一致。
func gptQuotaGroupRank(group string) int {
	switch group {
	case GPTQuotaGroupXunyou:
		return 0
	case GPTQuotaGroupWsdashi:
		return 1
	default:
		return 2
	}
}

func sortGPTQuotaEntries(entries []GPTQuotaDisplayEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		return compareGPTQuotaNames(entries[i].AccountName, entries[i].AccountID, entries[j].AccountName, entries[j].AccountID) < 0
	})
}

func gptQuotaEntryAccountIDs(entries []GPTQuotaDisplayEntry) []int64 {
	ids := make([]int64, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.AccountID)
	}
	return ids
}

// duplicateChatGPTAccountIDs 返回共享同一 chatgpt_account_id 的本地账号，每个都会各打一次上游。
func duplicateChatGPTAccountIDs(accounts []*Account) []int64 {
	byChatGPT := map[string][]int64{}
	for _, account := range accounts {
		if account == nil {
			continue
		}
		key := strings.TrimSpace(account.GetCredential("chatgpt_account_id"))
		if key == "" {
			key = strings.TrimSpace(account.GetCredential("organization_id"))
		}
		if key != "" {
			byChatGPT[key] = append(byChatGPT[key], account.ID)
		}
	}
	var out []int64
	for _, ids := range byChatGPT {
		if len(ids) > 1 {
			out = append(out, ids...)
		}
	}
	return out
}

// ---- 额度归一化 ----

// NormalizeGPTQuotaUsage 只解析 rate_limit 的 primary/secondary 窗口：时长 (0, 6h] 归 5 小时，
// 大于 6 小时归 7 天；两个窗口落入同一类别、数值非法时该类别为 nil（未提供），未知时长不归类。
func NormalizeGPTQuotaUsage(usage *OpenAIQuotaUsage, sampledAt time.Time) (fiveHour, sevenDay *GPTQuotaWindow) {
	if usage == nil || usage.RateLimit == nil {
		return nil, nil
	}
	var five, seven []*OpenAIRateLimitWindow
	for _, w := range []*OpenAIRateLimitWindow{usage.RateLimit.PrimaryWindow, usage.RateLimit.SecondaryWindow} {
		if w == nil || w.LimitWindowSeconds <= 0 {
			continue
		}
		if w.LimitWindowSeconds <= gptQuotaFiveHourMaxSeconds {
			five = append(five, w)
		} else {
			seven = append(seven, w)
		}
	}
	return normalizeGPTQuotaCategory(five, sampledAt), normalizeGPTQuotaCategory(seven, sampledAt)
}

func normalizeGPTQuotaCategory(windows []*OpenAIRateLimitWindow, sampledAt time.Time) *GPTQuotaWindow {
	if len(windows) != 1 {
		return nil
	}
	w := windows[0]
	if math.IsNaN(w.UsedPercent) || math.IsInf(w.UsedPercent, 0) || w.UsedPercent < 0 {
		return nil
	}
	remaining := math.Max(0, math.Min(100, 100-w.UsedPercent))
	out := &GPTQuotaWindow{RemainingPercent: math.Round(remaining*10) / 10}
	horizon := sampledAt.Add(gptQuotaMaxResetHorizon)
	// reset_after_seconds 与 reset_at 都是 int64，缺失即 0，因此只接受正值；超出合理范围（如毫秒时间戳）视为未提供。
	if w.ResetAt > 0 {
		if resetAt := time.Unix(w.ResetAt, 0).UTC(); !resetAt.After(horizon) {
			out.ResetAt = &resetAt
			out.ResetTimeSource = "reset_at"
		}
	}
	if out.ResetAt == nil && w.ResetAfterSeconds > 0 {
		if resetAt := sampledAt.Add(time.Duration(w.ResetAfterSeconds) * time.Second).UTC(); !resetAt.After(horizon) {
			out.ResetAt = &resetAt
			out.ResetTimeSource = "reset_after_seconds"
		}
	}
	return out
}

// ---- 排程 ----

func normalizeGPTQuotaInterval(interval int) int {
	if interval == 60 {
		return 60
	}
	return 30
}

// gptQuotaDaySlots 返回 day 所在日期（按 day 的时区）的计划时点：09:30 起按间隔到 18:00，
// 60 分钟档额外包含 18:00 收尾。
func gptQuotaDaySlots(day time.Time, interval int) []time.Time {
	step := normalizeGPTQuotaInterval(interval)
	y, m, d := day.Date()
	loc := day.Location()
	var slots []time.Time
	last := 0
	for minute := gptQuotaDisplayStartMinute; minute <= gptQuotaDisplayEndMinute; minute += step {
		slots = append(slots, time.Date(y, m, d, minute/60, minute%60, 0, 0, loc))
		last = minute
	}
	if last != gptQuotaDisplayEndMinute {
		slots = append(slots, time.Date(y, m, d, gptQuotaDisplayEndMinute/60, 0, 0, 0, loc))
	}
	return slots
}

// gptQuotaDueSlot 返回当前可执行的最近计划时点。仅在该时点到下一时点之间有效（末槽为 5 分钟宽限），
// 重启或短暂停机只补最近一个到期槽位，不追补历史时点。
func gptQuotaDueSlot(now time.Time, interval int) (time.Time, bool) {
	slots := gptQuotaDaySlots(now, interval)
	for i := len(slots) - 1; i >= 0; i-- {
		if now.Before(slots[i]) {
			continue
		}
		until := slots[i].Add(gptQuotaDisplayStaleGrace)
		if i+1 < len(slots) {
			until = slots[i+1]
		}
		return slots[i], now.Before(until)
	}
	return time.Time{}, false
}

func gptQuotaNextSlot(now time.Time, interval int) time.Time {
	for offset := 0; offset < 2; offset++ {
		for _, slot := range gptQuotaDaySlots(now.AddDate(0, 0, offset), interval) {
			if slot.After(now) {
				return slot
			}
		}
	}
	return gptQuotaDaySlots(now.AddDate(0, 0, 1), interval)[0]
}

func gptQuotaInWindow(now time.Time) bool {
	minute := now.Hour()*60 + now.Minute()
	return minute >= gptQuotaDisplayStartMinute && minute <= gptQuotaDisplayEndMinute
}

// gptQuotaIsStale：最近一个已过 5 分钟宽限的计划时点之后仍无成功采样即过期（允许冷却期内的提前采样）。
// 夜间与非采集时段不会因自然时间流逝变成过期。
func gptQuotaIsStale(sampledAt *time.Time, now time.Time, interval int) bool {
	if sampledAt == nil {
		return false
	}
	anchor := now.Add(-gptQuotaDisplayStaleGrace)
	for offset := 0; offset < 2; offset++ {
		slots := gptQuotaDaySlots(anchor.AddDate(0, 0, -offset), interval)
		for i := len(slots) - 1; i >= 0; i-- {
			if !anchor.Before(slots[i]) {
				return sampledAt.Before(slots[i].Add(-gptQuotaDisplayCooldown))
			}
		}
	}
	return false
}
