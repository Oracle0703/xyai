package admin

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type organizationUsageService interface {
	Summary(context.Context, service.OrganizationUsageSummaryQuery) (*service.OrganizationUsageSummaryResponse, error)
	Periods(context.Context, service.OrganizationUsagePeriodsQuery) (*service.OrganizationUsagePeriodsResponse, error)
	Trend(context.Context, service.OrganizationUsageTrendQuery) (*service.OrganizationUsageTrendResponse, error)
}

type OrganizationUsageHandler struct {
	service organizationUsageService
}

func NewOrganizationUsageHandler(usageService *service.OrganizationUsageService) *OrganizationUsageHandler {
	return &OrganizationUsageHandler{service: usageService}
}

func (h *OrganizationUsageHandler) Summary(c *gin.Context) {
	page, pageSize, ok := parseOrganizationUsagePagination(c)
	if !ok {
		return
	}
	result, err := h.service.Summary(c.Request.Context(), service.OrganizationUsageSummaryQuery{
		StartDate: c.Query("start_date"), EndDate: c.Query("end_date"), AsOf: c.Query("as_of"),
		Organization: c.Query("organization"), Q: c.Query("q"),
		Page: page, PageSize: pageSize,
		SortBy: c.Query("sort_by"), SortOrder: c.Query("sort_order"),
	})
	if writeOrganizationUsageError(c, err) {
		return
	}
	response.Success(c, result)
}

func (h *OrganizationUsageHandler) Periods(c *gin.Context) {
	page, pageSize, ok := parseOrganizationUsagePagination(c)
	if !ok {
		return
	}
	result, err := h.service.Periods(c.Request.Context(), service.OrganizationUsagePeriodsQuery{
		StartDate: c.Query("start_date"), EndDate: c.Query("end_date"), AsOf: c.Query("as_of"),
		Organization: c.Query("organization"), Q: c.Query("q"),
		Page: page, PageSize: pageSize, Granularity: c.Query("granularity"),
	})
	if writeOrganizationUsageError(c, err) {
		return
	}
	response.Success(c, result)
}

func (h *OrganizationUsageHandler) Trend(c *gin.Context) {
	result, err := h.service.Trend(c.Request.Context(), service.OrganizationUsageTrendQuery{
		StartDate: c.Query("start_date"), EndDate: c.Query("end_date"), AsOf: c.Query("as_of"),
		Organization: c.Query("organization"), Q: c.Query("q"),
		Granularity: c.Query("granularity"),
	})
	if writeOrganizationUsageError(c, err) {
		return
	}
	response.Success(c, result)
}

func parseOrganizationUsagePagination(c *gin.Context) (int, int, bool) {
	page, ok := parseOptionalOrganizationUsageInt(c.Query("page"), "page", c)
	if !ok {
		return 0, 0, false
	}
	pageSize, ok := parseOptionalOrganizationUsageInt(c.Query("page_size"), "page_size", c)
	if !ok {
		return 0, 0, false
	}
	return page, pageSize, true
}

func parseOptionalOrganizationUsageInt(raw, field string, c *gin.Context) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, true
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		response.BadRequest(c, "invalid "+field)
		return 0, false
	}
	return value, true
}

func writeOrganizationUsageError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	var validationErr *service.OrganizationUsageValidationError
	if errors.As(err, &validationErr) {
		response.BadRequest(c, validationErr.Error())
		return true
	}
	response.ErrorFrom(c, err)
	return true
}
