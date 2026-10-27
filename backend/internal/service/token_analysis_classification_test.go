package service

import (
	"context"
	"encoding/hex"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

var tokenAnalysisClsBase = time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)

// testTokenAnalysisClsRow 构造一行已确认关联、走原生 Responses、上游解析 usage 的
// OpenAI 请求; at 为相对基准时间的秒数, 完成时间为开始后 5 秒。
func testTokenAnalysisClsRow(id int64, archiveID string, at int, input, read int64) TokenAnalysisClassificationRow {
	start := tokenAnalysisClsBase.Add(time.Duration(at) * time.Second)
	done := start.Add(5 * time.Second)
	status := 200
	return TokenAnalysisClassificationRow{
		ID: id, ArchiveID: archiveID, EventTime: start, CompletedAt: &done, ResponseStatus: &status,
		Endpoint: "/v1/responses", UsageLinkVersion: 1, UsageLinkCheckedVersion: 1, FPVersion: 7,
		CandInputVersion: 1, CandCheckedVersion: 1, CandState: TokenAnalysisCandidateStateOK,
		ForwardPath: TokenAnalysisForwardPathOpenAIResponses, UpstreamCacheKeyHash: "k1",
		UsageCacheSource: TokenAnalysisUsageCacheSourceUpstreamParsed, ClsInputVersion: 3,
		// 最终条目与入口条目相同(条目哈希相等), 父子最终顶层字段相同。
		ItemsEndHash: []byte(archiveID), UpstreamItemsHash: hex.EncodeToString([]byte(archiveID)), UpstreamRootHash: "r1",
		Usage: &TokenAnalysisClassificationUsage{
			UsageLogID: id + 1000, AccountID: 5, Platform: PlatformOpenAI, SentModel: "gpt-5.5",
			InputTokens: input, CacheReadTokens: read,
			InputCost:     decimal.NewFromInt(input).Mul(decimal.RequireFromString("0.000001")),
			CacheReadCost: decimal.NewFromInt(read).Mul(decimal.RequireFromString("0.0000001")),
		},
	}
}

func testTokenAnalysisClsFacts(rows []TokenAnalysisClassificationRow, edges []TokenAnalysisStoredEdge) *tokenAnalysisClassificationFacts {
	f := newTokenAnalysisClassificationFacts("secret")
	for i := range rows {
		f.rows[rows[i].ArchiveID] = &rows[i]
		f.edges[rows[i].ArchiveID] = []TokenAnalysisStoredEdge{}
	}
	for _, e := range edges {
		f.edges[e.ChildArchiveID] = append(f.edges[e.ChildArchiveID], e)
	}
	return f
}

func testTokenAnalysisClassify(t *testing.T, f *tokenAnalysisClassificationFacts, archiveID string) TokenAnalysisClassificationResult {
	t.Helper()
	res, need := f.classify(f.rows[archiveID])
	require.True(t, need.empty(), "unexpected need: %+v", need)
	return res
}

func TestTokenAnalysisObserveCache(t *testing.T) {
	row := testTokenAnalysisClsRow(1, "c", 0, 60000, 2000)
	require.Equal(t, TokenAnalysisObsCold, tokenAnalysisObserveCache(&row))
	row.Usage.CacheReadTokens = 20000 // 20000/80000 = 0.25
	require.Equal(t, TokenAnalysisObsHit, tokenAnalysisObserveCache(&row))
	row.Usage.InputTokens, row.Usage.CacheReadTokens = 40000, 0
	require.Equal(t, TokenAnalysisObsSmall, tokenAnalysisObserveCache(&row))
	row.Usage.BillingMode = "per_request"
	require.Equal(t, TokenAnalysisObsUnsupported, tokenAnalysisObserveCache(&row))
	row.Usage = nil
	require.Equal(t, TokenAnalysisObsNoUsage, tokenAnalysisObserveCache(&row))

	failed := testTokenAnalysisClsRow(2, "f", 0, 60000, 0)
	status := 500
	failed.ResponseStatus = &status
	require.Equal(t, TokenAnalysisObsNoUsage, tokenAnalysisObserveCache(&failed))
	count := testTokenAnalysisClsRow(3, "n", 0, 60000, 0)
	count.Endpoint = "/v1/messages/count_tokens"
	require.Equal(t, TokenAnalysisObsNoUsage, tokenAnalysisObserveCache(&count))
}

// 有资格的完整前缀父请求: 选中父请求并给出条件性复用机会金额。
// 父总提示词 9000 → 可复用 8960; 子请求缓存读 2000, R = 6960;
// 输入价 1e-6, 读价 1e-7, 金额 = 6960 × 9e-7 = 0.006264。
func TestTokenAnalysisClassifyEligibleFullPrefixAmount(t *testing.T) {
	p := testTokenAnalysisClsRow(1, "p", 0, 9000, 0)
	c := testTokenAnalysisClsRow(2, "c", 60, 60000, 2000)
	c.Usage.ImageInputCost = decimal.RequireFromString("0.001")
	f := testTokenAnalysisClsFacts([]TokenAnalysisClassificationRow{p, c},
		[]TokenAnalysisStoredEdge{{ChildArchiveID: "c", ParentArchiveID: "p", Kind: TokenAnalysisEdgeFullPrefix, MatchedItems: 4}})

	res := testTokenAnalysisClassify(t, f, "c")
	require.Equal(t, TokenAnalysisClsClassified, res.ClsState)
	require.Equal(t, TokenAnalysisObsCold, res.CacheObsState)
	require.Equal(t, TokenAnalysisCauseMissedWithEligibleParent, res.Cause)
	require.Equal(t, TokenAnalysisPrefixFull, res.PrefixEvidence)
	require.Equal(t, TokenAnalysisEligibleAssumed, res.CacheEligibility)
	require.Equal(t, TokenAnalysisPriceRowExact, res.PriceEvidence)
	require.Equal(t, "p", res.ParentArchiveID)
	require.Equal(t, int64(3), *res.ParentClsVersion)
	require.Equal(t, int64(8960), *res.ReusableTokens)
	require.Equal(t, int64(6960), *res.RecoverableTokens)
	require.Equal(t, "0.006264", res.Amount.String())
	// 冷输入费用 = 输入 0.06 + 图片输入 0.001 + 缓存写入 0。
	require.Equal(t, "0.061", res.ColdInputCost.String())
	require.Len(t, res.AmountInputsHash, tokenAnalysisFingerprintHashLen)
	require.Equal(t, int64(3), res.ClsInputVersion)
	require.Equal(t, int64(1), res.CandCheckedVersion)

	// 金额输入绑定账单: 父请求换了账单, 哈希随之变化。
	again := testTokenAnalysisClassify(t, f, "c")
	require.Equal(t, res.AmountInputsHash, again.AmountInputsHash)
	f.rows["p"].Usage.UsageLogID = 77
	require.NotEqual(t, res.AmountInputsHash, testTokenAnalysisClassify(t, f, "c").AmountInputsHash)
}

// 金额门槛: 去尾前缀、缓存写入、图片、无缓存读都不给金额, 资格仍为 eligible_assumed。
func TestTokenAnalysisClassifyAmountRequiresExactPrice(t *testing.T) {
	cases := map[string]struct {
		mutate func(p, c *TokenAnalysisClassificationRow)
		kind   string
		price  string
	}{
		"prefix_minus_tail": {func(p, c *TokenAnalysisClassificationRow) {}, TokenAnalysisEdgePrefixMinusTail, tokenAnalysisPriceNotFullPrefix},
		"parent_cache_write": {func(p, c *TokenAnalysisClassificationRow) { p.Usage.CacheCreationTokens = 10 },
			TokenAnalysisEdgeFullPrefix, tokenAnalysisPriceOpenAICacheWrite},
		"image":         {func(p, c *TokenAnalysisClassificationRow) { c.ImageCount = 1 }, TokenAnalysisEdgeFullPrefix, tokenAnalysisPriceImageInput},
		"no_cache_read": {func(p, c *TokenAnalysisClassificationRow) { c.Usage.CacheReadTokens = 0 }, TokenAnalysisEdgeFullPrefix, tokenAnalysisPriceNoCacheRead},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			p := testTokenAnalysisClsRow(1, "p", 0, 9000, 0)
			c := testTokenAnalysisClsRow(2, "c", 60, 60000, 2000)
			tc.mutate(&p, &c)
			f := testTokenAnalysisClsFacts([]TokenAnalysisClassificationRow{p, c},
				[]TokenAnalysisStoredEdge{{ChildArchiveID: "c", ParentArchiveID: "p", Kind: tc.kind, MatchedItems: 4}})
			res := testTokenAnalysisClassify(t, f, "c")
			require.Equal(t, TokenAnalysisEligibleAssumed, res.CacheEligibility)
			require.Equal(t, tc.price, res.PriceEvidence)
			require.Nil(t, res.Amount)
			require.Nil(t, res.AmountInputsHash)
		})
	}

	// 父请求不足 1024 个可复用 token: 金额为 0(不是 NULL)。
	p := testTokenAnalysisClsRow(1, "p", 0, 1000, 0)
	c := testTokenAnalysisClsRow(2, "c", 60, 60000, 2000)
	f := testTokenAnalysisClsFacts([]TokenAnalysisClassificationRow{p, c},
		[]TokenAnalysisStoredEdge{{ChildArchiveID: "c", ParentArchiveID: "p", Kind: TokenAnalysisEdgeFullPrefix, MatchedItems: 4}})
	res := testTokenAnalysisClassify(t, f, "c")
	require.Equal(t, TokenAnalysisPriceRowExact, res.PriceEvidence)
	require.Equal(t, int64(0), *res.ReusableTokens)
	require.True(t, res.Amount.IsZero())
}

// 门槛按设计第 7 节的顺序检查, 原因报告第一个不合格项。
func TestTokenAnalysisClassifyGateOrder(t *testing.T) {
	cases := map[string]struct {
		mutate      func(p, c *TokenAnalysisClassificationRow)
		cause       string
		eligibility string
	}{
		"parent_pending": {func(p, c *TokenAnalysisClassificationRow) { p.UsageLinkVersion = 2 },
			TokenAnalysisCauseParentPending, TokenAnalysisEligibilityPending},
		"parent_unusable": {func(p, c *TokenAnalysisClassificationRow) { p.Usage = nil; c.Usage.SentModel = "other" },
			TokenAnalysisCauseParentUnusable, "ineligible:parent_unusable"},
		"parent_completion_missing": {func(p, c *TokenAnalysisClassificationRow) { p.CompletedAt = nil },
			TokenAnalysisCauseParentPending, TokenAnalysisEligibilityPending},
		"parent_overlapping": {func(p, c *TokenAnalysisClassificationRow) {
			done := c.EventTime
			p.CompletedAt = &done
		}, TokenAnalysisCauseParentOverlapping, "ineligible:parent_overlapping"},
		"model_changed": {func(p, c *TokenAnalysisClassificationRow) { p.Usage.SentModel = "gpt-5"; p.Usage.AccountID = 9 },
			TokenAnalysisCauseModelChanged, "ineligible:model_changed"},
		"account_changed": {func(p, c *TokenAnalysisClassificationRow) { p.Usage.AccountID = 9; p.ForwardPath = "" },
			TokenAnalysisCauseAccountChanged, "ineligible:account_changed"},
		"continuation_ref": {func(p, c *TokenAnalysisClassificationRow) { c.HasContinuationRef = true },
			TokenAnalysisCauseContinuationRef, "ineligible:continuation_ref"},
		"history_path": {func(p, c *TokenAnalysisClassificationRow) { p.ForwardPath = "" },
			TokenAnalysisCauseUnsupportedPath, tokenAnalysisEligibilityTransform},
		// 网关改写了子请求的最终条目(如把 compaction 触发器移到末尾): 入口前缀不代表上游前缀。
		"child_items_rewritten": {func(p, c *TokenAnalysisClassificationRow) { c.UpstreamItemsHash = "00000000000000000000000000000000" },
			TokenAnalysisCausePromptRewritten, "ineligible:prompt_rewritten"},
		"parent_root_differs": {func(p, c *TokenAnalysisClassificationRow) { p.UpstreamRootHash = "r2" },
			TokenAnalysisCausePromptRewritten, "ineligible:prompt_rewritten"},
		"prompt_hash_missing": {func(p, c *TokenAnalysisClassificationRow) { p.UpstreamItemsHash = "" },
			TokenAnalysisCausePromptRewritten, tokenAnalysisEligibilityTransform},
		"plugin_path": {func(p, c *TokenAnalysisClassificationRow) {
			c.ForwardPath = TokenAnalysisForwardPathOpenAIResponsesPlugin
		},
			TokenAnalysisCauseUnsupportedPath, tokenAnalysisEligibilityTransform},
		"cache_key_gateway": {func(p, c *TokenAnalysisClassificationRow) { p.UpstreamCacheKeyHash = "k2" },
			"cache_key_changed:gateway", "ineligible:cache_key_changed"},
		"cache_key_client": {func(p, c *TokenAnalysisClassificationRow) {
			p.UpstreamCacheKeyHash, p.ClientCacheKeyHash = "k2", []byte("a")
		}, "cache_key_changed:client", "ineligible:cache_key_changed"},
		"usage_not_upstream": {func(p, c *TokenAnalysisClassificationRow) { p.UsageCacheSource = "" },
			TokenAnalysisCauseUsageNotUpstream, "ineligible:usage_not_upstream"},
		"beyond_reuse_window": {func(p, c *TokenAnalysisClassificationRow) { c.EventTime = p.EventTime.Add(301 * time.Second) },
			TokenAnalysisCauseBeyondReuseWindow, "ineligible:beyond_reuse_window"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			p := testTokenAnalysisClsRow(1, "p", 0, 9000, 0)
			c := testTokenAnalysisClsRow(2, "c", 60, 60000, 2000)
			tc.mutate(&p, &c)
			f := testTokenAnalysisClsFacts([]TokenAnalysisClassificationRow{p, c},
				[]TokenAnalysisStoredEdge{{ChildArchiveID: "c", ParentArchiveID: "p", Kind: TokenAnalysisEdgeFullPrefix, MatchedItems: 4}})
			res := testTokenAnalysisClassify(t, f, "c")
			require.Equal(t, TokenAnalysisClsClassified, res.ClsState)
			require.Equal(t, tc.cause, res.Cause)
			require.Equal(t, tc.eligibility, res.CacheEligibility)
			require.Equal(t, TokenAnalysisPrefixFull, res.PrefixEvidence)
			require.Empty(t, res.ParentArchiveID)
			require.Nil(t, res.Amount)
		})
	}
}

// 没有可用前缀边时的原因; 热请求不给原因。
func TestTokenAnalysisClassifyNoCandidate(t *testing.T) {
	c := testTokenAnalysisClsRow(2, "c", 60, 60000, 2000)
	f := testTokenAnalysisClsFacts([]TokenAnalysisClassificationRow{c}, nil)
	res := testTokenAnalysisClassify(t, f, "c")
	require.Equal(t, TokenAnalysisPrefixNone, res.PrefixEvidence)
	require.Equal(t, "no_candidate:none_in_window", res.Cause)

	f.edges["c"] = []TokenAnalysisStoredEdge{{ChildArchiveID: "c", ParentArchiveID: "x", Kind: TokenAnalysisEdgeItemsLink, MatchedItems: 3}}
	res = testTokenAnalysisClassify(t, f, "c")
	require.Equal(t, TokenAnalysisPrefixItemsLink, res.PrefixEvidence)
	require.Equal(t, TokenAnalysisCausePrefixChanged, res.Cause)

	for state, cause := range map[string]string{
		TokenAnalysisCandidateStateTieOverflow:     "no_candidate:tie_overflow",
		TokenAnalysisCandidateStateBodyUnavailable: "no_candidate:body_unavailable",
		TokenAnalysisCandidateStateNoAPIKey:        "no_candidate:none_in_window",
		"unsupported:body_truncated":               "no_candidate:fingerprint_unavailable",
	} {
		f.rows["c"].CandState = state
		require.Equal(t, cause, testTokenAnalysisClassify(t, f, "c").Cause, state)
	}

	f.rows["c"].CandState = TokenAnalysisCandidateStateOK
	f.rows["c"].Usage.CacheReadTokens = 30000
	res = testTokenAnalysisClassify(t, f, "c")
	require.Equal(t, TokenAnalysisObsHit, res.CacheObsState)
	require.Empty(t, res.Cause)
	require.Nil(t, res.ColdInputCost)
}

// 账单待复核与候选快照未完成: 冷暖照常给出, 归因等待; 小请求不找父请求。
func TestTokenAnalysisClassifyPendingStates(t *testing.T) {
	c := testTokenAnalysisClsRow(2, "c", 60, 60000, 2000)
	c.UsageLinkVersion = 2
	f := testTokenAnalysisClsFacts([]TokenAnalysisClassificationRow{c}, nil)
	res := testTokenAnalysisClassify(t, f, "c")
	require.Equal(t, TokenAnalysisClsPendingUsageLink, res.ClsState)
	require.Equal(t, TokenAnalysisObsCold, res.CacheObsState)
	require.Equal(t, TokenAnalysisEligibilityPending, res.CacheEligibility)
	require.NotNil(t, res.ColdInputCost)

	// 缺完成事实与账单待复核同属关联状态(设计第 7 节表)。
	f.rows["c"].UsageLinkVersion = 1
	completed := f.rows["c"].CompletedAt
	f.rows["c"].CompletedAt = nil
	res = testTokenAnalysisClassify(t, f, "c")
	require.Equal(t, TokenAnalysisClsPendingUsageLink, res.ClsState)
	require.Equal(t, TokenAnalysisEligibilityPending, res.CacheEligibility)
	f.rows["c"].CompletedAt = completed

	f.rows["c"].CandInputVersion = 2
	res = testTokenAnalysisClassify(t, f, "c")
	require.Equal(t, TokenAnalysisClsPendingCandidates, res.ClsState)
	require.Empty(t, res.Cause)

	f.rows["c"].Usage.InputTokens = 1000
	res = testTokenAnalysisClassify(t, f, "c")
	require.Equal(t, TokenAnalysisClsClassified, res.ClsState)
	require.Equal(t, TokenAnalysisObsSmall, res.CacheObsState)
}

// 第一层没有资格时沿祖先回溯: 原因取第一层, 选中更早的祖先; 去尾边之后的完整前缀边
// 只有祖先条目数不超过共享条目数时才算子请求的完整前缀。
func TestTokenAnalysisClassifyWalksAncestors(t *testing.T) {
	g := testTokenAnalysisClsRow(1, "g", 0, 9000, 0)
	p := testTokenAnalysisClsRow(2, "p", 30, 9500, 0)
	p.Usage.SentModel = "gpt-5" // 第一层: 模型不同
	c := testTokenAnalysisClsRow(3, "c", 60, 60000, 2000)
	edges := []TokenAnalysisStoredEdge{
		{ChildArchiveID: "c", ParentArchiveID: "p", Kind: TokenAnalysisEdgeFullPrefix, MatchedItems: 6},
		{ChildArchiveID: "p", ParentArchiveID: "g", Kind: TokenAnalysisEdgeFullPrefix, MatchedItems: 4},
	}
	f := testTokenAnalysisClsFacts([]TokenAnalysisClassificationRow{g, p, c}, edges)
	res := testTokenAnalysisClassify(t, f, "c")
	require.Equal(t, "g", res.ParentArchiveID)
	require.Equal(t, TokenAnalysisPrefixFull, res.PrefixEvidence)
	require.Equal(t, TokenAnalysisCauseMissedWithEligibleParent, res.Cause)
	require.NotNil(t, res.Amount)

	// c→p 为去尾边(共享 5 条), g 与 p 完全相同(6 条): g 也只是去尾前缀。
	edges[0] = TokenAnalysisStoredEdge{ChildArchiveID: "c", ParentArchiveID: "p", Kind: TokenAnalysisEdgePrefixMinusTail, MatchedItems: 5}
	edges[1].MatchedItems = 6
	f = testTokenAnalysisClsFacts([]TokenAnalysisClassificationRow{g, p, c}, edges)
	res = testTokenAnalysisClassify(t, f, "c")
	require.Equal(t, "g", res.ParentArchiveID)
	require.Equal(t, TokenAnalysisPrefixMinusTail, res.PrefixEvidence)
	require.Nil(t, res.Amount)

	// 第一层账单待复核: 停止回溯, 资格 pending。
	f.rows["p"].UsageLinkVersion = 5
	res = testTokenAnalysisClassify(t, f, "c")
	require.Empty(t, res.ParentArchiveID)
	require.Equal(t, TokenAnalysisCauseParentPending, res.Cause)
	require.Equal(t, TokenAnalysisEligibilityPending, res.CacheEligibility)

	// 祖先的候选快照未完成: 不展开它的边, 资格 pending, 原因仍取第一层。
	f.rows["p"].UsageLinkVersion = 1
	f.rows["p"].CandInputVersion = 9
	res = testTokenAnalysisClassify(t, f, "c")
	require.Empty(t, res.ParentArchiveID)
	require.Equal(t, TokenAnalysisCauseModelChanged, res.Cause)
	require.Equal(t, TokenAnalysisEligibilityPending, res.CacheEligibility)

	// 超出复用窗口的祖先不展开; 子请求自身不满足门槛时也不展开。
	f.rows["p"].CandInputVersion = 1
	f.rows["p"].EventTime = c.EventTime.Add(-400 * time.Second)
	res = testTokenAnalysisClassify(t, f, "c")
	require.Empty(t, res.ParentArchiveID)
	require.Equal(t, TokenAnalysisCauseModelChanged, res.Cause)
	require.Equal(t, "ineligible:model_changed", res.CacheEligibility)
}

// 回溯访问节点超过预算时记为 unknown:walk_budget。
func TestTokenAnalysisClassifyWalkBudget(t *testing.T) {
	c := testTokenAnalysisClsRow(1, "c", 280, 60000, 2000)
	rows := []TokenAnalysisClassificationRow{c}
	var edges []TokenAnalysisStoredEdge
	p := testTokenAnalysisClsRow(2, "p", 270, 9000, 0)
	p.Usage.AccountID = 9
	rows = append(rows, p)
	edges = append(edges, TokenAnalysisStoredEdge{ChildArchiveID: "c", ParentArchiveID: "p", Kind: TokenAnalysisEdgeFullPrefix, MatchedItems: 5})
	for i := 0; i < tokenAnalysisClassificationWalkBudget; i++ {
		id := "a" + time.Duration(i).String()
		a := testTokenAnalysisClsRow(int64(10+i), id, 100, 9000, 0)
		a.Usage.AccountID = 9
		rows = append(rows, a)
		edges = append(edges, TokenAnalysisStoredEdge{ChildArchiveID: "p", ParentArchiveID: id, Kind: TokenAnalysisEdgeFullPrefix, MatchedItems: 4})
	}
	f := testTokenAnalysisClsFacts(rows, edges)
	res := testTokenAnalysisClassify(t, f, "c")
	require.Equal(t, tokenAnalysisEligibilityWalkBudget, res.CacheEligibility)
	require.Equal(t, TokenAnalysisCauseAccountChanged, res.Cause)
}

// 批处理按需加载候选边与祖先, 全部完成后一次写回。
func TestTokenAnalysisClassificationBatchLoadsFacts(t *testing.T) {
	g := testTokenAnalysisClsRow(1, "g", 0, 9000, 0)
	p := testTokenAnalysisClsRow(2, "p", 30, 9500, 0)
	p.Usage.SentModel = "gpt-5"
	c := testTokenAnalysisClsRow(3, "c", 60, 60000, 2000)
	repo := &tokenAnalysisRepoStub{
		clsRows: []TokenAnalysisClassificationRow{g, p, c},
		clsEdges: []TokenAnalysisStoredEdge{
			{ChildArchiveID: "c", ParentArchiveID: "p", Kind: TokenAnalysisEdgeFullPrefix, MatchedItems: 6},
			{ChildArchiveID: "p", ParentArchiveID: "g", Kind: TokenAnalysisEdgeFullPrefix, MatchedItems: 4},
		},
	}
	svc := NewTokenAnalysisService(repo, nil, nil)
	written, err := svc.runTokenAnalysisClassification(context.Background())
	require.NoError(t, err)
	require.Equal(t, 3, written)
	byID := map[string]TokenAnalysisClassificationResult{}
	for _, w := range repo.clsWrites {
		byID[w.ArchiveID] = w
	}
	require.Equal(t, "g", byID["c"].ParentArchiveID)
	require.Equal(t, TokenAnalysisObsSmall, byID["g"].CacheObsState)
	// 待分类行本身已在内存中, 不重复读取; 只按需读边。
	require.Empty(t, repo.clsRowLoads)
	require.NotEmpty(t, repo.clsEdgeLoads)
}

// 维护扫描分段推进, 扫完一遍后等待 1 小时。
func TestTokenAnalysisClassificationSweepCursor(t *testing.T) {
	repo := &tokenAnalysisRepoStub{clsMaxID: 250000}
	svc := NewTokenAnalysisService(repo, nil, nil)
	now := tokenAnalysisClsBase
	_, err := svc.sweepTokenAnalysisClassification(context.Background(), now)
	require.NoError(t, err)
	require.Len(t, repo.staleMarks, tokenAnalysisClassificationSweepSpans)
	require.Equal(t, [2]int64{0, 20000}, repo.staleMarks[0])
	_, err = svc.sweepTokenAnalysisClassification(context.Background(), now)
	require.NoError(t, err)
	require.Equal(t, [2]int64{240000, 250000}, repo.staleMarks[len(repo.staleMarks)-1])
	require.Equal(t, now.Add(tokenAnalysisClassificationSweepRecheck), svc.classificationSweep.nextRoundAt)
	marks := len(repo.staleMarks)
	_, err = svc.sweepTokenAnalysisClassification(context.Background(), now.Add(time.Minute))
	require.NoError(t, err)
	require.Len(t, repo.staleMarks, marks)
}

// 纯文本字符串 content 不是图片: 摘要的图片计数为 0, 金额仍可按本行单价给出。
func TestTokenAnalysisTextOnlySummaryKeepsExactPrice(t *testing.T) {
	summary, err := SummarizeTokenAnalysisRequest("/v1/responses", []byte(`{"input":[{"role":"user","content":"plain text"},{"type":"message","role":"user","content":[{"type":"input_text","text":"more"}]}]}`), 100)
	require.NoError(t, err)
	require.Equal(t, 0, summary.ImageCount)

	p := testTokenAnalysisClsRow(1, "p", 0, 9000, 0)
	c := testTokenAnalysisClsRow(2, "c", 60, 60000, 2000)
	c.ImageCount = summary.ImageCount
	f := testTokenAnalysisClsFacts([]TokenAnalysisClassificationRow{p, c},
		[]TokenAnalysisStoredEdge{{ChildArchiveID: "c", ParentArchiveID: "p", Kind: TokenAnalysisEdgeFullPrefix, MatchedItems: 4}})
	require.Equal(t, TokenAnalysisPriceRowExact, testTokenAnalysisClassify(t, f, "c").PriceEvidence)
}

// 工具结果里的图片同样计入图片数, 金额门槛据此拒绝(2026-10-27 复核 N1); 纯文本工具结果为 0。
func TestTokenAnalysisToolOutputImageBlocksExactPrice(t *testing.T) {
	withImage, err := SummarizeTokenAnalysisRequest("/v1/responses", []byte(`{"model":"gpt-5.5","input":[{"type":"function_call","call_id":"call_image","name":"view_image","arguments":"{}"},{"type":"function_call_output","call_id":"call_image","output":[{"type":"input_image","image_url":"data:image/png;base64,AQID"}]},{"role":"user","content":"Describe this image"}]}`), 300)
	require.NoError(t, err)
	require.Equal(t, 1, withImage.ImageCount)
	textOnly, err := SummarizeTokenAnalysisRequest("/v1/responses", []byte(`{"model":"gpt-5.5","input":[{"type":"function_call","call_id":"call_1","name":"read","arguments":"{}"},{"type":"function_call_output","call_id":"call_1","output":"file contents"}]}`), 300)
	require.NoError(t, err)
	require.Equal(t, 0, textOnly.ImageCount)

	p := testTokenAnalysisClsRow(1, "p", 0, 9000, 0)
	c := testTokenAnalysisClsRow(2, "c", 60, 60000, 2000)
	c.ImageCount = withImage.ImageCount
	f := testTokenAnalysisClsFacts([]TokenAnalysisClassificationRow{p, c},
		[]TokenAnalysisStoredEdge{{ChildArchiveID: "c", ParentArchiveID: "p", Kind: TokenAnalysisEdgeFullPrefix, MatchedItems: 2}})
	res := testTokenAnalysisClassify(t, f, "c")
	require.Equal(t, tokenAnalysisPriceImageInput, res.PriceEvidence)
	require.Nil(t, res.Amount)
}

// 金额只在最后做一次 10 位舍入(2026-10-27 复核 N4):
// 798976 × (1/1000000 − 0.0001024001/1024) = 0.719078321975 → 0.7190783220。
func TestTokenAnalysisAmountRoundsOnlyOnce(t *testing.T) {
	p := testTokenAnalysisClsRow(1, "p", 0, 800000, 0)
	c := testTokenAnalysisClsRow(2, "c", 60, 1000000, 1024)
	c.Usage.InputCost = decimal.RequireFromString("1.0000000000")
	c.Usage.CacheReadCost = decimal.RequireFromString("0.0001024001")
	f := testTokenAnalysisClsFacts([]TokenAnalysisClassificationRow{p, c},
		[]TokenAnalysisStoredEdge{{ChildArchiveID: "c", ParentArchiveID: "p", Kind: TokenAnalysisEdgeFullPrefix, MatchedItems: 4}})
	res := testTokenAnalysisClassify(t, f, "c")
	require.Equal(t, int64(798976), *res.RecoverableTokens)
	require.True(t, decimal.RequireFromString("0.7190783220").Equal(*res.Amount), "actual=%s", res.Amount)
}
