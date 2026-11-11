// Package tenant contains tenant/user use cases.
package tenant

import (
	"context"

	domain "multi-tenant-hybrid-search/internal/domain/tenant"
)

type Service struct{ repo domain.Repository }

func NewService(repo domain.Repository) *Service { return &Service{repo: repo} }

// Profile is the caller's tenant and user.
type Profile struct {
	Tenant domain.Tenant
	User   domain.User
}

// Me returns the caller's tenant and user. Either being invisible under RLS yields ErrNotFound.
func (s *Service) Me(ctx context.Context, id domain.Identity) (Profile, error) {
	t, err := s.repo.GetTenant(ctx, id.TenantID, id.TenantID)
	if err != nil {
		return Profile{}, err
	}
	u, err := s.repo.GetUser(ctx, id.TenantID, id.UserID)
	if err != nil {
		return Profile{}, err
	}
	return Profile{Tenant: t, User: u}, nil
}
