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
		{"subscription assignment allowed", []string{AdminPermissionSubscriptions}, http.MethodPost, "/api/v1/admin/subscriptions/assign", true},
		{"subscription bulk assignment allowed", []string{AdminPermissionSubscriptions}, http.MethodPost, "/api/v1/admin/subscriptions/bulk-assign", true},
		{"subscription assignment group options allowed", []string{AdminPermissionSubscriptions}, http.MethodGet, "/api/v1/admin/subscriptions/assignable-groups", true},
		{"subscription assignment user search allowed", []string{AdminPermissionSubscriptions}, http.MethodGet, "/api/v1/admin/subscriptions/search-users", true},
		{"subscription full user catalog denied", []string{AdminPermissionSubscriptions}, http.MethodGet, "/api/v1/admin/users", false},
		{"assignment without permission denied", nil, http.MethodPost, "/api/v1/admin/subscriptions/assign", false},
		{"usage permission cannot assign", []string{AdminPermissionUsage}, http.MethodPost, "/api/v1/admin/subscriptions/assign", false},
		{"usage permission cannot bulk assign", []string{AdminPermissionUsage}, http.MethodPost, "/api/v1/admin/subscriptions/bulk-assign", false},
		{"usage permission cannot read assignment groups", []string{AdminPermissionUsage}, http.MethodGet, "/api/v1/admin/subscriptions/assignable-groups", false},
		{"usage permission cannot search assignment users", []string{AdminPermissionUsage}, http.MethodGet, "/api/v1/admin/subscriptions/search-users", false},
		{"subscription extend denied", []string{AdminPermissionSubscriptions}, http.MethodPost, "/api/v1/admin/subscriptions/:id/extend", false},
		{"subscription revoke denied", []string{AdminPermissionSubscriptions}, http.MethodPost, "/api/v1/admin/subscriptions/:id/revoke", false},
		{"subscription restore denied", []string{AdminPermissionSubscriptions}, http.MethodPost, "/api/v1/admin/subscriptions/:id/restore", false},
		{"subscription delete denied", []string{AdminPermissionSubscriptions}, http.MethodDelete, "/api/v1/admin/subscriptions/:id", false},
		{"subscription bulk management denied", []string{AdminPermissionSubscriptions}, http.MethodPost, "/api/v1/admin/subscriptions/bulk-action", false},
		{"usage read allowed", []string{AdminPermissionUsage}, http.MethodGet, "/api/v1/admin/usage", true},
		{"usage compact account search allowed", []string{AdminPermissionUsage}, http.MethodGet, "/api/v1/admin/usage/search-accounts", true},
		{"usage cleanup denied", []string{AdminPermissionUsage}, http.MethodPost, "/api/v1/admin/usage/cleanup-tasks", false},
		{"token analysis read allowed", []string{AdminPermissionTokenAnalysis}, http.MethodGet, "/api/v1/admin/token-analysis/requests/input", true},
		{"token analysis index denied", []string{AdminPermissionTokenAnalysis}, http.MethodPost, "/api/v1/admin/token-analysis/index", false},
		{"account management denied", []string{AdminPermissionUsage, AdminPermissionSubscriptions, AdminPermissionTokenAnalysis}, http.MethodGet, "/api/v1/admin/accounts", false},
		{"permission catalog denied", []string{AdminPermissionUsage, AdminPermissionSubscriptions, AdminPermissionTokenAnalysis}, http.MethodGet, "/api/v1/admin/permissions/catalog", false},
		{"unknown get route defaults to denied", []string{AdminPermissionUsage}, http.MethodGet, "/api/v1/admin/future-feature", false},
		{"compliance status allowed for authenticated sub admin", nil, http.MethodGet, "/api/v1/admin/compliance", true},
		{"accounts operator list allowed", []string{AdminPermissionAccountsOperator}, http.MethodGet, "/api/v1/admin/accounts", true},
		{"accounts operator compact form options allowed", []string{AdminPermissionAccountsOperator}, http.MethodGet, "/api/v1/admin/accounts/form-options", true},
		{"accounts operator create allowed", []string{AdminPermissionAccountsOperator}, http.MethodPost, "/api/v1/admin/accounts", true},
		{"accounts operator test allowed", []string{AdminPermissionAccountsOperator}, http.MethodPost, "/api/v1/admin/accounts/:id/test", true},
		{"accounts operator reauth apply allowed", []string{AdminPermissionAccountsOperator}, http.MethodPost, "/api/v1/admin/accounts/:id/apply-oauth-credentials", true},
		{"accounts operator oauth exchange allowed", []string{AdminPermissionAccountsOperator}, http.MethodPost, "/api/v1/admin/openai/exchange-code", true},
		{"accounts operator full update denied", []string{AdminPermissionAccountsOperator}, http.MethodPut, "/api/v1/admin/accounts/:id", false},
		{"accounts operator delete denied", []string{AdminPermissionAccountsOperator}, http.MethodDelete, "/api/v1/admin/accounts/:id", false},
		{"accounts operator duplicate denied", []string{AdminPermissionAccountsOperator}, http.MethodPost, "/api/v1/admin/accounts/:id/duplicate", false},
		{"accounts operator schedulable toggle denied", []string{AdminPermissionAccountsOperator}, http.MethodPost, "/api/v1/admin/accounts/:id/schedulable", false},
		{"accounts operator token refresh denied", []string{AdminPermissionAccountsOperator}, http.MethodPost, "/api/v1/admin/accounts/:id/refresh", false},
		{"accounts operator clear error denied", []string{AdminPermissionAccountsOperator}, http.MethodPost, "/api/v1/admin/accounts/:id/clear-error", false},
		{"accounts operator recover state denied", []string{AdminPermissionAccountsOperator}, http.MethodPost, "/api/v1/admin/accounts/:id/recover-state", false},
		{"accounts operator bulk update denied", []string{AdminPermissionAccountsOperator}, http.MethodPost, "/api/v1/admin/accounts/bulk-update", false},
		{"accounts operator batch create denied", []string{AdminPermissionAccountsOperator}, http.MethodPost, "/api/v1/admin/accounts/batch", false},
		{"accounts operator crs sync denied", []string{AdminPermissionAccountsOperator}, http.MethodPost, "/api/v1/admin/accounts/sync/crs", false},
		{"accounts operator data export denied", []string{AdminPermissionAccountsOperator}, http.MethodGet, "/api/v1/admin/accounts/data", false},
		{"accounts operator upstream model sync denied", []string{AdminPermissionAccountsOperator}, http.MethodPost, "/api/v1/admin/accounts/:id/models/sync-upstream", false},
		{"accounts operator redacted account detail allowed", []string{AdminPermissionAccountsOperator}, http.MethodGet, "/api/v1/admin/accounts/:id", true},
		{"accounts operator full proxy catalog denied", []string{AdminPermissionAccountsOperator}, http.MethodGet, "/api/v1/admin/proxies/all", false},
		{"accounts operator full group catalog denied", []string{AdminPermissionAccountsOperator}, http.MethodGet, "/api/v1/admin/groups/all", false},
		{"accounts operator settings denied", []string{AdminPermissionAccountsOperator}, http.MethodGet, "/api/v1/admin/settings/web-search-emulation", false},
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
	require.Len(t, first, 7)
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
	for _, permission := range AdminPermissionCatalog() {
		user := &User{Role: RoleSubAdmin, AdminPermissions: []string{permission.Code}}
		for _, method := range []string{http.MethodGet, http.MethodPut} {
			require.False(t, CanAccessAdminRoute(user, method, "/api/v1/admin/subscriptions/self-reset-policy"))
		}
	}
	allowedWrites := map[string]map[adminRouteRule]struct{}{
		AdminPermissionDepartmentMembers: {
			{method: http.MethodPost, route: "/api/v1/admin/department-members/assign"}: {},
			{method: http.MethodPut, route: "/api/v1/admin/department-members/:id"}:     {},
		},
		AdminPermissionOrganizationUsage: {},
		AdminPermissionDepartmentSubscriptions: {
			{method: http.MethodPost, route: "/api/v1/admin/subscriptions/:id/reset-quota"}:      {},
			{method: http.MethodPost, route: "/api/v1/admin/subscriptions/reset-daily-filtered"}: {},
		},
		AdminPermissionSubscriptions: {
			{method: http.MethodPost, route: "/api/v1/admin/subscriptions/assign"}:               {},
			{method: http.MethodPost, route: "/api/v1/admin/subscriptions/bulk-assign"}:          {},
			{method: http.MethodPost, route: "/api/v1/admin/subscriptions/:id/reset-quota"}:      {},
			{method: http.MethodPost, route: "/api/v1/admin/subscriptions/reset-daily-filtered"}: {},
		},
		AdminPermissionUsage:         {},
		AdminPermissionTokenAnalysis: {},
		AdminPermissionAccountsOperator: accountOperatorRuleSet(func(kind AccountOperatorActivityKind) bool {
			return true
		}),
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

func accountOperatorRuleSet(include func(kind AccountOperatorActivityKind) bool) map[adminRouteRule]struct{} {
	out := make(map[adminRouteRule]struct{})
	for rule, kind := range accountOperatorActivityRoutes {
		if include(kind) {
			out[rule] = struct{}{}
		}
	}
	return out
}

func TestAccountOperatorWritesAreClassifiedForNotification(t *testing.T) {
	writes := make(map[adminRouteRule]struct{})
	for _, rule := range adminPermissionRouteRules[AdminPermissionAccountsOperator] {
		if rule.method != http.MethodGet {
			writes[rule] = struct{}{}
		}
	}
	// 白名单里的每个写路由都必须声明通知方式，反之亦然。
	require.Equal(t, accountOperatorRuleSet(func(AccountOperatorActivityKind) bool { return true }), writes)

	persistent := accountOperatorRuleSet(func(kind AccountOperatorActivityKind) bool { return kind != AccountOperatorActivityNone })
	require.Equal(t, map[adminRouteRule]struct{}{
		{method: http.MethodPost, route: "/api/v1/admin/accounts"}:                             {},
		{method: http.MethodPost, route: "/api/v1/admin/accounts/import/codex-session"}:        {},
		{method: http.MethodPost, route: "/api/v1/admin/openai/create-from-codex-pat"}:         {},
		{method: http.MethodPost, route: "/api/v1/admin/grok/sso-to-oauth"}:                    {},
		{method: http.MethodPost, route: "/api/v1/admin/accounts/:id/test"}:                    {},
		{method: http.MethodPost, route: "/api/v1/admin/accounts/:id/apply-oauth-credentials"}: {},
	}, persistent)
}
