package service

import (
	"fmt"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	SubscriptionOrganizationXunyou  = "xunyou"
	SubscriptionOrganizationWsdashi = "wsdashi"
)

type SubscriptionAdminFilter struct {
	UserID       *int64
	GroupID      *int64
	Status       string
	Platform     string
	Organization string
	SortBy       string
	SortOrder    string
}

func NormalizeSubscriptionAdminFilter(filter SubscriptionAdminFilter) (SubscriptionAdminFilter, error) {
	filter.Status = strings.ToLower(strings.TrimSpace(filter.Status))
	filter.Platform = strings.ToLower(strings.TrimSpace(filter.Platform))
	filter.Organization = strings.ToLower(strings.TrimSpace(filter.Organization))
	filter.SortBy = strings.ToLower(strings.TrimSpace(filter.SortBy))
	filter.SortOrder = strings.ToLower(strings.TrimSpace(filter.SortOrder))

	if filter.UserID != nil && *filter.UserID <= 0 {
		return SubscriptionAdminFilter{}, invalidSubscriptionAdminFilter("user_id", "must be greater than zero")
	}
	if filter.GroupID != nil && *filter.GroupID <= 0 {
		return SubscriptionAdminFilter{}, invalidSubscriptionAdminFilter("group_id", "must be greater than zero")
	}
	if !subscriptionAdminFilterValueAllowed(filter.Status, "", SubscriptionStatusActive, SubscriptionStatusExpired, SubscriptionStatusRevoked, SubscriptionStatusSuspended) {
		return SubscriptionAdminFilter{}, invalidSubscriptionAdminFilter("status", "invalid status")
	}
	if !subscriptionAdminFilterValueAllowed(filter.Platform, "", PlatformAnthropic, PlatformOpenAI, PlatformGemini, PlatformAntigravity, PlatformGrok, PlatformComposite) {
		return SubscriptionAdminFilter{}, invalidSubscriptionAdminFilter("platform", "invalid platform")
	}
	if !subscriptionAdminFilterValueAllowed(filter.Organization, "", SubscriptionOrganizationXunyou, SubscriptionOrganizationWsdashi) {
		return SubscriptionAdminFilter{}, invalidSubscriptionAdminFilter("organization", "invalid organization")
	}
	if filter.SortBy == "" {
		filter.SortBy = "created_at"
	}
	if !subscriptionAdminFilterValueAllowed(filter.SortBy, "created_at", "expires_at", "status") {
		return SubscriptionAdminFilter{}, invalidSubscriptionAdminFilter("sort_by", "invalid sort_by")
	}
	if filter.SortOrder == "" {
		filter.SortOrder = "desc"
	}
	if !subscriptionAdminFilterValueAllowed(filter.SortOrder, "asc", "desc") {
		return SubscriptionAdminFilter{}, invalidSubscriptionAdminFilter("sort_order", "invalid sort_order")
	}

	return filter, nil
}

func subscriptionAdminFilterValueAllowed(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func invalidSubscriptionAdminFilter(field, message string) error {
	return infraerrors.BadRequest(
		"INVALID_SUBSCRIPTION_FILTER",
		fmt.Sprintf("%s: %s", field, message),
	).WithMetadata(map[string]string{"field": field})
}
