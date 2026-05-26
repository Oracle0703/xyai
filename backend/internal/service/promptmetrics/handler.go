package promptmetrics

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

// Handler 提供管理端 Prompt Metrics API.
type Handler struct {
	service *Service
}

// NewHandler 创建管理 API handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Overview(c *gin.Context) {
	filters, ok := parseFilters(c)
	if !ok {
		return
	}
	data, err := h.service.Overview(c.Request.Context(), filters)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, data)
}

func (h *Handler) Trend(c *gin.Context) {
	filters, ok := parseFilters(c)
	if !ok {
		return
	}
	bucket := strings.ToLower(strings.TrimSpace(c.DefaultQuery("bucket", "day")))
	if bucket != "day" && bucket != "hour" {
		response.BadRequest(c, "Invalid bucket")
		return
	}
	data, err := h.service.Trend(c.Request.Context(), filters, bucket)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, data)
}

func (h *Handler) Rank(c *gin.Context) {
	filters, ok := parseFilters(c)
	if !ok {
		return
	}
	limit := parsePositiveInt(c.DefaultQuery("limit", "20"), 20)
	data, err := h.service.Rank(c.Request.Context(), filters, c.DefaultQuery("dimension", "project"), limit)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, data)
}

func (h *Handler) Events(c *gin.Context) {
	filters, ok := parseFilters(c)
	if !ok {
		return
	}
	page, pageSize := response.ParsePagination(c)
	items, pagination, err := h.service.ListEvents(c.Request.Context(), filters, page, pageSize)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Paginated(c, items, pagination.Total, pagination.Page, pagination.PageSize)
}

func (h *Handler) Event(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	event, err := h.service.EventByID(c.Request.Context(), id)
	if err != nil {
		if isNotFound(err) {
			response.NotFound(c, "Prompt event not found")
			return
		}
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, event)
}

func (h *Handler) Reanalyze(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	analysis, err := h.service.Reanalyze(c.Request.Context(), id)
	if err != nil {
		if isNotFound(err) {
			response.NotFound(c, "Prompt event not found")
			return
		}
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, analysis)
}

func parseIDParam(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid id")
		return 0, false
	}
	return id, true
}

func parseFilters(c *gin.Context) (Filters, bool) {
	var filters Filters
	if raw := strings.TrimSpace(c.Query("from")); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			response.BadRequest(c, "Invalid from")
			return filters, false
		}
		filters.From = &t
	}
	if raw := strings.TrimSpace(c.Query("to")); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			response.BadRequest(c, "Invalid to")
			return filters, false
		}
		filters.To = &t
	}
	if !parseOptionalInt64(c, "user_id", &filters.UserID) ||
		!parseOptionalInt64(c, "api_key_id", &filters.APIKeyID) ||
		!parseOptionalInt64(c, "group_id", &filters.GroupID) {
		return filters, false
	}
	filters.ProjectName = firstQuery(c, "project", "project_name")
	filters.GitBranch = firstQuery(c, "branch", "git_branch")
	filters.ClientName = firstQuery(c, "client", "client_name")
	filters.Model = strings.TrimSpace(c.Query("model"))
	filters.Endpoint = strings.TrimSpace(c.Query("endpoint"))
	filters.Hash = strings.TrimSpace(c.Query("hash"))
	if raw := strings.TrimSpace(c.Query("min_quality")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 || value > 100 {
			response.BadRequest(c, "Invalid min_quality")
			return filters, false
		}
		filters.MinQuality = &value
	}
	if raw := strings.TrimSpace(c.Query("max_quality")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 || value > 100 {
			response.BadRequest(c, "Invalid max_quality")
			return filters, false
		}
		filters.MaxQuality = &value
	}
	if raw := strings.TrimSpace(c.Query("only_low_quality")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			response.BadRequest(c, "Invalid only_low_quality")
			return filters, false
		}
		filters.OnlyLowQuality = value
	}
	return filters, true
}

func parseOptionalInt64(c *gin.Context, name string, target **int64) bool {
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

func firstQuery(c *gin.Context, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(c.Query(name)); value != "" {
			return value
		}
	}
	return ""
}

func parsePositiveInt(raw string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
