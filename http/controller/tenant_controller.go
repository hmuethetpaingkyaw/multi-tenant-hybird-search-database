package controller

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"multi-tenant-hybrid-search/domain"
	"multi-tenant-hybrid-search/http/response"
	"multi-tenant-hybrid-search/usecase"
)

// TenantUsecasePort is what TenantController needs from the use case layer.
type TenantUsecasePort interface {
	Me(ctx context.Context, id domain.Identity) (usecase.TenantProfile, error)
}

type TenantController struct{ tenantUsecase TenantUsecasePort }

func NewTenantController(tenantUsecase TenantUsecasePort) *TenantController {
	return &TenantController{tenantUsecase: tenantUsecase}
}

// Me returns the caller's tenant and user (GET /api/v1/me).
func (co *TenantController) Me(c *gin.Context) {
	id, err := identityOf(c)
	if err != nil {
		fail(c, err)
		return
	}
	p, err := co.tenantUsecase.Me(c.Request.Context(), id)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, response.ToMeResponse(p.Tenant, p.User))
}
