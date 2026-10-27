//go:build integration

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// 以下用例沿用 2026-10-27 分支审核(docs/reviews/2026-10-27-token-analysis-branch-code-review-cn.md)
// 的复现交错, 断言改为修正后的合同。只能在独立测试库运行。

func waitTokenAnalysisDBCondition(ctx context.Context, query string, args ...any) error {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		var ready bool
		if err := integrationDB.QueryRowContext(ctx, query, args...).Scan(&ready); err != nil {
			return err
		}
		if ready {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// R2: 父请求 P 的写入事务已标记过后续行、尚未提交时, 子请求 C 入库并计算候选。C 的写入
// 与候选查找都要取同一 Key 的候选锁, 因此等 P 提交后才进行: C 最终要么有指向 P 的边,
// 要么保持待重新匹配, 不会带着空候选集合对齐版本。
func TestTokenAnalysisConcurrentParentCannotEscapeChildCandidates(t *testing.T) {
	f := newTokenAnalysisFingerprintFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	gate := time.Now().UnixNano()%1_000_000_000 + 1_000_000_000
	control, err := integrationDB.Conn(ctx)
	require.NoError(t, err)
	defer func() { _ = control.Close() }()
	_, err = control.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, gate)
	require.NoError(t, err)
	released := false
	release := func() {
		if !released {
			_, _ = control.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, gate)
			released = true
		}
	}
	defer release()
	// 测试专用: P 的分类版本加一时(写入事务的最后一步)停在 gate 上, 不改变生产逻辑。
	fn := fmt.Sprintf("ta_test_parent_gate_%d", gate)
	trigger := fn + "_trigger"
	_, err = integrationDB.ExecContext(ctx, fmt.Sprintf(`
CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_advisory_xact_lock(%d::bigint);
    RETURN NEW;
END $$;
CREATE TRIGGER %s BEFORE UPDATE OF cls_input_version ON token_analysis_request_summaries
FOR EACH ROW WHEN (NEW.archive_id = '%s-p' AND NEW.cls_input_version > OLD.cls_input_version)
EXECUTE FUNCTION %s();`, fn, gate, trigger, f.prefix, fn))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON token_analysis_request_summaries; DROP FUNCTION IF EXISTS %s();`, trigger, fn))
	})

	parentDone := make(chan error, 1)
	go func() {
		_, err := f.repo.UpsertRequestSummary(ctx, &service.TokenAnalysisRequestSummary{
			ArchiveID: f.prefix + "-p", EventTime: f.base, UserID: &f.userID, APIKeyID: &f.apiKeyID,
			Endpoint: "/v1/responses", Fingerprint: testFingerprint("P2", "P1", "G2", 2),
		})
		parentDone <- err
	}()
	require.NoError(t, waitTokenAnalysisDBCondition(ctx, `
SELECT EXISTS (SELECT 1 FROM pg_locks
 WHERE locktype='advisory' AND classid=0 AND objid=$1::oid AND NOT granted)`, gate))

	childFP := testFingerprint("C3", "P2", "G3", 3)
	query := service.TokenAnalysisPrefixCandidateQuery{
		ArchiveID: f.prefix + "-c", APIKeyID: f.apiKeyID, FPVersion: 77,
		From: f.base.Add(-time.Hour), To: f.base.Add(time.Minute),
		PrefixHashes: [][]byte{testFingerprintHash("P1"), testFingerprintHash("P2"), testFingerprintHash("C3")},
		ItemsHashes:  [][]byte{testFingerprintHash("G1"), testFingerprintHash("G2"), testFingerprintHash("G3")},
	}
	childDone := make(chan error, 1)
	go func() {
		upserted, err := f.repo.UpsertRequestSummary(ctx, &service.TokenAnalysisRequestSummary{
			ArchiveID: f.prefix + "-c", EventTime: f.base.Add(time.Minute), UserID: &f.userID, APIKeyID: &f.apiKeyID,
			Endpoint: "/v1/responses", Fingerprint: childFP,
		})
		if err != nil {
			childDone <- err
			return
		}
		_, err = f.repo.ReplacePrefixEdges(ctx, service.TokenAnalysisPrefixEdgeWrite{
			ArchiveID: f.prefix + "-c", ExpectedCandVersion: upserted.CandInputVersion, State: service.TokenAnalysisCandidateStateOK,
			Query: &query,
			Select: func(candidates []service.TokenAnalysisPrefixCandidate) ([]service.TokenAnalysisPrefixEdge, bool) {
				var edges []service.TokenAnalysisPrefixEdge
				for _, c := range candidates {
					edges = append(edges, service.TokenAnalysisPrefixEdge{ParentArchiveID: c.ArchiveID, Kind: service.TokenAnalysisEdgeFullPrefix, MatchedItems: c.PrefixItems})
				}
				return edges, false
			},
		})
		childDone <- err
	}()
	select {
	case err := <-childDone:
		t.Fatalf("child write did not wait for the uncommitted parent: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	release()
	require.NoError(t, <-parentDone)
	require.NoError(t, <-childDone)

	input, checked, _ := f.versions("-c")
	var edges int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM token_analysis_prefix_edges WHERE child_archive_id = $1 AND parent_archive_id = $2`,
		f.prefix+"-c", f.prefix+"-p").Scan(&edges))
	require.True(t, edges == 1 || input != checked, "child must reference the committed parent or stay due for rematch")
}

// R3: 源写入事务先持有高 id 的 P, 分类批量写回同时写低 id 的 C 与 P。写回跳过被持有的
// P, 不等待, 源事务随后的传播也能完成, 不形成死锁。
func TestTokenAnalysisSourceLockOrderDoesNotDeadlockClassification(t *testing.T) {
	f := newTokenAnalysisClassificationFixture(t)
	f.request("-c", 60, "C", 3)
	f.request("-p", 0, "P", 2)
	f.edges("-c", "-p")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	rows, err := f.repo.LoadClassificationRows(ctx, []string{f.prefix + "-c", f.prefix + "-p"})
	require.NoError(t, err)
	require.Len(t, rows, 2)
	results := make([]service.TokenAnalysisClassificationResult, 0, 2)
	for _, row := range rows {
		results = append(results, service.TokenAnalysisClassificationResult{
			ArchiveID: row.ArchiveID, ClsInputVersion: row.ClsInputVersion, ClsDepVersion: row.ClsDepVersion,
			CandCheckedVersion: row.CandCheckedVersion, RuleVersion: service.TokenAnalysisClassificationRuleVersion,
			ClsState: service.TokenAnalysisClsClassified, CacheObsState: service.TokenAnalysisObsNoUsage,
		})
	}
	sourceTx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = sourceTx.Rollback() }()
	// RecordResponse 在传播前的第一条 UPDATE。
	_, err = sourceTx.ExecContext(ctx, `UPDATE token_analysis_request_summaries SET response_status = 200 WHERE archive_id = $1`, f.prefix+"-p")
	require.NoError(t, err)

	written, err := f.repo.WriteClassifications(ctx, results)
	require.NoError(t, err)
	require.Equal(t, 1, written, "C is written, the held P is skipped")
	require.NoError(t, propagateTokenAnalysisClassification(ctx, sourceTx, []string{f.prefix + "-p"}))
	require.NoError(t, sourceTx.Commit())
	input, checked := f.clsVersions("-p")
	require.NotEqual(t, input, checked, "skipped P stays due")
}

// R4: C 已用 P 的事实分类, P 仍待分类时账单被删除; 下一轮分类先把 P 判为 no_usage。
// 触发器写下的失效登记独立于分类结果, 处理登记后 C 重新待分类。
func TestTokenAnalysisUsageDeletionWhileParentDirtyStillInvalidatesChild(t *testing.T) {
	f := newTokenAnalysisClassificationFixture(t)
	f.request("-c", 60, "C", 3)
	f.request("-p", 0, "P", 2)
	f.edges("-c", "-p")
	pUsage := f.complete("-p", 9000, 0)
	f.complete("-c", 60000, 2000)
	rows, err := f.repo.LoadClassificationRows(f.ctx, []string{f.prefix + "-c", f.prefix + "-p"})
	require.NoError(t, err)
	var parent, child service.TokenAnalysisClassificationRow
	for _, row := range rows {
		if row.ArchiveID == f.prefix+"-p" {
			parent = row
		} else {
			child = row
		}
	}
	require.Less(t, child.ID, parent.ID)
	parentVersion := parent.ClsInputVersion
	written, err := f.repo.WriteClassifications(f.ctx, []service.TokenAnalysisClassificationResult{{
		ArchiveID: child.ArchiveID, ClsInputVersion: child.ClsInputVersion, ClsDepVersion: child.ClsDepVersion,
		CandCheckedVersion: child.CandCheckedVersion, RuleVersion: service.TokenAnalysisClassificationRuleVersion,
		ClsState: service.TokenAnalysisClsClassified, CacheObsState: service.TokenAnalysisObsCold,
		Cause:          service.TokenAnalysisCauseMissedWithEligibleParent,
		PrefixEvidence: service.TokenAnalysisPrefixFull, CacheEligibility: service.TokenAnalysisEligibleAssumed,
		PriceEvidence:   service.TokenAnalysisPriceRowExact,
		ParentArchiveID: parent.ArchiveID, ParentClsVersion: &parentVersion,
	}})
	require.NoError(t, err)
	require.Equal(t, 1, written)

	_, err = integrationDB.ExecContext(f.ctx, `DELETE FROM usage_logs WHERE id = $1`, pUsage)
	require.NoError(t, err)
	_, err = f.svc.RunClassification(f.ctx)
	require.NoError(t, err)
	var parentObs string
	require.NoError(t, integrationDB.QueryRowContext(f.ctx,
		`SELECT cache_obs_state FROM token_analysis_request_summaries WHERE archive_id = $1`, parent.ArchiveID).Scan(&parentObs))
	require.Equal(t, service.TokenAnalysisObsNoUsage, parentObs)

	_, err = f.repo.DrainClassificationInvalidations(f.ctx, 100)
	require.NoError(t, err)
	cInput, cChecked := f.clsVersions("-c")
	require.NotEqual(t, cInput, cChecked, "deleting P's usage must invalidate C even after P was classified no_usage")
}

// N3(2026-10-27 复核): P 的完成事实/最终缓存键变化尚未提交时, C 在候选锁内建立 C→P 并
// 提交, 分类读到 P 的旧事实把 C 判为有资格。源事务只给自身加版本并登记失效, 登记在
// P 提交后才被处理, 那时 C→P 已提交可见, C 被重新登记并按 P 的新事实重算。
func TestTokenAnalysisNewEdgeCannotMissConcurrentParentFactChange(t *testing.T) {
	f := newTokenAnalysisClassificationFixture(t)
	var edgeForeignKeys int
	require.NoError(t, integrationDB.QueryRowContext(f.ctx, `
SELECT count(*) FROM pg_constraint
WHERE conrelid = 'token_analysis_prefix_edges'::regclass AND contype = 'f'`).Scan(&edgeForeignKeys))
	require.Zero(t, edgeForeignKeys, "the interleaving requires edges without a foreign key to the parent row")
	f.request("-p", 0, "P", 2)
	f.request("-c", 60, "C", 3)
	f.complete("-p", 9000, 0)
	f.complete("-c", 60000, 2000)
	f.drain()
	_, err := f.svc.RunClassification(f.ctx)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	gate := time.Now().UnixNano()%1_000_000_000 + 1_000_000_000
	control, err := integrationDB.Conn(ctx)
	require.NoError(t, err)
	defer func() { _ = control.Close() }()
	_, err = control.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, gate)
	require.NoError(t, err)
	released := false
	release := func() {
		if !released {
			_, _ = control.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, gate)
			released = true
		}
	}
	defer release()
	// 测试专用: P 的分类版本加一时停在 gate 上, 源事务保持未提交。
	fn := fmt.Sprintf("ta_test_fact_gate_%d", gate)
	trigger := fn + "_trigger"
	_, err = integrationDB.ExecContext(ctx, fmt.Sprintf(`
CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_advisory_xact_lock(%d::bigint);
    RETURN NEW;
END $$;
CREATE TRIGGER %s BEFORE UPDATE OF cls_input_version ON token_analysis_request_summaries
FOR EACH ROW WHEN (NEW.archive_id = '%s-p' AND NEW.cls_input_version > OLD.cls_input_version)
EXECUTE FUNCTION %s();`, fn, gate, trigger, f.prefix, fn))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON token_analysis_request_summaries; DROP FUNCTION IF EXISTS %s();`, trigger, fn))
	})

	facts := tokenAnalysisClassificationNativeFacts(testFingerprintHash("GP"))
	facts.UpstreamCacheKeyHash = "fedcba9876543210fedcba9876543210"
	parentDone := make(chan error, 1)
	go func() {
		parentDone <- f.repo.RecordResponse(ctx, service.TokenAnalysisResponseFacts{
			ArchiveID: f.prefix + "-p", CompletedAt: f.base.Add(6 * time.Second), Status: 200, Forward: facts,
		})
	}()
	require.NoError(t, waitTokenAnalysisDBCondition(ctx, `
SELECT EXISTS (SELECT 1 FROM pg_locks
 WHERE locktype = 'advisory' AND classid = 0 AND objid = $1::oid AND NOT granted)`, gate))

	var candVersion int64
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT cand_input_version FROM token_analysis_request_summaries WHERE archive_id = $1`, f.prefix+"-c").Scan(&candVersion))
	query := service.TokenAnalysisPrefixCandidateQuery{
		ArchiveID: f.prefix + "-c", APIKeyID: f.apiKeyID, FPVersion: 77,
		From: f.base.Add(-time.Hour), To: f.base.Add(time.Minute),
		PrefixHashes: [][]byte{testFingerprintHash("H1"), testFingerprintHash("P"), testFingerprintHash("C")},
		ItemsHashes:  [][]byte{testFingerprintHash("G1"), testFingerprintHash("GP"), testFingerprintHash("GC")},
	}
	applied, err := f.repo.ReplacePrefixEdges(ctx, service.TokenAnalysisPrefixEdgeWrite{
		ArchiveID: f.prefix + "-c", ExpectedCandVersion: candVersion, State: service.TokenAnalysisCandidateStateOK,
		Query: &query,
		Select: func(candidates []service.TokenAnalysisPrefixCandidate) ([]service.TokenAnalysisPrefixEdge, bool) {
			var edges []service.TokenAnalysisPrefixEdge
			for _, c := range candidates {
				edges = append(edges, service.TokenAnalysisPrefixEdge{ParentArchiveID: c.ArchiveID, Kind: service.TokenAnalysisEdgeFullPrefix, MatchedItems: c.PrefixItems})
			}
			return edges, false
		},
	})
	require.NoError(t, err)
	require.True(t, applied)
	f.drain()
	_, err = f.svc.RunClassification(ctx)
	require.NoError(t, err)
	var cause string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT cause FROM token_analysis_request_summaries WHERE archive_id = $1`, f.prefix+"-c").Scan(&cause))
	require.Equal(t, service.TokenAnalysisCauseMissedWithEligibleParent, cause, "classified against P's old facts while P is uncommitted")

	release()
	require.NoError(t, <-parentDone)
	f.drain()
	cInput, cChecked := f.clsVersions("-c")
	require.NotEqual(t, cInput, cChecked, "P's committed invalidation reaches the newly attached child")
	_, err = f.svc.RunClassification(ctx)
	require.NoError(t, err)
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT cause FROM token_analysis_request_summaries WHERE archive_id = $1`, f.prefix+"-c").Scan(&cause))
	require.Equal(t, service.TokenAnalysisCauseCacheKeyChanged+":gateway", cause)
}

// T1(2026-10-27 第三轮复核): 登记被一个消费者删除后由新的源事务重建, 另一个持有旧快照的
// 消费者不能按旧 (archive_id, seq) 误删新登记。包装器只在依赖发现之后、加锁 UPDATE 之前暂停。
type tokenAnalysisPauseBeforePropagationLock struct {
	tx      *sql.Tx
	reached chan struct{}
	resume  chan struct{}
}

func (w *tokenAnalysisPauseBeforePropagationLock) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return w.tx.QueryContext(ctx, query, args...)
}

func (w *tokenAnalysisPauseBeforePropagationLock) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	close(w.reached)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-w.resume:
	}
	return w.tx.ExecContext(ctx, query, args...)
}

func TestTokenAnalysisInvalidationSeqCannotReuseDeletedGeneration(t *testing.T) {
	f := newTokenAnalysisClassificationFixture(t)
	f.request("-p", 0, "P", 2)
	f.request("-c", 60, "C", 3)
	f.complete("-p", 9000, 0)
	f.complete("-c", 60000, 2000)
	f.drain()
	_, err := f.svc.RunClassification(f.ctx)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	parentID := f.prefix + "-p"
	childID := f.prefix + "-c"
	// Create an old pending generation, then retain B's non-locking snapshot.
	oldFacts := tokenAnalysisClassificationNativeFacts(testFingerprintHash("GP"))
	require.NoError(t, f.repo.RecordResponse(ctx, service.TokenAnalysisResponseFacts{
		ArchiveID: parentID, CompletedAt: f.base.Add(5 * time.Second), Status: 200, Forward: oldFacts,
	}))
	var oldSeq int64
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT seq FROM token_analysis_cls_invalidations WHERE archive_id=$1`, parentID).Scan(&oldSeq))
	// Consumer A drains/deletes the old generation while B still owns its snapshot.
	_, err = f.repo.DrainClassificationInvalidations(ctx, 10000)
	require.NoError(t, err)
	var pending int
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT count(*) FROM token_analysis_cls_invalidations WHERE archive_id=$1`, parentID).Scan(&pending))
	require.Zero(t, pending)
	_, err = f.svc.RunClassification(ctx)
	require.NoError(t, err)

	bTx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = bTx.Rollback() }()
	var bPID int
	require.NoError(t, bTx.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&bPID))
	wrapper := &tokenAnalysisPauseBeforePropagationLock{tx: bTx, reached: make(chan struct{}), resume: make(chan struct{})}
	resumed := false
	resumeB := func() {
		if !resumed {
			close(wrapper.resume)
			resumed = true
		}
	}
	defer resumeB()
	bDone := make(chan error, 1)
	go func() {
		err := propagateTokenAnalysisClassification(ctx, wrapper, []string{parentID})
		if err == nil {
			// Exactly the compare-and-delete SQL used by the current drain method.
			_, err = bTx.ExecContext(ctx, `
DELETE FROM token_analysis_cls_invalidations q
USING (SELECT unnest($1::text[]) AS archive_id, unnest($2::bigint[]) AS seq) d
WHERE q.archive_id=d.archive_id AND q.seq=d.seq`, pq.Array([]string{parentID}), pq.Array([]int64{oldSeq}))
		}
		if err == nil {
			err = bTx.Commit()
		} else {
			_ = bTx.Rollback()
		}
		bDone <- err
	}()
	select {
	case <-wrapper.reached:
	case <-ctx.Done():
		t.Fatal("B never reached the post-discovery/pre-row-lock pause")
	}

	// A real new source transaction S is held before its registration becomes
	// visible. Its source row is locked; no C -> P edge existed during B's scan.
	gate := time.Now().UnixNano()%1_000_000_000 + 1_000_000_000
	control, err := integrationDB.Conn(ctx)
	require.NoError(t, err)
	defer func() { _ = control.Close() }()
	_, err = control.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, gate)
	require.NoError(t, err)
	released := false
	releaseSource := func() {
		if !released {
			_, _ = control.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, gate)
			released = true
		}
	}
	defer releaseSource()
	fn := fmt.Sprintf("ta_test_aba_gate_%d", gate)
	trigger := fn + "_trigger"
	_, err = integrationDB.ExecContext(ctx, fmt.Sprintf(`
CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_advisory_xact_lock(%d::bigint);
    RETURN NEW;
END $$;
CREATE TRIGGER %s BEFORE UPDATE OF cls_input_version ON token_analysis_request_summaries
FOR EACH ROW WHEN (NEW.archive_id='%s' AND NEW.cls_input_version > OLD.cls_input_version)
EXECUTE FUNCTION %s();`, fn, gate, trigger, parentID, fn))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON token_analysis_request_summaries; DROP FUNCTION IF EXISTS %s();`, trigger, fn))
	})
	newFacts := oldFacts
	newFacts.UpstreamCacheKeyHash = "fedcba9876543210fedcba9876543210"
	sDone := make(chan error, 1)
	go func() {
		sDone <- f.repo.RecordResponse(ctx, service.TokenAnalysisResponseFacts{
			ArchiveID: parentID, CompletedAt: f.base.Add(6 * time.Second), Status: 200, Forward: newFacts,
		})
	}()
	require.NoError(t, waitTokenAnalysisDBCondition(ctx, `
SELECT EXISTS (SELECT 1 FROM pg_locks
 WHERE locktype='advisory' AND classid=0 AND objid=$1::oid AND NOT granted)`, gate))
	resumeB()
	require.NoError(t, waitTokenAnalysisDBCondition(ctx, `
SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE pid=$1 AND wait_event_type='Lock' AND cardinality(pg_blocking_pids(pid))>0)`, bPID))

	var candVersion int64
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT cand_input_version FROM token_analysis_request_summaries WHERE archive_id=$1`, childID).Scan(&candVersion))
	query := service.TokenAnalysisPrefixCandidateQuery{
		ArchiveID: childID, APIKeyID: f.apiKeyID, FPVersion: 77,
		From: f.base.Add(-time.Hour), To: f.base.Add(time.Minute),
		PrefixHashes: [][]byte{testFingerprintHash("H1"), testFingerprintHash("P"), testFingerprintHash("C")},
		ItemsHashes:  [][]byte{testFingerprintHash("G1"), testFingerprintHash("GP"), testFingerprintHash("GC")},
	}
	applied, err := f.repo.ReplacePrefixEdges(ctx, service.TokenAnalysisPrefixEdgeWrite{
		ArchiveID: childID, ExpectedCandVersion: candVersion, State: service.TokenAnalysisCandidateStateOK, Query: &query,
		Select: func(candidates []service.TokenAnalysisPrefixCandidate) ([]service.TokenAnalysisPrefixEdge, bool) {
			var edges []service.TokenAnalysisPrefixEdge
			for _, p := range candidates {
				edges = append(edges, service.TokenAnalysisPrefixEdge{ParentArchiveID: p.ArchiveID, Kind: service.TokenAnalysisEdgeFullPrefix, MatchedItems: p.PrefixItems})
			}
			return edges, false
		},
	})
	require.NoError(t, err)
	require.True(t, applied)
	// Drain C's own edge-change notification before S commits; this is essential
	// so an independent C event cannot accidentally repair the lost P event.
	f.drain()
	_, err = f.svc.RunClassification(ctx)
	require.NoError(t, err)
	var cause string
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT cause FROM token_analysis_request_summaries WHERE archive_id=$1`, childID).Scan(&cause))
	require.Equal(t, service.TokenAnalysisCauseMissedWithEligibleParent, cause)
	releaseSource()
	require.NoError(t, <-sDone)
	require.NoError(t, <-bDone)
	f.drain()
	_, err = f.svc.RunClassification(ctx)
	require.NoError(t, err)
	cInput, cChecked := f.clsVersions("-c")
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT cause FROM token_analysis_request_summaries WHERE archive_id=$1`, childID).Scan(&cause))
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT count(*) FROM token_analysis_cls_invalidations WHERE archive_id=ANY($1::text[])`, pq.Array([]string{parentID, childID})).Scan(&pending))
	require.Zero(t, pending, "neither P nor C has a remaining notification that could repair this state")
	t.Logf("old seq=%d, P/C queue rows=%d, child versions=%d/%d, cause=%s", oldSeq, pending, cInput, cChecked, cause)
	require.True(t, cInput != cChecked || cause != service.TokenAnalysisCauseMissedWithEligibleParent,
		"a stale consumer must not delete a re-created notification with a reused seq and leave C permanently clean")
}

// F1(2026-10-27 第四轮复核): 过期的重复消费者 B 在 A 已完成登记后才处理旧快照。传播只改
// 子孙的依赖版本、不改任何行的输入版本, B 不会让已对齐的 C 与父请求的输入版本失配。
// B 仍会给 P 加依赖版本(P 重算一次), 但 P 的输入版本不变。
func TestTokenAnalysisStaleConsumerCannotStrandCleanChild(t *testing.T) {
	f := newTokenAnalysisClassificationFixture(t)
	f.request("-p", 0, "P", 2)
	f.request("-c", 60, "C", 3)
	f.complete("-p", 9000, 0)
	f.complete("-c", 60000, 2000)
	f.drain()
	_, err := f.svc.RunClassification(f.ctx)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	parentID, childID := f.prefix+"-p", f.prefix+"-c"
	require.NoError(t, f.repo.RecordResponse(ctx, service.TokenAnalysisResponseFacts{
		ArchiveID: parentID, CompletedAt: f.base.Add(5 * time.Second), Status: 200,
		Forward: tokenAnalysisClassificationNativeFacts(testFingerprintHash("GP")),
	}))
	// B keeps its non-locking snapshot; A completes that generation first.
	var oldSeq int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT seq FROM token_analysis_cls_invalidations WHERE archive_id=$1`, parentID).Scan(&oldSeq))
	_, err = f.repo.DrainClassificationInvalidations(ctx, 10000)
	require.NoError(t, err)
	_, err = f.svc.RunClassification(ctx)
	require.NoError(t, err)
	var pending int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT count(*) FROM token_analysis_cls_invalidations WHERE archive_id=$1`, parentID).Scan(&pending))
	require.Zero(t, pending)

	bTx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = bTx.Rollback() }()
	wrapper := &tokenAnalysisPauseBeforePropagationLock{tx: bTx, reached: make(chan struct{}), resume: make(chan struct{})}
	resumed := false
	resumeB := func() {
		if !resumed {
			close(wrapper.resume)
			resumed = true
		}
	}
	defer resumeB()
	bDone := make(chan error, 1)
	var deleted int64
	go func() {
		err := propagateTokenAnalysisClassification(ctx, wrapper, []string{parentID})
		if err == nil {
			result, deleteErr := bTx.ExecContext(ctx, `
DELETE FROM token_analysis_cls_invalidations q
USING (SELECT unnest($1::text[]) AS archive_id, unnest($2::bigint[]) AS seq) d
WHERE q.archive_id=d.archive_id AND q.seq=d.seq`, pq.Array([]string{parentID}), pq.Array([]int64{oldSeq}))
			err = deleteErr
			if err == nil {
				deleted, err = result.RowsAffected()
			}
		}
		if err == nil {
			err = bTx.Commit()
		} else {
			_ = bTx.Rollback()
		}
		bDone <- err
	}()
	bFinished := false
	select {
	case <-wrapper.reached:
	case err = <-bDone:
		require.NoError(t, err)
		bFinished = true
	case <-ctx.Done():
		t.Fatal("B neither paused nor finished")
	}

	// B has already found no children, but does not yet hold P's row lock.
	// C now commits a real edge and finishes classification using P's current version.
	var candVersion int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT cand_input_version FROM token_analysis_request_summaries WHERE archive_id=$1`, childID).Scan(&candVersion))
	query := service.TokenAnalysisPrefixCandidateQuery{
		ArchiveID: childID, APIKeyID: f.apiKeyID, FPVersion: 77, From: f.base.Add(-time.Hour), To: f.base.Add(time.Minute),
		PrefixHashes: [][]byte{testFingerprintHash("H1"), testFingerprintHash("P"), testFingerprintHash("C")},
		ItemsHashes:  [][]byte{testFingerprintHash("G1"), testFingerprintHash("GP"), testFingerprintHash("GC")},
	}
	applied, err := f.repo.ReplacePrefixEdges(ctx, service.TokenAnalysisPrefixEdgeWrite{
		ArchiveID: childID, ExpectedCandVersion: candVersion, State: service.TokenAnalysisCandidateStateOK, Query: &query,
		Select: func(candidates []service.TokenAnalysisPrefixCandidate) ([]service.TokenAnalysisPrefixEdge, bool) {
			var edges []service.TokenAnalysisPrefixEdge
			for _, p := range candidates {
				edges = append(edges, service.TokenAnalysisPrefixEdge{ParentArchiveID: p.ArchiveID, Kind: service.TokenAnalysisEdgeFullPrefix, MatchedItems: p.PrefixItems})
			}
			return edges, false
		},
	})
	require.NoError(t, err)
	require.True(t, applied)
	f.drain()
	_, err = f.svc.RunClassification(ctx)
	require.NoError(t, err)
	var parentVersion, usedParentVersion int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT p.cls_input_version,c.parent_cls_version_used FROM token_analysis_request_summaries p,token_analysis_request_summaries c WHERE p.archive_id=$1 AND c.archive_id=$2`, parentID, childID).Scan(&parentVersion, &usedParentVersion))
	require.Equal(t, parentVersion, usedParentVersion)

	resumeB()
	if !bFinished {
		select {
		case err = <-bDone:
			require.NoError(t, err)
		case <-ctx.Done():
			t.Fatal("B did not finish")
		}
	}
	require.Zero(t, deleted, "A already removed this queue generation")

	// Even running another complete drain + classify round does not repair C.
	f.drain()
	_, err = f.svc.RunClassification(ctx)
	require.NoError(t, err)
	cInput, cChecked := f.clsVersions("-c")
	require.Equal(t, cInput, cChecked, "C looks clean to ListClassificationDue")
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT count(*) FROM token_analysis_cls_invalidations WHERE archive_id=ANY($1::text[])`, pq.Array([]string{parentID, childID})).Scan(&pending))
	require.Zero(t, pending, "no notification remains to repair C")
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT p.cls_input_version,c.parent_cls_version_used FROM token_analysis_request_summaries p,token_analysis_request_summaries c WHERE p.archive_id=$1 AND c.archive_id=$2`, parentID, childID).Scan(&parentVersion, &usedParentVersion))
	t.Logf("queue=%d child versions=%d/%d parent version now=%d used=%d stale delete rows=%d", pending, cInput, cChecked, parentVersion, usedParentVersion, deleted)
	require.Equal(t, parentVersion, usedParentVersion, "a stale duplicate consumer must not permanently strand an already-clean child behind its parent's publication version")
}
