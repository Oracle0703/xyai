package admin

import (
	"bytes"
	"context"
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// accountActivityMaxCapturedBody 限制新增类接口响应体的捕获大小，超出部分不再解析。
const accountActivityMaxCapturedBody = 8 << 20

// AccountOperatorHandler 为受限账号运维子管理员（admin.accounts_operator）提供
// 不含敏感字段的表单下拉数据，并在其账号写操作后发送通知邮件。
type AccountOperatorHandler struct {
	adminService service.AdminService
	activity     accountOperatorActivityRecorder
}

type accountOperatorActivityRecorder interface {
	Snapshot(ctx context.Context, accountID int64) *service.AccountActivitySnapshot
	RecordChange(actor service.AccountOperatorActor, kind service.AccountOperatorActivityKind, accountID int64, before *service.AccountActivitySnapshot)
	RecordCreated(actor service.AccountOperatorActor, refs []service.AccountActivityRef)
}

func NewAccountOperatorHandler(adminService service.AdminService, activity *service.AccountOperatorActivityService) *AccountOperatorHandler {
	h := &AccountOperatorHandler{adminService: adminService}
	if activity != nil {
		h.activity = activity
	}
	return h
}

type accountFormGroupOption struct {
	ID                        int64   `json:"id"`
	Name                      string  `json:"name"`
	Description               string  `json:"description"`
	Platform                  string  `json:"platform"`
	Status                    string  `json:"status"`
	SubscriptionType          string  `json:"subscription_type"`
	RateMultiplier            float64 `json:"rate_multiplier"`
	AccountCount              int64   `json:"account_count"`
	LongContextPricingEnabled bool    `json:"long_context_pricing_enabled"`
}

type accountFormProxyOption struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Protocol     string `json:"protocol"`
	Host         string `json:"host"`
	Port         int    `json:"port"`
	Status       string `json:"status"`
	AccountCount int64  `json:"account_count"`
}

type accountFormOptionsResponse struct {
	Groups  []accountFormGroupOption `json:"groups"`
	Proxies []accountFormProxyOption `json:"proxies"`
}

// FormOptions 返回账号页筛选和新增表单需要的分组、代理精简列表（不含代理认证信息）。
// GET /api/v1/admin/accounts/form-options
func (h *AccountOperatorHandler) FormOptions(c *gin.Context) {
	ctx := c.Request.Context()
	groups, err := h.adminService.GetAllGroups(ctx)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	proxies, err := h.adminService.GetAllProxiesWithAccountCount(ctx)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := accountFormOptionsResponse{
		Groups:  make([]accountFormGroupOption, 0, len(groups)),
		Proxies: make([]accountFormProxyOption, 0, len(proxies)),
	}
	for i := range groups {
		g := &groups[i]
		out.Groups = append(out.Groups, accountFormGroupOption{
			ID:                        g.ID,
			Name:                      g.Name,
			Description:               g.Description,
			Platform:                  g.Platform,
			Status:                    g.Status,
			SubscriptionType:          g.SubscriptionType,
			RateMultiplier:            g.RateMultiplier,
			AccountCount:              g.AccountCount,
			LongContextPricingEnabled: g.LongContextPricingEnabled,
		})
	}
	for i := range proxies {
		p := &proxies[i]
		out.Proxies = append(out.Proxies, accountFormProxyOption{
			ID:           p.ID,
			Name:         p.Name,
			Protocol:     p.Protocol,
			Host:         p.Host,
			Port:         p.Port,
			Status:       p.Status,
			AccountCount: p.AccountCount,
		})
	}
	response.Success(c, out)
}

// ActivityTracker 在子管理员的账号写操作完成后发送通知；完整管理员、GET 和无副作用的路由直接放行。
func (h *AccountOperatorHandler) ActivityTracker() gin.HandlerFunc {
	return func(c *gin.Context) {
		if h == nil || h.activity == nil || c.Request.Method == http.MethodGet {
			c.Next()
			return
		}
		if role, _ := middleware2.GetUserRoleFromContext(c); role != service.RoleSubAdmin {
			c.Next()
			return
		}
		kind, ok := service.AccountOperatorActivityKindForRoute(c.Request.Method, c.FullPath())
		if !ok || kind == service.AccountOperatorActivityNone {
			c.Next()
			return
		}
		subject, _ := middleware2.GetAuthSubjectFromContext(c)
		actor := service.AccountOperatorActor{
			UserID:   subject.UserID,
			Email:    c.GetString(middleware2.ContextKeyAuthEmail),
			ClientIP: middleware2.SecurityClientIP(c),
			Method:   c.Request.Method,
			Path:     c.FullPath(),
		}

		if kind == service.AccountOperatorActivityCreate {
			recorder := &accountActivityBodyRecorder{ResponseWriter: c.Writer}
			c.Writer = recorder
			c.Next()
			c.Writer = recorder.ResponseWriter
			if recorder.Status() >= http.StatusMultipleChoices {
				return
			}
			h.activity.RecordCreated(actor, service.ExtractAccountActivityRefs(recorder.body.Bytes()))
			return
		}

		accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || accountID <= 0 {
			c.Next()
			return
		}
		before := h.activity.Snapshot(c.Request.Context(), accountID)
		c.Next()
		// 测试连接走 SSE，失败也返回 200，统一按前后快照判断；重新授权仅在成功时通知。
		if kind == service.AccountOperatorActivityReauth && c.Writer.Status() >= http.StatusMultipleChoices {
			return
		}
		h.activity.RecordChange(actor, kind, accountID, before)
	}
}

type accountActivityBodyRecorder struct {
	gin.ResponseWriter
	body bytes.Buffer
}

func (w *accountActivityBodyRecorder) Write(data []byte) (int, error) {
	if w.body.Len()+len(data) <= accountActivityMaxCapturedBody {
		w.body.Write(data)
	}
	return w.ResponseWriter.Write(data)
}

func (w *accountActivityBodyRecorder) WriteString(data string) (int, error) {
	if w.body.Len()+len(data) <= accountActivityMaxCapturedBody {
		w.body.WriteString(data)
	}
	return w.ResponseWriter.WriteString(data)
}
