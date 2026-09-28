package admin

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// SearchSubscriptionAssignmentUsers returns current users without user-management fields.
func (h *UserHandler) SearchSubscriptionAssignmentUsers(c *gin.Context) {
	type userOption struct {
		ID    int64  `json:"id"`
		Email string `json:"email"`
	}
	result := make([]userOption, 0)
	keyword := strings.TrimSpace(c.Query("q"))
	if keyword == "" {
		response.Success(c, result)
		return
	}
	users, _, err := h.adminService.ListUsers(c.Request.Context(), 1, 30,
		service.UserListFilters{Search: keyword}, "email", "asc")
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	for _, user := range users {
		result = append(result, userOption{ID: user.ID, Email: user.Email})
	}
	response.Success(c, result)
}

// SubscriptionAssignmentGroups returns active subscription groups with only picker fields.
func (h *GroupHandler) SubscriptionAssignmentGroups(c *gin.Context) {
	groups, err := h.adminService.GetAllGroups(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	type groupOption struct {
		ID               int64   `json:"id"`
		Name             string  `json:"name"`
		Description      string  `json:"description"`
		Platform         string  `json:"platform"`
		RateMultiplier   float64 `json:"rate_multiplier"`
		SubscriptionType string  `json:"subscription_type"`
		Status           string  `json:"status"`
	}
	result := make([]groupOption, 0, len(groups))
	for _, group := range groups {
		if !group.IsSubscriptionType() {
			continue
		}
		result = append(result, groupOption{
			ID: group.ID, Name: group.Name, Description: group.Description,
			Platform: group.Platform, RateMultiplier: group.RateMultiplier,
			SubscriptionType: group.SubscriptionType, Status: group.Status,
		})
	}
	response.Success(c, result)
}
