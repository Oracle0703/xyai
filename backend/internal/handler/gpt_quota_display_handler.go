package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// GPTQuotaDisplayHandler 提供 GPT 账号额度共享展示的用户只读接口与完整管理员配置接口。
// 所有读取只查数据库快照，不触发上游采集。
type GPTQuotaDisplayHandler struct {
	service *service.GPTQuotaDisplayService
}

func NewGPTQuotaDisplayHandler(s *service.GPTQuotaDisplayService) *GPTQuotaDisplayHandler {
	return &GPTQuotaDisplayHandler{service: s}
}

// Status GET /api/v1/gpt-quota/status：供侧栏决定是否显示菜单。
func (h *GPTQuotaDisplayHandler) Status(c *gin.Context) {
	enabled, err := h.service.Enabled(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"enabled": enabled})
}

// Get GET /api/v1/gpt-quota
func (h *GPTQuotaDisplayHandler) Get(c *gin.Context) {
	view, err := h.service.UserView(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, view)
}

// AdminGet GET /api/v1/admin/gpt-quota
func (h *GPTQuotaDisplayHandler) AdminGet(c *gin.Context) {
	view, err := h.service.AdminView(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, view)
}

// AdminCandidates GET /api/v1/admin/gpt-quota/candidates?search=&page=&page_size=
func (h *GPTQuotaDisplayHandler) AdminCandidates(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	result, err := h.service.Candidates(c.Request.Context(), c.Query("search"), page, pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// UpdateConfig PUT /api/v1/admin/gpt-quota/config
func (h *GPTQuotaDisplayHandler) UpdateConfig(c *gin.Context) {
	var req service.GPTQuotaSaveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	var updatedBy int64
	if subject, ok := middleware.GetAuthSubjectFromContext(c); ok {
		updatedBy = subject.UserID
	}
	result, err := h.service.SaveConfig(c.Request.Context(), req, updatedBy)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

type gptQuotaRefreshRequest struct {
	EntryID *int64 `json:"entry_id"`
	All     bool   `json:"all"`
}

// Refresh POST /api/v1/admin/gpt-quota/refresh
// {"entry_id": n} 同步刷新单个展示条目；{"all": true} 异步刷新全部展示条目。
func (h *GPTQuotaDisplayHandler) Refresh(c *gin.Context) {
	var req gptQuotaRefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	switch {
	case req.EntryID != nil && !req.All:
		if *req.EntryID <= 0 {
			response.BadRequest(c, "entry_id must be positive")
			return
		}
		result, err := h.service.RefreshEntry(c.Request.Context(), *req.EntryID)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		response.Success(c, result)
	case req.EntryID == nil && req.All:
		batch, err := h.service.RefreshAll(c.Request.Context())
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		response.Accepted(c, batch)
	default:
		response.BadRequest(c, "specify exactly one of entry_id or all")
	}
}
