//go:build unit

package service

import (
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestNormalizeSubscriptionAdminFilter(t *testing.T) {
	userID, groupID := int64(11), int64(21)

	got, err := NormalizeSubscriptionAdminFilter(SubscriptionAdminFilter{
		UserID:       &userID,
		GroupID:      &groupID,
		Status:       " ACTIVE ",
		Platform:     " OPENAI ",
		Organization: " XUNYOU ",
		SortBy:       " expires_at ",
		SortOrder:    " ASC ",
	})

	require.NoError(t, err)
	require.Same(t, &userID, got.UserID)
	require.Same(t, &groupID, got.GroupID)
	require.Equal(t, SubscriptionStatusActive, got.Status)
	require.Equal(t, PlatformOpenAI, got.Platform)
	require.Equal(t, SubscriptionOrganizationXunyou, got.Organization)
	require.Equal(t, "expires_at", got.SortBy)
	require.Equal(t, "asc", got.SortOrder)
}

func TestNormalizeSubscriptionAdminFilterDefaultsSort(t *testing.T) {
	got, err := NormalizeSubscriptionAdminFilter(SubscriptionAdminFilter{})

	require.NoError(t, err)
	require.Equal(t, "created_at", got.SortBy)
	require.Equal(t, "desc", got.SortOrder)
}

func TestNormalizeSubscriptionAdminFilterRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name   string
		filter SubscriptionAdminFilter
	}{
		{name: "organization", filter: SubscriptionAdminFilter{Organization: "unknown"}},
		{name: "status", filter: SubscriptionAdminFilter{Status: "unknown"}},
		{name: "platform", filter: SubscriptionAdminFilter{Platform: "unknown"}},
		{name: "sort field", filter: SubscriptionAdminFilter{SortBy: "unknown"}},
		{name: "sort order", filter: SubscriptionAdminFilter{SortOrder: "sideways"}},
		{name: "non-positive user", filter: SubscriptionAdminFilter{UserID: subscriptionFilterInt64Ptr(0)}},
		{name: "non-positive group", filter: SubscriptionAdminFilter{GroupID: subscriptionFilterInt64Ptr(-1)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NormalizeSubscriptionAdminFilter(tt.filter)

			require.Error(t, err)
			require.Equal(t, 400, infraerrors.Code(err))
		})
	}
}

func subscriptionFilterInt64Ptr(value int64) *int64 {
	return &value
}
