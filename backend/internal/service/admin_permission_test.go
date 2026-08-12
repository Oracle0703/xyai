//go:build unit

package service

import (
	"net/http"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestNormalizeAdminPermissions(t *testing.T) {
	t.Run("sub admin permissions follow catalog order and are deduplicated", func(t *testing.T) {
		permissions, err := NormalizeAdminPermissions(RoleSubAdmin, []string{
			AdminPermissionTokenAnalysis,
			AdminPermissionSubscriptions,
			AdminPermissionTokenAnalysis,
		})
		require.NoError(t, err)
		require.Equal(t, []string{AdminPermissionSubscriptions, AdminPermissionTokenAnalysis}, permissions)
	})

	t.Run("unknown permission is rejected", func(t *testing.T) {
		_, err := NormalizeAdminPermissions(RoleSubAdmin, []string{"admin.accounts"})
		require.ErrorContains(t, err, "unknown admin permission")
		require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
	})

	t.Run("non sub admin permissions are cleared", func(t *testing.T) {
		permissions, err := NormalizeAdminPermissions(RoleUser, []string{AdminPermissionUsage})
		require.NoError(t, err)
		require.Empty(t, permissions)
	})
}

func TestCanAccessAdminRoute(t *testing.T) {
	tests := []struct {
		name        string
		permissions []string
		method      string
		route       string
		allowed     bool
	}{
		{"subscription read allowed", []string{AdminPermissionSubscriptions}, http.MethodGet, "/api/v1/admin/subscriptions", true},
		{"subscription quota reset allowed", []string{AdminPermissionSubscriptions}, http.MethodPost, "/api/v1/admin/subscriptions/:id/reset-quota", true},
		{"subscription filtered daily reset allowed", []string{AdminPermissionSubscriptions}, http.MethodPost, "/api/v1/admin/subscriptions/reset-daily-filtered", true},
		{"subscription compact group search allowed", []string{AdminPermissionSubscriptions}, http.MethodGet, "/api/v1/admin/subscriptions/search-groups", true},
		{"subscription full group catalog denied", []string{AdminPermissionSubscriptions}, http.MethodGet, "/api/v1/admin/groups/all", false},
		{"subscription assignment denied", []string{AdminPermissionSubscriptions}, http.MethodPost, "/api/v1/admin/subscriptions/assign", false},
		{"usage read allowed", []string{AdminPermissionUsage}, http.MethodGet, "/api/v1/admin/usage", true},
		{"usage compact account search allowed", []string{AdminPermissionUsage}, http.MethodGet, "/api/v1/admin/usage/search-accounts", true},
		{"usage cleanup denied", []string{AdminPermissionUsage}, http.MethodPost, "/api/v1/admin/usage/cleanup-tasks", false},
		{"token analysis read allowed", []string{AdminPermissionTokenAnalysis}, http.MethodGet, "/api/v1/admin/token-analysis/requests/input", true},
		{"token analysis index denied", []string{AdminPermissionTokenAnalysis}, http.MethodPost, "/api/v1/admin/token-analysis/index", false},
		{"account management denied", []string{AdminPermissionUsage, AdminPermissionSubscriptions, AdminPermissionTokenAnalysis}, http.MethodGet, "/api/v1/admin/accounts", false},
		{"permission catalog denied", []string{AdminPermissionUsage, AdminPermissionSubscriptions, AdminPermissionTokenAnalysis}, http.MethodGet, "/api/v1/admin/permissions/catalog", false},
		{"unknown get route defaults to denied", []string{AdminPermissionUsage}, http.MethodGet, "/api/v1/admin/future-feature", false},
		{"compliance status allowed for authenticated sub admin", nil, http.MethodGet, "/api/v1/admin/compliance", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := &User{Role: RoleSubAdmin, AdminPermissions: tt.permissions}
			require.Equal(t, tt.allowed, CanAccessAdminRoute(user, tt.method, tt.route))
		})
	}

	t.Run("admin bypasses route catalog", func(t *testing.T) {
		user := &User{Role: RoleAdmin}
		require.True(t, CanAccessAdminRoute(user, http.MethodDelete, "/api/v1/admin/accounts/:id"))
	})
}

func TestAdminPermissionCatalogReturnsDefensiveCopy(t *testing.T) {
	first := AdminPermissionCatalog()
	require.Len(t, first, 3)
	require.Equal(t, AdminPermissionSubscriptions, first[0].Code)
	first[0].Code = "changed"
	second := AdminPermissionCatalog()
	require.Equal(t, AdminPermissionSubscriptions, second[0].Code)
}

func TestCanAccessAdmin(t *testing.T) {
	require.True(t, CanAccessAdmin(&User{Role: RoleAdmin}))
	require.True(t, CanAccessAdmin(&User{Role: RoleSubAdmin, AdminPermissions: []string{AdminPermissionUsage}}))
	require.False(t, CanAccessAdmin(&User{Role: RoleSubAdmin}))
	require.False(t, CanAccessAdmin(&User{Role: RoleUser}))
}

func TestSubAdminWriteWhitelistStaysNarrow(t *testing.T) {
	allowedWrites := map[string]map[adminRouteRule]struct{}{
		AdminPermissionSubscriptions: {
			{method: http.MethodPost, route: "/api/v1/admin/subscriptions/:id/reset-quota"}:      {},
			{method: http.MethodPost, route: "/api/v1/admin/subscriptions/reset-daily-filtered"}: {},
		},
		AdminPermissionUsage:         {},
		AdminPermissionTokenAnalysis: {},
	}

	for permission, rules := range adminPermissionRouteRules {
		actualWrites := make(map[adminRouteRule]struct{})
		for _, rule := range rules {
			if rule.method != http.MethodGet {
				actualWrites[rule] = struct{}{}
			}
		}
		require.Equal(t, allowedWrites[permission], actualWrites, permission)
	}
}
