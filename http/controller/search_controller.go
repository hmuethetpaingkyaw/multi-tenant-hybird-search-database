package controller

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"multi-tenant-hybrid-search/constants"
	"multi-tenant-hybrid-search/domain"
	"multi-tenant-hybrid-search/http/request"
	"multi-tenant-hybrid-search/http/response"
)

// SearchUsecasePort is what SearchController needs from the use case layer.
type SearchUsecasePort interface {
	Semantic(ctx context.Context, id domain.Identity, query string, limit *int) ([]domain.SearchResult, error)
}

type SearchController struct{ searchUsecase SearchUsecasePort }

func NewSearchController(searchUsecase SearchUsecasePort) *SearchController {
	return &SearchController{searchUsecase: searchUsecase}
}

// Semantic handles POST /api/v1/search/semantic.
func (co *SearchController) Semantic(c *gin.Context) {
	id, err := identityOf(c)
	if err != nil {
		fail(c, err)
		return
	}
	var req request.SearchRequest
	if err := bindStrictJSON(c, &req, constants.MaxSearchBodyBytes); err != nil {
		fail(c, err)
		return
	}
	if err := req.Validate(); err != nil {
		fail(c, err)
		return
	}
	results, err := co.searchUsecase.Semantic(c.Request.Context(), id, req.QueryText(), req.Limit)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, response.ToSearchResponse(results))
}
