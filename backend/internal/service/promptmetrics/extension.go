package promptmetrics

import (
	"database/sql"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/google/wire"
)

// Extension 是 prompt metrics 对 server 层暴露的唯一集成对象.
// server 只需要挂载 CaptureMiddleware 并在已鉴权 admin group 下注册管理 API.
type Extension struct {
	capture   *PromptCapture
	handler   *Handler
	publisher *AsyncPublisher
}

// NewExtension 组装采集, 仓储, 服务和管理 API.
func NewExtension(cfg *config.Config, db *sql.DB) *Extension {
	pmCfg := config.PromptMetricsConfig{}
	if cfg != nil {
		pmCfg = cfg.PromptMetrics
	}
	pmCfg = normalizeConfig(pmCfg)
	repo := NewRepository(db)
	var publisher *AsyncPublisher
	if pmCfg.Enabled {
		publisher = NewAsyncPublisher(repo, pmCfg.WorkerCount, pmCfg.QueueSize, time.Duration(pmCfg.WriteTimeoutSeconds)*time.Second)
	}
	extractor := NewExtractor()
	service := NewService(repo)
	return &Extension{
		capture:   NewPromptCapture(pmCfg, publisher, extractor),
		handler:   NewHandler(service),
		publisher: publisher,
	}
}

// CaptureMiddleware 返回可全局挂载的采集中间件.
func (e *Extension) CaptureMiddleware() gin.HandlerFunc {
	if e == nil || e.capture == nil {
		return func(c *gin.Context) { c.Next() }
	}
	return e.capture.Handler()
}

// RegisterAdminRoutes 在已鉴权的 /admin group 下注册管理 API.
func (e *Extension) RegisterAdminRoutes(admin *gin.RouterGroup) {
	if e == nil || e.handler == nil || admin == nil {
		return
	}
	group := admin.Group("/prompt-metrics")
	{
		group.GET("/overview", e.handler.Overview)
		group.GET("/trend", e.handler.Trend)
		group.GET("/rank", e.handler.Rank)
		group.GET("/events", e.handler.Events)
		group.GET("/events/:id", e.handler.Event)
		group.POST("/events/:id/reanalyze", e.handler.Reanalyze)
	}
}

// Stop 释放异步 worker pool, 供应用退出清理调用.
func (e *Extension) Stop(timeout time.Duration) {
	if e == nil || e.publisher == nil {
		return
	}
	e.publisher.Stop(timeout)
}

// ProviderSet 提供 prompt metrics 功能岛的 Wire 注入入口.
var ProviderSet = wire.NewSet(NewExtension)
