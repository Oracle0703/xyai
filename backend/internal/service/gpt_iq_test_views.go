package service

import (
	"context"
	"log/slog"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// 降智测试的读取视图与判定。所有读取只查数据库，任何读取路径都不触发测试或上游调用。
// 判定在读取时按当前阈值计算，不落库（设计 §8）。

const (
	GPTIQVerdictUntested     = "untested"
	GPTIQVerdictSkipped      = "skipped"
	GPTIQVerdictFailed       = "failed"
	GPTIQVerdictInsufficient = "insufficient"
	GPTIQVerdictSuspect      = "suspect"
	GPTIQVerdictNormal       = "normal"
	GPTIQVerdictConnected    = "connected"

	GPTIQReasonLowAccuracy         = "low_accuracy"
	GPTIQReasonLowReasoning        = "low_reasoning"
	GPTIQReasonInsufficientSamples = "insufficient_samples"
	GPTIQReasonAllFailed           = "all_failed"
	GPTIQReasonChatFailed          = "chat_failed"
	// GPTIQReasonInterrupted：只有取消或执行中断的调用，没有上游失败证据，按样本不足处理。
	GPTIQReasonInterrupted = "interrupted"

	gptIQTrendDays          = 14
	gptIQDetailMaxDays      = 30
	gptIQPreviewRunes       = 120
	gptIQListAnswerChars    = 400
	gptIQBatchAnswerChars   = 2000
	gptIQBatchListLimit     = 10
	gptIQReasoningMinPeers  = 3
	gptIQMarkdownStripChars = "*_`#>"
)

var gptIQMarkdownLinkPattern = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)

type GPTIQScheduleView struct {
	Enabled  bool   `json:"enabled"`
	RunTime  string `json:"run_time"`
	Timezone string `json:"timezone"`
}

type GPTIQPublicCase struct {
	ID int64 `json:"id"`
	// RevisionID 是当前版本 ID；前端据此给历史结果标"第 N 版"。
	RevisionID      int64  `json:"revision_id"`
	Name            string `json:"name"`
	Kind            string `json:"kind"`
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	Runs            int    `json:"runs"`
	Prompt          string `json:"prompt,omitempty"`
	PublicAnswer    bool   `json:"public_answer"`
}

type GPTIQRunView struct {
	Index           int        `json:"index"`
	Status          string     `json:"status"`
	Reason          string     `json:"reason,omitempty"`
	Correct         *bool      `json:"correct"`
	AnswerPreview   string     `json:"answer_preview,omitempty"`
	Answer          string     `json:"answer,omitempty"`
	AnswerTruncated bool       `json:"answer_truncated,omitempty"`
	InputTokens     *int       `json:"input_tokens"`
	OutputTokens    *int       `json:"output_tokens"`
	ReasoningTokens *int       `json:"reasoning_tokens"`
	DurationMs      *int       `json:"duration_ms"`
	TTFTMs          *int       `json:"ttft_ms"`
	TPS             *float64   `json:"tps"`
	StartedAt       time.Time  `json:"started_at"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
}

type GPTIQCaseSetView struct {
	CaseID                int64          `json:"case_id"`
	BatchID               int64          `json:"batch_id"`
	Trigger               string         `json:"trigger"`
	RunDate               string         `json:"run_date"`
	TestedAt              time.Time      `json:"tested_at"`
	Runs                  []GPTIQRunView `json:"runs"`
	Total                 int            `json:"total"`
	OK                    int            `json:"ok"`
	Errors                int            `json:"errors"`
	Skipped               int            `json:"skipped"`
	Graded                int            `json:"graded"`
	Correct               int            `json:"correct"`
	Interrupted           int            `json:"interrupted"`
	MedianReasoningTokens *int           `json:"median_reasoning_tokens"`
	AvgDurationMs         *int           `json:"avg_duration_ms"`
	AvgTPS                *float64       `json:"avg_tps"`
	// Revision 是这组调用实际执行的题目版本（历史结果不套用当前题面与参数）；题面只在测试项当前公开时返回。
	Revision *GPTIQSetRevision `json:"revision"`
}

type GPTIQSetRevision struct {
	ID              int64  `json:"id"`
	Revision        int    `json:"revision"`
	Kind            string `json:"kind"`
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	Runs            int    `json:"runs"`
	Prompt          string `json:"prompt,omitempty"`
}

type GPTIQReason struct {
	Code    string `json:"code"`
	CaseID  int64  `json:"case_id,omitempty"`
	Correct int    `json:"correct"`
	Graded  int    `json:"graded"`
	Percent int    `json:"percent"`
	Detail  string `json:"detail,omitempty"`
}

type GPTIQTrendPoint struct {
	Date    string `json:"date"`
	Verdict string `json:"verdict"`
	Correct int    `json:"correct"`
	Graded  int    `json:"graded"`
}

type GPTIQUserCard struct {
	ID          int64              `json:"id"`
	DisplayName string             `json:"display_name"`
	Verdict     string             `json:"verdict"`
	Reasons     []GPTIQReason      `json:"reasons"`
	TestedAt    *time.Time         `json:"tested_at"`
	Cases       []GPTIQCaseSetView `json:"cases"`
	Trend       []GPTIQTrendPoint  `json:"trend"`
}

type GPTIQUserGroups struct {
	Xunyou  []GPTIQUserCard `json:"xunyou"`
	Wsdashi []GPTIQUserCard `json:"wsdashi"`
}

type GPTIQUserView struct {
	Enabled             bool              `json:"enabled"`
	ServerTime          time.Time         `json:"server_time"`
	PollIntervalSeconds int               `json:"poll_interval_seconds"`
	Schedule            GPTIQScheduleView `json:"schedule"`
	NextRunAt           *time.Time        `json:"next_run_at"`
	Thresholds          GPTIQThresholds   `json:"thresholds"`
	Cases               []GPTIQPublicCase `json:"cases"`
	Groups              GPTIQUserGroups   `json:"groups"`
}

type GPTIQThresholds struct {
	MinAccuracyPercent    int `json:"min_accuracy_percent"`
	MinGradedRuns         int `json:"min_graded_runs"`
	ReasoningRatioPercent int `json:"reasoning_ratio_percent"`
}

type GPTIQEntryDetail struct {
	Enabled     bool               `json:"enabled"`
	ID          int64              `json:"id"`
	DisplayName string             `json:"display_name"`
	Days        int                `json:"days"`
	Cases       []GPTIQPublicCase  `json:"cases"`
	Sets        []GPTIQCaseSetView `json:"sets"`
}

type GPTIQAdminEntry struct {
	ID           int64      `json:"id"`
	DisplayName  string     `json:"display_name"`
	Group        string     `json:"group"`
	Runnable     bool       `json:"runnable"`
	BackoffUntil *time.Time `json:"backoff_until,omitempty"`
}

type GPTIQActorScope struct {
	FullScope    bool     `json:"full_scope"`
	CanConfigure bool     `json:"can_configure"`
	Groups       []string `json:"groups"`
}

type GPTIQAdminView struct {
	Config          *GPTIQTestConfig        `json:"config"`
	ServerTime      time.Time               `json:"server_time"`
	Timezone        string                  `json:"timezone"`
	NextRunAt       *time.Time              `json:"next_run_at"`
	Scope           GPTIQActorScope         `json:"scope"`
	Cases           []GPTIQTestCase         `json:"cases"`
	Templates       []GPTIQCaseTemplate     `json:"templates"`
	Entries         []GPTIQAdminEntry       `json:"entries"`
	Batches         []GPTIQTestBatchSummary `json:"batches"`
	RunningBatchID  *int64                  `json:"running_batch_id"`
	DispatchedToday int                     `json:"dispatched_today"`
}

type GPTIQBatchResultView struct {
	GPTIQTestResult
	EntryName string   `json:"entry_name"`
	Group     string   `json:"group"`
	TPS       *float64 `json:"tps"`
}

type GPTIQBatchDetail struct {
	Batch   *GPTIQTestBatch        `json:"batch"`
	Results []GPTIQBatchResultView `json:"results"`
	// VisibleTotalCalls 是操作者范围内的计划调用数；组织负责人看全站批次时用它算进度。
	VisibleTotalCalls int `json:"visible_total_calls"`
}

// Status 供侧栏决定 /gpt-quota 菜单：额度展示或降智测试任一开启即显示。两边相互独立：一边读取失败
// 按"未开启"处理并记录日志，不影响另一边；两边都失败才返回错误。
func (s *GPTIQTestService) Status(ctx context.Context) (quotaEnabled, iqEnabled bool, err error) {
	var quotaErr, iqErr error
	if s.quota != nil && s.quota.repo != nil {
		if quotaEnabled, quotaErr = s.quota.Enabled(ctx); quotaErr != nil {
			quotaEnabled = false
			slog.Warn("gpt_quota_status_failed", "error", quotaErr)
		}
	}
	if s.repo != nil {
		cfg, cfgErr := s.repo.GetConfig(ctx)
		if cfgErr != nil {
			iqErr = cfgErr
			slog.Warn("gpt_iq_test_status_failed", "error", cfgErr)
		} else {
			iqEnabled = cfg.Enabled
		}
	}
	if quotaErr != nil && iqErr != nil {
		return false, false, quotaErr
	}
	return quotaEnabled, iqEnabled, nil
}

func (s *GPTIQTestService) nextRunAt(cfg *GPTIQTestConfig) *time.Time {
	if !cfg.ScheduleEnabled {
		return nil
	}
	now := s.localNow()
	start, _, ok := gptIQRunWindow(now, cfg.RunTime)
	if !ok {
		return nil
	}
	// 当天未领取且仍在窗口内（含被手动批次推迟）时，下一次就是今天的计划时刻。
	_, end, _ := gptIQRunWindow(now, cfg.RunTime)
	if cfg.LastRunDate == now.Format(gptIQDateLayout) || !now.Before(end) {
		start = start.AddDate(0, 0, 1)
	}
	utc := start.UTC()
	return &utc
}

func publicGPTIQCase(c GPTIQTestCase) GPTIQPublicCase {
	out := GPTIQPublicCase{ID: c.ID, RevisionID: c.RevisionID, Name: c.Name, Kind: c.Kind, Model: c.Model, ReasoningEffort: c.ReasoningEffort, Runs: c.Runs, PublicAnswer: c.PublicAnswer}
	if c.PublicPrompt {
		out.Prompt = c.Prompt
	}
	return out
}

// UserView 返回 /gpt-quota 下方的降智测试区。关闭时分组为空，不泄漏历史结果；
// 账号只取当前可见的额度展示条目，测试项只取当前启用项，并按当前公开开关过滤回答。
func (s *GPTIQTestService) UserView(ctx context.Context) (*GPTIQUserView, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	cfg, err := s.repo.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	view := &GPTIQUserView{
		Enabled:             cfg.Enabled,
		ServerTime:          s.now().UTC(),
		PollIntervalSeconds: GPTIQTestPollIntervalSeconds,
		Schedule:            GPTIQScheduleView{Enabled: cfg.ScheduleEnabled, RunTime: cfg.RunTime, Timezone: gptQuotaDisplayTimezone},
		Thresholds:          GPTIQThresholds{MinAccuracyPercent: cfg.MinAccuracyPercent, MinGradedRuns: cfg.MinGradedRuns, ReasoningRatioPercent: cfg.ReasoningRatioPercent},
		Cases:               []GPTIQPublicCase{},
		Groups:              GPTIQUserGroups{Xunyou: []GPTIQUserCard{}, Wsdashi: []GPTIQUserCard{}},
	}
	if !cfg.Enabled {
		return view, nil
	}
	view.NextRunAt = s.nextRunAt(cfg)
	entries, err := s.visibleEntries(ctx)
	if err != nil {
		return nil, err
	}
	cases, err := s.enabledCases(ctx)
	if err != nil {
		return nil, err
	}
	for _, c := range cases {
		view.Cases = append(view.Cases, publicGPTIQCase(c))
	}
	if len(entries) == 0 || len(cases) == 0 {
		for _, e := range entries {
			view.appendCard(e.Group, GPTIQUserCard{ID: e.ID, DisplayName: e.DisplayName, Verdict: GPTIQVerdictUntested, Reasons: []GPTIQReason{}, Cases: []GPTIQCaseSetView{}, Trend: []GPTIQTrendPoint{}})
		}
		return view, nil
	}
	since := s.localNow().AddDate(0, 0, -(gptIQTrendDays - 1)).Format(gptIQDateLayout)
	entryIDs := make([]int64, 0, len(entries))
	for _, e := range entries {
		entryIDs = append(entryIDs, e.ID)
	}
	results, err := s.repo.ListResultsSince(ctx, since, entryIDs, gptIQListAnswerChars)
	if err != nil {
		return nil, err
	}
	caseByID := make(map[int64]GPTIQTestCase, len(cases))
	for _, c := range cases {
		caseByID[c.ID] = c
	}
	filtered := results[:0]
	for _, r := range results {
		if _, ok := caseByID[r.CaseID]; ok {
			filtered = append(filtered, r)
		}
	}
	revs, err := s.revisionsFor(ctx, filtered)
	if err != nil {
		return nil, err
	}
	sets := buildGPTIQSets(filtered)
	// 运行中批次的结果不完整（例如 3 次只跑了 1 次），不参与卡片、趋势与基线，批次结束后再体现。
	if err := s.dropRunningBatchSets(ctx, sets); err != nil {
		return nil, err
	}
	baselines := gptIQBaselines(sets)
	for _, e := range entries {
		card := GPTIQUserCard{ID: e.ID, DisplayName: e.DisplayName, Reasons: []GPTIQReason{}, Cases: []GPTIQCaseSetView{}, Trend: []GPTIQTrendPoint{}}
		current := make([]*gptIQSet, 0, len(cases))
		for _, c := range cases {
			if set := latestGPTIQSet(sets, e.ID, c.ID, ""); set != nil {
				current = append(current, set)
				card.Cases = append(card.Cases, set.view(c.PublicAnswer, false, c.PublicPrompt, revs[set.revisionID]))
				if card.TestedAt == nil || set.testedAt.After(*card.TestedAt) {
					t := set.testedAt
					card.TestedAt = &t
				}
			}
		}
		card.Verdict, card.Reasons = gptIQVerdict(current, revs, baselines, cfg)
		card.Trend = gptIQTrend(sets, e.ID, cases, revs, baselines, cfg)
		view.appendCard(e.Group, card)
	}
	return view, nil
}

func (v *GPTIQUserView) appendCard(group string, card GPTIQUserCard) {
	if group == GPTQuotaGroupXunyou {
		v.Groups.Xunyou = append(v.Groups.Xunyou, card)
	} else {
		v.Groups.Wsdashi = append(v.Groups.Wsdashi, card)
	}
}

func (s *GPTIQTestService) enabledCases(ctx context.Context) ([]GPTIQTestCase, error) {
	all, err := s.repo.ListCases(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]GPTIQTestCase, 0, len(all))
	for _, c := range all {
		if c.Enabled {
			out = append(out, c)
		}
	}
	return out, nil
}

func (s *GPTIQTestService) revisionsFor(ctx context.Context, results []GPTIQTestResult) (map[int64]*GPTIQTestRevision, error) {
	ids := make([]int64, 0)
	seen := map[int64]bool{}
	for _, r := range results {
		if !seen[r.RevisionID] {
			seen[r.RevisionID] = true
			ids = append(ids, r.RevisionID)
		}
	}
	if len(ids) == 0 {
		return map[int64]*GPTIQTestRevision{}, nil
	}
	return s.repo.ListRevisions(ctx, ids)
}

// EntryDetail 返回单个账号最近 days 天的全部调用（回答全文受当前 public_answer 约束）。
func (s *GPTIQTestService) EntryDetail(ctx context.Context, entryID, caseID int64, days int) (*GPTIQEntryDetail, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if days <= 0 {
		days = gptIQTrendDays
	}
	days = min(days, gptIQDetailMaxDays)
	cfg, err := s.repo.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	detail := &GPTIQEntryDetail{Enabled: cfg.Enabled, ID: entryID, Days: days, Cases: []GPTIQPublicCase{}, Sets: []GPTIQCaseSetView{}}
	if !cfg.Enabled {
		return detail, nil
	}
	entries, err := s.visibleEntries(ctx)
	if err != nil {
		return nil, err
	}
	var entry *gptIQEntry
	for i := range entries {
		if entries[i].ID == entryID {
			entry = &entries[i]
			break
		}
	}
	if entry == nil {
		return nil, ErrGPTIQEntryNotFound
	}
	detail.DisplayName = entry.DisplayName
	cases, err := s.enabledCases(ctx)
	if err != nil {
		return nil, err
	}
	caseByID := map[int64]GPTIQTestCase{}
	for _, c := range cases {
		if caseID > 0 && c.ID != caseID {
			continue
		}
		caseByID[c.ID] = c
		detail.Cases = append(detail.Cases, publicGPTIQCase(c))
	}
	if caseID > 0 && len(caseByID) == 0 {
		return nil, ErrGPTIQCaseNotFound
	}
	since := s.localNow().AddDate(0, 0, -(days - 1)).Format(gptIQDateLayout)
	results, err := s.repo.ListEntryResults(ctx, entryID, caseID, since)
	if err != nil {
		return nil, err
	}
	filtered := results[:0]
	for _, r := range results {
		if _, ok := caseByID[r.CaseID]; ok {
			filtered = append(filtered, r)
		}
	}
	revs, err := s.revisionsFor(ctx, filtered)
	if err != nil {
		return nil, err
	}
	sets := buildGPTIQSets(filtered)
	ordered := make([]*gptIQSet, 0, len(sets))
	for _, set := range sets {
		ordered = append(ordered, set)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].batchID != ordered[j].batchID {
			return ordered[i].batchID > ordered[j].batchID
		}
		return ordered[i].caseID < ordered[j].caseID
	})
	for _, set := range ordered {
		c := caseByID[set.caseID]
		detail.Sets = append(detail.Sets, set.view(c.PublicAnswer, true, c.PublicPrompt, revs[set.revisionID]))
	}
	return detail, nil
}

// AdminView 返回配置页数据。账号只给条目 ID、展示名与分组，不返回账号 ID 或原名
// （组织负责人可访问本页）。
func (s *GPTIQTestService) AdminView(ctx context.Context, actor *GPTIQTestActor) (*GPTIQAdminView, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	cfg, err := s.repo.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	cases, err := s.repo.ListCases(ctx)
	if err != nil {
		return nil, err
	}
	entries, err := s.visibleEntries(ctx)
	if err != nil {
		return nil, err
	}
	selected, err := s.selectedEntries(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]gptIQEntry, len(selected))
	for _, e := range selected {
		byID[e.ID] = e
	}
	// 组织负责人只看到包含其组织条目的批次，计数也只统计其条目。
	var scopeEntryIDs []int64
	if !actor.FullScope {
		scopeEntryIDs = []int64{}
		for _, e := range selected {
			if actor.allows(e.Group) {
				scopeEntryIDs = append(scopeEntryIDs, e.ID)
			}
		}
	}
	batches, err := s.repo.ListBatchSummaries(ctx, gptIQBatchListLimit, scopeEntryIDs)
	if err != nil {
		return nil, err
	}
	for i := range batches {
		batches[i].GPTIQTestBatch = *projectGPTIQBatch(&batches[i].GPTIQTestBatch, actor, byID)
	}
	templates := []GPTIQCaseTemplate{}
	if actor.CanConfigure {
		templates = GPTIQCaseTemplates()
	}
	dispatched, err := s.repo.DispatchedOn(ctx, s.localDate())
	if err != nil {
		return nil, err
	}
	view := &GPTIQAdminView{
		Config: cfg, ServerTime: s.now().UTC(), Timezone: gptQuotaDisplayTimezone, NextRunAt: s.nextRunAt(cfg),
		Scope:     GPTIQActorScope{FullScope: actor.FullScope, CanConfigure: actor.CanConfigure, Groups: actor.scopeGroups()},
		Cases:     cases,
		Templates: templates,
		Entries:   make([]GPTIQAdminEntry, 0, len(entries)),
		Batches:   batches, DispatchedToday: dispatched,
	}
	// 退避信息只给操作者范围内的条目。
	accountIDs := make([]int64, 0, len(entries))
	for _, e := range entries {
		if actor.allows(e.Group) {
			accountIDs = append(accountIDs, e.AccountID)
		}
	}
	backoffs, err := s.repo.ListBackoff(ctx, accountIDs)
	if err != nil {
		return nil, err
	}
	now := s.now()
	for _, e := range entries {
		// 管理视图只列操作者范围内的条目；其他组织的条目不返回（公开结果页另有全员可见的规则）。
		if !actor.allows(e.Group) {
			continue
		}
		item := GPTIQAdminEntry{ID: e.ID, DisplayName: e.DisplayName, Group: e.Group, Runnable: true}
		if until, ok := backoffs[e.AccountID]; ok && until.After(now) {
			u := until.UTC()
			item.BackoffUntil = &u
		}
		view.Entries = append(view.Entries, item)
	}
	for _, b := range batches {
		if b.Status == GPTIQBatchRunning {
			id := b.ID
			view.RunningBatchID = &id
			break
		}
	}
	return view, nil
}

// BatchDetail 返回批次进度与逐条结果（含脱敏错误摘要）；组织负责人只看到其授权组织的条目。
func (s *GPTIQTestService) BatchDetail(ctx context.Context, actor *GPTIQTestActor, batchID int64) (*GPTIQBatchDetail, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	batch, err := s.repo.GetBatch(ctx, batchID)
	if err != nil {
		return nil, err
	}
	results, err := s.repo.ListBatchResults(ctx, batchID, gptIQBatchAnswerChars)
	if err != nil {
		return nil, err
	}
	entries, err := s.selectedEntries(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]gptIQEntry, len(entries))
	for _, e := range entries {
		byID[e.ID] = e
	}
	projected := projectGPTIQBatch(batch, actor, byID)
	if !actor.FullScope && len(projected.EntryIDs) == 0 && (batch.TriggeredBy == nil || *batch.TriggeredBy != actor.UserID) {
		// 不含其组织条目的批次对组织负责人不可见。
		return nil, ErrGPTIQBatchNotFound
	}
	detail := &GPTIQBatchDetail{Batch: projected, Results: make([]GPTIQBatchResultView, 0, len(results)), VisibleTotalCalls: projected.TotalCalls}
	for _, r := range results {
		e, ok := byID[r.EntryID]
		if !ok {
			// 条目已不在额度展示中：只有全站范围可见，名称用条目 ID 代替。
			if !actor.FullScope {
				continue
			}
			e = gptIQEntry{ID: r.EntryID, DisplayName: "#" + strconv.FormatInt(r.EntryID, 10)}
		} else if !actor.allows(e.Group) {
			continue
		}
		detail.Results = append(detail.Results, GPTIQBatchResultView{GPTIQTestResult: r, EntryName: e.DisplayName, Group: e.Group, TPS: gptIQTPS(r.OutputTokens, r.DurationMs)})
	}
	return detail, nil
}

// projectGPTIQBatch 把批次元数据投影到操作者范围：组织负责人只看到其组织的条目 ID、对应的计划调用数，
// 发起人只在是本人时返回。全站范围原样返回。
func projectGPTIQBatch(b *GPTIQTestBatch, actor *GPTIQTestActor, byID map[int64]gptIQEntry) *GPTIQTestBatch {
	if actor.FullScope {
		return b
	}
	out := *b
	out.EntryIDs = []int64{}
	for _, id := range b.EntryIDs {
		if e, ok := byID[id]; ok && actor.allows(e.Group) {
			out.EntryIDs = append(out.EntryIDs, id)
		}
	}
	out.TotalCalls = 0
	if len(b.EntryIDs) > 0 {
		// 每个条目的计划调用数相同（各测试项次数之和），按范围内条目数折算。
		out.TotalCalls = b.TotalCalls / len(b.EntryIDs) * len(out.EntryIDs)
	}
	if b.TriggeredBy == nil || *b.TriggeredBy != actor.UserID {
		out.TriggeredBy = nil
	}
	out.ScopeGroups = []string{}
	for _, g := range b.ScopeGroups {
		if actor.allows(g) {
			out.ScopeGroups = append(out.ScopeGroups, g)
		}
	}
	return &out
}

// ---- 结果分组与判定 ----

// gptIQSet 是同一批次内某账号某测试项的全部调用。
type gptIQSet struct {
	batchID    int64
	entryID    int64
	caseID     int64
	revisionID int64
	trigger    string
	runDate    string
	testedAt   time.Time
	runs       []GPTIQTestResult

	// model 是实际请求的上游模型（账号映射后），思考 token 只在同模型的账号之间比较。
	model string

	ok, errors, skipped, interrupted, graded, correct int
	medianReasoning                                   *int
}

type gptIQSetKey struct{ entryID, caseID, batchID int64 }

type gptIQBaselineKey struct {
	batchID, revisionID int64
	model               string
}

func buildGPTIQSets(results []GPTIQTestResult) map[gptIQSetKey]*gptIQSet {
	sets := map[gptIQSetKey]*gptIQSet{}
	for _, r := range results {
		key := gptIQSetKey{r.EntryID, r.CaseID, r.BatchID}
		set := sets[key]
		if set == nil {
			set = &gptIQSet{batchID: r.BatchID, entryID: r.EntryID, caseID: r.CaseID, revisionID: r.RevisionID, trigger: r.Trigger, runDate: r.RunDate, testedAt: r.StartedAt}
			sets[key] = set
		}
		if r.StartedAt.Before(set.testedAt) {
			set.testedAt = r.StartedAt
		}
		set.runs = append(set.runs, r)
	}
	for _, set := range sets {
		sort.Slice(set.runs, func(i, j int) bool { return set.runs[i].RunIndex < set.runs[j].RunIndex })
		reasoning := []int{}
		for _, r := range set.runs {
			switch r.Status {
			case GPTIQResultOK:
				set.ok++
				if r.Correct != nil {
					set.graded++
					if *r.Correct {
						set.correct++
					}
				}
				if r.ReasoningTokens != nil {
					reasoning = append(reasoning, *r.ReasoningTokens)
				}
			case GPTIQResultError:
				set.errors++
			case GPTIQResultCancelled, GPTIQResultAbandoned:
				// 执行中断不是上游失败，不能把账号标成失败。
				set.interrupted++
			case GPTIQResultSkipped:
				set.skipped++
			}
			if set.model == "" && (r.Status == GPTIQResultOK || r.Status == GPTIQResultError) {
				set.model = r.ModelRequested
			}
		}
		set.medianReasoning = gptIQMedian(reasoning)
	}
	return sets
}

func gptIQMedian(values []int) *int {
	if len(values) == 0 {
		return nil
	}
	sorted := append([]int{}, values...)
	sort.Ints(sorted)
	mid := len(sorted) / 2
	m := sorted[mid]
	if len(sorted)%2 == 0 {
		m = (sorted[mid-1] + sorted[mid]) / 2
	}
	return &m
}

// gptIQBaselines 计算同批次、同题目版本、同实际请求模型下各账号思考 token 中位数的中位数；少于 3 个账号时不给基线。
// 思考强度属于题目版本，所以同版本即同强度。
func gptIQBaselines(sets map[gptIQSetKey]*gptIQSet) map[gptIQBaselineKey]int {
	values := map[gptIQBaselineKey][]int{}
	for _, set := range sets {
		if set.medianReasoning != nil && set.model != "" {
			key := gptIQBaselineKey{set.batchID, set.revisionID, set.model}
			values[key] = append(values[key], *set.medianReasoning)
		}
	}
	out := map[gptIQBaselineKey]int{}
	for key, v := range values {
		if len(v) >= gptIQReasoningMinPeers {
			if m := gptIQMedian(v); m != nil && *m > 0 {
				out[key] = *m
			}
		}
	}
	return out
}

func (s *GPTIQTestService) dropRunningBatchSets(ctx context.Context, sets map[gptIQSetKey]*gptIQSet) error {
	ids := make([]int64, 0)
	seen := map[int64]bool{}
	for key := range sets {
		if !seen[key.batchID] {
			seen[key.batchID] = true
			ids = append(ids, key.batchID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	statuses, err := s.repo.ListBatchStatuses(ctx, ids)
	if err != nil {
		return err
	}
	for key := range sets {
		if statuses[key.batchID] == GPTIQBatchRunning {
			delete(sets, key)
		}
	}
	return nil
}

// latestGPTIQSet 取某账号某测试项最近一次批次的结果。date 为空（卡片）时优先取有实际调用（ok/error）的
// 最近一组，全部跳过的复测（冷却、预算）不覆盖之前的结论，只有没有其他结果时才显示；date 非空（趋势）
// 时只取该日的定时批次，手动复测不进入正式日趋势。
func latestGPTIQSet(sets map[gptIQSetKey]*gptIQSet, entryID, caseID int64, date string) *gptIQSet {
	if date == "" {
		var informative, latest *gptIQSet
		for key, set := range sets {
			if key.entryID != entryID || key.caseID != caseID {
				continue
			}
			if latest == nil || set.batchID > latest.batchID {
				latest = set
			}
			if set.ok+set.errors > 0 && (informative == nil || set.batchID > informative.batchID) {
				informative = set
			}
		}
		if informative != nil {
			return informative
		}
		return latest
	}
	var best *gptIQSet
	for key, set := range sets {
		if key.entryID != entryID || key.caseID != caseID {
			continue
		}
		if set.runDate != date || set.trigger != GPTIQTriggerSchedule {
			continue
		}
		if best == nil || set.batchID > best.batchID {
			best = set
		}
	}
	return best
}

func (set *gptIQSet) view(publicAnswer, fullAnswer, publicPrompt bool, rev *GPTIQTestRevision) GPTIQCaseSetView {
	out := GPTIQCaseSetView{
		CaseID: set.caseID, BatchID: set.batchID, Trigger: set.trigger, RunDate: set.runDate, TestedAt: set.testedAt,
		Runs: make([]GPTIQRunView, 0, len(set.runs)), Total: len(set.runs),
		OK: set.ok, Errors: set.errors, Skipped: set.skipped, Interrupted: set.interrupted, Graded: set.graded, Correct: set.correct,
		MedianReasoningTokens: set.medianReasoning,
	}
	if rev != nil {
		out.Revision = &GPTIQSetRevision{ID: rev.ID, Revision: rev.Revision, Kind: rev.Kind, Model: rev.Model, ReasoningEffort: rev.ReasoningEffort, Runs: rev.Runs}
		if publicPrompt {
			out.Revision.Prompt = rev.Prompt
		}
	}
	var durationSum, durationN int
	var tpsSum float64
	var tpsN int
	for _, r := range set.runs {
		run := GPTIQRunView{
			Index: r.RunIndex, Status: r.Status, Reason: r.Reason, Correct: r.Correct,
			InputTokens: r.InputTokens, OutputTokens: r.OutputTokens, ReasoningTokens: r.ReasoningTokens,
			DurationMs: r.DurationMs, TTFTMs: r.TTFTMs, TPS: gptIQTPS(r.OutputTokens, r.DurationMs),
			StartedAt: r.StartedAt, FinishedAt: r.FinishedAt,
		}
		if publicAnswer && r.Status == GPTIQResultOK {
			run.AnswerPreview = gptIQPreview(r.AnswerText)
			if fullAnswer {
				run.Answer = r.AnswerText
				run.AnswerTruncated = r.AnswerTruncated
			}
		}
		if r.Status == GPTIQResultOK && r.DurationMs != nil {
			durationSum += *r.DurationMs
			durationN++
		}
		if run.TPS != nil {
			tpsSum += *run.TPS
			tpsN++
		}
		out.Runs = append(out.Runs, run)
	}
	if durationN > 0 {
		avg := durationSum / durationN
		out.AvgDurationMs = &avg
	}
	if tpsN > 0 {
		avg := tpsSum / float64(tpsN)
		out.AvgTPS = &avg
	}
	return out
}

func gptIQTPS(outputTokens, durationMs *int) *float64 {
	if outputTokens == nil || durationMs == nil || *durationMs <= 0 {
		return nil
	}
	v := float64(*outputTokens) / (float64(*durationMs) / 1000)
	return &v
}

// gptIQPreview 去掉常见 Markdown 标记、合并空白后截到 120 个字符。
func gptIQPreview(answer string) string {
	s := gptIQMarkdownLinkPattern.ReplaceAllString(answer, "$1")
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(gptIQMarkdownStripChars, r) {
			return -1
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > gptIQPreviewRunes {
		s = string([]rune(s)[:gptIQPreviewRunes]) + "…"
	}
	return s
}

// gptIQVerdict 由一组测试项结果给出判定与原因（设计 §8）。
func gptIQVerdict(sets []*gptIQSet, revs map[int64]*GPTIQTestRevision, baselines map[gptIQBaselineKey]int, cfg *GPTIQTestConfig) (string, []GPTIQReason) {
	reasons := []GPTIQReason{}
	if len(sets) == 0 {
		return GPTIQVerdictUntested, reasons
	}
	active := make([]*gptIQSet, 0, len(sets))
	anySkipped, anyInterrupted := false, false
	for _, set := range sets {
		if set.ok+set.errors > 0 {
			active = append(active, set)
		}
		if set.skipped > 0 {
			anySkipped = true
		}
		if set.interrupted > 0 {
			anyInterrupted = true
		}
	}
	if len(active) == 0 {
		switch {
		case anyInterrupted:
			return GPTIQVerdictInsufficient, append(reasons, GPTIQReason{Code: GPTIQReasonInterrupted})
		case anySkipped:
			return GPTIQVerdictSkipped, reasons
		}
		return GPTIQVerdictUntested, reasons
	}
	anyOK := false
	for _, set := range active {
		if set.ok > 0 {
			anyOK = true
		}
	}
	if !anyOK {
		return GPTIQVerdictFailed, append(reasons, GPTIQReason{Code: GPTIQReasonAllFailed, Detail: gptIQMostCommonReason(active)})
	}
	suspect, insufficient := false, false
	evaluated := 0
	for _, set := range active {
		rev := revs[set.revisionID]
		if rev == nil || rev.Kind != GPTIQKindGraded {
			if set.ok == 0 {
				reasons = append(reasons, GPTIQReason{Code: GPTIQReasonChatFailed, CaseID: set.caseID, Detail: gptIQMostCommonReason([]*gptIQSet{set})})
			}
			continue
		}
		if set.graded < cfg.MinGradedRuns {
			insufficient = true
			reasons = append(reasons, GPTIQReason{Code: GPTIQReasonInsufficientSamples, CaseID: set.caseID, Correct: set.correct, Graded: set.graded})
			continue
		}
		evaluated++
		if set.correct*100 < cfg.MinAccuracyPercent*set.graded {
			suspect = true
			reasons = append(reasons, GPTIQReason{Code: GPTIQReasonLowAccuracy, CaseID: set.caseID, Correct: set.correct, Graded: set.graded})
		}
		if set.medianReasoning != nil && cfg.ReasoningRatioPercent > 0 {
			if base := baselines[gptIQBaselineKey{set.batchID, set.revisionID, set.model}]; base > 0 && *set.medianReasoning*100 < base*cfg.ReasoningRatioPercent {
				suspect = true
				reasons = append(reasons, GPTIQReason{Code: GPTIQReasonLowReasoning, CaseID: set.caseID, Percent: *set.medianReasoning * 100 / base})
			}
		}
	}
	switch {
	case suspect:
		return GPTIQVerdictSuspect, reasons
	case insufficient:
		return GPTIQVerdictInsufficient, reasons
	case evaluated == 0:
		return GPTIQVerdictConnected, reasons
	default:
		return GPTIQVerdictNormal, reasons
	}
}

func gptIQMostCommonReason(sets []*gptIQSet) string {
	counts := map[string]int{}
	best, bestN := "", 0
	for _, set := range sets {
		for _, r := range set.runs {
			if r.Status != GPTIQResultError {
				continue
			}
			reason := r.Reason
			counts[reason]++
			if counts[reason] > bestN || (counts[reason] == bestN && reason < best) {
				best, bestN = reason, counts[reason]
			}
		}
	}
	return best
}

// gptIQTrend 按日给出判定，只取定时批次；当天没有定时结果就没有该日的点（手动复测在卡片上以"复测"展示）。
func gptIQTrend(sets map[gptIQSetKey]*gptIQSet, entryID int64, cases []GPTIQTestCase, revs map[int64]*GPTIQTestRevision, baselines map[gptIQBaselineKey]int, cfg *GPTIQTestConfig) []GPTIQTrendPoint {
	dates := map[string]bool{}
	for key, set := range sets {
		if key.entryID == entryID {
			dates[set.runDate] = true
		}
	}
	ordered := make([]string, 0, len(dates))
	for d := range dates {
		ordered = append(ordered, d)
	}
	sort.Strings(ordered)
	out := make([]GPTIQTrendPoint, 0, len(ordered))
	for _, date := range ordered {
		daySets := make([]*gptIQSet, 0, len(cases))
		point := GPTIQTrendPoint{Date: date}
		for _, c := range cases {
			if set := latestGPTIQSet(sets, entryID, c.ID, date); set != nil {
				daySets = append(daySets, set)
				if rev := revs[set.revisionID]; rev != nil && rev.Kind == GPTIQKindGraded {
					point.Correct += set.correct
					point.Graded += set.graded
				}
			}
		}
		if len(daySets) == 0 {
			continue
		}
		point.Verdict, _ = gptIQVerdict(daySets, revs, baselines, cfg)
		out = append(out, point)
	}
	return out
}
