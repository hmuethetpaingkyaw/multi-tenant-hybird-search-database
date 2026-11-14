package controller

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"multi-tenant-hybrid-search/constants"
	"multi-tenant-hybrid-search/domain"
	"multi-tenant-hybrid-search/http/request"
	"multi-tenant-hybrid-search/http/response"
	"multi-tenant-hybrid-search/usecase"
)

// DocumentUsecasePort is what DocumentController needs from the use case layer.
type DocumentUsecasePort interface {
	Create(ctx context.Context, id domain.Identity, in domain.DocumentInput) (domain.Document, error)
	Get(ctx context.Context, id domain.Identity, docID string) (domain.Document, error)
	List(ctx context.Context, id domain.Identity, limit, offset *int) (usecase.DocumentListResult, error)
	Update(ctx context.Context, id domain.Identity, docID string, in domain.DocumentInput) (domain.Document, error)
	Delete(ctx context.Context, id domain.Identity, docID string) error
}

type DocumentController struct{ documentUsecase DocumentUsecasePort }

func NewDocumentController(documentUsecase DocumentUsecasePort) *DocumentController {
	return &DocumentController{documentUsecase: documentUsecase}
}

// Create handles POST /api/v1/documents.
func (co *DocumentController) Create(c *gin.Context) {
	id, in, err := co.identityAndInput(c)
	if err != nil {
		fail(c, err)
		return
	}
	doc, err := co.documentUsecase.Create(c.Request.Context(), id, in)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, response.ToDocumentDTO(doc))
}

// List handles GET /api/v1/documents.
func (co *DocumentController) List(c *gin.Context) {
	id, err := identityOf(c)
	if err != nil {
		fail(c, err)
		return
	}
	req, err := request.NewDocumentListRequest(queryParam(c, "limit"), queryParam(c, "offset"))
	if err != nil {
		fail(c, err)
		return
	}
	res, err := co.documentUsecase.List(c.Request.Context(), id, req.Limit, req.Offset)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, response.ToDocumentListResponse(res.Documents, res.Limit, res.Offset))
}

// Get handles GET /api/v1/documents/:id.
func (co *DocumentController) Get(c *gin.Context) {
	id, err := identityOf(c)
	if err != nil {
		fail(c, err)
		return
	}
	docID, err := documentIDParam(c)
	if err != nil {
		fail(c, err)
		return
	}
	doc, err := co.documentUsecase.Get(c.Request.Context(), id, docID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, response.ToDocumentDTO(doc))
}

// Update handles PUT /api/v1/documents/:id (full replace of title and content).
func (co *DocumentController) Update(c *gin.Context) {
	docID, err := documentIDParam(c)
	if err != nil {
		fail(c, err)
		return
	}
	id, in, err := co.identityAndInput(c)
	if err != nil {
		fail(c, err)
		return
	}
	doc, err := co.documentUsecase.Update(c.Request.Context(), id, docID, in)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, response.ToDocumentDTO(doc))
}

// Delete handles DELETE /api/v1/documents/:id.
func (co *DocumentController) Delete(c *gin.Context) {
	id, err := identityOf(c)
	if err != nil {
		fail(c, err)
		return
	}
	docID, err := documentIDParam(c)
	if err != nil {
		fail(c, err)
		return
	}
	if err := co.documentUsecase.Delete(c.Request.Context(), id, docID); err != nil {
		fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// identityAndInput authenticates, strictly decodes {"title","content"} and validates it.
func (co *DocumentController) identityAndInput(c *gin.Context) (domain.Identity, domain.DocumentInput, error) {
	id, err := identityOf(c)
	if err != nil {
		return id, domain.DocumentInput{}, err
	}
	var req request.DocumentRequest
	if err := bindStrictJSON(c, &req, constants.MaxDocumentBodyBytes); err != nil {
		return id, domain.DocumentInput{}, err
	}
	if err := req.Validate(); err != nil {
		return id, domain.DocumentInput{}, err
	}
	return id, req.ToInput(), nil
}

// queryParam returns a pointer to the raw query value, or nil when the parameter is absent.
func queryParam(c *gin.Context, key string) *string {
	if v, ok := c.GetQuery(key); ok {
		return &v
	}
	return nil
}
