package service

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

type TokenAnalysisService struct {
	repo TokenAnalysisRepository
	cfg  *config.Config
	// settings 用于读取运行态归档目录(后台可热切换); nil 时回退 config。
	settings *SettingService
	mu       sync.Mutex
	running  bool
	// 后台索引(自动循环 + 手动异步触发)共享的生命周期控制:
	// StopAutoIndex 取消 lifecycleCtx 并等待 backgroundWG, 停机不被长索引拖住。
	lifecycleCtx       context.Context
	lifecycleCancel    context.CancelFunc
	lifecycleMu        sync.Mutex
	lifecycleStopped   bool
	backgroundWG       sync.WaitGroup
	autoIndexStartOnce sync.Once
	autoIndexStop      chan struct{}
	autoIndexStopOnce  sync.Once
}

func NewTokenAnalysisService(repo TokenAnalysisRepository, cfg *config.Config, settings *SettingService) *TokenAnalysisService {
	if cfg == nil {
		cfg = &config.Config{}
	}
	lifecycleCtx, lifecycleCancel := context.WithCancel(context.Background())
	return &TokenAnalysisService{
		repo: repo, cfg: cfg, settings: settings,
		lifecycleCtx: lifecycleCtx, lifecycleCancel: lifecycleCancel,
		autoIndexStop: make(chan struct{}),
	}
}

func (s *TokenAnalysisService) GetSummary(ctx context.Context, filters TokenAnalysisFilters) (*TokenAnalysisSummary, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("token analysis service is not configured")
	}
	// 概览口径固定为"全部已归档已索引"(含未匹配行), include_unmatched 只影响
	// 请求明细列表: 否则页面默认 include_unmatched=false 时 where 带上
	// usage_log_id IS NOT NULL, 未匹配数/未匹配率卡片恒为 0, 失去意义。
	filters.IncludeUnmatched = true
	summary, err := s.repo.GetSummary(ctx, filters)
	if err != nil {
		return nil, err
	}
	billed, err := s.repo.CountBilledRequests(ctx, filters)
	if err != nil {
		return nil, err
	}
	summary.BilledRequests = billed
	if billed > 0 {
		summary.ArchiveCoverage = float64(summary.MatchedRequests) / float64(billed)
	}
	return summary, nil
}

func (s *TokenAnalysisService) ListUserUsage(ctx context.Context, filters TokenAnalysisFilters, params pagination.PaginationParams) ([]TokenAnalysisUserUsage, *pagination.PaginationResult, error) {
	if s == nil || s.repo == nil {
		return nil, nil, fmt.Errorf("token analysis service is not configured")
	}
	return s.repo.ListUserUsage(ctx, filters, params)
}

func (s *TokenAnalysisService) ListProjectUsage(ctx context.Context, filters TokenAnalysisFilters, params pagination.PaginationParams) ([]TokenAnalysisProjectUsage, *pagination.PaginationResult, error) {
	if s == nil || s.repo == nil {
		return nil, nil, fmt.Errorf("token analysis service is not configured")
	}
	return s.repo.ListProjectUsage(ctx, filters, params)
}

func (s *TokenAnalysisService) ListRequests(ctx context.Context, filters TokenAnalysisFilters, params pagination.PaginationParams) ([]TokenAnalysisRequestItem, *pagination.PaginationResult, error) {
	if s == nil || s.repo == nil {
		return nil, nil, fmt.Errorf("token analysis service is not configured")
	}
	return s.repo.ListRequests(ctx, filters, params)
}

func (s *TokenAnalysisService) GetUserInput(ctx context.Context, archiveID string) (*TokenAnalysisUserInput, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("token analysis service is not configured")
	}
	input, err := s.repo.GetUserInput(ctx, archiveID)
	if err != nil {
		return nil, err
	}
	if input == nil {
		return nil, infraerrors.NotFound("TOKEN_ANALYSIS_INPUT_NOT_FOUND", "user input not found for archive id")
	}
	return input, nil
}

func (s *TokenAnalysisService) GetIndexStatus(ctx context.Context) (*TokenAnalysisIndexStatus, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("token analysis service is not configured")
	}
	status, err := s.repo.GetIndexStatus(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	status.Running = status.Running || s.running
	s.mu.Unlock()
	return status, nil
}

func (s *TokenAnalysisService) IndexRange(ctx context.Context, req TokenAnalysisIndexRequest) (*TokenAnalysisIndexResult, error) {
	if err := s.acquireIndexRun(); err != nil {
		return nil, err
	}
	defer s.releaseIndexRun()
	return s.indexRange(ctx, req)
}

// IndexRangeAsync 校验请求后在后台启动一次索引并立即返回: 大文件回扫可达
// 分钟级, 同步等待会撞前端/反向代理超时; 进度经 GetIndexStatus 轮询呈现。
// 参数校验与并发冲突仍同步报错, 便于前端立刻提示。
func (s *TokenAnalysisService) IndexRangeAsync(req TokenAnalysisIndexRequest) error {
	if _, _, err := tokenAnalysisIndexDates(req); err != nil {
		return infraerrors.BadRequest("TOKEN_ANALYSIS_INDEX_INVALID_RANGE", err.Error())
	}
	if err := s.acquireIndexRun(); err != nil {
		return err
	}
	if !s.beginBackgroundTask() {
		s.releaseIndexRun()
		return infraerrors.Conflict("TOKEN_ANALYSIS_INDEX_STOPPED", "token analysis indexing is stopped")
	}
	go func() {
		defer s.endBackgroundTask()
		defer s.releaseIndexRun()
		result, err := s.indexRange(s.lifecycleCtx, req)
		if err != nil {
			// 停机取消属预期, 无需告警。
			if s.lifecycleCtx.Err() != nil {
				return
			}
			log.Printf("[TokenAnalysisIndex] manual index range failed: %v", err)
			return
		}
		if result != nil {
			log.Printf("[TokenAnalysisIndex] manual indexed=%d skipped=%d failed=%d files=%d", result.IndexedRows, result.SkippedRows, result.FailedRows, result.Files)
		}
	}()
	return nil
}

func (s *TokenAnalysisService) beginBackgroundTask() bool {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	if s.lifecycleStopped {
		return false
	}
	s.backgroundWG.Add(1)
	return true
}

func (s *TokenAnalysisService) endBackgroundTask() {
	s.backgroundWG.Done()
}

// acquireIndexRun 占用单飞索引槽位: 服务未配置/索引关闭/已有轮次在跑时报错。
func (s *TokenAnalysisService) acquireIndexRun() error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("token analysis service is not configured")
	}
	if s.cfg != nil && !s.cfg.TokenAnalysis.IndexEnabled {
		return infraerrors.Conflict("TOKEN_ANALYSIS_INDEX_DISABLED", "token analysis indexing is disabled")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return infraerrors.Conflict("TOKEN_ANALYSIS_INDEX_RUNNING", "token analysis indexing is already running")
	}
	s.running = true
	return nil
}

func (s *TokenAnalysisService) releaseIndexRun() {
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()
}
