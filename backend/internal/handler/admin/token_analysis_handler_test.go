package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestTokenAnalysisHandlerSummaryParsesDateRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &tokenAnalysisServiceStub{
		summary: &service.TokenAnalysisSummary{
			TotalRequests: 3,
			TotalTokens:   1000,
			RiskyRequests: 1,
		},
	}
	h := &TokenAnalysisHandler{service: svc}

	r := gin.New()
	r.GET("/summary", h.Summary)
	req := httptest.NewRequest(http.MethodGet, "/summary?start_date=2026-05-19&end_date=2026-05-19&timezone=Asia/Shanghai&risk_min=30", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, int64(30), svc.lastFilters.RiskMin)
	require.NotNil(t, svc.lastFilters.StartTime)
	require.NotNil(t, svc.lastFilters.EndTime)
	require.Contains(t, w.Body.String(), `"total_tokens":1000`)
}

type tokenAnalysisServiceStub struct {
	summary     *service.TokenAnalysisSummary
	lastFilters service.TokenAnalysisFilters
}

func (s *tokenAnalysisServiceStub) GetSummary(ctx context.Context, filters service.TokenAnalysisFilters) (*service.TokenAnalysisSummary, error) {
	s.lastFilters = filters
	return s.summary, nil
}

func (s *tokenAnalysisServiceStub) ListUserUsage(ctx context.Context, filters service.TokenAnalysisFilters, params pagination.PaginationParams) ([]service.TokenAnalysisUserUsage, *pagination.PaginationResult, error) {
	s.lastFilters = filters
	return []service.TokenAnalysisUserUsage{}, &pagination.PaginationResult{Page: params.Page, PageSize: params.PageSize}, nil
}

func (s *tokenAnalysisServiceStub) ListRequests(ctx context.Context, filters service.TokenAnalysisFilters, params pagination.PaginationParams) ([]service.TokenAnalysisRequestItem, *pagination.PaginationResult, error) {
	s.lastFilters = filters
	return []service.TokenAnalysisRequestItem{}, &pagination.PaginationResult{Page: params.Page, PageSize: params.PageSize}, nil
}

func (s *tokenAnalysisServiceStub) GetIndexStatus(ctx context.Context) (*service.TokenAnalysisIndexStatus, error) {
	return &service.TokenAnalysisIndexStatus{}, nil
}

func (s *tokenAnalysisServiceStub) IndexRange(ctx context.Context, req service.TokenAnalysisIndexRequest) (*service.TokenAnalysisIndexResult, error) {
	return &service.TokenAnalysisIndexResult{}, nil
}

var _ = time.Time{}
