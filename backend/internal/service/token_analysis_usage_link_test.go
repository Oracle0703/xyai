package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

func newTokenAnalysisLinkTestService(repo TokenAnalysisRepository) *TokenAnalysisService {
	return NewTokenAnalysisService(repo, &config.Config{TokenAnalysis: config.TokenAnalysisConfig{UsageMatchWindowSeconds: 10}}, nil)
}

// 有关联键但请求尚未完成时只做精确关联; 完成后才允许时序兜底。
func TestTokenAnalysisLinkUsageWaitsForCompletionBeforeTiming(t *testing.T) {
	start := time.Date(2026, 10, 24, 6, 11, 12, 0, time.UTC)
	done := start.Add(18 * time.Second)
	repo := &tokenAnalysisRepoStub{
		linkCandidates: []TokenAnalysisUsageLinkCandidate{
			{ID: 1, ArchiveID: "running", ClientRequestID: "cid-1", EventTime: start},
			{ID: 2, ArchiveID: "finished", ClientRequestID: "cid-2", EventTime: start, CompletedAt: &done},
			{ID: 3, ArchiveID: "legacy", EventTime: start},
		},
		findUsage: func(TokenAnalysisUsageLinkQuery) *TokenAnalysisUsageMatch { return nil },
	}

	require.NoError(t, newTokenAnalysisLinkTestService(repo).linkUsageRange(context.Background(), start, start.Add(time.Hour)))

	require.Len(t, repo.usageLookups, 3)
	require.True(t, repo.usageLookups[0].ExactOnly)
	require.False(t, repo.usageLookups[1].ExactOnly)
	require.Equal(t, done, *repo.usageLookups[1].CompletedAt)
	require.False(t, repo.usageLookups[2].ExactOnly)
	require.Equal(t, 10*time.Second, repo.usageLookups[2].Window)
	// 仍未关联且原本没有关联的行不写库, 下一轮重试。
	require.Empty(t, repo.links)
}

// 关联后按真实 usage 重算风险: 沿用索引时记录的重复数; 旧行未记录时现查并扣掉自身。
func TestTokenAnalysisLinkUsageRescoresRisk(t *testing.T) {
	start := time.Date(2026, 10, 24, 6, 11, 12, 0, time.UTC)
	stored := 3
	repo := &tokenAnalysisRepoStub{
		linkCandidates: []TokenAnalysisUsageLinkCandidate{
			{ID: 1, ArchiveID: "new", ClientRequestID: "cid", EventTime: start, SameBodyRecentCount: &stored},
			{ID: 2, ArchiveID: "legacy", EventTime: start},
		},
		findUsage: func(TokenAnalysisUsageLinkQuery) *TokenAnalysisUsageMatch {
			return &TokenAnalysisUsageMatch{UsageLogID: 9, Method: TokenAnalysisUsageMatchTiming, InputTokens: 156582, OutputTokens: 120}
		},
	}

	require.NoError(t, newTokenAnalysisLinkTestService(repo).linkUsageRange(context.Background(), start, start.Add(time.Hour)))

	codes := func(archiveID string) map[string]any {
		out := map[string]any{}
		for _, reason := range repo.linkRisk[archiveID] {
			out[reason.Code] = reason.Metrics
		}
		return out
	}
	require.Contains(t, codes("new"), TokenAnalysisRiskHugeInputTinyOutput)
	require.Equal(t, 3, codes("new")[TokenAnalysisRiskRepeatUncachedBody].(map[string]any)["same_body_recent_count"])
	// 桩 CountSameBodyRecent 返回 1(仅本行), 扣掉自身后为 0, 不触发重复原因。
	require.Contains(t, codes("legacy"), TokenAnalysisRiskHugeInputTinyOutput)
	require.NotContains(t, codes("legacy"), TokenAnalysisRiskRepeatUncachedBody)
}

// 旧窗口关联被改判后释放的账单, 由本轮已处理过的请求在下一轮认领。
func TestTokenAnalysisLinkUsageReleasedBillIsClaimedNextRound(t *testing.T) {
	start := time.Date(2026, 10, 24, 6, 11, 12, 0, time.UTC)
	held := int64(5)
	repo := &tokenAnalysisRepoStub{
		linkCandidates: []TokenAnalysisUsageLinkCandidate{
			{ID: 1, ArchiveID: "owner", EventTime: start.Add(-20 * time.Second)},
			{ID: 2, ArchiveID: "legacy", EventTime: start, UsageLogID: &held},
		},
	}
	repo.findUsage = func(q TokenAnalysisUsageLinkQuery) *TokenAnalysisUsageMatch {
		switch q.ArchiveID {
		case "legacy":
			return &TokenAnalysisUsageMatch{UsageLogID: 6, Method: TokenAnalysisUsageMatchTiming}
		case "owner":
			if link := repo.links["legacy"]; link != nil && link.UsageLogID == 6 {
				return &TokenAnalysisUsageMatch{UsageLogID: 5, Method: TokenAnalysisUsageMatchTiming}
			}
		}
		return nil
	}

	require.NoError(t, newTokenAnalysisLinkTestService(repo).linkUsageRange(context.Background(), start.Add(-time.Hour), start.Add(time.Hour)))

	require.Equal(t, int64(6), repo.links["legacy"].UsageLogID)
	require.Equal(t, int64(5), repo.links["owner"].UsageLogID)
}

// 旧关联在新规则下找不到账单时解除, 依赖 usage 的风险原因随之清除。
func TestTokenAnalysisLinkUsageUnlinksUnconfirmedLegacyLink(t *testing.T) {
	start := time.Date(2026, 10, 24, 6, 11, 12, 0, time.UTC)
	held := int64(5)
	stored := 0
	repo := &tokenAnalysisRepoStub{
		linkCandidates: []TokenAnalysisUsageLinkCandidate{{ID: 1, ArchiveID: "legacy", EventTime: start, UsageLogID: &held, SameBodyRecentCount: &stored}},
		findUsage:      func(TokenAnalysisUsageLinkQuery) *TokenAnalysisUsageMatch { return nil },
	}

	require.NoError(t, newTokenAnalysisLinkTestService(repo).linkUsageRange(context.Background(), start, start.Add(time.Hour)))

	link, ok := repo.links["legacy"]
	require.True(t, ok)
	require.Nil(t, link)
	require.Empty(t, repo.linkRisk["legacy"])
}

// 请求明细始终是完整归档视角, 并按关联状态区分已关联/未关联/失败/不计费。
func TestTokenAnalysisListRequestsShowsEveryArchivedRequestWithLinkStatus(t *testing.T) {
	usageID := int64(9)
	failed, ok := 502, 200
	repo := &tokenAnalysisRepoStub{requestItems: []TokenAnalysisRequestItem{
		{ArchiveID: "legacy", UsageLogID: &usageID},
		{ArchiveID: "exact", UsageLogID: &usageID, UsageMatchMethod: TokenAnalysisUsageMatchRequestID},
		{ArchiveID: "pending", Endpoint: "/v1/responses", ResponseStatus: &ok},
		{ArchiveID: "failed", Endpoint: "/v1/responses", ResponseStatus: &failed},
		{ArchiveID: "probe", Endpoint: "/v1/messages/count_tokens"},
	}}
	svc := newTokenAnalysisLinkTestService(repo)

	for _, include := range []bool{false, true} {
		items, _, err := svc.ListRequests(context.Background(), TokenAnalysisFilters{IncludeUnmatched: include}, pagination.PaginationParams{Page: 1, PageSize: 20})
		require.NoError(t, err)
		require.True(t, repo.requestFilters.IncludeUnmatched)
		got := map[string][2]string{}
		for _, item := range items {
			got[item.ArchiveID] = [2]string{item.UsageLinkStatus, item.UsageMatchMethod}
		}
		require.Equal(t, map[string][2]string{
			"legacy":  {TokenAnalysisUsageLinked, TokenAnalysisUsageMatchLegacyWindow},
			"exact":   {TokenAnalysisUsageLinked, TokenAnalysisUsageMatchRequestID},
			"pending": {TokenAnalysisUsageUnlinked, ""},
			"failed":  {TokenAnalysisUsageFailed, ""},
			"probe":   {TokenAnalysisUsageNotBillable, ""},
		}, got)
	}
}

func TestTokenAnalysisArchiveOutputTokens(t *testing.T) {
	require.Equal(t, int64(547), *tokenAnalysisArchiveOutputTokens(map[string]any{"input_tokens": 156582.0, "output_tokens": 547.0}))
	require.Equal(t, int64(12), *tokenAnalysisArchiveOutputTokens(map[string]any{"completion_tokens": 12.0}))
	require.Nil(t, tokenAnalysisArchiveOutputTokens(map[string]any{"candidatesTokenCount": 3.0}))
	require.Nil(t, tokenAnalysisArchiveOutputTokens(nil))
}

// 失败请求只接受精确证据: 旧行没有关联键时不查库, 有旧关联则解除; 有关联键时只做精确查找。
func TestTokenAnalysisLinkUsageFailedRowsUseOnlyExactEvidence(t *testing.T) {
	start := time.Date(2026, 10, 24, 6, 0, 0, 0, time.UTC)
	failed := 502
	held := int64(5)
	stored := 0
	repo := &tokenAnalysisRepoStub{
		linkCandidates: []TokenAnalysisUsageLinkCandidate{
			{ID: 1, ArchiveID: "failed-linked", EventTime: start, ResponseStatus: &failed, UsageLogID: &held, SameBodyRecentCount: &stored},
			{ID: 2, ArchiveID: "failed-unlinked", EventTime: start, ResponseStatus: &failed},
			{ID: 3, ArchiveID: "failed-keyed", ClientRequestID: "cid", EventTime: start, ResponseStatus: &failed, CompletedAt: &start},
		},
		findUsage: func(TokenAnalysisUsageLinkQuery) *TokenAnalysisUsageMatch { return nil },
	}

	require.NoError(t, newTokenAnalysisLinkTestService(repo).linkUsageRange(context.Background(), start, start.Add(time.Hour)))

	// 解除旧关联释放了账单, 会再跑一轮; 每次查库都只为有关联键的失败请求做精确查找。
	require.NotEmpty(t, repo.usageLookups)
	for _, q := range repo.usageLookups {
		require.Equal(t, "failed-keyed", q.ArchiveID)
		require.True(t, q.ExactOnly)
	}
	link, ok := repo.links["failed-linked"]
	require.True(t, ok)
	require.Nil(t, link)
	require.NotContains(t, repo.links, "failed-unlinked")
}

// 时序关联只在出现精确证据时改变; 被夺回账单的行找不到账单时解除并重算风险;
// 带上游 response.id 的新请求即使已完成也只做精确关联。
func TestTokenAnalysisLinkUsageWeakAndReleasedRows(t *testing.T) {
	start := time.Date(2026, 10, 24, 6, 0, 0, 0, time.UTC)
	held := int64(5)
	stored := 0
	exact := &TokenAnalysisUsageMatch{UsageLogID: 9, Method: TokenAnalysisUsageMatchRequestID}
	repo := &tokenAnalysisRepoStub{
		linkCandidates: []TokenAnalysisUsageLinkCandidate{
			{ID: 1, ArchiveID: "timing-no-evidence", ClientRequestID: "cid-1", EventTime: start, CompletedAt: &start, UsageLogID: &held, Method: TokenAnalysisUsageMatchTiming},
			{ID: 2, ArchiveID: "timing-corrected", ClientRequestID: "cid-2", EventTime: start, CompletedAt: &start, UsageLogID: &held, Method: TokenAnalysisUsageMatchTiming, SameBodyRecentCount: &stored},
			{ID: 3, ArchiveID: "released", EventTime: start, Method: TokenAnalysisUsageMatchReleased, SameBodyRecentCount: &stored},
			{ID: 4, ArchiveID: "ws", ClientRequestID: "cid-4", ResponseID: "resp_4", EventTime: start, CompletedAt: &start},
		},
		findUsage: func(q TokenAnalysisUsageLinkQuery) *TokenAnalysisUsageMatch {
			if q.ArchiveID == "timing-corrected" {
				return exact
			}
			return nil
		},
	}

	require.NoError(t, newTokenAnalysisLinkTestService(repo).linkUsageRange(context.Background(), start, start.Add(time.Hour)))

	exactOnly := map[string]bool{}
	for _, q := range repo.usageLookups {
		exactOnly[q.ArchiveID] = q.ExactOnly
	}
	require.Equal(t, map[string]bool{"timing-no-evidence": true, "timing-corrected": true, "released": false, "ws": true}, exactOnly)
	require.NotContains(t, repo.links, "timing-no-evidence")
	require.Equal(t, exact, repo.links["timing-corrected"])
	link, ok := repo.links["released"]
	require.True(t, ok)
	require.Nil(t, link)
	require.NotContains(t, repo.links, "ws")
}

// 完成事实变化(usage_link_pending)后时序关联按新事实完整复核; 复核后无需改动的
// 行只清除标记, 不写关联。
func TestTokenAnalysisLinkUsagePendingRowsAreReverified(t *testing.T) {
	start := time.Date(2026, 10, 24, 6, 0, 0, 0, time.UTC)
	failed := 502
	held := int64(5)
	stored := 0
	repo := &tokenAnalysisRepoStub{
		linkCandidates: []TokenAnalysisUsageLinkCandidate{
			{ID: 1, ArchiveID: "timing-now-failed", EventTime: start, ResponseStatus: &failed, UsageLogID: &held, Method: TokenAnalysisUsageMatchTiming, LinkPending: true, SameBodyRecentCount: &stored},
			{ID: 2, ArchiveID: "timing-reverified", EventTime: start, CompletedAt: &start, UsageLogID: &held, Method: TokenAnalysisUsageMatchTiming, LinkPending: true},
			{ID: 3, ArchiveID: "still-unlinked", ClientRequestID: "cid", ResponseID: "resp_3", EventTime: start, CompletedAt: &start, LinkPending: true},
		},
		findUsage: func(q TokenAnalysisUsageLinkQuery) *TokenAnalysisUsageMatch {
			if q.ArchiveID == "timing-reverified" {
				return &TokenAnalysisUsageMatch{UsageLogID: held, Method: TokenAnalysisUsageMatchTiming}
			}
			return nil
		},
	}

	require.NoError(t, newTokenAnalysisLinkTestService(repo).linkUsageRange(context.Background(), start, start.Add(time.Hour)))

	lookups := map[string]bool{}
	for _, q := range repo.usageLookups {
		lookups[q.ArchiveID] = q.ExactOnly
	}
	// 失败且无关联键: 不查库直接解除; 复核中的时序行允许重新做时序推断。
	require.Equal(t, map[string]bool{"timing-reverified": false, "still-unlinked": true}, lookups)
	link, ok := repo.links["timing-now-failed"]
	require.True(t, ok)
	require.Nil(t, link)
	require.NotContains(t, repo.links, "timing-reverified")
	require.ElementsMatch(t, []string{"timing-reverified", "still-unlinked"}, repo.pendingCleared)
}

// 等待晚到账单: 只有带关联键、未失败、未过截止时间的复核任务保留; 解除关联(重算
// 风险)时也不确认版本。截止后或无关联键的存量行一次查不到即确认。
func TestTokenAnalysisLinkUsageKeepsRetryingLateBillsUntilDeadline(t *testing.T) {
	start := time.Date(2026, 10, 24, 6, 0, 0, 0, time.UTC)
	future, past := time.Now().Add(time.Hour), time.Now().Add(-time.Hour)
	stored := 0
	repo := &tokenAnalysisRepoStub{
		linkCandidates: []TokenAnalysisUsageLinkCandidate{
			{ID: 1, ArchiveID: "awaiting", ClientRequestID: "cid-1", ResponseID: "resp_1", EventTime: start, CompletedAt: &start, LinkPending: true, LinkVersion: 2, RetryUntil: &future},
			{ID: 2, ArchiveID: "released-awaiting", ClientRequestID: "cid-2", EventTime: start, Method: TokenAnalysisUsageMatchReleased, LinkPending: true, LinkVersion: 3, RetryUntil: &future, SameBodyRecentCount: &stored},
			{ID: 3, ArchiveID: "expired", ClientRequestID: "cid-3", ResponseID: "resp_3", EventTime: start, CompletedAt: &start, LinkPending: true, LinkVersion: 4, RetryUntil: &past},
			{ID: 4, ArchiveID: "legacy", EventTime: start, LinkPending: true, LinkVersion: 5, RetryUntil: &future},
		},
		findUsage: func(TokenAnalysisUsageLinkQuery) *TokenAnalysisUsageMatch { return nil },
	}

	require.NoError(t, newTokenAnalysisLinkTestService(repo).linkUsageRange(context.Background(), start, start.Add(time.Hour)))

	require.ElementsMatch(t, []string{"expired", "legacy"}, repo.pendingCleared)
	require.Equal(t, []string{"released-awaiting"}, repo.keptPending)
	link, ok := repo.links["released-awaiting"]
	require.True(t, ok)
	require.Nil(t, link)
}
