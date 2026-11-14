// Package router registers the routes. Routes of a nil controller are not registered.
package router

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"multi-tenant-hybrid-search/http/controller"
	"multi-tenant-hybrid-search/http/middleware"
)

type Controllers struct {
	Health   *controller.HealthController
	Tenant   *controller.TenantController
	Document *controller.DocumentController
	Search   *controller.SearchController
}

// New builds the engine: recovery, the central error handler, the open probes, and /api/v1 behind auth.
func New(log *slog.Logger, auth gin.HandlerFunc, c Controllers) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery(), middleware.ErrorHandler(log))

	if c.Health != nil {
		r.GET("/healthz", c.Health.Healthz)
		r.GET("/readyz", c.Health.Readyz)
	}

	if auth == nil {
		return r
	}
	api := r.Group("/api/v1", auth)
	if c.Tenant != nil {
		api.GET("/me", c.Tenant.Me)
	}
	if c.Document != nil {
		docs := api.Group("/documents")
		docs.POST("", c.Document.Create)
		docs.GET("", c.Document.List)
		docs.GET("/:id", c.Document.Get)
		docs.PUT("/:id", c.Document.Update)
		docs.DELETE("/:id", c.Document.Delete)
	}
	if c.Search != nil {
		api.POST("/search/semantic", c.Search.Semantic)
	}
	return r
}
