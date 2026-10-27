//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// tokenAnalysisLinkFixture 为每个用例建独立用户/密钥, 用例间互不抢账单。
type tokenAnalysisLinkFixture struct {
	t         *testing.T
	ctx       context.Context
	repo      service.TokenAnalysisRepository
	svc       *service.TokenAnalysisService
	userID    int64
	apiKeyID  int64
	accountID int64
	day       time.Time
}

const tokenAnalysisLinkModel = "gpt-6-astra"

func newTokenAnalysisLinkFixture(t *testing.T, day time.Time) *tokenAnalysisLinkFixture {
	t.Helper()
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	user := mustCreateUser(t, integrationEntClient, &service.User{Email: fmt.Sprintf("ta-link-%d@example.com", suffix)})
	key := mustCreateApiKey(t, integrationEntClient, &service.APIKey{UserID: user.ID, Key: fmt.Sprintf("sk-ta-link-%d", suffix), Name: "cx"})
	account := mustCreateAccount(t, integrationEntClient, &service.Account{Name: fmt.Sprintf("ta-link-%d", suffix), Platform: service.PlatformOpenAI})
	repo := NewTokenAnalysisRepository(integrationDB)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM token_analysis_request_summaries WHERE user_id = $1`, user.ID)
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM usage_logs WHERE user_id = $1`, user.ID)
	})
	svc := service.NewTokenAnalysisService(repo, &config.Config{
		Gateway:       config.GatewayConfig{RequestArchive: config.GatewayRequestArchiveConfig{Dir: t.TempDir()}},
		TokenAnalysis: config.TokenAnalysisConfig{IndexEnabled: true, UsageMatchWindowSeconds: 10},
	}, nil)
	return &tokenAnalysisLinkFixture{t: t, ctx: ctx, repo: repo, svc: svc, userID: user.ID, apiKeyID: key.ID, accountID: account.ID, day: day}
}

// request 写入一条已索引请求(来源为人工, 与真实索引一致不带账单)。
func (f *tokenAnalysisLinkFixture) request(archiveID, clientRequestID string, start time.Time) {
	f.t.Helper()
	requireTokenAnalysisUpsert(f.t, f.repo, f.ctx, &service.TokenAnalysisRequestSummary{
		ArchiveID: archiveID, ClientRequestID: clientRequestID, EventTime: start,
		UserID: &f.userID, APIKeyID: &f.apiKeyID, Model: tokenAnalysisLinkModel, Endpoint: "/v1/responses", Method: "POST",
		SummaryJSON: map[string]any{
			"request_origin":         map[string]any{"kind": "human", "reason": "user_input", "confidence": "medium", "version": 1},
			"same_body_recent_count": 0,
		},
		RiskReasons: []service.TokenAnalysisRiskReason{},
	})
}

func (f *tokenAnalysisLinkFixture) response(archiveID string, completedAt time.Time, status int) {
	f.t.Helper()
	f.responseWithID(archiveID, completedAt, status, "")
}

// responseWithID 记录带上游 response.id 的完成事实(WS 上游以它记账)。
func (f *tokenAnalysisLinkFixture) responseWithID(archiveID string, completedAt time.Time, status int, responseID string) {
	f.t.Helper()
	require.NoError(f.t, f.repo.RecordResponse(f.ctx, service.TokenAnalysisResponseFacts{ArchiveID: archiveID, CompletedAt: completedAt, Status: status, ResponseID: responseID}))
}

// usage 写入一条账单: created_at 为落库时间, durationMs 为转发耗时。
func (f *tokenAnalysisLinkFixture) usage(requestID string, createdAt time.Time, durationMs, outputTokens int) int64 {
	f.t.Helper()
	row, err := integrationEntClient.UsageLog.Create().
		SetUserID(f.userID).SetAPIKeyID(f.apiKeyID).SetAccountID(f.accountID).
		SetRequestID(requestID).SetModel(tokenAnalysisLinkModel).
		SetInputTokens(294).SetOutputTokens(outputTokens).SetCacheReadTokens(156288).
		SetTotalCost(0.186578).SetActualCost(0.186578).
		SetDurationMs(durationMs).SetCreatedAt(createdAt).
		Save(f.ctx)
	require.NoError(f.t, err)
	return row.ID
}

func (f *tokenAnalysisLinkFixture) link() {
	f.t.Helper()
	day := f.day.Format("2006-01-02")
	_, err := f.svc.IndexRange(f.ctx, service.TokenAnalysisIndexRequest{StartDate: day, EndDate: day, Timezone: "UTC"})
	require.NoError(f.t, err)
}

func (f *tokenAnalysisLinkFixture) linked(archiveID string) (usageLogID *int64, method string) {
	f.t.Helper()
	require.NoError(f.t, integrationDB.QueryRowContext(f.ctx,
		`SELECT usage_log_id, usage_match_method FROM token_analysis_request_summaries WHERE archive_id = $1`, archiveID,
	).Scan(&usageLogID, &method))
	return usageLogID, method
}

func (f *tokenAnalysisLinkFixture) requireLinked(archiveID string, usageLogID int64, method string) {
	f.t.Helper()
	got, gotMethod := f.linked(archiveID)
	require.NotNil(f.t, got, archiveID)
	require.Equal(f.t, usageLogID, *got, archiveID)
	require.Equal(f.t, method, gotMethod, archiveID)
}

func (f *tokenAnalysisLinkFixture) requireUnlinked(archiveID string) {
	f.t.Helper()
	got, method := f.linked(archiveID)
	require.Nil(f.t, got, archiveID)
	require.Empty(f.t, method, archiveID)
}

// state 读取一行当前的关联状态与复核版本, 即索引实例查找前会读到的值。
func (f *tokenAnalysisLinkFixture) state(archiveID string) service.TokenAnalysisUsageLinkState {
	f.t.Helper()
	var state service.TokenAnalysisUsageLinkState
	require.NoError(f.t, integrationDB.QueryRowContext(f.ctx,
		`SELECT usage_log_id, usage_match_method, usage_link_version FROM token_analysis_request_summaries WHERE archive_id = $1`, archiveID,
	).Scan(&state.UsageLogID, &state.Method, &state.Version))
	return state
}

func (f *tokenAnalysisLinkFixture) recheckPending(archiveID string) bool {
	f.t.Helper()
	var pending bool
	require.NoError(f.t, integrationDB.QueryRowContext(f.ctx,
		`SELECT usage_link_version <> usage_link_checked_version FROM token_analysis_request_summaries WHERE archive_id = $1`, archiveID,
	).Scan(&pending))
	return pending
}

func (f *tokenAnalysisLinkFixture) requireNoSharedUsage() {
	f.t.Helper()
	var shared int
	require.NoError(f.t, integrationDB.QueryRowContext(f.ctx, `
SELECT COUNT(*) FROM (
    SELECT usage_log_id FROM token_analysis_request_summaries
    WHERE user_id = $1 AND usage_log_id IS NOT NULL
    GROUP BY usage_log_id HAVING COUNT(*) > 1
) dup`, f.userID).Scan(&shared))
	require.Zero(f.t, shared, "a usage log must not be counted by two requests")
}

func (f *tokenAnalysisLinkFixture) listRequests(includeUnmatched bool) map[string]service.TokenAnalysisRequestItem {
	f.t.Helper()
	start, end := f.day, f.day.AddDate(0, 0, 1)
	items, _, err := f.svc.ListRequests(f.ctx, service.TokenAnalysisFilters{
		StartTime: &start, EndTime: &end, UserID: &f.userID, IncludeUnmatched: includeUnmatched,
	}, pagination.PaginationParams{Page: 1, PageSize: 50})
	require.NoError(f.t, err)
	byID := make(map[string]service.TokenAnalysisRequestItem, len(items))
	for _, item := range items {
		byID[item.ArchiveID] = item
	}
	return byID
}

// 生产缺陷复现(archive 73eb6d29...): 18.5 秒的人工首发, 账单在 14:11:31 落库;
// 旧规则只查开始时间 ±10 秒, 会漏掉它或误取前一请求刚落库的账单。存量行没有
// 关联键和完成时间, 只能按推算的转发开始时间关联。
func TestTokenAnalysisUsageLinkLongRunningLegacyRequest(t *testing.T) {
	day := time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
	f := newTokenAnalysisLinkFixture(t, day)
	at := func(clock string) time.Time {
		ts, err := time.Parse(time.RFC3339Nano, "2026-10-24T"+clock+"Z")
		require.NoError(t, err)
		return ts
	}

	f.request("prev", "", at("06:10:50"))
	prevUsage := f.usage("client:prev", at("06:11:12.300"), 22000, 120)
	f.request("target", "", at("06:11:12.654557512"))
	targetUsage := f.usage("client:target", at("06:11:31.202047834"), 18410, 547)
	f.request("next", "", at("06:11:31.5"))
	nextUsage := f.usage("client:next", at("06:11:40"), 8000, 80)
	// 旧规则的结果: target 取了时间最近的上一请求账单。
	_, err := integrationDB.ExecContext(f.ctx, `UPDATE token_analysis_request_summaries SET usage_log_id = $1 WHERE archive_id = 'target'`, prevUsage)
	require.NoError(t, err)

	f.link()

	f.requireLinked("target", targetUsage, service.TokenAnalysisUsageMatchTiming)
	f.requireLinked("prev", prevUsage, service.TokenAnalysisUsageMatchTiming)
	f.requireLinked("next", nextUsage, service.TokenAnalysisUsageMatchTiming)
	f.requireNoSharedUsage()

	// 勾选与否请求明细都展示这条已计费人工请求, 用量来自真实账单。
	for _, include := range []bool{false, true} {
		item, ok := f.listRequests(include)["target"]
		require.True(t, ok, "include_unmatched=%v", include)
		require.Equal(t, service.TokenAnalysisUsageLinked, item.UsageLinkStatus)
		require.Equal(t, "human", item.SummaryJSON["request_origin"].(map[string]any)["kind"])
		require.EqualValues(t, 294, item.InputTokens)
		require.EqualValues(t, 547, item.OutputTokens)
		require.InDelta(t, 0.186578, item.ActualCost, 1e-9)
	}
}

// 新归档带 client_request_id: 首轮账单未落库保持未关联且不抢相邻请求的账单,
// 账单出现后下一轮按 request_id 精确关联; 重复运行不改变结果。
func TestTokenAnalysisUsageLinkLateBillingByRequestID(t *testing.T) {
	day := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	f := newTokenAnalysisLinkFixture(t, day)
	start := day.Add(6 * time.Hour)
	f.request("late", "cid-late", start)
	f.response("late", start.Add(18*time.Second), 200)
	neighbor := f.usage("client:cid-neighbor", start.Add(18100*time.Millisecond), 17000, 547)

	f.link()
	f.requireUnlinked("late")
	item := f.listRequests(false)["late"]
	require.Equal(t, service.TokenAnalysisUsageUnlinked, item.UsageLinkStatus)
	require.Zero(t, item.ActualCost)

	late := f.usage("client:cid-late", start.Add(25*time.Second), 17900, 547)
	f.link()
	f.link()
	f.requireLinked("late", late, service.TokenAnalysisUsageMatchRequestID)
	var claimed int
	require.NoError(t, integrationDB.QueryRowContext(f.ctx, `SELECT COUNT(*) FROM token_analysis_request_summaries WHERE usage_log_id = $1`, neighbor).Scan(&claimed))
	require.Zero(t, claimed)
}

// 同用户/密钥/模型并发与重试: 失败请求不关联; 客户端重试按各自关联键;
// WS 上游 ID 记账的并发请求按完成时间与耗时约束各取各的账单。
func TestTokenAnalysisUsageLinkConcurrentAndRetriedRequests(t *testing.T) {
	day := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	f := newTokenAnalysisLinkFixture(t, day)
	t0 := day.Add(8 * time.Hour)
	ms := func(n int) time.Time { return t0.Add(time.Duration(n) * time.Millisecond) }

	f.request("failed", "cid-a", ms(0))
	f.response("failed", ms(900), 502)
	f.request("retry", "cid-b", ms(1000))
	f.response("retry", ms(20000), 200)
	retryUsage := f.usage("client:cid-b", ms(20010), 18900, 300)
	f.request("ws-short", "cid-c", ms(1200))
	f.response("ws-short", ms(5000), 200)
	shortUsage := f.usage("resp_short", ms(5050), 3800, 40)
	f.request("ws-long", "cid-d", ms(1300))
	f.response("ws-long", ms(25000), 200)
	longUsage := f.usage("resp_long", ms(25020), 23600, 900)

	f.link()
	f.link()

	f.requireUnlinked("failed")
	f.requireLinked("retry", retryUsage, service.TokenAnalysisUsageMatchRequestID)
	f.requireLinked("ws-short", shortUsage, service.TokenAnalysisUsageMatchTiming)
	f.requireLinked("ws-long", longUsage, service.TokenAnalysisUsageMatchTiming)
	f.requireNoSharedUsage()
	require.Equal(t, service.TokenAnalysisUsageFailed, f.listRequests(false)["failed"].UsageLinkStatus)
}

// 存量行只有开始时间: "首发失败后 2 秒重试"与"首发的计费任务排队 2 秒"在时序上
// 无法区分(推算转发开始都落在重试之后), 两条请求都能解释这笔账单, 保持未关联。
func TestTokenAnalysisUsageLinkLegacyAmbiguousBillStaysUnlinked(t *testing.T) {
	day := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	f := newTokenAnalysisLinkFixture(t, day)
	t0 := day.Add(9 * time.Hour)
	f.request("first", "", t0)
	f.request("again", "", t0.Add(2100*time.Millisecond))
	f.usage("client:legacy-retry", t0.Add(20*time.Second), 17800, 547)
	f.request("queued-first", "", t0.Add(time.Hour))
	f.request("queued-next", "", t0.Add(time.Hour+2100*time.Millisecond))
	f.usage("client:legacy-queued", t0.Add(time.Hour+4*time.Second), 1000, 547)

	f.link()

	for _, archiveID := range []string{"first", "again", "queued-first", "queued-next"} {
		f.requireUnlinked(archiveID)
	}
}

// 新请求的精确账单晚到: 首轮只有 WS 邻居的账单, 两条请求都能解释它, 不猜测;
// 精确账单到达后按 request_id 关联, 邻居随之成为唯一归属。
func TestTokenAnalysisUsageLinkLateExactBillBeatsNeighborTiming(t *testing.T) {
	day := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	f := newTokenAnalysisLinkFixture(t, day)
	start := day.Add(6 * time.Hour)
	f.request("late", "cid-late", start)
	f.response("late", start.Add(18*time.Second), 200)
	f.request("ws", "cid-ws", start.Add(time.Second))
	f.response("ws", start.Add(18100*time.Millisecond), 200)
	neighbor := f.usage("resp_neighbor", start.Add(18100*time.Millisecond), 17000, 547)

	f.link()
	f.requireUnlinked("late")
	f.requireUnlinked("ws")

	own := f.usage("client:cid-late", start.Add(25*time.Second), 17900, 547)
	f.link()
	f.requireLinked("late", own, service.TokenAnalysisUsageMatchRequestID)
	f.requireLinked("ws", neighbor, service.TokenAnalysisUsageMatchTiming)
	f.requireNoSharedUsage()
}

// 时序兜底不能占用另一条带关联键请求的精确账单, 即使旧行先被处理、且只看开始
// 时间时这笔账单对旧行也合理。
func TestTokenAnalysisUsageLinkLegacyCannotTakeExactOwnersBill(t *testing.T) {
	day := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	f := newTokenAnalysisLinkFixture(t, day)
	start := day.Add(6 * time.Hour)
	f.request("legacy", "", start)
	f.request("exact", "cid-exact", start.Add(time.Second))
	f.response("exact", start.Add(5*time.Second), 200)
	own := f.usage("client:cid-exact", start.Add(5010*time.Millisecond), 4000, 547)
	match, err := f.repo.FindUsageLink(f.ctx, service.TokenAnalysisUsageLinkQuery{
		ArchiveID: "legacy", StartedAt: start, UserID: &f.userID, APIKeyID: &f.apiKeyID, Model: tokenAnalysisLinkModel, Window: 10 * time.Second,
	})
	require.NoError(t, err)
	require.Nil(t, match)

	f.link()

	f.requireUnlinked("legacy")
	f.requireLinked("exact", own, service.TokenAnalysisUsageMatchRequestID)
}

// WS 上游以 response.id 记账: 归档捕获的 response.id 直接精确关联。之前按时序
// 占用该账单的旧行被夺回后重新关联(此处无其他账单, 解除关联并重算风险)。
func TestTokenAnalysisUsageLinkResponseIDCorrectsTimingClaim(t *testing.T) {
	day := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
	f := newTokenAnalysisLinkFixture(t, day)
	start := day.Add(6 * time.Hour)
	f.request("legacy", "", start)
	f.response("legacy", start.Add(20*time.Second), 200)
	bill := f.usage("resp_ws_owner", start.Add(20100*time.Millisecond), 19000, 80)
	f.link()
	f.requireLinked("legacy", bill, service.TokenAnalysisUsageMatchTiming)

	f.request("owner", "cid-owner", start.Add(500*time.Millisecond))
	f.responseWithID("owner", start.Add(20*time.Second), 200, "resp_ws_owner")
	f.link()

	f.requireLinked("owner", bill, service.TokenAnalysisUsageMatchRequestID)
	f.requireUnlinked("legacy")
	f.requireNoSharedUsage()
}

// 多个索引实例并发认领同一账单: 认领检查与写入在按账单加锁的事务内完成。
// 测试持有 race-a 的行锁, 让它在检查之后、提交之前停住, 再让 race-b 认领:
// 没有账单锁时 race-b 看不到未提交的认领, 两条都会写入同一账单。
func TestTokenAnalysisUsageLinkConcurrentClaimsAreAtomic(t *testing.T) {
	day := time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)
	f := newTokenAnalysisLinkFixture(t, day)
	start := day.Add(6 * time.Hour)
	bill := f.usage("resp_race", start.Add(time.Second), 900, 547)
	f.request("race-a", "", start)
	f.request("race-b", "", start.Add(50*time.Millisecond))

	hold, err := integrationDB.BeginTx(f.ctx, nil)
	require.NoError(t, err)
	defer func() { _ = hold.Rollback() }()
	_, err = hold.ExecContext(f.ctx, `SELECT 1 FROM token_analysis_request_summaries WHERE archive_id = 'race-a' FOR UPDATE`)
	require.NoError(t, err)

	type claimResult struct {
		write service.TokenAnalysisUsageLinkWrite
		err   error
	}
	claim := func(archiveID string) <-chan claimResult {
		done := make(chan claimResult, 1)
		go func() {
			write, err := f.repo.SetUsageLink(f.ctx, archiveID, service.TokenAnalysisUsageLinkState{}, &service.TokenAnalysisUsageMatch{
				UsageLogID: bill, MatchConfidence: 3, Method: service.TokenAnalysisUsageMatchTiming,
			}, 0, []service.TokenAnalysisRiskReason{})
			done <- claimResult{write, err}
		}()
		return done
	}
	first := claim("race-a")
	time.Sleep(300 * time.Millisecond)
	second := claim("race-b")
	time.Sleep(300 * time.Millisecond)
	require.NoError(t, hold.Rollback())

	a, b := <-first, <-second
	require.NoError(t, a.err)
	require.NoError(t, b.err)
	require.True(t, a.write.Applied)
	require.False(t, b.write.Applied)
	f.requireNoSharedUsage()
}

// 跨日请求(D 日 23:59:59 开始, D+1 日完成, WS 账单按 response.id)无论按什么顺序
// 回填都能关联: 逆序时 response 先暂存后合并; 升序分日索引时, 写回完成事实的
// D 日行被标记重新关联, 在只索引 D+1 的那一轮里也会处理; 也可一次选两日。
func TestTokenAnalysisUsageLinkCrossDayRequestInAnyIndexOrder(t *testing.T) {
	orders := map[string][][2]string{
		"descending": {{"D+1", "D+1"}, {"D", "D"}},
		"ascending":  {{"D", "D"}, {"D+1", "D+1"}},
		"full range": {{"D", "D+1"}},
	}
	offset := 0
	for name, runs := range orders {
		offset++
		t.Run(name, func(t *testing.T) {
			day := time.Date(2026, 11, 20+2*offset, 0, 0, 0, 0, time.UTC)
			f := newTokenAnalysisLinkFixture(t, day)
			dir := t.TempDir()
			f.svc = service.NewTokenAnalysisService(f.repo, &config.Config{
				Gateway:       config.GatewayConfig{RequestArchive: config.GatewayRequestArchiveConfig{Dir: dir}},
				TokenAnalysis: config.TokenAnalysisConfig{IndexEnabled: true, UsageMatchWindowSeconds: 10},
			}, nil)
			start := day.Add(24*time.Hour - time.Second)
			done := start.Add(21 * time.Second)
			dates := map[string]string{"D": day.Format("2006-01-02"), "D+1": day.AddDate(0, 0, 1).Format("2006-01-02")}
			write := func(date string, event map[string]any) {
				data, err := json.Marshal(event)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(filepath.Join(dir, dates[date]+".jsonl"), append(data, '\n'), 0o600))
			}
			write("D", map[string]any{
				"archive_id": "midnight", "client_request_id": "cid-midnight", "event": "request",
				"timestamp": start.Format(time.RFC3339Nano), "endpoint": "/v1/responses", "path": "/v1/responses",
				"user_id": f.userID, "api_key_id": f.apiKeyID, "model": tokenAnalysisLinkModel,
				"body": `{"model":"gpt-6-astra","input":"human question"}`,
			})
			write("D+1", map[string]any{
				"archive_id": "midnight", "event": "response", "timestamp": done.Format(time.RFC3339Nano), "status": 200, "response_id": "resp_midnight",
			})
			bill := f.usage("resp_midnight", done.Add(10*time.Millisecond), 21000, 547)
			for _, run := range runs {
				_, err := f.svc.IndexRange(f.ctx, service.TokenAnalysisIndexRequest{StartDate: dates[run[0]], EndDate: dates[run[1]], Timezone: "UTC"})
				require.NoError(t, err)
			}

			f.requireLinked("midnight", bill, service.TokenAnalysisUsageMatchRequestID)
			var pending int
			require.NoError(t, integrationDB.QueryRowContext(f.ctx, `SELECT COUNT(*) FROM token_analysis_pending_responses WHERE archive_id = 'midnight'`).Scan(&pending))
			require.Zero(t, pending)
			require.False(t, f.recheckPending("midnight"))
		})
	}
}

// 晚到的失败响应改变了完成事实: 之前按时序关联的账单没有精确证据支撑, 解除关联;
// 响应同时带回 response.id 时也只按它精确查找。
func TestTokenAnalysisUsageLinkLateFailedResponseClearsTimingLink(t *testing.T) {
	for i, responseID := range []string{"", "resp_failed_owner"} {
		t.Run("response_id="+responseID, func(t *testing.T) {
			day := time.Date(2026, 11, 11+i, 0, 0, 0, 0, time.UTC)
			f := newTokenAnalysisLinkFixture(t, day)
			start := day.Add(6 * time.Hour)
			f.request("late-failed", "", start)
			other := f.usage("client:unrelated", start.Add(time.Second), 900, 547)
			f.link()
			f.requireLinked("late-failed", other, service.TokenAnalysisUsageMatchTiming)

			f.responseWithID("late-failed", start.Add(2*time.Second), 502, responseID)
			f.link()

			f.requireUnlinked("late-failed")
		})
	}
}

// 多个索引实例交错: 写入按查找前读到的状态条件更新, 陈旧的时序结果或解除结果
// 不能覆盖另一实例刚写入的精确关联。
func TestTokenAnalysisUsageLinkStaleWritesCannotOverrideNewerLink(t *testing.T) {
	day := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
	f := newTokenAnalysisLinkFixture(t, day)
	start := day.Add(6 * time.Hour)
	done := start.Add(2 * time.Second)
	query := func(archiveID, cid string) service.TokenAnalysisUsageLinkQuery {
		return service.TokenAnalysisUsageLinkQuery{ArchiveID: archiveID, ClientRequestID: cid, StartedAt: start, CompletedAt: &done,
			UserID: &f.userID, APIKeyID: &f.apiKeyID, Model: tokenAnalysisLinkModel, Window: 10 * time.Second}
	}
	none := []service.TokenAnalysisRiskReason{}

	// A 读到未关联并找到 WS 邻居账单(时序), 尚未写入; 自身账单到达后 B 精确关联。
	f.request("stale-timing", "cid-stale-timing", start)
	f.response("stale-timing", done, 200)
	read := f.state("stale-timing")
	neighbor := f.usage("resp_stale_neighbor", done.Add(10*time.Millisecond), 1900, 547)
	staleMatch, err := f.repo.FindUsageLink(f.ctx, query("stale-timing", "cid-stale-timing"))
	require.NoError(t, err)
	require.Equal(t, neighbor, staleMatch.UsageLogID)
	own := f.usage("client:cid-stale-timing", done.Add(time.Second), 1900, 547)
	exact, err := f.repo.FindUsageLink(f.ctx, query("stale-timing", "cid-stale-timing"))
	require.NoError(t, err)
	_, err = f.repo.SetUsageLink(f.ctx, "stale-timing", read, exact, 0, none)
	require.NoError(t, err)
	write, err := f.repo.SetUsageLink(f.ctx, "stale-timing", read, staleMatch, 0, none)
	require.NoError(t, err)
	require.False(t, write.Applied)
	f.requireLinked("stale-timing", own, service.TokenAnalysisUsageMatchRequestID)

	// A 复核旧窗口错链得到"解除", 尚未写入; B 按新到的精确账单修好同一行。
	f.request("stale-clear", "cid-stale-clear", start)
	f.response("stale-clear", done, 200)
	wrong := f.usage("client:unrelated-stale", start.Add(-time.Hour), 900, 547)
	_, err = integrationDB.ExecContext(f.ctx, `UPDATE token_analysis_request_summaries SET usage_log_id = $1 WHERE archive_id = 'stale-clear'`, wrong)
	require.NoError(t, err)
	read = f.state("stale-clear")
	fixed := f.usage("client:cid-stale-clear", done.Add(time.Second), 1900, 547)
	exact, err = f.repo.FindUsageLink(f.ctx, query("stale-clear", "cid-stale-clear"))
	require.NoError(t, err)
	_, err = f.repo.SetUsageLink(f.ctx, "stale-clear", read, exact, 0, none)
	require.NoError(t, err)
	write, err = f.repo.SetUsageLink(f.ctx, "stale-clear", read, nil, 0, none)
	require.NoError(t, err)
	require.False(t, write.Applied)
	f.requireLinked("stale-clear", fixed, service.TokenAnalysisUsageMatchRequestID)
}

// 前一日的旧行按时序占用了次日账单; 只索引次日时真正的请求精确夺回账单, 前一日
// 行在同一次索引里解除关联并重算风险: 账单带来的风险消失, body 自身的风险保留。
func TestTokenAnalysisUsageLinkDisplacedRowOnOtherDayIsRescored(t *testing.T) {
	day := time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)
	f := newTokenAnalysisLinkFixture(t, day)
	start := day.Add(-time.Second)
	done := day.Add(20 * time.Second)
	f.request("previous-day", "", start)
	_, err := integrationDB.ExecContext(f.ctx, `UPDATE token_analysis_request_summaries SET system_chars = 100, user_chars = 10 WHERE archive_id = 'previous-day'`)
	require.NoError(t, err)
	f.response("previous-day", done, 200)
	bill := f.usage("resp_risk_owner", done.Add(10*time.Millisecond), 20000, 80)
	_, err = integrationDB.ExecContext(f.ctx, `UPDATE usage_logs SET input_tokens = 100000, cache_read_tokens = 0 WHERE id = $1`, bill)
	require.NoError(t, err)
	previous := day.AddDate(0, 0, -1).Format("2006-01-02")
	_, err = f.svc.IndexRange(f.ctx, service.TokenAnalysisIndexRequest{StartDate: previous, EndDate: previous, Timezone: "UTC"})
	require.NoError(t, err)
	f.requireLinked("previous-day", bill, service.TokenAnalysisUsageMatchTiming)
	risk := func() (int, string) {
		var score int
		var reasons string
		require.NoError(t, integrationDB.QueryRowContext(f.ctx, `SELECT risk_score, risk_reasons::text FROM token_analysis_request_summaries WHERE archive_id = 'previous-day'`).Scan(&score, &reasons))
		return score, reasons
	}
	score, _ := risk()
	require.Equal(t, 90, score)

	f.request("owner", "cid-risk-owner", day)
	f.responseWithID("owner", done, 200, "resp_risk_owner")
	f.link()

	f.requireLinked("owner", bill, service.TokenAnalysisUsageMatchRequestID)
	f.requireUnlinked("previous-day")
	score, reasons := risk()
	require.Equal(t, 20, score)
	require.Contains(t, reasons, service.TokenAnalysisRiskOversizedSystemPrompt)
	require.NotContains(t, reasons, service.TokenAnalysisRiskHugeInputTinyOutput)
}

// 旧失败请求的错误关联被清除; 失败请求若有精确账单(真实计费证据)仍关联。
func TestTokenAnalysisUsageLinkFailedRequestsKeepOnlyExactBills(t *testing.T) {
	day := time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)
	f := newTokenAnalysisLinkFixture(t, day)
	start := day.Add(6 * time.Hour)
	f.request("failed-old", "", start)
	unrelated := f.usage("client:other", start.Add(time.Second), 900, 547)
	_, err := integrationDB.ExecContext(f.ctx, `UPDATE token_analysis_request_summaries SET usage_log_id = $1 WHERE archive_id = 'failed-old'`, unrelated)
	require.NoError(t, err)
	f.response("failed-old", start.Add(2*time.Second), 502)
	f.request("failed-billed", "cid-failed", start.Add(time.Minute))
	f.response("failed-billed", start.Add(time.Minute+3*time.Second), 499)
	billed := f.usage("client:cid-failed", start.Add(time.Minute+3*time.Second), 2500, 12)

	f.link()

	f.requireUnlinked("failed-old")
	f.requireLinked("failed-billed", billed, service.TokenAnalysisUsageMatchRequestID)
}

// 来源筛选作用于 summary_json.request_origin.kind; "未识别"同时包含尚未写入来源的旧行。
func TestTokenAnalysisListRequestsFiltersByOrigin(t *testing.T) {
	day := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	f := newTokenAnalysisLinkFixture(t, day)
	upsert := func(archiveID string, summary map[string]any) {
		requireTokenAnalysisUpsert(t, f.repo, f.ctx, &service.TokenAnalysisRequestSummary{
			ArchiveID: archiveID, EventTime: day.Add(time.Hour), UserID: &f.userID, APIKeyID: &f.apiKeyID,
			Model: tokenAnalysisLinkModel, Endpoint: "/v1/responses", SummaryJSON: summary, RiskReasons: []service.TokenAnalysisRiskReason{},
		})
	}
	upsert("human", map[string]any{"request_origin": map[string]any{"kind": "human", "reason": "user_input", "confidence": "medium", "version": 1}})
	upsert("ai", map[string]any{"request_origin": map[string]any{"kind": "ai", "reason": "tool_result", "confidence": "high", "version": 1}})
	upsert("unknown", map[string]any{"request_origin": map[string]any{"kind": "unknown", "reason": "body_truncated", "confidence": "none", "version": 1}})
	upsert("legacy", map[string]any{"shape": "responses"})

	start, end := day, day.AddDate(0, 0, 1)
	for origin, want := range map[string][]string{"human": {"human"}, "ai": {"ai"}, "unknown": {"legacy", "unknown"}, "": {"ai", "human", "legacy", "unknown"}} {
		items, _, err := f.svc.ListRequests(f.ctx, service.TokenAnalysisFilters{StartTime: &start, EndTime: &end, UserID: &f.userID, Origin: origin}, pagination.PaginationParams{Page: 1, PageSize: 50})
		require.NoError(t, err)
		got := make([]string, 0, len(items))
		for _, item := range items {
			got = append(got, item.ArchiveID)
		}
		require.ElementsMatch(t, want, got, "origin=%q", origin)
	}
}

// 带关联键的跨日请求: response 已写回但账单尚未落库时保留复核任务, 账单晚到后即使
// 只索引非请求日也能关联; 账单一直不来的请求在等待截止后确认为未关联, 不再重试。
func TestTokenAnalysisUsageLinkLateBillIsRetriedUntilDeadline(t *testing.T) {
	day := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	f := newTokenAnalysisLinkFixture(t, day)
	dir := t.TempDir()
	f.svc = service.NewTokenAnalysisService(f.repo, &config.Config{
		Gateway:       config.GatewayConfig{RequestArchive: config.GatewayRequestArchiveConfig{Dir: dir}},
		TokenAnalysis: config.TokenAnalysisConfig{IndexEnabled: true, UsageMatchWindowSeconds: 10},
	}, nil)
	start := day.Add(24*time.Hour - time.Second)
	done := start.Add(21 * time.Second)
	lines := map[string][]map[string]any{
		"2026-10-10": {
			{"archive_id": "late-bill", "client_request_id": "cid-late-bill", "event": "request", "timestamp": start.Format(time.RFC3339Nano),
				"endpoint": "/v1/responses", "path": "/v1/responses", "user_id": f.userID, "api_key_id": f.apiKeyID, "model": tokenAnalysisLinkModel,
				"body": `{"model":"gpt-6-astra","input":"human question"}`},
			{"archive_id": "never-billed", "client_request_id": "cid-never-billed", "event": "request", "timestamp": start.Add(-time.Hour).Format(time.RFC3339Nano),
				"endpoint": "/v1/responses", "path": "/v1/responses", "user_id": f.userID, "api_key_id": f.apiKeyID, "model": tokenAnalysisLinkModel,
				"body": `{"model":"gpt-6-astra","input":"other question"}`},
		},
		"2026-10-11": {
			{"archive_id": "late-bill", "event": "response", "timestamp": done.Format(time.RFC3339Nano), "status": 200, "response_id": "resp_late_bill"},
			{"archive_id": "never-billed", "event": "response", "timestamp": done.Format(time.RFC3339Nano), "status": 200, "response_id": "resp_never_billed"},
		},
	}
	for date, events := range lines {
		var data []byte
		for _, event := range events {
			line, err := json.Marshal(event)
			require.NoError(t, err)
			data = append(append(data, line...), '\n')
		}
		require.NoError(t, os.WriteFile(filepath.Join(dir, date+".jsonl"), data, 0o600))
	}
	index := func(date string) {
		_, err := f.svc.IndexRange(f.ctx, service.TokenAnalysisIndexRequest{StartDate: date, EndDate: date, Timezone: "UTC"})
		require.NoError(t, err)
	}
	index("2026-10-10")
	index("2026-10-11")
	f.requireUnlinked("late-bill")
	require.True(t, f.recheckPending("late-bill"))

	bill := f.usage("resp_late_bill", done.Add(10*time.Second), 21000, 547)
	index("2026-10-24")
	f.requireLinked("late-bill", bill, service.TokenAnalysisUsageMatchRequestID)
	require.False(t, f.recheckPending("late-bill"))

	require.True(t, f.recheckPending("never-billed"))
	_, err := integrationDB.ExecContext(f.ctx, `UPDATE token_analysis_request_summaries SET usage_link_retry_until = NOW() - INTERVAL '1 second' WHERE archive_id = 'never-billed'`)
	require.NoError(t, err)
	index("2026-10-24")
	f.requireUnlinked("never-billed")
	require.False(t, f.recheckPending("never-billed"))
}

// 复核版本: 查找之后才登记的复核(首次失败响应、账单被夺回)使陈旧结果失效; 陈旧的
// 时序写入被拒绝, 陈旧的确认也不能吞掉新任务。
func TestTokenAnalysisUsageLinkNewRecheckRejectsStaleResults(t *testing.T) {
	day := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	f := newTokenAnalysisLinkFixture(t, day)
	none := []service.TokenAnalysisRiskReason{}

	// A 在任何完成事实之前读到并找到邻近账单; B 随后写回首个 502 响应。
	start := day.Add(6 * time.Hour)
	f.request("first-failure", "", start)
	read := f.state("first-failure")
	bill := f.usage("client:unrelated-first-failure", start.Add(time.Second), 900, 547)
	stale, err := f.repo.FindUsageLink(f.ctx, service.TokenAnalysisUsageLinkQuery{
		ArchiveID: "first-failure", StartedAt: start, UserID: &f.userID, APIKeyID: &f.apiKeyID, Model: tokenAnalysisLinkModel, Window: 10 * time.Second,
	})
	require.NoError(t, err)
	require.Equal(t, bill, stale.UsageLogID)
	f.response("first-failure", start.Add(2*time.Second), 502)
	write, err := f.repo.SetUsageLink(f.ctx, "first-failure", read, stale, 0, none)
	require.NoError(t, err)
	require.False(t, write.Applied)
	f.link()
	f.requireUnlinked("first-failure")

	// 前一日旧行按时序占用账单(风险 70); 首个 response 到达后 A 复核得出"无需改动",
	// 尚未确认时 B 发现精确 owner 并夺回账单; A 的确认不能清掉 B 登记的新复核。
	old := day.Add(-time.Second)
	done := day.Add(20 * time.Second)
	f.request("old-holder", "", old)
	owned := f.usage("resp_owner_v3", done.Add(200*time.Millisecond), 20000, 80)
	_, err = integrationDB.ExecContext(f.ctx, `UPDATE usage_logs SET input_tokens = 100000, cache_read_tokens = 0 WHERE id = $1`, owned)
	require.NoError(t, err)
	previous := day.AddDate(0, 0, -1).Format("2006-01-02")
	_, err = f.svc.IndexRange(f.ctx, service.TokenAnalysisIndexRequest{StartDate: previous, EndDate: previous, Timezone: "UTC"})
	require.NoError(t, err)
	f.requireLinked("old-holder", owned, service.TokenAnalysisUsageMatchTiming)
	f.response("old-holder", done, 200)
	aRead := f.state("old-holder")
	require.True(t, f.recheckPending("old-holder"))

	f.request("owner", "cid-owner-v3", day)
	f.responseWithID("owner", done, 200, "resp_owner_v3")
	exact, err := f.repo.FindUsageLink(f.ctx, service.TokenAnalysisUsageLinkQuery{
		ArchiveID: "owner", ClientRequestID: "cid-owner-v3", ResponseID: "resp_owner_v3", StartedAt: day, CompletedAt: &done,
		UserID: &f.userID, APIKeyID: &f.apiKeyID, Model: tokenAnalysisLinkModel, ExactOnly: true,
	})
	require.NoError(t, err)
	claimed, err := f.repo.SetUsageLink(f.ctx, "owner", f.state("owner"), exact, 0, none)
	require.NoError(t, err)
	require.True(t, claimed.Displaced)
	require.NoError(t, f.repo.ClearUsageLinkPending(f.ctx, "old-holder", aRead.Version))
	require.True(t, f.recheckPending("old-holder"))

	f.link()
	f.requireUnlinked("old-holder")
	var risk int
	require.NoError(t, integrationDB.QueryRowContext(f.ctx, `SELECT risk_score FROM token_analysis_request_summaries WHERE archive_id = 'old-holder'`).Scan(&risk))
	require.Zero(t, risk)
	require.False(t, f.recheckPending("old-holder"))
}
