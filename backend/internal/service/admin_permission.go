package service

import (
	"fmt"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	AdminPermissionSubscriptions = "admin.subscriptions"
	AdminPermissionUsage         = "admin.usage"
	AdminPermissionTokenAnalysis = "admin.token_analysis"
)

type AdminPermissionCatalogItem struct {
	Code    string `json:"code"`
	MenuKey string `json:"menu_key"`
	Route   string `json:"route"`
}

type adminRouteRule struct {
	method string
	route  string
}

var adminPermissionCatalog = []AdminPermissionCatalogItem{
	{Code: AdminPermissionSubscriptions, MenuKey: "subscriptions", Route: "/admin/subscriptions"},
	{Code: AdminPermissionUsage, MenuKey: "usage", Route: "/admin/usage"},
	{Code: AdminPermissionTokenAnalysis, MenuKey: "token_analysis", Route: "/admin/token-analysis"},
}

var adminPermissionRouteRules = map[string][]adminRouteRule{
	AdminPermissionSubscriptions: {
		{"GET", "/api/v1/admin/subscriptions"},
		{"GET", "/api/v1/admin/subscriptions/:id"},
		{"GET", "/api/v1/admin/subscriptions/:id/progress"},
		{"GET", "/api/v1/admin/groups/:id/subscriptions"},
		{"GET", "/api/v1/admin/users/:id/subscriptions"},
		{"GET", "/api/v1/admin/subscriptions/search-groups"},
		{"GET", "/api/v1/admin/usage/search-users"},
		{"POST", "/api/v1/admin/subscriptions/:id/reset-quota"},
		{"POST", "/api/v1/admin/subscriptions/reset-daily-filtered"},
	},
	AdminPermissionUsage: {
		{"GET", "/api/v1/admin/usage"},
		{"GET", "/api/v1/admin/usage/stats"},
		{"GET", "/api/v1/admin/usage/search-users"},
		{"GET", "/api/v1/admin/usage/search-api-keys"},
		{"GET", "/api/v1/admin/usage/search-accounts"},
		{"GET", "/api/v1/admin/usage/search-groups"},
		{"GET", "/api/v1/admin/dashboard/snapshot-v2"},
		{"GET", "/api/v1/admin/dashboard/models"},
		{"GET", "/api/v1/admin/dashboard/user-breakdown"},
		{"GET", "/api/v1/admin/dashboard/users-ranking"},
		{"GET", "/api/v1/admin/ops/errors"},
		{"GET", "/api/v1/admin/ops/errors/:id"},
		{"GET", "/api/v1/admin/ops/request-errors"},
		{"GET", "/api/v1/admin/ops/request-errors/:id"},
		{"GET", "/api/v1/admin/ops/request-errors/:id/upstream-errors"},
		{"GET", "/api/v1/admin/ops/upstream-errors"},
		{"GET", "/api/v1/admin/ops/upstream-errors/:id"},
	},
	AdminPermissionTokenAnalysis: {
		{"GET", "/api/v1/admin/token-analysis/summary"},
		{"GET", "/api/v1/admin/token-analysis/users"},
		{"GET", "/api/v1/admin/token-analysis/projects"},
		{"GET", "/api/v1/admin/token-analysis/requests"},
		{"GET", "/api/v1/admin/token-analysis/requests/input"},
		{"GET", "/api/v1/admin/token-analysis/index/status"},
		{"GET", "/api/v1/admin/token-analysis/archive-files"},
		{"GET", "/api/v1/admin/dashboard/users-trend"},
	},
}

var subAdminCommonRouteRules = []adminRouteRule{
	{"GET", "/api/v1/admin/compliance"},
	{"POST", "/api/v1/admin/compliance/accept"},
}

func AdminPermissionCatalog() []AdminPermissionCatalogItem {
	return append([]AdminPermissionCatalogItem(nil), adminPermissionCatalog...)
}

func NormalizeAdminPermissions(role string, permissions []string) ([]string, error) {
	requested := make(map[string]struct{}, len(permissions))
	for _, permission := range permissions {
		permission = strings.TrimSpace(permission)
		if permission == "" {
			continue
		}
		if !isKnownAdminPermission(permission) {
			return nil, infraerrors.BadRequest(
				"UNKNOWN_ADMIN_PERMISSION",
				fmt.Sprintf("unknown admin permission: %s", permission),
			)
		}
		requested[permission] = struct{}{}
	}
	if role != RoleSubAdmin {
		return []string{}, nil
	}

	normalized := make([]string, 0, len(requested))
	for _, item := range adminPermissionCatalog {
		if _, ok := requested[item.Code]; ok {
			normalized = append(normalized, item.Code)
		}
	}
	return normalized, nil
}

func isKnownAdminPermission(permission string) bool {
	for _, item := range adminPermissionCatalog {
		if item.Code == permission {
			return true
		}
	}
	return false
}

func HasAdminPermission(user *User, permission string) bool {
	if user == nil {
		return false
	}
	if user.Role == RoleAdmin {
		return true
	}
	if user.Role != RoleSubAdmin {
		return false
	}
	for _, current := range user.AdminPermissions {
		if current == permission {
			return true
		}
	}
	return false
}

func CanAccessAdmin(user *User) bool {
	if user == nil {
		return false
	}
	return user.Role == RoleAdmin || (user.Role == RoleSubAdmin && len(user.AdminPermissions) > 0)
}

func CanAccessAdminRoute(user *User, method, route string) bool {
	if user == nil {
		return false
	}
	if user.Role == RoleAdmin {
		return true
	}
	if user.Role != RoleSubAdmin {
		return false
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	for _, rule := range subAdminCommonRouteRules {
		if rule.method == method && rule.route == route {
			return true
		}
	}
	for _, permission := range user.AdminPermissions {
		for _, rule := range adminPermissionRouteRules[permission] {
			if rule.method == method && rule.route == route {
				return true
			}
		}
	}
	return false
}
