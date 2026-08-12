package admin

import (
	"context"
	"encoding/json"
	"io"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// toResponsePagination converts pagination.PaginationResult to response.PaginationResult
func toResponsePagination(p *pagination.PaginationResult) *response.PaginationResult {
	if p == nil {
		return nil
	}
	return &response.PaginationResult{
		Total:    p.Total,
		Page:     p.Page,
		PageSize: p.PageSize,
		Pages:    p.Pages,
	}
}

// SubscriptionHandler handles admin subscription management
type SubscriptionHandler struct {
	subscriptionService subscriptionHandlerService
}

type subscriptionHandlerService interface {
	ListAdmin(ctx context.Context, page, pageSize int, filter service.SubscriptionAdminFilter) ([]service.UserSubscription, *pagination.PaginationResult, error)
	GetByID(ctx context.Context, id int64) (*service.UserSubscription, error)
	GetSubscriptionProgress(ctx context.Context, subscriptionID int64) (*service.SubscriptionProgress, error)
	AssignSubscription(ctx context.Context, input *service.AssignSubscriptionInput) (*service.UserSubscription, error)
	BulkAssignSubscription(ctx context.Context, input *service.BulkAssignSubscriptionInput) (*service.BulkAssignResult, error)
	ExtendSubscription(ctx context.Context, subscriptionID int64, days int) (*service.UserSubscription, error)
	AdminResetQuota(ctx context.Context, subscriptionID int64, resetDaily, resetWeekly, resetMonthly bool) (*service.UserSubscription, error)
	AdminResetDailyFiltered(ctx context.Context, filter service.SubscriptionAdminFilter) (int, error)
	RevokeSubscription(ctx context.Context, subscriptionID int64) error
	RestoreSubscription(ctx context.Context, subscriptionID int64) (*service.UserSubscription, error)
	ListGroupSubscriptions(ctx context.Context, groupID int64, page, pageSize int) ([]service.UserSubscription, *pagination.PaginationResult, error)
	ListUserSubscriptions(ctx context.Context, userID int64) ([]service.UserSubscription, error)
}

// NewSubscriptionHandler creates a new admin subscription handler
func NewSubscriptionHandler(subscriptionService *service.SubscriptionService) *SubscriptionHandler {
	return newSubscriptionHandler(subscriptionService)
}

func newSubscriptionHandler(subscriptionService subscriptionHandlerService) *SubscriptionHandler {
	return &SubscriptionHandler{
		subscriptionService: subscriptionService,
	}
}

// AssignSubscriptionRequest represents assign subscription request
type AssignSubscriptionRequest struct {
	UserID       int64  `json:"user_id" binding:"required"`
	GroupID      int64  `json:"group_id" binding:"required"`
	ValidityDays int    `json:"validity_days" binding:"omitempty,max=36500"` // max 100 years
	Notes        string `json:"notes"`
}

// BulkAssignSubscriptionRequest represents bulk assign subscription request
type BulkAssignSubscriptionRequest struct {
	UserIDs      []int64 `json:"user_ids" binding:"required,min=1"`
	GroupID      int64   `json:"group_id" binding:"required"`
	ValidityDays int     `json:"validity_days" binding:"omitempty,max=36500"` // max 100 years
	Notes        string  `json:"notes"`
}

// AdjustSubscriptionRequest represents adjust subscription request (extend or shorten)
type AdjustSubscriptionRequest struct {
	Days int `json:"days" binding:"required,min=-36500,max=36500"` // negative to shorten, positive to extend
}

// List handles listing all subscriptions with pagination and filters
// GET /api/v1/admin/subscriptions
func (h *SubscriptionHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)

	userID, err := parseOptionalPositiveInt64(c.Query("user_id"), "user_id")
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	groupID, err := parseOptionalPositiveInt64(c.Query("group_id"), "group_id")
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	subscriptions, pagination, err := h.subscriptionService.ListAdmin(c.Request.Context(), page, pageSize, service.SubscriptionAdminFilter{
		UserID:       userID,
		GroupID:      groupID,
		Status:       c.Query("status"),
		Platform:     c.Query("platform"),
		Organization: c.Query("organization"),
		SortBy:       c.DefaultQuery("sort_by", "created_at"),
		SortOrder:    c.DefaultQuery("sort_order", "desc"),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]dto.AdminUserSubscription, 0, len(subscriptions))
	for i := range subscriptions {
		out = append(out, *dto.UserSubscriptionFromServiceAdmin(&subscriptions[i]))
	}
	response.PaginatedWithResult(c, out, toResponsePagination(pagination))
}

// GetByID handles getting a subscription by ID
// GET /api/v1/admin/subscriptions/:id
func (h *SubscriptionHandler) GetByID(c *gin.Context) {
	subscriptionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid subscription ID")
		return
	}

	subscription, err := h.subscriptionService.GetByID(c.Request.Context(), subscriptionID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.UserSubscriptionFromServiceAdmin(subscription))
}

// GetProgress handles getting subscription usage progress
// GET /api/v1/admin/subscriptions/:id/progress
func (h *SubscriptionHandler) GetProgress(c *gin.Context) {
	subscriptionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid subscription ID")
		return
	}

	progress, err := h.subscriptionService.GetSubscriptionProgress(c.Request.Context(), subscriptionID)
	if err != nil {
		response.NotFound(c, "Subscription not found")
		return
	}

	response.Success(c, progress)
}

// Assign handles assigning a subscription to a user
// POST /api/v1/admin/subscriptions/assign
func (h *SubscriptionHandler) Assign(c *gin.Context) {
	var req AssignSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// Get admin user ID from context
	adminID := getAdminIDFromContext(c)

	subscription, err := h.subscriptionService.AssignSubscription(c.Request.Context(), &service.AssignSubscriptionInput{
		UserID:       req.UserID,
		GroupID:      req.GroupID,
		ValidityDays: req.ValidityDays,
		AssignedBy:   adminID,
		Notes:        req.Notes,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.UserSubscriptionFromServiceAdmin(subscription))
}

// BulkAssign handles bulk assigning subscriptions to multiple users
// POST /api/v1/admin/subscriptions/bulk-assign
func (h *SubscriptionHandler) BulkAssign(c *gin.Context) {
	var req BulkAssignSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// Get admin user ID from context
	adminID := getAdminIDFromContext(c)

	result, err := h.subscriptionService.BulkAssignSubscription(c.Request.Context(), &service.BulkAssignSubscriptionInput{
		UserIDs:      req.UserIDs,
		GroupID:      req.GroupID,
		ValidityDays: req.ValidityDays,
		AssignedBy:   adminID,
		Notes:        req.Notes,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.BulkAssignResultFromService(result))
}

// Extend handles adjusting a subscription (extend or shorten)
// POST /api/v1/admin/subscriptions/:id/extend
func (h *SubscriptionHandler) Extend(c *gin.Context) {
	subscriptionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid subscription ID")
		return
	}

	var req AdjustSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	idempotencyPayload := struct {
		SubscriptionID int64                     `json:"subscription_id"`
		Body           AdjustSubscriptionRequest `json:"body"`
	}{
		SubscriptionID: subscriptionID,
		Body:           req,
	}
	executeAdminIdempotentJSON(c, "admin.subscriptions.extend", idempotencyPayload, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		subscription, execErr := h.subscriptionService.ExtendSubscription(ctx, subscriptionID, req.Days)
		if execErr != nil {
			return nil, execErr
		}
		return dto.UserSubscriptionFromServiceAdmin(subscription), nil
	})
}

// ResetSubscriptionQuotaRequest represents the reset quota request
type ResetSubscriptionQuotaRequest struct {
	Daily   bool `json:"daily"`
	Weekly  bool `json:"weekly"`
	Monthly bool `json:"monthly"`
}

type ResetDailyFilteredRequest struct {
	Status       string `json:"status"`
	UserID       *int64 `json:"user_id"`
	GroupID      *int64 `json:"group_id"`
	Platform     string `json:"platform"`
	Organization string `json:"organization"`
}

type ResetDailyFilteredResponse struct {
	ResetCount int `json:"reset_count"`
}

// ResetDailyFiltered resets daily usage for every active subscription matching the supplied filters.
// POST /api/v1/admin/subscriptions/reset-daily-filtered
func (h *SubscriptionHandler) ResetDailyFiltered(c *gin.Context) {
	var req *ResetDailyFilteredRequest
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		response.BadRequest(c, "Invalid request: request body must contain a single JSON object")
		return
	}
	if req == nil {
		response.BadRequest(c, "Invalid request: request body must be a JSON object")
		return
	}
	filter, err := service.NormalizeSubscriptionAdminFilter(service.SubscriptionAdminFilter{
		Status:       req.Status,
		UserID:       req.UserID,
		GroupID:      req.GroupID,
		Platform:     req.Platform,
		Organization: req.Organization,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	idempotencyKey, err := service.NormalizeIdempotencyKey(c.GetHeader("Idempotency-Key"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if idempotencyKey == "" {
		response.ErrorFrom(c, service.ErrIdempotencyKeyRequired)
		return
	}

	executeAdminIdempotentJSONAtomicSuccess(c, "admin.subscriptions.reset_daily_filtered", filter, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		count, execErr := h.subscriptionService.AdminResetDailyFiltered(ctx, filter)
		if execErr != nil {
			return nil, execErr
		}
		return ResetDailyFilteredResponse{ResetCount: count}, nil
	})
}

// ResetQuota resets daily, weekly, and/or monthly usage for a subscription.
// POST /api/v1/admin/subscriptions/:id/reset-quota
func (h *SubscriptionHandler) ResetQuota(c *gin.Context) {
	subscriptionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid subscription ID")
		return
	}
	var req ResetSubscriptionQuotaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if !req.Daily && !req.Weekly && !req.Monthly {
		response.BadRequest(c, "At least one of 'daily', 'weekly', or 'monthly' must be true")
		return
	}
	sub, err := h.subscriptionService.AdminResetQuota(c.Request.Context(), subscriptionID, req.Daily, req.Weekly, req.Monthly)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.UserSubscriptionFromServiceAdmin(sub))
}

// Revoke handles revoking a subscription.
// POST /api/v1/admin/subscriptions/:id/revoke
// DELETE /api/v1/admin/subscriptions/:id is kept for backward compatibility.
func (h *SubscriptionHandler) Revoke(c *gin.Context) {
	subscriptionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid subscription ID")
		return
	}

	err = h.subscriptionService.RevokeSubscription(c.Request.Context(), subscriptionID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{"message": "Subscription revoked successfully"})
}

// Restore handles restoring a revoked subscription.
// POST /api/v1/admin/subscriptions/:id/restore
func (h *SubscriptionHandler) Restore(c *gin.Context) {
	subscriptionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid subscription ID")
		return
	}

	subscription, err := h.subscriptionService.RestoreSubscription(c.Request.Context(), subscriptionID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.UserSubscriptionFromServiceAdmin(subscription))
}

// ListByGroup handles listing subscriptions for a specific group
// GET /api/v1/admin/groups/:id/subscriptions
func (h *SubscriptionHandler) ListByGroup(c *gin.Context) {
	groupID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid group ID")
		return
	}

	page, pageSize := response.ParsePagination(c)

	subscriptions, pagination, err := h.subscriptionService.ListGroupSubscriptions(c.Request.Context(), groupID, page, pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]dto.AdminUserSubscription, 0, len(subscriptions))
	for i := range subscriptions {
		out = append(out, *dto.UserSubscriptionFromServiceAdmin(&subscriptions[i]))
	}
	response.PaginatedWithResult(c, out, toResponsePagination(pagination))
}

// ListByUser handles listing subscriptions for a specific user
// GET /api/v1/admin/users/:id/subscriptions
func (h *SubscriptionHandler) ListByUser(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid user ID")
		return
	}

	subscriptions, err := h.subscriptionService.ListUserSubscriptions(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]dto.AdminUserSubscription, 0, len(subscriptions))
	for i := range subscriptions {
		out = append(out, *dto.UserSubscriptionFromServiceAdmin(&subscriptions[i]))
	}
	response.Success(c, out)
}

// Helper function to get admin ID from context
func getAdminIDFromContext(c *gin.Context) int64 {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		return 0
	}
	return subject.UserID
}

func parseOptionalPositiveInt64(raw, field string) (*int64, error) {
	if raw == "" {
		return nil, nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err == nil && id > 0 {
		return &id, nil
	}
	invalid := int64(0)
	if err == nil {
		invalid = id
	}
	filter := service.SubscriptionAdminFilter{}
	if field == "user_id" {
		filter.UserID = &invalid
	} else {
		filter.GroupID = &invalid
	}
	_, validationErr := service.NormalizeSubscriptionAdminFilter(filter)
	return nil, validationErr
}
