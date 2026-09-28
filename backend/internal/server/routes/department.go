package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/gin-gonic/gin"
)

func registerDepartmentRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	d := admin.Group("/departments")
	d.GET("", h.Admin.Department.List)
	d.POST("", h.Admin.Department.Create)
	d.GET("/members", h.Admin.Department.Members)
	d.POST("/assign-members", h.Admin.Department.Assign)
	d.PUT("/:id", h.Admin.Department.Update)
	d.GET("/:id/members", h.Admin.Department.Members)
	admin.GET("/users/:id/department-scope", h.Admin.Department.GetAccess)
	admin.PUT("/users/:id/department-scope", h.Admin.Department.SetAccess)
	admin.GET("/usage/organization-report/scope", h.Admin.Department.ReportScope)
	admin.GET("/subscriptions/scope", h.Admin.Department.SubscriptionScope)
}
