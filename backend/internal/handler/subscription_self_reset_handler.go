package handler

import (
	"context"
	"encoding/json"
	"io"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type SubscriptionSelfResetHandler struct {
	service *service.SubscriptionSelfResetService
}

func NewSubscriptionSelfResetHandler(s *service.SubscriptionSelfResetService) *SubscriptionSelfResetHandler {
	return &SubscriptionSelfResetHandler{service: s}
}

func (h *SubscriptionSelfResetHandler) Status(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "Authentication required")
		return
	}
	role, _ := middleware.GetUserRoleFromContext(c)
	result, err := h.service.Status(c.Request.Context(), subject.UserID, role)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *SubscriptionSelfResetHandler) Reset(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "Authentication required")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid subscription ID")
		return
	}
	var input *struct {
		QuotaDate string `json:"quota_date"`
	}
	decoder := json.NewDecoder(io.LimitReader(c.Request.Body, 4097))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || input == nil {
		response.BadRequest(c, "Invalid reset request")
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		response.BadRequest(c, "Expected one JSON object")
		return
	}
	if _, err := time.Parse(time.DateOnly, input.QuotaDate); err != nil {
		response.BadRequest(c, "Invalid quota_date")
		return
	}
	key, err := service.NormalizeIdempotencyKey(c.GetHeader("Idempotency-Key"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if key == "" {
		response.ErrorFrom(c, service.ErrIdempotencyKeyRequired)
		return
	}
	coordinator := service.DefaultIdempotencyCoordinator()
	if coordinator == nil {
		response.ErrorFrom(c, service.ErrIdempotencyStoreUnavail)
		return
	}
	role, _ := middleware.GetUserRoleFromContext(c)
	result, err := coordinator.Execute(c.Request.Context(), service.IdempotencyExecuteOptions{
		Scope: service.SubscriptionSelfResetIdempotencyScope(subject.UserID), ActorScope: "user:" + strconv.FormatInt(subject.UserID, 10), Method: c.Request.Method, Route: c.FullPath(),
		IdempotencyKey: key, Payload: struct {
			SubscriptionID int64  `json:"subscription_id"`
			QuotaDate      string `json:"quota_date"`
		}{id, input.QuotaDate},
		RequireKey: true, AtomicSuccess: true, TTL: 48 * time.Hour,
	}, func(ctx context.Context) (any, error) {
		return h.service.Reset(ctx, subject.UserID, id, role, input.QuotaDate)
	})
	if err != nil {
		if retry := service.RetryAfterSecondsFromError(err); retry > 0 {
			c.Header("Retry-After", strconv.Itoa(retry))
		}
		response.ErrorFrom(c, err)
		return
	}
	if result.Replayed {
		c.Header("X-Idempotency-Replayed", "true")
	}
	response.Success(c, result.Data)
}

func (h *SubscriptionSelfResetHandler) GetPolicy(c *gin.Context) {
	policy, err := h.service.Policy(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, policy)
}

func (h *SubscriptionSelfResetHandler) SetPolicy(c *gin.Context) {
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 4097))
	if err != nil || len(raw) > 4096 {
		response.BadRequest(c, "Invalid policy request")
		return
	}
	policy, err := h.service.SetPolicy(c.Request.Context(), raw)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, policy)
}
