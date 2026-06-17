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
	GetUserInput(ctx context.Context, archiveID string) (*service.TokenAnalysisUserInput, error)
	ListArchiveFiles(ctx context.Context) ([]service.TokenAnalysisArchiveFile, error)
	GetIndexStatus(ctx context.Context) (*service.TokenAnalysisIndexStatus, error)
	IndexRangeAsync(req service.TokenAnalysisIndexRequest) error
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

// RequestInput 按 archive_id 返回留存的用户净输入全文(列表只带预览, 全文懒加载)。
func (h *TokenAnalysisHandler) RequestInput(c *gin.Context) {
	archiveID := strings.TrimSpace(c.Query("archive_id"))
	if archiveID == "" {
		response.BadRequest(c, "archive_id is required")
		return
	}
	input, err := h.service.GetUserInput(c.Request.Context(), archiveID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, input)
}

// TriggerIndex 异步触发索引并立即返回 202: 大范围回扫可达分钟级, 同步等待
// 会撞前端 30s 超时; 进度由前端轮询 IndexStatus 获取。
func (h *TokenAnalysisHandler) TriggerIndex(c *gin.Context) {
	var req service.TokenAnalysisIndexRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	if err := h.service.IndexRangeAsync(req); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Accepted(c, nil)
}

// ArchiveFiles 列出归档目录 JSONL 文件与索引水位, 已入库完成的文件打可删除标签。
func (h *TokenAnalysisHandler) ArchiveFiles(c *gin.Context) {
	files, err := h.service.ListArchiveFiles(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, files)
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
	filters.UserEmail = strings.TrimSpace(c.Query("user_email"))
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
