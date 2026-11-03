package admin

import (
	"encoding/json"
	"slices"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func scopedAdminWriteOnly(c *gin.Context, readPermission string) bool {
	p := adminaccess.FromContext(c.Request.Context())
	return p != nil && !slices.Contains(p.Policy.GlobalPermissions, readPermission)
}

var (
	subscriptionUsageKeys = []string{"user_id", "group_id", "starts_at", "expires_at", "status",
		"daily_window_start", "weekly_window_start", "monthly_window_start",
		"daily_usage_usd", "weekly_usage_usd", "monthly_usage_usd",
		"created_at", "updated_at", "revoked_at"}
	subscriptionAssignmentKeys = []string{"assigned_by", "assigned_at", "notes"}

	// 子管理员订阅页只需要用户标识和分组标签/额度，余额、通知设置、分组配置等不下发。
	subAdminSubscriptionNested = map[string][]string{
		"user": {"id", "email", "username", "status"},
		"group": {"id", "name", "platform", "status", "subscription_type", "rate_multiplier",
			"daily_limit_usd", "weekly_limit_usd", "monthly_limit_usd"},
	}
)

// subscriptionView 是订阅响应的字段白名单；nested 中的嵌入对象按各自白名单保留，其余嵌入对象丢弃。
type subscriptionView struct {
	keys   []string
	nested map[string][]string
}

// adminSubscriptionResponse 在缓存重放后也应用字段白名单。受限管理密钥不嵌入用户、分配人或分组配置，
// 写权限只确认订阅 ID，订阅用量等字段必须另有读取权限；JWT 子管理员（全站或部门）不返回分配元数据、
// 分配人和成员账户信息。管理员与非受限管理密钥保持完整 DTO。
func adminSubscriptionResponse(c *gin.Context, data any) any {
	var view subscriptionView
	switch {
	case adminaccess.FromContext(c.Request.Context()) != nil:
		view.keys = []string{"id"}
		if !scopedAdminWriteOnly(c, "subscriptions.read") {
			view.keys = slices.Concat(view.keys, subscriptionUsageKeys, subscriptionAssignmentKeys)
		}
	case isSubAdminRequest(c):
		view = subscriptionView{keys: slices.Concat([]string{"id"}, subscriptionUsageKeys), nested: subAdminSubscriptionNested}
	default:
		return data
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return nil
	}
	return projectSubscriptionJSON(raw, view)
}

func isSubAdminRequest(c *gin.Context) bool {
	role, ok := middleware2.GetUserRoleFromContext(c)
	return ok && role == service.RoleSubAdmin
}

func projectSubscriptionJSON(raw json.RawMessage, view subscriptionView) any {
	var list []json.RawMessage
	if len(raw) > 0 && raw[0] == '[' && json.Unmarshal(raw, &list) == nil {
		out := make([]any, 0, len(list))
		for _, item := range list {
			out = append(out, projectSubscriptionJSON(item, view))
		}
		return out
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return nil
	}
	out := make(map[string]any)
	if subscriptions, ok := fields["subscriptions"]; ok {
		for _, key := range []string{"success_count", "created_count", "reused_count", "failed_count", "errors", "statuses"} {
			if value, exists := fields[key]; exists {
				out[key] = value
			}
		}
		out["subscriptions"] = projectSubscriptionJSON(subscriptions, view)
		return out
	}
	for _, key := range view.keys {
		if value, exists := fields[key]; exists {
			out[key] = value
		}
	}
	for key, allowed := range view.nested {
		if value, exists := fields[key]; exists {
			out[key] = projectJSONObject(value, allowed)
		}
	}
	return out
}

// projectJSONObject 只保留对象中的白名单字段；null 或非对象原样按 null 处理。
func projectJSONObject(raw json.RawMessage, keys []string) any {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return nil
	}
	out := make(map[string]json.RawMessage, len(keys))
	for _, key := range keys {
		if value, exists := fields[key]; exists {
			out[key] = value
		}
	}
	return out
}
