package service

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"
)

// 分类作业资源(设计 10.5)。锁是 best-effort 的, 只用来减少多实例重复工作; 正确性由
// 条件写回保证。
const (
	tokenAnalysisClassificationLeaderKey  = "token-analysis:classification:leader"
	tokenAnalysisClassificationInterval   = time.Minute
	tokenAnalysisClassificationRunBudget  = 5 * time.Minute
	tokenAnalysisClassificationLockTTL    = 10 * time.Minute
	tokenAnalysisClassificationStmtBudget = 30 * time.Second
	tokenAnalysisClassificationBatch      = 2000
	tokenAnalysisClassificationMaxBatches = 20
	// 失效登记(源写入与账单删除)每批条数与每轮最多批数, 每条沿边传播。
	tokenAnalysisClassificationDrainBatch      = 2000
	tokenAnalysisClassificationDrainMaxBatches = 10
	// 维护扫描: 每轮最多扫 10 段、每段 2 万个 id; 扫完全表后 1 小时再扫。
	tokenAnalysisClassificationSweepSpan    = 20000
	tokenAnalysisClassificationSweepSpans   = 10
	tokenAnalysisClassificationSweepRecheck = time.Hour
)

// tokenAnalysisClassificationSweep 是规则版本/账单删除维护扫描的进度。
type tokenAnalysisClassificationSweep struct {
	mu          sync.Mutex
	cursor      int64
	nextRoundAt time.Time
}

// SetLeaderLock 设置分类作业的单实例锁后端(Redis 优先, 回退 PG 咨询锁)。
func (s *TokenAnalysisService) SetLeaderLock(cache LeaderLockCache, db *sql.DB) {
	if s == nil {
		return
	}
	s.lockCache, s.db = cache, db
}

// StartClassification 启动分类作业循环; 与自动索引共用停机控制(StopAutoIndex)。
// index_enabled=false 时不启动。
func (s *TokenAnalysisService) StartClassification() {
	if s == nil || s.cfg == nil || !s.cfg.TokenAnalysis.IndexEnabled {
		return
	}
	s.startClassificationWithInterval(tokenAnalysisClassificationInterval)
}

func (s *TokenAnalysisService) startClassificationWithInterval(interval time.Duration) {
	if s == nil || s.repo == nil || interval <= 0 {
		return
	}
	s.backgroundWG.Add(1)
	go func() {
		defer s.backgroundWG.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.classificationOnce(s.lifecycleCtx)
			case <-s.autoIndexStop:
				return
			}
		}
	}()
}

func (s *TokenAnalysisService) classificationOnce(baseCtx context.Context) {
	owner := fmt.Sprintf("token-analysis-classification-%d", time.Now().UnixNano())
	release, acquired := tryAcquireSingletonLeaderLock(baseCtx, s.lockCache, s.db, tokenAnalysisClassificationLeaderKey, owner, tokenAnalysisClassificationLockTTL)
	if !acquired {
		return
	}
	defer release()
	ctx, cancel := context.WithTimeout(baseCtx, tokenAnalysisClassificationRunBudget)
	defer cancel()
	// 先处理失效登记: 源写入提交后才沿边传播给子孙(设计 10.2), 账单删除由触发器登记(10.7)。
	drained, err := s.drainTokenAnalysisClassificationInvalidations(ctx)
	if err != nil && ctx.Err() == nil {
		log.Printf("[TokenAnalysisClassification] drain invalidations failed: %v", err)
	}
	if drained > 0 {
		log.Printf("[TokenAnalysisClassification] invalidations drained=%d", drained)
	}
	written, err := s.runTokenAnalysisClassification(ctx)
	if err != nil && ctx.Err() == nil {
		log.Printf("[TokenAnalysisClassification] classify failed: %v", err)
	}
	if written > 0 {
		log.Printf("[TokenAnalysisClassification] classified=%d", written)
	}
	marked, err := s.sweepTokenAnalysisClassification(ctx, time.Now())
	if err != nil && ctx.Err() == nil {
		log.Printf("[TokenAnalysisClassification] sweep failed: %v", err)
	}
	if marked > 0 {
		log.Printf("[TokenAnalysisClassification] marked_stale=%d", marked)
	}
}

func (s *TokenAnalysisService) drainTokenAnalysisClassificationInvalidations(ctx context.Context) (int, error) {
	total := 0
	for batch := 0; batch < tokenAnalysisClassificationDrainMaxBatches; batch++ {
		var n int
		if err := withTokenAnalysisStmtBudget(ctx, func(ctx context.Context) (err error) {
			n, err = s.repo.DrainClassificationInvalidations(ctx, tokenAnalysisClassificationDrainBatch)
			return err
		}); err != nil {
			return total, err
		}
		total += n
		if n < tokenAnalysisClassificationDrainBatch {
			break
		}
	}
	return total, nil
}

// RunClassification 执行一轮分类(不取单实例锁、不做维护扫描), 供集成测试与手动触发。
func (s *TokenAnalysisService) RunClassification(ctx context.Context) (int, error) {
	return s.runTokenAnalysisClassification(ctx)
}

// runTokenAnalysisClassification 按 id 顺序处理待分类行, 每批 2000 行、最多 20 批,
// 批次之间检查期限。写回失败(版本已变)的行保持待分类, 下一轮重试。
func (s *TokenAnalysisService) runTokenAnalysisClassification(ctx context.Context) (int, error) {
	afterID, total := int64(0), 0
	for batch := 0; batch < tokenAnalysisClassificationMaxBatches; batch++ {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		var due []TokenAnalysisClassificationRow
		err := withTokenAnalysisStmtBudget(ctx, func(ctx context.Context) (err error) {
			due, err = s.repo.ListClassificationDue(ctx, afterID, tokenAnalysisClassificationBatch)
			return err
		})
		if err != nil || len(due) == 0 {
			return total, err
		}
		afterID = due[len(due)-1].ID
		results, err := s.classifyTokenAnalysisBatch(ctx, due)
		if err != nil {
			return total, err
		}
		var written int
		if err := withTokenAnalysisStmtBudget(ctx, func(ctx context.Context) (err error) {
			written, err = s.repo.WriteClassifications(ctx, results)
			return err
		}); err != nil {
			return total, err
		}
		total += written
		if len(due) < tokenAnalysisClassificationBatch {
			break
		}
	}
	return total, nil
}

// classifyTokenAnalysisBatch 对一批子请求分类: 分类器缺少祖先或候选边时, 整批汇总
// 缺口一次加载, 再对未完成的行重算, 直到全部完成。回溯每层至多需要两次加载。
func (s *TokenAnalysisService) classifyTokenAnalysisBatch(ctx context.Context, due []TokenAnalysisClassificationRow) ([]TokenAnalysisClassificationResult, error) {
	facts := newTokenAnalysisClassificationFacts(s.cfg.TokenAnalysis.FingerprintSecret)
	pending := make([]*TokenAnalysisClassificationRow, len(due))
	for i := range due {
		facts.rows[due[i].ArchiveID] = &due[i]
		pending[i] = &due[i]
	}
	results := make([]TokenAnalysisClassificationResult, 0, len(due))
	for round := 0; len(pending) > 0 && round <= 2*TokenAnalysisClassificationMaxDepth+2; round++ {
		needRows, needEdges := map[string]struct{}{}, map[string]struct{}{}
		var next []*TokenAnalysisClassificationRow
		for _, c := range pending {
			res, need := facts.classify(c)
			if need.empty() {
				results = append(results, res)
				continue
			}
			for _, id := range need.rows {
				if _, loaded := facts.rows[id]; !loaded {
					needRows[id] = struct{}{}
				}
			}
			for _, id := range need.edges {
				if _, loaded := facts.edges[id]; !loaded {
					needEdges[id] = struct{}{}
				}
			}
			next = append(next, c)
		}
		if len(next) == 0 || (len(needRows) == 0 && len(needEdges) == 0) {
			break
		}
		if err := s.loadTokenAnalysisClassificationFacts(ctx, facts, tokenAnalysisSetKeys(needRows), tokenAnalysisSetKeys(needEdges)); err != nil {
			return nil, err
		}
		pending = next
	}
	return results, nil
}

func (s *TokenAnalysisService) loadTokenAnalysisClassificationFacts(ctx context.Context, facts *tokenAnalysisClassificationFacts, rowIDs, edgeIDs []string) error {
	if len(edgeIDs) > 0 {
		var edges []TokenAnalysisStoredEdge
		if err := withTokenAnalysisStmtBudget(ctx, func(ctx context.Context) (err error) {
			edges, err = s.repo.LoadPrefixEdges(ctx, edgeIDs)
			return err
		}); err != nil {
			return err
		}
		for _, id := range edgeIDs {
			facts.edges[id] = []TokenAnalysisStoredEdge{}
		}
		for _, e := range edges {
			facts.edges[e.ChildArchiveID] = append(facts.edges[e.ChildArchiveID], e)
		}
	}
	if len(rowIDs) > 0 {
		var rows []TokenAnalysisClassificationRow
		if err := withTokenAnalysisStmtBudget(ctx, func(ctx context.Context) (err error) {
			rows, err = s.repo.LoadClassificationRows(ctx, rowIDs)
			return err
		}); err != nil {
			return err
		}
		for _, id := range rowIDs {
			facts.rows[id] = nil
		}
		for i := range rows {
			facts.rows[rows[i].ArchiveID] = &rows[i]
		}
	}
	return nil
}

// sweepTokenAnalysisClassification 分段扫描全表, 登记规则版本过期的行;
// 扫完一遍后等待 1 小时再开始下一遍。
func (s *TokenAnalysisService) sweepTokenAnalysisClassification(ctx context.Context, now time.Time) (int64, error) {
	sweep := &s.classificationSweep
	sweep.mu.Lock()
	defer sweep.mu.Unlock()
	if now.Before(sweep.nextRoundAt) {
		return 0, nil
	}
	var maxID int64
	if err := withTokenAnalysisStmtBudget(ctx, func(ctx context.Context) (err error) {
		maxID, err = s.repo.MaxSummaryID(ctx)
		return err
	}); err != nil {
		return 0, err
	}
	var marked int64
	for i := 0; i < tokenAnalysisClassificationSweepSpans; i++ {
		if sweep.cursor >= maxID {
			sweep.cursor, sweep.nextRoundAt = 0, now.Add(tokenAnalysisClassificationSweepRecheck)
			return marked, nil
		}
		to := min(sweep.cursor+tokenAnalysisClassificationSweepSpan, maxID)
		var n int64
		if err := withTokenAnalysisStmtBudget(ctx, func(ctx context.Context) (err error) {
			n, err = s.repo.MarkClassificationStale(ctx, TokenAnalysisClassificationRuleVersion, sweep.cursor, to)
			return err
		}); err != nil {
			return marked, err
		}
		marked += n
		sweep.cursor = to
	}
	return marked, nil
}

func withTokenAnalysisStmtBudget(ctx context.Context, fn func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, tokenAnalysisClassificationStmtBudget)
	defer cancel()
	return fn(ctx)
}

func tokenAnalysisSetKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	return keys
}
