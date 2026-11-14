package controller

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"multi-tenant-hybrid-search/http/response"
)

// ReadyFunc reports whether the service can serve traffic (DB reachable, extensions present).
type ReadyFunc func(ctx context.Context) error

type HealthController struct {
	ready ReadyFunc
	log   *slog.Logger
}

func NewHealthController(ready ReadyFunc, log *slog.Logger) *HealthController {
	return &HealthController{ready: ready, log: log}
}

// Healthz is liveness: the process is up. It does not touch the database.
func (h *HealthController) Healthz(c *gin.Context) {
	c.JSON(http.StatusOK, response.StatusResponse{Status: "ok"})
}

// Readyz is readiness. Failure details are logged, not returned to the client.
func (h *HealthController) Readyz(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	if err := h.ready(ctx); err != nil {
		h.log.Error("readiness check failed", "error", err)
		c.JSON(http.StatusServiceUnavailable, response.StatusResponse{Status: "not ready"})
		return
	}
	c.JSON(http.StatusOK, response.StatusResponse{Status: "ready"})
}
