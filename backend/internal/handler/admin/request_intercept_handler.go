package admin

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type RequestInterceptHandler struct {
	service *service.RequestInterceptRulesService
}

func NewRequestInterceptHandler(svc *service.RequestInterceptRulesService) *RequestInterceptHandler {
	return &RequestInterceptHandler{service: svc}
}

type requestInterceptRulesSaveRequest struct {
	Rules []service.RequestInterceptRuleConfig `json:"rules"`
}

type requestInterceptRuleUpsertRequest struct {
	Name            string                                `json:"name"`
	Enabled         *bool                                 `json:"enabled"`
	Priority        int                                   `json:"priority"`
	MatchMode       string                                `json:"match_mode"`
	Keywords        []string                              `json:"keywords"`
	Reply           string                                `json:"reply"`
	Scopes          []string                              `json:"scopes"`
	Normalize       service.RequestInterceptNormalization `json:"normalize"`
	CaseInsensitive *bool                                 `json:"case_insensitive"`
	Description     string                                `json:"description"`
}

type requestInterceptTestRequest struct {
	Text     string `json:"text"`
	Endpoint string `json:"endpoint"`
}

type requestInterceptConfigRequest struct {
	Enabled bool `json:"enabled"`
}

func (h *RequestInterceptHandler) Config(c *gin.Context) {
	cfg, err := h.service.GetConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

func (h *RequestInterceptHandler) UpdateConfig(c *gin.Context) {
	var req requestInterceptConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	cfg, err := h.service.SetConfig(c.Request.Context(), service.RequestInterceptConfig{Enabled: req.Enabled})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

func (h *RequestInterceptHandler) List(c *gin.Context) {
	rules, err := h.service.ListRules(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"rules": rules})
}

func (h *RequestInterceptHandler) SaveAll(c *gin.Context) {
	var req requestInterceptRulesSaveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	rules, err := h.service.SaveRules(c.Request.Context(), req.Rules)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, gin.H{"rules": rules})
}

func (h *RequestInterceptHandler) Upsert(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		response.BadRequest(c, "Invalid rule ID")
		return
	}
	var req requestInterceptRuleUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	caseInsensitive := true
	if req.CaseInsensitive != nil {
		caseInsensitive = *req.CaseInsensitive
	}
	normalize := req.Normalize
	if normalize == (service.RequestInterceptNormalization{}) {
		normalize = service.DefaultRequestInterceptNormalization()
	}
	rule, err := h.service.UpsertRule(c.Request.Context(), service.RequestInterceptRuleConfig{
		ID:              id,
		Name:            req.Name,
		Enabled:         enabled,
		Priority:        req.Priority,
		MatchMode:       req.MatchMode,
		Keywords:        req.Keywords,
		Reply:           req.Reply,
		Scopes:          req.Scopes,
		Normalize:       normalize,
		CaseInsensitive: caseInsensitive,
		Description:     req.Description,
	})
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, rule)
}

func (h *RequestInterceptHandler) Delete(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		response.BadRequest(c, "Invalid rule ID")
		return
	}
	if err := h.service.DeleteRule(c.Request.Context(), id); err != nil {
		if err == service.ErrSettingNotFound {
			response.NotFound(c, "Rule not found")
			return
		}
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "Rule deleted successfully"})
}

func (h *RequestInterceptHandler) Test(c *gin.Context) {
	var req requestInterceptTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	decision, matched, err := h.service.TestRules(c.Request.Context(), service.RequestInterceptMatchInput{
		Text:     req.Text,
		Endpoint: req.Endpoint,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{
		"matched":  matched,
		"decision": decision,
	})
}
