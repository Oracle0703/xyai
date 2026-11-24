package handler

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// GPTIQTestHandler 提供降智测试的用户只读接口与"降智测试配置"管理接口。
// 用户读取只查数据库，不触发测试；管理接口的范围由服务层按操作者重读判定。
type GPTIQTestHandler struct {
	service *service.GPTIQTestService
}

func NewGPTIQTestHandler(s *service.GPTIQTestService) *GPTIQTestHandler {
	return &GPTIQTestHandler{service: s}
}

// Status GET /api/v1/gpt-quota/status：额度展示与降智测试的公开开关，供侧栏决定 /gpt-quota 菜单。
func (h *GPTIQTestHandler) Status(c *gin.Context) {
	quotaEnabled, iqEnabled, err := h.service.Status(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"enabled": quotaEnabled, "iq_test_enabled": iqEnabled})
}

// Get GET /api/v1/gpt-quota/iq-tests
func (h *GPTIQTestHandler) Get(c *gin.Context) {
	view, err := h.service.UserView(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, view)
}

// EntryDetail GET /api/v1/gpt-quota/iq-tests/entries/:entry_id?case_id=&days=
func (h *GPTIQTestHandler) EntryDetail(c *gin.Context) {
	entryID, ok := parsePositiveIDParam(c, "entry_id")
	if !ok {
		return
	}
	var caseID int64
	if raw := c.Query("case_id"); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v <= 0 {
			response.BadRequest(c, "case_id must be positive")
			return
		}
		caseID = v
	}
	days := 0
	if raw := c.Query("days"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v <= 0 {
			response.BadRequest(c, "days must be positive")
			return
		}
		days = v
	}
	detail, err := h.service.EntryDetail(c.Request.Context(), entryID, caseID, days)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, detail)
}

func (h *GPTIQTestHandler) actor(c *gin.Context) (*service.GPTIQTestActor, bool) {
	actor, err := h.service.Actor(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return nil, false
	}
	return actor, true
}

// AdminGet GET /api/v1/admin/gpt-iq-test
func (h *GPTIQTestHandler) AdminGet(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	view, err := h.service.AdminView(c.Request.Context(), actor)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, view)
}

// UpdateConfig PUT /api/v1/admin/gpt-iq-test/config
func (h *GPTIQTestHandler) UpdateConfig(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var req service.GPTIQTestConfigInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	cfg, err := h.service.SaveConfig(c.Request.Context(), actor, req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

type gptIQCaseRequest struct {
	service.GPTIQCaseInput
	ExpectedVersion int64 `json:"expected_version"`
}

// CreateCase POST /api/v1/admin/gpt-iq-test/cases
func (h *GPTIQTestHandler) CreateCase(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var req gptIQCaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	created, err := h.service.CreateCase(c.Request.Context(), actor, req.GPTIQCaseInput)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, created)
}

// UpdateCase PUT /api/v1/admin/gpt-iq-test/cases/:id
func (h *GPTIQTestHandler) UpdateCase(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	id, ok := parsePositiveIDParam(c, "id")
	if !ok {
		return
	}
	var req gptIQCaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	updated, err := h.service.UpdateCase(c.Request.Context(), actor, id, req.ExpectedVersion, req.GPTIQCaseInput)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, updated)
}

// DeleteCase DELETE /api/v1/admin/gpt-iq-test/cases/:id?expected_version=
func (h *GPTIQTestHandler) DeleteCase(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	id, ok := parsePositiveIDParam(c, "id")
	if !ok {
		return
	}
	version, err := strconv.ParseInt(c.Query("expected_version"), 10, 64)
	if err != nil || version <= 0 {
		response.BadRequest(c, "expected_version is required")
		return
	}
	if err := h.service.DeleteCase(c.Request.Context(), actor, id, version); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

// Rejudge POST /api/v1/admin/gpt-iq-test/cases/:id/rejudge
func (h *GPTIQTestHandler) Rejudge(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	id, ok := parsePositiveIDParam(c, "id")
	if !ok {
		return
	}
	// 可选 {revision_id}：页面加载时的版本，测试项此后被改过则 409，避免用意料之外的规则重评。
	var req struct {
		RevisionID int64 `json:"revision_id"`
	}
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			response.BadRequest(c, "invalid request")
			return
		}
	}
	result, err := h.service.RejudgeAt(c.Request.Context(), actor, id, req.RevisionID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// StartRun POST /api/v1/admin/gpt-iq-test/runs：{entry_ids?, case_ids?}，空为操作者范围内的全部。
func (h *GPTIQTestHandler) StartRun(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var req service.GPTIQRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	batch, err := h.service.StartRun(c.Request.Context(), actor, req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Accepted(c, batch)
}

// GetRun GET /api/v1/admin/gpt-iq-test/runs/:id
func (h *GPTIQTestHandler) GetRun(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	id, ok := parsePositiveIDParam(c, "id")
	if !ok {
		return
	}
	detail, err := h.service.BatchDetail(c.Request.Context(), actor, id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, detail)
}

// CancelRun POST /api/v1/admin/gpt-iq-test/runs/:id/cancel
func (h *GPTIQTestHandler) CancelRun(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	id, ok := parsePositiveIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.service.CancelRun(c.Request.Context(), actor, id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"cancel_requested": true})
}

func parsePositiveIDParam(c *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, name+" must be positive")
		return 0, false
	}
	return id, true
}
