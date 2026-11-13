package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"multi-tenant-hybrid-search/internal/delivery/middleware"
	"multi-tenant-hybrid-search/internal/domain/embedding"
	domain "multi-tenant-hybrid-search/internal/domain/search"
	"multi-tenant-hybrid-search/internal/domain/tenant"
)

// maxSearchBodyBytes caps search request bodies (a query is at most 2000 characters).
const maxSearchBodyBytes = 16 << 10

// SearchService is the use-case surface the search handlers need.
type SearchService interface {
	Semantic(ctx context.Context, id tenant.Identity, query string, limit *int) ([]domain.Result, error)
}

type resultJSON struct {
	DocumentID string  `json:"document_id"`
	Title      string  `json:"title"`
	Score      float64 `json:"score"`
}

func registerSearch(api *gin.RouterGroup, d Deps) {
	h := &searchHandlers{svc: d.Search, deps: d}
	api.POST("/search/semantic", h.semantic)
}

type searchHandlers struct {
	svc  SearchService
	deps Deps
}

func (h *searchHandlers) semantic(c *gin.Context) {
	id, ok := middleware.IdentityFrom(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var req struct {
		Query string `json:"query"`
		Limit *int   `json:"limit"`
	}
	if !decodeJSON(c, &req, maxSearchBodyBytes) {
		return
	}
	res, err := h.svc.Semantic(c.Request.Context(), id, req.Query, req.Limit)
	if err != nil {
		h.fail(c, err)
		return
	}
	items := make([]resultJSON, 0, len(res))
	for _, r := range res {
		items = append(items, resultJSON{r.DocumentID, r.Title, r.Score})
	}
	c.JSON(http.StatusOK, gin.H{"results": items, "total": len(items)})
}

func (h *searchHandlers) fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalid):
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, embedding.ErrUnavailable):
		h.deps.Log.Error("embedding failed", "method", c.Request.Method, "path", c.FullPath(), "error", err)
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "embedding service unavailable"})
	default:
		h.deps.Log.Error("search request failed", "method", c.Request.Method, "path", c.FullPath(), "error", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	}
}
