package admin

import (
	"context"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type tokenAnalysisService interface {
	GetSummary(ctx context.Context, filters service.TokenAnalysisFilters) (*service.TokenAnalysisSummary, error)
	ListUserUsage(ctx context.Context, filters service.TokenAnalysisFilters, params pagination.PaginationParams) ([]service.TokenAnalysisUserUsage, *pagination.PaginationResult, error)
	ListProjectUsage(ctx context.Context, filters service.TokenAnalysisFilters, params pagination.PaginationParams) ([]service.TokenAnalysisProjectUsage, *pagination.PaginationResult, error)
	ListRequests(ctx context.Context, filters service.TokenAnalysisFilters, params pagination.PaginationParams) ([]service.TokenAnalysisRequestItem, *pagination.PaginationResult, error)
	GetIndexStatus(ctx context.Context) (*service.TokenAnalysisIndexStatus, error)
	IndexRange(ctx context.Context, req service.TokenAnalysisIndexRequest) (*service.TokenAnalysisIndexResult, error)
}

type TokenAnalysisHandler struct {
	service tokenAnalysisService
}

func NewTokenAnalysisHandler(service *service.TokenAnalysisService) *TokenAnalysisHandler {
	return &TokenAnalysisHandler{service: service}
}

func (h *TokenAnalysisHandler) Summary(c *gin.Context) {
	filters, ok := h.parseFilters(c)
	if !ok {
		return
	}
	summary, err := h.service.GetSummary(c.Request.Context(), filters)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, summary)
}

func (h *TokenAnalysisHandler) Users(c *gin.Context) {
	filters, ok := h.parseFilters(c)
	if !ok {
		return
	}
	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    c.DefaultQuery("sort_by", "actual_cost"),
		SortOrder: c.DefaultQuery("sort_order", "desc"),
	}
	items, result, err := h.service.ListUserUsage(c.Request.Context(), filters, params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, result.Total, result.Page, result.PageSize)
}

// Projects 返回"项目 × 成员"维度的 token 消耗聚合。
func (h *TokenAnalysisHandler) Projects(c *gin.Context) {
	filters, ok := h.parseFilters(c)
	if !ok {
		return
	}
	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    c.DefaultQuery("sort_by", "total_tokens"),
		SortOrder: c.DefaultQuery("sort_order", "desc"),
	}
	items, result, err := h.service.ListProjectUsage(c.Request.Context(), filters, params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, result.Total, result.Page, result.PageSize)
}

func (h *TokenAnalysisHandler) Requests(c *gin.Context) {
	filters, ok := h.parseFilters(c)
	if !ok {
		return
	}
	page, pageSize := response.ParsePagination(c)
	sortBy := c.DefaultQuery("sort_by", "risk_score")
	switch sortBy {
	case "event_time", "risk_score", "total_tokens", "actual_cost":
	default:
		response.BadRequest(c, "Invalid sort_by")
		return
	}
	params := pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    sortBy,
		SortOrder: c.DefaultQuery("sort_order", "desc"),
	}
	items, result, err := h.service.ListRequests(c.Request.Context(), filters, params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, result.Total, result.Page, result.PageSize)
}

func (h *TokenAnalysisHandler) TriggerIndex(c *gin.Context) {
	var req service.TokenAnalysisIndexRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	result, err := h.service.IndexRange(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *TokenAnalysisHandler) IndexStatus(c *gin.Context) {
	status, err := h.service.GetIndexStatus(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, status)
}

func (h *TokenAnalysisHandler) parseFilters(c *gin.Context) (service.TokenAnalysisFilters, bool) {
	var filters service.TokenAnalysisFilters
	userTZ := c.Query("timezone")
	if raw := strings.TrimSpace(c.Query("start_date")); raw != "" {
		t, err := timezone.ParseInUserLocation("2006-01-02", raw, userTZ)
		if err != nil {
			response.BadRequest(c, "Invalid start_date format, use YYYY-MM-DD")
			return filters, false
		}
		filters.StartTime = &t
	}
	if raw := strings.TrimSpace(c.Query("end_date")); raw != "" {
		t, err := timezone.ParseInUserLocation("2006-01-02", raw, userTZ)
		if err != nil {
			response.BadRequest(c, "Invalid end_date format, use YYYY-MM-DD")
			return filters, false
		}
		t = t.AddDate(0, 0, 1)
		filters.EndTime = &t
	}
	if !parseOptionalInt64Query(c, "user_id", &filters.UserID) ||
		!parseOptionalInt64Query(c, "api_key_id", &filters.APIKeyID) ||
		!parseOptionalInt64Query(c, "account_id", &filters.AccountID) ||
		!parseOptionalInt64Query(c, "group_id", &filters.GroupID) {
		return filters, false
	}
	filters.Model = strings.TrimSpace(c.Query("model"))
	filters.Endpoint = strings.TrimSpace(c.Query("endpoint"))
	filters.RiskReason = strings.TrimSpace(c.Query("risk_reason"))
	filters.Project = strings.TrimSpace(c.Query("project"))
	if raw := strings.TrimSpace(c.Query("risk_min")); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v < 0 || v > 100 {
			response.BadRequest(c, "Invalid risk_min")
			return filters, false
		}
		filters.RiskMin = v
	}
	if raw := strings.TrimSpace(c.Query("include_unmatched")); raw != "" {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			response.BadRequest(c, "Invalid include_unmatched")
			return filters, false
		}
		filters.IncludeUnmatched = v
	}
	return filters, true
}

func parseOptionalInt64Query(c *gin.Context, name string, target **int64) bool {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return true
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		response.BadRequest(c, "Invalid "+name)
		return false
	}
	*target = &value
	return true
}
