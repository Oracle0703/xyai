package service

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// OrganizationUsageRollupRepository maintains the per-day usage rollup read by the
// organization usage report. Every method runs one short transaction.
type OrganizationUsageRollupRepository interface {
	InitializeOrganizationUsageRollups(ctx context.Context) (bool, error)
	CloseNextOrganizationUsageRollupDay(ctx context.Context) (bool, error)
	RebuildNextDirtyOrganizationUsageRollupDay(ctx context.Context) (bool, error)
	// ReconcileOrganizationUsageRollups runs at most once per `every` across all instances.
	ReconcileOrganizationUsageRollups(ctx context.Context, days int, every time.Duration) ([]string, error)
	AnalyzeOrganizationUsageRollups(ctx context.Context) error
}

const (
	OrganizationUsageRollupLeaderKey = "organization-usage:rollup:leader"

	organizationUsageRollupTickInterval      = time.Minute
	organizationUsageRollupRunBudget         = 45 * time.Second
	organizationUsageRollupLockTTL           = 2 * time.Minute
	organizationUsageRollupReconcileInterval = time.Hour
	organizationUsageRollupReconcileDays     = 3
	// A backfill this large can change plans before autovacuum analyzes the table; reports
	// measured 12-19s outliers on a freshly filled, unanalyzed rollup table.
	organizationUsageRollupAnalyzeAfterDays = 7
)

// OrganizationUsageRollupService publishes and repairs usage_user_daily_rollups. Reports stay
// correct without it: unpublished or dirty days are read from usage_logs.
type OrganizationUsageRollupService struct {
	repo  OrganizationUsageRollupRepository
	locks LeaderLockCache
	db    *sql.DB

	lifecycleMu sync.Mutex
	started     bool
	stopped     bool
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

func NewOrganizationUsageRollupService(repo OrganizationUsageRollupRepository, locks LeaderLockCache, db *sql.DB) *OrganizationUsageRollupService {
	ctx, cancel := context.WithCancel(context.Background())
	return &OrganizationUsageRollupService{repo: repo, locks: locks, db: db, ctx: ctx, cancel: cancel}
}

func (s *OrganizationUsageRollupService) Start() {
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

// Stop cancels the in-flight run and waits for the loop to exit.
func (s *OrganizationUsageRollupService) Stop() {
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

func (s *OrganizationUsageRollupService) runLoop() {
	defer s.wg.Done()
	s.runScheduled(s.ctx)
	ticker := time.NewTicker(organizationUsageRollupTickInterval)
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

func (s *OrganizationUsageRollupService) runScheduled(ctx context.Context) {
	owner := fmt.Sprintf("organization-usage-rollup-%d", time.Now().UnixNano())
	release, acquired := tryAcquireSingletonLeaderLock(ctx, s.locks, s.db, OrganizationUsageRollupLeaderKey, owner, organizationUsageRollupLockTTL)
	if !acquired {
		return
	}
	defer release()
	runCtx, cancel := context.WithTimeout(ctx, organizationUsageRollupRunBudget)
	defer cancel()
	if err := s.RunOnce(runCtx); err != nil && ctx.Err() == nil {
		slog.Warn("organization_usage_rollup_run_failed", "error", err)
	}
}

// RunOnce initializes, closes finished days and rebuilds dirty days until the work or the
// context budget runs out, then reconciles recent days when the shared interval is due.
func (s *OrganizationUsageRollupService) RunOnce(ctx context.Context) error {
	if _, err := s.repo.InitializeOrganizationUsageRollups(ctx); err != nil {
		return fmt.Errorf("initialize: %w", err)
	}
	closed, err := drainOrganizationUsageRollupSteps(ctx, s.repo.CloseNextOrganizationUsageRollupDay)
	if err != nil {
		return fmt.Errorf("close day: %w", err)
	}
	rebuilt, err := drainOrganizationUsageRollupSteps(ctx, s.repo.RebuildNextDirtyOrganizationUsageRollupDay)
	if err != nil {
		return fmt.Errorf("rebuild dirty day: %w", err)
	}
	if closed > 0 || rebuilt > 0 {
		slog.Info("organization_usage_rollup_synced", "closed_days", closed, "rebuilt_days", rebuilt)
	}
	if closed+rebuilt >= organizationUsageRollupAnalyzeAfterDays {
		if err := s.repo.AnalyzeOrganizationUsageRollups(ctx); err != nil && ctx.Err() == nil {
			return fmt.Errorf("analyze: %w", err)
		}
	}
	if ctx.Err() == nil {
		marked, err := s.repo.ReconcileOrganizationUsageRollups(ctx, organizationUsageRollupReconcileDays, organizationUsageRollupReconcileInterval)
		if err != nil {
			return fmt.Errorf("reconcile: %w", err)
		}
		if len(marked) > 0 {
			slog.Warn("organization_usage_rollup_reconcile_mismatch", "days", marked)
		}
	}
	return nil
}

func drainOrganizationUsageRollupSteps(ctx context.Context, step func(context.Context) (bool, error)) (int, error) {
	count := 0
	for ctx.Err() == nil {
		done, err := step(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return count, nil
			}
			return count, err
		}
		if !done {
			break
		}
		count++
	}
	return count, nil
}
