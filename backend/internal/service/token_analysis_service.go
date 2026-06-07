package service

import (
	"context"
	"fmt"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

type TokenAnalysisService struct {
	repo    TokenAnalysisRepository
	cfg     *config.Config
	mu      sync.Mutex
	running bool
}

func NewTokenAnalysisService(repo TokenAnalysisRepository, cfg *config.Config) *TokenAnalysisService {
	if cfg == nil {
		cfg = &config.Config{}
	}
	return &TokenAnalysisService{repo: repo, cfg: cfg}
}

func (s *TokenAnalysisService) GetSummary(ctx context.Context, filters TokenAnalysisFilters) (*TokenAnalysisSummary, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("token analysis service is not configured")
	}
	return s.repo.GetSummary(ctx, filters)
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
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("token analysis service is not configured")
	}
	if s.cfg != nil && !s.cfg.TokenAnalysis.IndexEnabled {
		return nil, infraerrors.Conflict("TOKEN_ANALYSIS_INDEX_DISABLED", "token analysis indexing is disabled")
	}
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil, infraerrors.Conflict("TOKEN_ANALYSIS_INDEX_RUNNING", "token analysis indexing is already running")
	}
	s.running = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()
	return s.indexRange(ctx, req)
}
