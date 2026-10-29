package service

import (
	"fmt"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	AdminPermissionSubscriptions           = "admin.subscriptions"
	AdminPermissionUsage                   = "admin.usage"
	AdminPermissionTokenAnalysis           = "admin.token_analysis"
	AdminPermissionOrganizationUsage       = "admin.organization_usage"
	AdminPermissionDepartmentSubscriptions = "admin.department_subscriptions"
	AdminPermissionDepartmentMembers       = "admin.department_members"
	AdminPermissionAccountsOperator        = "admin.accounts_operator"
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
	{Code: AdminPermissionOrganizationUsage, MenuKey: "organization_usage", Route: "/admin/organization-usage"},
	{Code: AdminPermissionDepartmentSubscriptions, MenuKey: "department_subscriptions", Route: "/admin/subscriptions"},
	{Code: AdminPermissionDepartmentMembers, MenuKey: "department_members", Route: "/admin/my-department"},
	{Code: AdminPermissionAccountsOperator, MenuKey: "accounts", Route: "/admin/accounts"},
}

var adminPermissionRouteRules = map[string][]adminRouteRule{
	AdminPermissionDepartmentMembers: {
		{"GET", "/api/v1/admin/department-members/scope"},
		{"GET", "/api/v1/admin/department-members"},
		{"GET", "/api/v1/admin/department-members/candidates"},
		{"POST", "/api/v1/admin/department-members/assign"},
		{"PUT", "/api/v1/admin/department-members/:id"},
	},
	AdminPermissionOrganizationUsage: {
		{"GET", "/api/v1/admin/usage/organization-report/scope"},
		{"GET", "/api/v1/admin/usage/organization-report/summary"},
		{"GET", "/api/v1/admin/usage/organization-report/periods"},
		{"GET", "/api/v1/admin/usage/organization-report/trend"},
	},
	AdminPermissionDepartmentSubscriptions: {
		{"GET", "/api/v1/admin/subscriptions"},
		{"GET", "/api/v1/admin/subscriptions/:id"},
		{"GET", "/api/v1/admin/subscriptions/:id/progress"},
		{"GET", "/api/v1/admin/users/:id/subscriptions"},
		{"GET", "/api/v1/admin/groups/:id/subscriptions"},
		{"GET", "/api/v1/admin/subscriptions/scope"},
		{"GET", "/api/v1/admin/subscriptions/search-users"},
		{"GET", "/api/v1/admin/subscriptions/search-groups"},
		{"POST", "/api/v1/admin/subscriptions/:id/reset-quota"},
		{"POST", "/api/v1/admin/subscriptions/reset-daily-filtered"},
	},
	AdminPermissionSubscriptions: {
		{"GET", "/api/v1/admin/subscriptions/scope"},
		{"GET", "/api/v1/admin/subscriptions/search-users"},
		{"GET", "/api/v1/admin/subscriptions"},
		{"GET", "/api/v1/admin/subscriptions/:id"},
		{"GET", "/api/v1/admin/subscriptions/:id/progress"},
		{"GET", "/api/v1/admin/groups/:id/subscriptions"},
		{"GET", "/api/v1/admin/users/:id/subscriptions"},
		{"GET", "/api/v1/admin/subscriptions/search-groups"},
		{"GET", "/api/v1/admin/subscriptions/assignable-groups"},
		{"GET", "/api/v1/admin/usage/search-users"},
		{"POST", "/api/v1/admin/subscriptions/assign"},
		{"POST", "/api/v1/admin/subscriptions/bulk-assign"},
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
	// 受限账号运维：全量查看 + 新增、测试连接、重新授权。写操作的通知分类见
	// AccountOperatorActivityKindForRoute，新增路由必须同时归类。
	AdminPermissionAccountsOperator: {
		{"GET", "/api/v1/admin/accounts"},
		{"GET", "/api/v1/admin/accounts/form-options"},
		{"GET", "/api/v1/admin/accounts/upstream-billing-rates"},
		{"GET", "/api/v1/admin/accounts/:id"},
		{"GET", "/api/v1/admin/accounts/:id/stats"},
		{"GET", "/api/v1/admin/accounts/:id/usage"},
		{"GET", "/api/v1/admin/accounts/:id/today-stats"},
		{"GET", "/api/v1/admin/accounts/:id/temp-unschedulable"},
		{"GET", "/api/v1/admin/accounts/:id/models"},
		{"GET", "/api/v1/admin/accounts/:id/claude/reset-credits"},
		{"GET", "/api/v1/admin/grok/accounts/:id/quota"},
		{"GET", "/api/v1/admin/cn-providers/accounts/:id/quota"},
		{"GET", "/api/v1/admin/cn-providers/accounts/:id/balance"},
		{"GET", "/api/v1/admin/gemini/oauth/capabilities"},
		{"GET", "/api/v1/admin/grok/oauth/capabilities"},
		{"POST", "/api/v1/admin/accounts/usage/batch"},
		{"POST", "/api/v1/admin/accounts/today-stats/batch"},
		{"POST", "/api/v1/admin/accounts/check-mixed-channel"},
		{"POST", "/api/v1/admin/accounts/models/sync-upstream-preview"},
		{"POST", "/api/v1/admin/accounts"},
		{"POST", "/api/v1/admin/accounts/import/codex-session"},
		{"POST", "/api/v1/admin/openai/create-from-codex-pat"},
		{"POST", "/api/v1/admin/grok/sso-to-oauth"},
		{"POST", "/api/v1/admin/accounts/:id/test"},
		{"POST", "/api/v1/admin/accounts/:id/apply-oauth-credentials"},
		{"POST", "/api/v1/admin/accounts/generate-auth-url"},
		{"POST", "/api/v1/admin/accounts/generate-setup-token-url"},
		{"POST", "/api/v1/admin/accounts/exchange-code"},
		{"POST", "/api/v1/admin/accounts/exchange-setup-token-code"},
		{"POST", "/api/v1/admin/accounts/cookie-auth"},
		{"POST", "/api/v1/admin/accounts/setup-token-cookie-auth"},
		{"POST", "/api/v1/admin/openai/generate-auth-url"},
		{"POST", "/api/v1/admin/openai/exchange-code"},
		{"POST", "/api/v1/admin/openai/refresh-token"},
		{"POST", "/api/v1/admin/gemini/oauth/auth-url"},
		{"POST", "/api/v1/admin/gemini/oauth/exchange-code"},
		{"POST", "/api/v1/admin/antigravity/oauth/auth-url"},
		{"POST", "/api/v1/admin/antigravity/oauth/exchange-code"},
		{"POST", "/api/v1/admin/antigravity/oauth/refresh-token"},
		{"POST", "/api/v1/admin/grok/oauth/auth-url"},
		{"POST", "/api/v1/admin/grok/oauth/exchange-code"},
		{"POST", "/api/v1/admin/grok/oauth/refresh-token"},
		{"POST", "/api/v1/admin/grok/oauth/sso-token"},
		{"POST", "/api/v1/admin/grok/oauth/password"},
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

// AdminPermissionRouteTemplates 返回权限白名单中的 "METHOD /route" 列表，供路由注册一致性测试使用。
func AdminPermissionRouteTemplates(permission string) []string {
	rules := adminPermissionRouteRules[permission]
	out := make([]string, 0, len(rules))
	for _, rule := range rules {
		out = append(out, rule.method+" "+rule.route)
	}
	return out
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
	_, globalSubscriptions := requested[AdminPermissionSubscriptions]
	_, departmentSubscriptions := requested[AdminPermissionDepartmentSubscriptions]
	if globalSubscriptions && departmentSubscriptions {
		return nil, infraerrors.BadRequest("CONFLICTING_ADMIN_PERMISSIONS", "global and department subscription permissions are mutually exclusive")
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
