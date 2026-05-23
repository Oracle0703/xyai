package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type UserConcurrencyPresetHandler struct {
	service *service.UserConcurrencyPresetService
}

func NewUserConcurrencyPresetHandler(service *service.UserConcurrencyPresetService) *UserConcurrencyPresetHandler {
	return &UserConcurrencyPresetHandler{service: service}
}

type userConcurrencyPresetRequest struct {
	Name              string  `json:"name"`
	Description       string  `json:"description"`
	TargetConcurrency int     `json:"target_concurrency"`
	UserIDs           []int64  `json:"user_ids"`
	ScheduleEnabled   bool    `json:"schedule_enabled"`
	ScheduleTime      string  `json:"schedule_time"`
}

func (h *UserConcurrencyPresetHandler) List(c *gin.Context) {
	presets, err := h.service.ListPresets(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, presets)
}

func (h *UserConcurrencyPresetHandler) Create(c *gin.Context) {
	var req userConcurrencyPresetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	created, err := h.service.CreatePreset(c.Request.Context(), requestToUserConcurrencyPreset(req))
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, created)
}

func (h *UserConcurrencyPresetHandler) Update(c *gin.Context) {
	id, ok := parsePresetID(c)
	if !ok {
		return
	}
	var req userConcurrencyPresetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	updated, err := h.service.UpdatePreset(c.Request.Context(), id, requestToUserConcurrencyPreset(req))
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, updated)
}

func (h *UserConcurrencyPresetHandler) Delete(c *gin.Context) {
	id, ok := parsePresetID(c)
	if !ok {
		return
	}
	if err := h.service.DeletePreset(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "deleted"})
}

func (h *UserConcurrencyPresetHandler) Apply(c *gin.Context) {
	id, ok := parsePresetID(c)
	if !ok {
		return
	}
	run, err := h.service.ApplyPreset(c.Request.Context(), id, service.UserConcurrencyPresetTriggerManual)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, run)
}

func (h *UserConcurrencyPresetHandler) ListRuns(c *gin.Context) {
	id, ok := parsePresetID(c)
	if !ok {
		return
	}
	limit := 20
	if raw := c.Query("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}
	runs, err := h.service.ListRuns(c.Request.Context(), id, limit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, runs)
}

func parsePresetID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid preset ID")
		return 0, false
	}
	return id, true
}

func requestToUserConcurrencyPreset(req userConcurrencyPresetRequest) *service.UserConcurrencyPreset {
	return &service.UserConcurrencyPreset{
		Name:              req.Name,
		Description:       req.Description,
		TargetConcurrency: req.TargetConcurrency,
		UserIDs:           req.UserIDs,
		ScheduleEnabled:   req.ScheduleEnabled,
		ScheduleTime:      req.ScheduleTime,
	}
}
