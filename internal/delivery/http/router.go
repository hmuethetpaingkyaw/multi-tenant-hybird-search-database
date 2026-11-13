// Package httpapi is the HTTP delivery layer.
package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	appt "multi-tenant-hybrid-search/internal/application/tenant"
	"multi-tenant-hybrid-search/internal/delivery/middleware"
	domain "multi-tenant-hybrid-search/internal/domain/tenant"
)

// ReadyFunc reports whether the service can serve traffic (DB reachable, extensions present).
type ReadyFunc func(ctx context.Context) error

// MeService returns the authenticated caller's tenant and user.
type MeService interface {
	Me(ctx context.Context, id domain.Identity) (appt.Profile, error)
}

type Deps struct {
	Ready ReadyFunc
	Log   *slog.Logger
	// Auth guards /api/v1. When nil, no API routes are registered; Me and Documents register their routes when non-nil.
	Auth      gin.HandlerFunc
	Me        MeService
	Documents DocumentService
}

func NewRouter(d Deps) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	// Liveness: the process is up. Does not touch the database.
	r.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })

	// Readiness: details are logged, not returned to the client.
	r.GET("/readyz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := d.Ready(ctx); err != nil {
			d.Log.Error("readiness check failed", "error", err)
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not ready"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})

	if d.Auth != nil {
		api := r.Group("/api/v1", d.Auth)
		if d.Me != nil {
			api.GET("/me", meHandler(d))
		}
		if d.Documents != nil {
			registerDocuments(api, d)
		}
	}
	return r
}

func meHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := middleware.IdentityFrom(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		p, err := d.Me.Me(c.Request.Context(), id)
		switch {
		case errors.Is(err, domain.ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		case err != nil:
			d.Log.Error("me failed", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		default:
			c.JSON(http.StatusOK, gin.H{
				"tenant": gin.H{"id": p.Tenant.ID, "name": p.Tenant.Name},
				"user":   gin.H{"id": p.User.ID, "email": p.User.Email},
			})
		}
	}
}
