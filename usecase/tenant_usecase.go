// Package usecase contains the application's use cases. It depends on repository Ports and api Ports,
// never on http/*: controllers convert requests into domain types before calling it.
package usecase

import (
	"context"

	"multi-tenant-hybrid-search/domain"
	"multi-tenant-hybrid-search/repository"
)

type TenantUsecase struct {
	tenantPort repository.TenantPort
}

func NewTenantUsecase(tenantPort repository.TenantPort) *TenantUsecase {
	return &TenantUsecase{tenantPort: tenantPort}
}

// TenantProfile is the caller's tenant and user.
type TenantProfile struct {
	Tenant domain.Tenant
	User   domain.User
}

// Me returns the caller's tenant and user. Either being invisible under RLS yields apperror.ErrNotFound.
func (u *TenantUsecase) Me(ctx context.Context, id domain.Identity) (TenantProfile, error) {
	t, err := u.tenantPort.GetTenant(ctx, id.TenantID, id.TenantID)
	if err != nil {
		return TenantProfile{}, err
	}
	user, err := u.tenantPort.GetUser(ctx, id.TenantID, id.UserID)
	if err != nil {
		return TenantProfile{}, err
	}
	return TenantProfile{Tenant: t, User: user}, nil
}
