package handler

import (
	"context"
	"encoding/json"
	"io"
	"strconv"
	"strings"
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

// ListEvents GET /api/v1/admin/subscriptions/self-reset-events
func (h *SubscriptionSelfResetHandler) ListEvents(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	pageSize = min(pageSize, 200)
	filter := service.SubscriptionSelfResetEventFilter{
		Email: strings.TrimSpace(c.Query("email")), Organization: strings.TrimSpace(c.Query("organization")),
		StartDate: strings.TrimSpace(c.Query("start_date")), EndDate: strings.TrimSpace(c.Query("end_date")),
		Page: page, PageSize: pageSize,
	}
	if raw := strings.TrimSpace(c.Query("group_id")); raw != "" {
		groupID, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || groupID <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
		}
		filter.GroupID = groupID
	}
	switch filter.Organization {
	case "", service.OrganizationXunyou, service.OrganizationWsdashi, service.OrganizationOther:
	default:
		response.BadRequest(c, "Invalid organization")
		return
	}
	for _, date := range []string{filter.StartDate, filter.EndDate} {
		if _, err := time.Parse(time.DateOnly, date); date != "" && err != nil {
			response.BadRequest(c, "Invalid date, expect YYYY-MM-DD")
			return
		}
	}
	events, total, err := h.service.ListEvents(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, events, total, page, pageSize)
}

func (h *SubscriptionSelfResetHandler) GetGroups(c *gin.Context) {
	groups, err := h.service.Groups(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"groups_by_organization": groups})
}

func (h *SubscriptionSelfResetHandler) GetEventGroups(c *gin.Context) {
	groups, err := h.service.EventGroups(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"event_groups": groups})
}

func parseSelfResetGroupChanges(raw map[string]json.RawMessage) (map[string]service.SubscriptionSelfResetGroupChanges, error) {
	if len(raw) == 0 {
		return nil, service.ErrInvalidSelfResetGroups
	}
	result := make(map[string]service.SubscriptionSelfResetGroupChanges, len(raw))
	for organization, encoded := range raw {
		if organization != service.OrganizationXunyou && organization != service.OrganizationWsdashi && organization != service.OrganizationOther {
			return nil, service.ErrInvalidSelfResetGroups
		}
		if len(encoded) == 0 || string(encoded) == "null" {
			return nil, service.ErrInvalidSelfResetGroups
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(encoded, &fields); err != nil || fields == nil {
			return nil, service.ErrInvalidSelfResetGroups
		}
		var change service.SubscriptionSelfResetGroupChanges
		for field, value := range fields {
			if field != "enable" && field != "disable" {
				return nil, service.ErrInvalidSelfResetGroups
			}
			var objectValue map[string]any
			if len(value) == 0 || string(value) == "null" || json.Unmarshal(value, &objectValue) == nil {
				return nil, service.ErrInvalidSelfResetGroups
			}
			var ids []int64
			if err := json.Unmarshal(value, &ids); err != nil || len(ids) > service.MaxSelfResetGroupChanges {
				return nil, service.ErrInvalidSelfResetGroups
			}
			seen := make(map[int64]struct{}, len(ids))
			for _, id := range ids {
				if id <= 0 {
					return nil, service.ErrInvalidSelfResetGroups
				}
				if _, exists := seen[id]; exists {
					return nil, service.ErrInvalidSelfResetGroups
				}
				seen[id] = struct{}{}
			}
			if field == "enable" {
				change.Enable = ids
			} else {
				change.Disable = ids
			}
		}
		// An organization listed without any group ID is not a change.
		if len(change.Enable) == 0 && len(change.Disable) == 0 {
			return nil, service.ErrInvalidSelfResetGroups
		}
		if len(change.Enable) > 0 && len(change.Disable) > 0 {
			disabled := make(map[int64]struct{}, len(change.Disable))
			for _, id := range change.Disable {
				disabled[id] = struct{}{}
			}
			for _, id := range change.Enable {
				if _, exists := disabled[id]; exists {
					return nil, service.ErrInvalidSelfResetGroups
				}
			}
		}
		result[organization] = change
	}
	return result, nil
}

func (h *SubscriptionSelfResetHandler) GetPolicy(c *gin.Context) {
	policy, err := h.service.Policy(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, policy)
}
