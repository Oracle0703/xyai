//go:build integration

package repository

import (
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// humanInputRows 写入带来源与指纹的摘要行; hash 为空表示无指纹(存量行)。
type humanInputRows struct {
	f      *tokenAnalysisLinkFixture
	prefix string
	base   time.Time
}

func newHumanInputRows(t *testing.T) *humanInputRows {
	base := time.Date(2026, 10, 27, 2, 0, 0, 0, time.UTC)
	return &humanInputRows{f: newTokenAnalysisLinkFixture(t, base), prefix: fmt.Sprintf("ta-hi-%d-", time.Now().UnixNano()), base: base}
}

func (r *humanInputRows) add(id string, offset time.Duration, kind, hash string, mutate ...func(*service.TokenAnalysisRequestSummary)) {
	r.f.t.Helper()
	reason := map[string]string{"human": "user_input", "ai": "tool_result"}[kind]
	summary := &service.TokenAnalysisRequestSummary{
		ArchiveID: r.prefix + id, EventTime: r.base.Add(offset), UserID: &r.f.userID, APIKeyID: &r.f.apiKeyID,
		Model: tokenAnalysisLinkModel, Endpoint: "/v1/responses",
		SummaryJSON: map[string]any{"request_origin": map[string]any{"kind": kind, "reason": reason, "version": 2}},
		RiskReasons: []service.TokenAnalysisRiskReason{},
	}
	if hash != "" {
		summary.Fingerprint = service.TokenAnalysisFingerprint{State: service.TokenAnalysisFingerprintOK, Version: 77, PrefixItems: 1,
			PrefixEndHash: testFingerprintHash(hash), ItemsEndHash: testFingerprintHash(hash)}
	}
	for _, m := range mutate {
		m(summary)
	}
	requireTokenAnalysisUpsert(r.f.t, r.f.repo, r.f.ctx, summary)
}

func (r *humanInputRows) summaryCount(filters service.TokenAnalysisFilters) int64 {
	r.f.t.Helper()
	filters.UserID = &r.f.userID
	got, err := r.f.svc.GetSummary(r.f.ctx, filters)
	require.NoError(r.f.t, err)
	return got.HumanInputCount
}

func (r *humanInputRows) userCount(filters service.TokenAnalysisFilters) int64 {
	r.f.t.Helper()
	filters.UserID = &r.f.userID
	items, _, err := r.f.svc.ListUserUsage(r.f.ctx, filters, pagination.PaginationParams{Page: 1, PageSize: 20})
	require.NoError(r.f.t, err)
	require.Len(r.f.t, items, 1)
	return items[0].HumanInputCount
}

// 归并条件: 同 Key、同指纹版本、同 prefix_end_hash 的人工行, 与前一条相距 ≤15 分钟
// 时只计链首; 同时间戳按 archive_id 定序; 无指纹、无 Key、不同版本或不同 Key 逐条计;
// AI 行既不计数也不充当前驱。
func TestTokenAnalysisHumanInputCountMergesRepeatedPrompts(t *testing.T) {
	r := newHumanInputRows(t)
	otherKey := mustCreateApiKey(t, integrationEntClient, &service.APIKey{UserID: r.f.userID, Key: fmt.Sprintf("sk-%sother", r.prefix), Name: "other"})
	withKey := func(id *int64) func(*service.TokenAnalysisRequestSummary) {
		return func(s *service.TokenAnalysisRequestSummary) { s.APIKeyID = id }
	}
	withVersion := func(v int32) func(*service.TokenAnalysisRequestSummary) {
		return func(s *service.TokenAnalysisRequestSummary) { s.Fingerprint.Version = v }
	}

	// 0/5/14 分钟: 链式归并, 计 1。
	r.add("a1", 0, "human", "chain")
	r.add("a2", 5*time.Minute, "human", "chain")
	r.add("a3", 14*time.Minute, "human", "chain")
	// 恰好 15 分钟仍归并; 再隔 15 分 1 秒另起一次, 计 2。
	r.add("b1", 30*time.Minute, "human", "edge")
	r.add("b2", 45*time.Minute, "human", "edge")
	r.add("b3", 60*time.Minute+time.Second, "human", "edge")
	// 只有 AI 前驱的人工行仍计 1。
	r.add("c0", time.Minute, "ai", "after-ai")
	r.add("c1", 2*time.Minute, "human", "after-ai")
	// 无指纹逐条计, 计 2。
	r.add("d1", 0, "human", "")
	r.add("d2", time.Minute, "human", "")
	// 不同指纹版本, 计 2。
	r.add("e1", 0, "human", "versioned")
	r.add("e2", time.Minute, "human", "versioned", withVersion(78))
	// 同时间戳按 archive_id 定序, 计 1。
	r.add("f1", 3*time.Minute, "human", "same-time")
	r.add("f2", 3*time.Minute, "human", "same-time")
	// 不同 Key 与无 Key 都不合并, 计 2 + 2。
	r.add("g1", 0, "human", "keys")
	r.add("g2", time.Minute, "human", "keys", withKey(&otherKey.ID))
	r.add("h1", 0, "human", "no-key", withKey(nil))
	r.add("h2", time.Minute, "human", "no-key", withKey(nil))

	// 1 + 2 + 1 + 2 + 2 + 1 + 2 + 2
	require.Equal(t, int64(13), r.summaryCount(service.TokenAnalysisFilters{}))
	require.Equal(t, int64(0), r.summaryCount(service.TokenAnalysisFilters{Origin: "ai"}))
	require.Equal(t, int64(13), r.summaryCount(service.TokenAnalysisFilters{Origin: "human"}))
}

// 前驱只在当前筛选集合内查找(设计"筛选归属"): 时间范围外、未关联或其他项目的前驱
// 不参与, 后继在集合内即自成一次; 邮箱筛选走 p 的用户 JOIN, 结果不变。
func TestTokenAnalysisHumanInputCountFollowsFilters(t *testing.T) {
	r := newHumanInputRows(t)
	usageID := r.f.usage(r.prefix+"bill", r.base.Add(6*time.Minute), 1000, 10)
	billed := func(s *service.TokenAnalysisRequestSummary) { s.UsageLogID = &usageID }
	project := func(name string) func(*service.TokenAnalysisRequestSummary) {
		return func(s *service.TokenAnalysisRequestSummary) { s.ClientProject = name }
	}

	// 首发失败无账单, 5 分钟后成功有账单, 同一提示词。
	r.add("fail", 0, "human", "retry", project("demo"))
	r.add("ok", 5*time.Minute, "human", "retry", billed, project("demo"))
	// 前驱在时间范围之前。
	r.add("early", -5*time.Minute, "human", "boundary", project("demo"))
	r.add("late", 5*time.Minute, "human", "boundary", project("demo"))
	// 前驱属于另一个项目桶。
	r.add("x", 0, "human", "moved", project("x"))
	r.add("y", 5*time.Minute, "human", "moved", project("y"))

	start, end := r.base, r.base.Add(time.Hour)
	inRange := service.TokenAnalysisFilters{StartTime: &start, EndTime: &end}

	// 概览含未关联: retry 链 1、boundary 的 late 1、moved 链 1。
	require.Equal(t, int64(3), r.summaryCount(inRange))
	// 不限时间: boundary 链首 early 计 1, late 被归并。
	require.Equal(t, int64(3), r.summaryCount(service.TokenAnalysisFilters{}))
	// 排行默认排除未关联: 只剩 ok, 它的前驱不在集合内, 计 1。
	require.Equal(t, int64(1), r.userCount(inRange))
	withEmail := inRange
	withEmail.UserEmail = "ta-link-"
	require.Equal(t, int64(1), r.userCount(withEmail))
	withEmail.IncludeUnmatched = true
	require.Equal(t, int64(3), r.userCount(withEmail))

	// 项目排行把次数归到链首所在的桶; 只看 y 时 y 自成一次。
	all := inRange
	all.IncludeUnmatched = true
	items, _, err := r.f.svc.ListProjectUsage(r.f.ctx, all, pagination.PaginationParams{Page: 1, PageSize: 20})
	require.NoError(t, err)
	byProject := map[string]int64{}
	for _, item := range items {
		byProject[item.Project] = item.HumanInputCount
	}
	require.Equal(t, map[string]int64{"demo": 2, "x": 1, "y": 0}, byProject)
	all.Project = "y"
	items, _, err = r.f.svc.ListProjectUsage(r.f.ctx, all, pagination.PaginationParams{Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, int64(1), items[0].HumanInputCount)
}

// 概览与同期计费请求数应用同一邮箱条件(审核 F1): 部分命中只统计命中用户, 不命中为 0;
// 风险原因、人工输入次数子查询各自 JOIN users。
func TestTokenAnalysisSummaryAppliesEmailFilter(t *testing.T) {
	a, b := newHumanInputRows(t), newHumanInputRows(t)
	a.add("a1", 0, "human", "same-prompt", func(s *service.TokenAnalysisRequestSummary) {
		s.RiskScore = 10
		s.RiskReasons = []service.TokenAnalysisRiskReason{{Code: "huge_input_tiny_output", Score: 10}}
	})
	a.add("a2", time.Minute, "human", "same-prompt")
	b.add("b1", 0, "human", "same-prompt")
	a.f.usage(a.prefix+"bill", a.base.Add(time.Minute), 1000, 10)
	b.f.usage(b.prefix+"bill", b.base.Add(time.Minute), 1000, 10)
	emailOf := func(r *humanInputRows) string {
		var email string
		require.NoError(t, integrationDB.QueryRowContext(r.f.ctx, `SELECT email FROM users WHERE id = $1`, r.f.userID).Scan(&email))
		return email
	}
	start, end := a.base, a.base.Add(time.Hour)
	summary := func(email string) *service.TokenAnalysisSummary {
		got, err := a.f.svc.GetSummary(a.f.ctx, service.TokenAnalysisFilters{StartTime: &start, EndTime: &end, UserEmail: email})
		require.NoError(t, err)
		return got
	}

	gotA := summary(emailOf(a))
	require.Equal(t, [3]int64{2, 1, 1}, [3]int64{gotA.TotalRequests, gotA.HumanInputCount, gotA.BilledRequests})
	require.Len(t, gotA.RiskReasons, 1)
	gotB := summary(emailOf(b))
	require.Equal(t, [3]int64{1, 1, 1}, [3]int64{gotB.TotalRequests, gotB.HumanInputCount, gotB.BilledRequests})
	require.Empty(t, gotB.RiskReasons)
	none := summary("no-match-origin-v2@example.invalid")
	require.Equal(t, [3]int64{0, 0, 0}, [3]int64{none.TotalRequests, none.HumanInputCount, none.BilledRequests})
	require.Empty(t, none.RiskReasons)
}

// 空项目与真实名为 unattributed 的项目展示同名, 人工输入次数按原始项目值关联(审核 F2)。
func TestTokenAnalysisHumanInputCountKeepsRawProjectBuckets(t *testing.T) {
	r := newHumanInputRows(t)
	r.add("missing", 0, "human", "prompt-a")
	r.add("named", time.Minute, "human", "prompt-b", func(s *service.TokenAnalysisRequestSummary) { s.ClientProject = "unattributed" })

	items, _, err := r.f.svc.ListProjectUsage(r.f.ctx, service.TokenAnalysisFilters{UserID: &r.f.userID, IncludeUnmatched: true}, pagination.PaginationParams{Page: 1, PageSize: 20})

	require.NoError(t, err)
	require.Len(t, items, 2)
	var total int64
	for _, item := range items {
		require.Equal(t, "unattributed", item.Project)
		require.Equal(t, int64(1), item.RequestCount)
		require.Equal(t, int64(1), item.HumanInputCount)
		total += item.HumanInputCount
	}
	require.Equal(t, int64(2), total)
}
