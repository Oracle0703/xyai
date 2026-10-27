//go:build integration

package repository

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type tokenAnalysisClassificationFixture struct {
	t         *testing.T
	ctx       context.Context
	repo      service.TokenAnalysisRepository
	svc       *service.TokenAnalysisService
	userID    int64
	apiKeyID  int64
	accountID int64
	prefix    string
	base      time.Time
}

func newTokenAnalysisClassificationFixture(t *testing.T) *tokenAnalysisClassificationFixture {
	t.Helper()
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	user := mustCreateUser(t, integrationEntClient, &service.User{Email: fmt.Sprintf("ta-cls-%d@example.com", suffix)})
	key := mustCreateApiKey(t, integrationEntClient, &service.APIKey{UserID: user.ID, Key: fmt.Sprintf("sk-ta-cls-%d", suffix), Name: "cls"})
	account := mustCreateAccount(t, integrationEntClient, &service.Account{Name: fmt.Sprintf("ta-cls-%d", suffix), Platform: service.PlatformOpenAI})
	repo := NewTokenAnalysisRepository(integrationDB)
	f := &tokenAnalysisClassificationFixture{
		t: t, ctx: ctx, repo: repo, userID: user.ID, apiKeyID: key.ID, accountID: account.ID,
		prefix: fmt.Sprintf("ta-cls-%d", suffix), base: time.Date(2026, 10, 27, 2, 0, 0, 0, time.UTC),
		svc: service.NewTokenAnalysisService(repo, &config.Config{
			TokenAnalysis: config.TokenAnalysisConfig{FingerprintSecret: "integration-secret"},
		}, nil),
	}
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM token_analysis_prefix_edges WHERE child_archive_id LIKE $1`, f.prefix+"%")
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM token_analysis_request_summaries WHERE user_id = $1`, user.ID)
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM usage_logs WHERE user_id = $1`, user.ID)
	})
	return f
}

// request 写入一条可比较指纹的请求, 并确认空的候选快照(边由 edge 单独写)。
func (f *tokenAnalysisClassificationFixture) request(id string, second int, end string, n int) {
	f.t.Helper()
	fp := testFingerprint(end, "", "G"+end, n)
	_, err := f.repo.UpsertRequestSummary(f.ctx, &service.TokenAnalysisRequestSummary{
		ArchiveID: f.prefix + id, EventTime: f.base.Add(time.Duration(second) * time.Second),
		UserID: &f.userID, APIKeyID: &f.apiKeyID, Endpoint: "/v1/responses", Method: "POST", Fingerprint: fp,
		SummaryJSON: map[string]any{}, RiskReasons: []service.TokenAnalysisRiskReason{},
	})
	require.NoError(f.t, err)
	f.edges(id)
}

// edges 以当前候选版本整体替换 id 的前缀边(parents 为完整前缀父请求)。
func (f *tokenAnalysisClassificationFixture) edges(id string, parents ...string) {
	f.t.Helper()
	var version int64
	require.NoError(f.t, integrationDB.QueryRowContext(f.ctx,
		`SELECT cand_input_version FROM token_analysis_request_summaries WHERE archive_id = $1`, f.prefix+id).Scan(&version))
	write := service.TokenAnalysisPrefixEdgeWrite{ArchiveID: f.prefix + id, ExpectedCandVersion: version, State: service.TokenAnalysisCandidateStateOK}
	for _, p := range parents {
		write.Edges = append(write.Edges, service.TokenAnalysisPrefixEdge{ParentArchiveID: f.prefix + p, Kind: service.TokenAnalysisEdgeFullPrefix, MatchedItems: 2})
	}
	applied, err := f.repo.ReplacePrefixEdges(f.ctx, write)
	require.NoError(f.t, err)
	require.True(f.t, applied)
}

// complete 记录原生 Responses 的完成事实, 并直接写入已确认的账单关联。
func (f *tokenAnalysisClassificationFixture) complete(id string, input, read int) int64 {
	f.t.Helper()
	var start time.Time
	var items []byte
	require.NoError(f.t, integrationDB.QueryRowContext(f.ctx,
		`SELECT event_time, items_end_hash FROM token_analysis_request_summaries WHERE archive_id = $1`, f.prefix+id).Scan(&start, &items))
	require.NoError(f.t, f.repo.RecordResponse(f.ctx, service.TokenAnalysisResponseFacts{
		ArchiveID: f.prefix + id, CompletedAt: start.Add(5 * time.Second), Status: 200,
		Forward: tokenAnalysisClassificationNativeFacts(items),
	}))
	usage, err := integrationEntClient.UsageLog.Create().
		SetUserID(f.userID).SetAPIKeyID(f.apiKeyID).SetAccountID(f.accountID).
		SetRequestID(f.prefix + id).SetModel("gpt-5.5").
		SetInputTokens(input).SetCacheReadTokens(read).SetOutputTokens(10).
		SetInputCost(float64(input) * 0.000001).SetCacheReadCost(float64(read) * 0.0000001).
		SetTotalCost(1).SetActualCost(1).SetCreatedAt(start.Add(5 * time.Second)).
		Save(f.ctx)
	require.NoError(f.t, err)
	_, err = integrationDB.ExecContext(f.ctx, `
UPDATE token_analysis_request_summaries
SET usage_log_id = $2, usage_match_method = 'request_id', usage_link_checked_version = usage_link_version
WHERE archive_id = $1`, f.prefix+id, usage.ID)
	require.NoError(f.t, err)
	return usage.ID
}

// clsVersions 返回两对分类版本之和(输入+依赖, 已确认输入+已确认依赖): 两者不等即待分类,
// 任一版本加一都会体现为加一。
func (f *tokenAnalysisClassificationFixture) clsVersions(id string) (int64, int64) {
	f.t.Helper()
	var input, checked int64
	require.NoError(f.t, integrationDB.QueryRowContext(f.ctx, `
SELECT cls_input_version + cls_dep_version, cls_checked_version + cls_dep_checked_version
FROM token_analysis_request_summaries WHERE archive_id = $1`, f.prefix+id).Scan(&input, &checked))
	return input, checked
}

// clsInputVersion 是子请求记下并比较的父请求输入版本。
func (f *tokenAnalysisClassificationFixture) clsInputVersion(id string) int64 {
	f.t.Helper()
	var input int64
	require.NoError(f.t, integrationDB.QueryRowContext(f.ctx,
		`SELECT cls_input_version FROM token_analysis_request_summaries WHERE archive_id = $1`, f.prefix+id).Scan(&input))
	return input
}

// drain 处理全部失效登记(作业每轮先做这一步)。
func (f *tokenAnalysisClassificationFixture) drain() {
	f.t.Helper()
	for {
		n, err := f.repo.DrainClassificationInvalidations(f.ctx, 1000)
		require.NoError(f.t, err)
		if n < 1000 {
			return
		}
	}
}

func (f *tokenAnalysisClassificationFixture) classify() {
	f.t.Helper()
	f.drain()
	_, err := f.svc.RunClassification(f.ctx)
	require.NoError(f.t, err)
	for _, id := range []string{"-p", "-c", "-d"} {
		input, checked := f.clsVersions(id)
		require.Equal(f.t, input, checked, id)
	}
}

// 分类输入变化在同一事务内给自身加版本并登记失效, 作业处理登记时再给子孙加版本;
// 分类作业按条件写回并给出金额; 过期的写回被拒绝; 账单删除由触发器登记, 规则版本由维护扫描登记。
func TestTokenAnalysisClassificationLifecycle(t *testing.T) {
	f := newTokenAnalysisClassificationFixture(t)
	f.request("-p", 0, "P", 2)
	f.request("-c", 60, "C", 3)
	f.request("-d", 120, "D", 4)
	f.edges("-c", "-p")
	f.edges("-d", "-c")
	f.complete("-p", 9000, 0)
	cUsage := f.complete("-c", 60000, 2000)
	f.complete("-d", 1000, 0)

	// 父请求的完成事实变化: 事务内只给本行加版本并登记, 提交后处理登记时沿边传播到子孙。
	f.drain()
	pBefore, _ := f.clsVersions("-p")
	cBefore, _ := f.clsVersions("-c")
	dBefore, _ := f.clsVersions("-d")
	require.NoError(t, f.repo.RecordResponse(f.ctx, service.TokenAnalysisResponseFacts{
		ArchiveID: f.prefix + "-p", CompletedAt: f.base.Add(6 * time.Second), Status: 200,
		Forward: tokenAnalysisClassificationNativeFacts(testFingerprintHash("GP")),
	}))
	pAfter, _ := f.clsVersions("-p")
	cAfter, _ := f.clsVersions("-c")
	require.Equal(t, pBefore+1, pAfter)
	require.Equal(t, cBefore, cAfter)
	cInputBefore, dInputBefore := f.clsInputVersion("-c"), f.clsInputVersion("-d")
	f.drain()
	// 传播只加依赖版本: 子孙的输入版本(其子请求比较的值)不变。
	require.Equal(t, cInputBefore, f.clsInputVersion("-c"))
	require.Equal(t, dInputBefore, f.clsInputVersion("-d"))
	cAfter, _ = f.clsVersions("-c")
	dAfter, _ := f.clsVersions("-d")
	require.Greater(t, cAfter, cBefore)
	require.Greater(t, dAfter, dBefore)

	f.classify()
	var (
		obs, cause, prefix, eligibility, price, parent string
		parentVersion, reusable, recoverable           *int64
		amount, coldCost                               *string
		hash                                           []byte
		rule                                           int32
	)
	require.NoError(t, integrationDB.QueryRowContext(f.ctx, `
SELECT cache_obs_state, cause, prefix_evidence, cache_eligibility, price_evidence, parent_archive_id,
    parent_cls_version_used, reusable_tokens, recoverable_tokens, amount::text, cold_input_cost::text, amount_inputs_hash, cls_rule_version
FROM token_analysis_request_summaries WHERE archive_id = $1`, f.prefix+"-c").
		Scan(&obs, &cause, &prefix, &eligibility, &price, &parent, &parentVersion, &reusable, &recoverable, &amount, &coldCost, &hash, &rule))
	require.Equal(t, service.TokenAnalysisObsCold, obs)
	require.Equal(t, service.TokenAnalysisCauseMissedWithEligibleParent, cause)
	require.Equal(t, service.TokenAnalysisPrefixFull, prefix)
	require.Equal(t, service.TokenAnalysisEligibleAssumed, eligibility)
	require.Equal(t, service.TokenAnalysisPriceRowExact, price)
	require.Equal(t, f.prefix+"-p", parent)
	pInput := f.clsInputVersion("-p")
	require.Equal(t, pInput, *parentVersion)
	require.EqualValues(t, 8960, *reusable)
	require.EqualValues(t, 6960, *recoverable)
	require.Equal(t, "0.0062640000", *amount)
	require.Equal(t, "0.0600000000", *coldCost)
	require.Len(t, hash, 16)
	require.Equal(t, service.TokenAnalysisClassificationRuleVersion, rule)

	// 过期写回的各个版本条件见 TestTokenAnalysisClassificationCASGuardsAreIndependent。

	// 关联确认与账单夺回都登记分类重算。
	_, err := integrationDB.ExecContext(f.ctx, `UPDATE token_analysis_request_summaries SET usage_link_version = usage_link_version + 1 WHERE archive_id = $1`, f.prefix+"-p")
	require.NoError(t, err)
	var linkVersion int64
	require.NoError(t, integrationDB.QueryRowContext(f.ctx, `SELECT usage_link_version FROM token_analysis_request_summaries WHERE archive_id = $1`, f.prefix+"-p").Scan(&linkVersion))
	require.NoError(t, f.repo.ClearUsageLinkPending(f.ctx, f.prefix+"-p", linkVersion))
	f.drain()
	input, checked := f.clsVersions("-d")
	require.Equal(t, checked+1, input)
	f.classify()

	// 账单被清理删除: 外键置空时触发器给本行加版本并登记失效, 处理登记时通知子孙。
	_, err = integrationDB.ExecContext(f.ctx, `DELETE FROM usage_logs WHERE id = $1`, cUsage)
	require.NoError(t, err)
	input, checked = f.clsVersions("-c")
	require.Equal(t, checked+1, input)
	drained, err := f.repo.DrainClassificationInvalidations(f.ctx, 100)
	require.NoError(t, err)
	require.GreaterOrEqual(t, drained, 1)
	input, checked = f.clsVersions("-d")
	require.Equal(t, checked+1, input)
	f.classify()
	require.NoError(t, integrationDB.QueryRowContext(f.ctx, `SELECT cache_obs_state, amount::text FROM token_analysis_request_summaries WHERE archive_id = $1`, f.prefix+"-c").Scan(&obs, &amount))
	require.Equal(t, service.TokenAnalysisObsNoUsage, obs)
	require.Nil(t, amount)

	// 规则版本不同的已对齐行被登记; 当前版本的行不受影响。
	var pID int64
	require.NoError(t, integrationDB.QueryRowContext(f.ctx, `SELECT id FROM token_analysis_request_summaries WHERE archive_id = $1`, f.prefix+"-p").Scan(&pID))
	marked, err := f.repo.MarkClassificationStale(f.ctx, service.TokenAnalysisClassificationRuleVersion+1, pID-1, pID)
	require.NoError(t, err)
	require.EqualValues(t, 1, marked)
	marked, err = f.repo.MarkClassificationStale(f.ctx, service.TokenAnalysisClassificationRuleVersion, pID-1, pID)
	require.NoError(t, err)
	require.Zero(t, marked)
}

// 暂存的完成事实合并后, 合并行在同一事务内登记分类重算。
func TestTokenAnalysisClassificationMergePendingBumps(t *testing.T) {
	f := newTokenAnalysisClassificationFixture(t)
	require.NoError(t, f.repo.RecordResponse(f.ctx, service.TokenAnalysisResponseFacts{
		ArchiveID: f.prefix + "-p", CompletedAt: f.base.Add(5 * time.Second), Status: 200,
	}))
	f.request("-p", 0, "P", 2)
	before, _ := f.clsVersions("-p")
	require.NoError(t, f.repo.MergePendingResponses(f.ctx))
	after, _ := f.clsVersions("-p")
	require.Equal(t, before+1, after)
	var pending int
	require.NoError(t, integrationDB.QueryRowContext(f.ctx, `SELECT COUNT(*) FROM token_analysis_pending_responses WHERE archive_id = $1`, f.prefix+"-p").Scan(&pending))
	require.Zero(t, pending)
}

// tokenAnalysisClassificationNativeFacts 是原生 Responses、网关未改写条目的转发记录:
// 最终条目哈希等于入口 items_end_hash。
func tokenAnalysisClassificationNativeFacts(itemsEndHash []byte) service.TokenAnalysisForwardFacts {
	return service.TokenAnalysisForwardFacts{
		ForwardPath: service.TokenAnalysisForwardPathOpenAIResponses, CacheKeyHandling: service.TokenAnalysisCacheKeyScoped,
		UpstreamCacheKeyHash: "0123456789abcdef0123456789abcdef", UsageCacheSource: service.TokenAnalysisUsageCacheSourceUpstreamParsed,
		UpstreamItemsHash: hex.EncodeToString(itemsEndHash), UpstreamRootHash: "abcdefabcdefabcdefabcdefabcdefab",
	}
}

// 审核 R4: 子请求已用父请求的事实分类, 父请求仍待分类时账单被删除, 随后父请求被分类为
// no_usage。失效登记独立于分类结果, 子请求仍被通知并重算(原因不再是 missed_with_eligible_parent)。
func TestTokenAnalysisClassificationUsageDeletionReachesDependents(t *testing.T) {
	f := newTokenAnalysisClassificationFixture(t)
	f.request("-p", 0, "P", 2)
	f.request("-c", 60, "C", 3)
	f.request("-d", 120, "D", 4)
	f.edges("-c", "-p")
	pUsage := f.complete("-p", 9000, 0)
	f.complete("-c", 60000, 2000)
	f.complete("-d", 1000, 0)
	f.classify()
	var cause string
	require.NoError(t, integrationDB.QueryRowContext(f.ctx, `SELECT cause FROM token_analysis_request_summaries WHERE archive_id = $1`, f.prefix+"-c").Scan(&cause))
	require.Equal(t, service.TokenAnalysisCauseMissedWithEligibleParent, cause)

	// 父请求变脏(不影响已写回的子请求), 账单随后被删除, 父请求先被分类为 no_usage。
	_, err := integrationDB.ExecContext(f.ctx, `UPDATE token_analysis_request_summaries SET cls_input_version = cls_input_version + 1 WHERE archive_id = $1`, f.prefix+"-p")
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(f.ctx, `DELETE FROM usage_logs WHERE id = $1`, pUsage)
	require.NoError(t, err)
	_, err = f.svc.RunClassification(f.ctx)
	require.NoError(t, err)
	var obs string
	require.NoError(t, integrationDB.QueryRowContext(f.ctx, `SELECT cache_obs_state FROM token_analysis_request_summaries WHERE archive_id = $1`, f.prefix+"-p").Scan(&obs))
	require.Equal(t, service.TokenAnalysisObsNoUsage, obs)

	drained, err := f.repo.DrainClassificationInvalidations(f.ctx, 100)
	require.NoError(t, err)
	require.GreaterOrEqual(t, drained, 1)
	f.classify()
	require.NoError(t, integrationDB.QueryRowContext(f.ctx, `SELECT cause FROM token_analysis_request_summaries WHERE archive_id = $1`, f.prefix+"-c").Scan(&cause))
	require.Equal(t, service.TokenAnalysisCauseParentUnusable, cause)
}

// 审核 R2: 新父请求的事务持有候选锁时, 子请求的候选查找等待它提交后才查, 因此能看到它。
func TestTokenAnalysisClassificationCandidateLookupWaitsForConcurrentParent(t *testing.T) {
	f := newTokenAnalysisClassificationFixture(t)
	f.request("-c", 60, "C", 3)
	var version int64
	require.NoError(t, integrationDB.QueryRowContext(f.ctx, `SELECT cand_input_version FROM token_analysis_request_summaries WHERE archive_id = $1`, f.prefix+"-c").Scan(&version))
	_, err := integrationDB.ExecContext(f.ctx, `UPDATE token_analysis_request_summaries SET cand_input_version = cand_input_version + 1 WHERE archive_id = $1`, f.prefix+"-c")
	require.NoError(t, err)

	// 模拟父请求写入事务: 持锁、插入可比较的父行, 暂不提交。
	tx, err := integrationDB.BeginTx(f.ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(f.ctx, `SELECT pg_advisory_xact_lock(hashtextextended('token_analysis_candidates:' || $1::text, 0))`, f.apiKeyID)
	require.NoError(t, err)
	_, err = tx.ExecContext(f.ctx, `
INSERT INTO token_analysis_request_summaries (archive_id, event_time, user_id, api_key_id, endpoint, method, summary_json, risk_reasons,
    fp_version, fp_state, prefix_end_hash, items_end_hash, prefix_items)
VALUES ($1, $2, $3, $4, '/v1/responses', 'POST', '{}'::jsonb, '[]'::jsonb, 77, 'ok', $5, $6, 1)`,
		f.prefix+"-p", f.base, f.userID, f.apiKeyID, testFingerprintHash("H1"), testFingerprintHash("I1"))
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() {
		_, err := f.repo.ReplacePrefixEdges(f.ctx, service.TokenAnalysisPrefixEdgeWrite{
			ArchiveID: f.prefix + "-c", ExpectedCandVersion: version + 1, State: service.TokenAnalysisCandidateStateOK,
			Query: &service.TokenAnalysisPrefixCandidateQuery{
				ArchiveID: f.prefix + "-c", APIKeyID: f.apiKeyID, FPVersion: 77,
				From: f.base.Add(-time.Hour), To: f.base.Add(time.Minute),
				PrefixHashes: [][]byte{testFingerprintHash("H1")}, ItemsHashes: [][]byte{testFingerprintHash("I1")},
			},
			Select: func(candidates []service.TokenAnalysisPrefixCandidate) ([]service.TokenAnalysisPrefixEdge, bool) {
				var edges []service.TokenAnalysisPrefixEdge
				for _, c := range candidates {
					edges = append(edges, service.TokenAnalysisPrefixEdge{ParentArchiveID: c.ArchiveID, Kind: service.TokenAnalysisEdgeFullPrefix, MatchedItems: 1})
				}
				return edges, false
			},
		})
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("candidate lookup did not wait for the parent transaction: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	require.NoError(t, tx.Commit())
	require.NoError(t, <-done)
	var parents int
	require.NoError(t, integrationDB.QueryRowContext(f.ctx, `SELECT COUNT(*) FROM token_analysis_prefix_edges WHERE child_archive_id = $1 AND parent_archive_id = $2`,
		f.prefix+"-c", f.prefix+"-p").Scan(&parents))
	require.Equal(t, 1, parents)
}

// 审核 R3: 分类写回跳过被其他事务持有的行, 不等待行锁, 因此不会与源写入互相等待。
func TestTokenAnalysisClassificationWriteSkipsLockedRows(t *testing.T) {
	f := newTokenAnalysisClassificationFixture(t)
	f.request("-c", 60, "C", 3)
	var input, cand int64
	require.NoError(t, integrationDB.QueryRowContext(f.ctx,
		`SELECT cls_input_version, cand_checked_version FROM token_analysis_request_summaries WHERE archive_id = $1`, f.prefix+"-c").Scan(&input, &cand))
	tx, err := integrationDB.BeginTx(f.ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(f.ctx, `SELECT 1 FROM token_analysis_request_summaries WHERE archive_id = $1 FOR UPDATE`, f.prefix+"-c")
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(f.ctx, 2*time.Second)
	defer cancel()
	written, err := f.repo.WriteClassifications(ctx, []service.TokenAnalysisClassificationResult{
		{ArchiveID: f.prefix + "-c", ClsInputVersion: input, CandCheckedVersion: cand, ClsState: service.TokenAnalysisClsClassified},
	})
	require.NoError(t, err)
	require.Zero(t, written)
	require.NoError(t, tx.Rollback())
	written, err = f.repo.WriteClassifications(f.ctx, []service.TokenAnalysisClassificationResult{
		{ArchiveID: f.prefix + "-c", ClsInputVersion: input, CandCheckedVersion: cand, ClsState: service.TokenAnalysisClsClassified},
	})
	require.NoError(t, err)
	require.Equal(t, 1, written)
}

// 条件写回的四个版本条件各自有效(2026-10-27 第五轮复核 P3): 每个子例先用当前快照证明可写,
// 再只改变一个版本, 旧快照被拒写, 刷新该版本后恢复可写, 拒写不可能来自其他条件。
func TestTokenAnalysisClassificationCASGuardsAreIndependent(t *testing.T) {
	for _, dimension := range []string{"input_only", "dependency_only", "parent_only", "candidate_only"} {
		t.Run(dimension, func(t *testing.T) {
			f := newTokenAnalysisClassificationFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			f.request("-p", 0, "P", 2)
			f.request("-c", 60, "C", 3)
			f.edges("-c", "-p")
			f.complete("-p", 9000, 0)
			f.complete("-c", 60000, 2000)
			f.drain()
			_, err := f.svc.RunClassification(ctx)
			require.NoError(t, err)

			load := func() (service.TokenAnalysisClassificationRow, service.TokenAnalysisClassificationRow) {
				rows, err := f.repo.LoadClassificationRows(ctx, []string{f.prefix + "-c", f.prefix + "-p"})
				require.NoError(t, err)
				require.Len(t, rows, 2)
				var child, parent service.TokenAnalysisClassificationRow
				for _, r := range rows {
					if r.ArchiveID == f.prefix+"-c" {
						child = r
					} else {
						parent = r
					}
				}
				return child, parent
			}
			result := func(c, p service.TokenAnalysisClassificationRow) service.TokenAnalysisClassificationResult {
				parentVersion := p.ClsInputVersion
				return service.TokenAnalysisClassificationResult{
					ArchiveID: c.ArchiveID, ClsInputVersion: c.ClsInputVersion, ClsDepVersion: c.ClsDepVersion,
					CandCheckedVersion: c.CandCheckedVersion, ParentArchiveID: p.ArchiveID, ParentClsVersion: &parentVersion,
					RuleVersion: service.TokenAnalysisClassificationRuleVersion,
					ClsState:    service.TokenAnalysisClsClassified, CacheObsState: service.TokenAnalysisObsCold,
					Cause:          service.TokenAnalysisCauseMissedWithEligibleParent,
					PrefixEvidence: service.TokenAnalysisPrefixFull, CacheEligibility: service.TokenAnalysisEligibleAssumed,
					PriceEvidence: service.TokenAnalysisPriceRowExact,
				}
			}
			beforeChild, beforeParent := load()
			require.Greater(t, beforeChild.ClsDepVersion, int64(0), "the fixture must exercise a nonzero dependency version")
			stale := result(beforeChild, beforeParent)
			// Positive control proves every unrelated guard and the result shape match.
			written, err := f.repo.WriteClassifications(ctx, []service.TokenAnalysisClassificationResult{stale})
			require.NoError(t, err)
			require.Equal(t, 1, written)

			switch dimension {
			case "input_only":
				require.NoError(t, f.repo.RecordResponse(ctx, service.TokenAnalysisResponseFacts{
					ArchiveID: beforeChild.ArchiveID, CompletedAt: f.base.Add(65 * time.Second), Status: 200,
					Forward: tokenAnalysisClassificationNativeFacts(testFingerprintHash("GC")),
				}))
			case "dependency_only":
				// The actual propagation helper changes C's dependency version only;
				// it must not mutate input, candidates, or P's input version.
				var tx *sql.Tx
				tx, err = integrationDB.BeginTx(ctx, nil)
				require.NoError(t, err)
				defer func() { _ = tx.Rollback() }()
				require.NoError(t, propagateTokenAnalysisClassification(ctx, tx, []string{beforeChild.ArchiveID}))
				require.NoError(t, tx.Commit())
			case "parent_only":
				// Real source registration changes P's input. Do not drain: C's own
				// guards must remain exact so only the parent predicate rejects.
				require.NoError(t, f.repo.RecordResponse(ctx, service.TokenAnalysisResponseFacts{
					ArchiveID: beforeParent.ArchiveID, CompletedAt: f.base.Add(5 * time.Second), Status: 200,
					Forward: tokenAnalysisClassificationNativeFacts(testFingerprintHash("GP")),
				}))
			case "candidate_only":
				// Controlled guard probe: advance only the aligned candidate pair.
				// A normal edge replacement also changes input, which would mask
				// whether the candidate-checked predicate itself is effective.
				_, err = integrationDB.ExecContext(ctx, `
UPDATE token_analysis_request_summaries
SET cand_input_version=cand_input_version+1, cand_checked_version=cand_checked_version+1
WHERE archive_id=$1`, beforeChild.ArchiveID)
				require.NoError(t, err)
			}
			afterChild, afterParent := load()
			if dimension == "input_only" {
				require.Equal(t, beforeChild.ClsInputVersion+1, afterChild.ClsInputVersion)
			} else {
				require.Equal(t, beforeChild.ClsInputVersion, afterChild.ClsInputVersion)
			}
			switch dimension {
			case "input_only":
				require.Equal(t, beforeChild.ClsDepVersion, afterChild.ClsDepVersion)
				require.Equal(t, beforeChild.CandCheckedVersion, afterChild.CandCheckedVersion)
				require.Equal(t, beforeChild.CandInputVersion, afterChild.CandInputVersion)
				require.Equal(t, beforeParent.ClsInputVersion, afterParent.ClsInputVersion)
			case "dependency_only":
				require.Equal(t, beforeChild.ClsDepVersion+1, afterChild.ClsDepVersion)
				require.Equal(t, beforeChild.CandCheckedVersion, afterChild.CandCheckedVersion)
				require.Equal(t, beforeChild.CandInputVersion, afterChild.CandInputVersion)
				require.Equal(t, beforeParent.ClsInputVersion, afterParent.ClsInputVersion)
			case "parent_only":
				require.Equal(t, beforeChild.ClsDepVersion, afterChild.ClsDepVersion)
				require.Equal(t, beforeChild.CandCheckedVersion, afterChild.CandCheckedVersion)
				require.Equal(t, beforeChild.CandInputVersion, afterChild.CandInputVersion)
				require.Equal(t, beforeParent.ClsInputVersion+1, afterParent.ClsInputVersion)
			case "candidate_only":
				require.Equal(t, beforeChild.ClsDepVersion, afterChild.ClsDepVersion)
				require.Equal(t, beforeChild.CandCheckedVersion+1, afterChild.CandCheckedVersion)
				require.Equal(t, beforeChild.CandInputVersion+1, afterChild.CandInputVersion)
				require.Equal(t, beforeParent.ClsInputVersion, afterParent.ClsInputVersion)
			}
			written, err = f.repo.WriteClassifications(ctx, []service.TokenAnalysisClassificationResult{stale})
			require.NoError(t, err)
			require.Zero(t, written, "only the named CAS dimension is stale")
			// Refreshing that one dimension restores writability; no other change
			// is required, so the rejection cannot be an unrelated stale guard.
			written, err = f.repo.WriteClassifications(ctx, []service.TokenAnalysisClassificationResult{result(afterChild, afterParent)})
			require.NoError(t, err)
			require.Equal(t, 1, written)
			var input, checked, dep, depChecked int64
			require.NoError(t, integrationDB.QueryRowContext(ctx, `
SELECT cls_input_version,cls_checked_version,cls_dep_version,cls_dep_checked_version
FROM token_analysis_request_summaries WHERE archive_id=$1`, beforeChild.ArchiveID).Scan(&input, &checked, &dep, &depChecked))
			require.Equal(t, input, checked)
			require.Equal(t, dep, depChecked)
		})
	}
}
