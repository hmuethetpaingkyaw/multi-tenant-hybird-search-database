package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	appdoc "multi-tenant-hybrid-search/internal/application/document"
	"multi-tenant-hybrid-search/internal/delivery/middleware"
	domain "multi-tenant-hybrid-search/internal/domain/document"
	"multi-tenant-hybrid-search/internal/domain/tenant"
)

// maxBodyBytes is the content limit plus room for JSON framing (title, keys, escaping of small content).
const maxBodyBytes = domain.MaxContentBytes + 64<<10

// DocumentService is the use-case surface the document handlers need.
type DocumentService interface {
	Create(ctx context.Context, id tenant.Identity, in domain.Input) (domain.Document, error)
	Get(ctx context.Context, id tenant.Identity, docID string) (domain.Document, error)
	List(ctx context.Context, id tenant.Identity, limit, offset *int) (appdoc.ListResult, error)
	Update(ctx context.Context, id tenant.Identity, docID string, in domain.Input) (domain.Document, error)
	Delete(ctx context.Context, id tenant.Identity, docID string) error
}

type documentJSON struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type summaryJSON struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func toDocJSON(d domain.Document) documentJSON {
	return documentJSON{d.ID, d.Title, d.Content, d.CreatedAt, d.UpdatedAt}
}

func registerDocuments(api *gin.RouterGroup, d Deps) {
	h := &documentHandlers{svc: d.Documents, deps: d}
	g := api.Group("/documents")
	g.POST("", h.create)
	g.GET("", h.list)
	g.GET("/:id", h.get)
	g.PUT("/:id", h.update)
	g.DELETE("/:id", h.delete)
}

type documentHandlers struct {
	svc  DocumentService
	deps Deps
}

func (h *documentHandlers) create(c *gin.Context) {
	id, in, ok := h.identityAndBody(c)
	if !ok {
		return
	}
	doc, err := h.svc.Create(c.Request.Context(), id, in)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, toDocJSON(doc))
}

func (h *documentHandlers) list(c *gin.Context) {
	id, ok := identity(c)
	if !ok {
		return
	}
	limit, err := optInt(c, "limit", domain.MsgLimit)
	if err != nil {
		h.fail(c, err)
		return
	}
	offset, err := optInt(c, "offset", domain.MsgOffset)
	if err != nil {
		h.fail(c, err)
		return
	}
	res, err := h.svc.List(c.Request.Context(), id, limit, offset)
	if err != nil {
		h.fail(c, err)
		return
	}
	items := make([]summaryJSON, 0, len(res.Documents))
	for _, s := range res.Documents {
		items = append(items, summaryJSON{s.ID, s.Title, s.CreatedAt, s.UpdatedAt})
	}
	c.JSON(http.StatusOK, gin.H{"documents": items, "limit": res.Limit, "offset": res.Offset})
}

func (h *documentHandlers) get(c *gin.Context) {
	id, ok := identity(c)
	if !ok {
		return
	}
	docID, ok := pathID(c)
	if !ok {
		return
	}
	doc, err := h.svc.Get(c.Request.Context(), id, docID)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, toDocJSON(doc))
}

func (h *documentHandlers) update(c *gin.Context) {
	docID, ok := pathID(c)
	if !ok {
		return
	}
	id, in, ok := h.identityAndBody(c)
	if !ok {
		return
	}
	doc, err := h.svc.Update(c.Request.Context(), id, docID, in)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, toDocJSON(doc))
}

func (h *documentHandlers) delete(c *gin.Context) {
	id, ok := identity(c)
	if !ok {
		return
	}
	docID, ok := pathID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id, docID); err != nil {
		h.fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func identity(c *gin.Context) (tenant.Identity, bool) {
	id, ok := middleware.IdentityFrom(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
	}
	return id, ok
}

func pathID(c *gin.Context) (string, bool) {
	u, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid document id"})
		return "", false
	}
	return u.String(), true
}

// optInt parses an optional integer query parameter; nil means absent.
func optInt(c *gin.Context, key, msg string) (*int, error) {
	raw, present := c.GetQuery(key)
	if !present {
		return nil, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return nil, domain.NewInvalid("%s", msg)
	}
	return &n, nil
}

// identityAndBody authenticates and strictly decodes {"title","content"}; it writes the error response itself.
func (h *documentHandlers) identityAndBody(c *gin.Context) (tenant.Identity, domain.Input, bool) {
	id, ok := identity(c)
	if !ok {
		return id, domain.Input{}, false
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes)
	dec := json.NewDecoder(c.Request.Body)
	dec.DisallowUnknownFields()
	var req struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	err := dec.Decode(&req)
	if err == nil {
		// Exactly one JSON value: reject trailing data.
		if err2 := dec.Decode(&struct{}{}); err2 != io.EOF {
			err = errors.New("trailing data")
			if err2 != nil {
				err = err2
			}
		}
	}
	if err != nil {
		var tooBig *http.MaxBytesError
		switch {
		case errors.As(err, &tooBig):
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
		case strings.HasPrefix(err.Error(), "json: unknown field "):
			c.AbortWithStatusJSON(http.StatusBadRequest,
				gin.H{"error": "invalid request body: " + strings.TrimPrefix(err.Error(), "json: ")})
		default:
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		}
		return id, domain.Input{}, false
	}
	return id, domain.Input{Title: req.Title, Content: req.Content}, true
}

// fail maps errors to responses. Internal errors are logged, never returned to the client.
func (h *documentHandlers) fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalid):
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, domain.ErrNotFound):
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "not found"})
	default:
		h.deps.Log.Error("document request failed", "method", c.Request.Method, "path", c.FullPath(), "error", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	}
}
