//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type tokenAnalysisFingerprintFixture struct {
	t        *testing.T
	ctx      context.Context
	repo     service.TokenAnalysisRepository
	userID   int64
	apiKeyID int64
	prefix   string
	base     time.Time
}

func newTokenAnalysisFingerprintFixture(t *testing.T) *tokenAnalysisFingerprintFixture {
	t.Helper()
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	user := mustCreateUser(t, integrationEntClient, &service.User{Email: fmt.Sprintf("ta-fp-%d@example.com", suffix)})
	key := mustCreateApiKey(t, integrationEntClient, &service.APIKey{UserID: user.ID, Key: fmt.Sprintf("sk-ta-fp-%d", suffix), Name: "fp"})
	f := &tokenAnalysisFingerprintFixture{
		t: t, ctx: ctx, repo: NewTokenAnalysisRepository(integrationDB), userID: user.ID, apiKeyID: key.ID,
		prefix: fmt.Sprintf("ta-fp-%d", suffix), base: time.Date(2026, 10, 27, 1, 0, 0, 0, time.UTC),
	}
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM token_analysis_prefix_edges WHERE child_archive_id LIKE $1`, f.prefix+"%")
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM token_analysis_request_summaries WHERE user_id = $1`, user.ID)
	})
	return f
}

func testFingerprintHash(label string) []byte {
	h := make([]byte, 16)
	copy(h, label)
	return h
}

func (f *tokenAnalysisFingerprintFixture) upsert(id string, minute int, fp service.TokenAnalysisFingerprint) service.TokenAnalysisSummaryUpsert {
	f.t.Helper()
	got, err := f.repo.UpsertRequestSummary(f.ctx, &service.TokenAnalysisRequestSummary{
		ArchiveID: f.prefix + id, EventTime: f.base.Add(time.Duration(minute) * time.Minute),
		UserID: &f.userID, APIKeyID: &f.apiKeyID, Endpoint: "/v1/responses", Fingerprint: fp,
	})
	require.NoError(f.t, err)
	return got
}

func (f *tokenAnalysisFingerprintFixture) versions(id string) (int64, int64, string) {
	f.t.Helper()
	var input, checked int64
	var state string
	require.NoError(f.t, integrationDB.QueryRowContext(f.ctx, `
SELECT cand_input_version, cand_checked_version, cand_state FROM token_analysis_request_summaries WHERE archive_id = $1`,
		f.prefix+id).Scan(&input, &checked, &state))
	return input, checked, state
}

func testFingerprint(end, tail, items string, n int) service.TokenAnalysisFingerprint {
	fp := service.TokenAnalysisFingerprint{State: service.TokenAnalysisFingerprintOK, Version: 77, PrefixItems: n,
		PrefixEndHash: testFingerprintHash(end), ItemsEndHash: testFingerprintHash(items)}
	if tail != "" {
		fp.PrefixTail1Hash = testFingerprintHash(tail)
	}
	return fp
}

// 候选快照生命周期: 新行待计算; 条件写回确认版本; 指纹不变的重写不触发; 指纹变化与
// 晚到父请求使相关行重新匹配; 变得不可比较时清空自身的边并通知子请求。
func TestTokenAnalysisFingerprintCandidateLifecycle(t *testing.T) {
	f := newTokenAnalysisFingerprintFixture(t)
	parent := testFingerprint("P2", "P1", "G2", 2)
	child := testFingerprint("C3", "P2", "G3", 3)

	got := f.upsert("-p", 0, parent)
	require.True(t, got.CandidatesDue)
	require.EqualValues(t, 1, got.CandInputVersion)
	applied, err := f.repo.ReplacePrefixEdges(f.ctx, service.TokenAnalysisPrefixEdgeWrite{ArchiveID: f.prefix + "-p", ExpectedCandVersion: 1, State: service.TokenAnalysisCandidateStateOK})
	require.NoError(t, err)
	require.True(t, applied)
	require.False(t, f.upsert("-p", 0, parent).CandidatesDue, "same fingerprint: nothing to recompute")

	require.True(t, f.upsert("-c", 5, child).CandidatesDue)
	candidates, err := f.repo.(*tokenAnalysisRepository).FindPrefixCandidates(f.ctx, service.TokenAnalysisPrefixCandidateQuery{
		ArchiveID: f.prefix + "-c", APIKeyID: f.apiKeyID, FPVersion: 77,
		From: f.base.Add(-time.Hour), To: f.base.Add(5 * time.Minute),
		PrefixHashes: [][]byte{testFingerprintHash("P1"), testFingerprintHash("P2"), testFingerprintHash("C3")},
		ItemsHashes:  [][]byte{testFingerprintHash("G1"), testFingerprintHash("G2"), testFingerprintHash("G3")},
	})
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, f.prefix+"-p", candidates[0].ArchiveID)
	require.Equal(t, 2, candidates[0].PrefixItems)
	require.Equal(t, testFingerprintHash("P2"), candidates[0].PrefixEndHash)

	// 版本已变的写回被拒绝; 匹配的版本写入边并确认。
	applied, err = f.repo.ReplacePrefixEdges(f.ctx, service.TokenAnalysisPrefixEdgeWrite{ArchiveID: f.prefix + "-c", ExpectedCandVersion: 0})
	require.NoError(t, err)
	require.False(t, applied)
	applied, err = f.repo.ReplacePrefixEdges(f.ctx, service.TokenAnalysisPrefixEdgeWrite{
		ArchiveID: f.prefix + "-c", ExpectedCandVersion: 1, State: service.TokenAnalysisCandidateStateOK,
		Edges: []service.TokenAnalysisPrefixEdge{{ParentArchiveID: f.prefix + "-p", Kind: service.TokenAnalysisEdgeFullPrefix, MatchedItems: 2}},
	})
	require.NoError(t, err)
	require.True(t, applied)
	input, checked, state := f.versions("-c")
	require.Equal(t, input, checked)
	require.Equal(t, service.TokenAnalysisCandidateStateOK, state)

	// 晚到的父请求: 时间早于 C 的新行写入后, C 被标为待重新匹配; 不同指纹版本的行不受影响。
	other := testFingerprint("O1", "", "H1", 1)
	other.Version = 78
	f.upsert("-other-version", 6, other)
	_, _ = f.repo.ReplacePrefixEdges(f.ctx, service.TokenAnalysisPrefixEdgeWrite{ArchiveID: f.prefix + "-other-version", ExpectedCandVersion: 1})
	f.upsert("-late", 3, testFingerprint("L1", "", "K1", 1))
	input, checked, _ = f.versions("-c")
	require.Equal(t, checked+1, input)
	input, checked, _ = f.versions("-other-version")
	require.Equal(t, checked, input)
	rematch, err := f.repo.ListCandidateRematch(f.ctx, 10000)
	require.NoError(t, err)
	var ids []string
	for _, row := range rematch {
		ids = append(ids, row.ArchiveID)
	}
	require.Contains(t, ids, f.prefix+"-c")

	// 父请求指纹变化: 以它为父的 C 再次标脏(经边表通知, 同时也在 P 之后的窗口内)。
	inputBefore, checkedBefore, _ := f.versions("-c")
	f.upsert("-p", 0, testFingerprint("P2x", "P1", "G2", 2))
	inputAfter, checkedAfter, _ := f.versions("-c")
	require.Greater(t, inputAfter, inputBefore)
	require.Equal(t, checkedBefore, checkedAfter)

	// C 变得不可比较: 自身的边被清空, 版本对齐, 状态记为指纹状态。
	got = f.upsert("-c", 5, service.TokenAnalysisFingerprint{State: "unsupported:body_truncated"})
	require.False(t, got.CandidatesDue)
	input, checked, state = f.versions("-c")
	require.Equal(t, input, checked)
	require.Equal(t, "unsupported:body_truncated", state)
	var edges int
	require.NoError(t, integrationDB.QueryRowContext(f.ctx, `SELECT COUNT(*) FROM token_analysis_prefix_edges WHERE child_archive_id = $1`, f.prefix+"-c").Scan(&edges))
	require.Zero(t, edges)
}
