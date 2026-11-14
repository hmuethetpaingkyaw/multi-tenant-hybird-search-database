package mock_repository

import (
	"context"

	"multi-tenant-hybrid-search/domain"
)

// MockTenantPort returns the canned values/errors and records the scope of each call.
type MockTenantPort struct {
	Tenant    domain.Tenant
	User      domain.User
	TenantErr error
	UserErr   error
	Scopes    []string
}

func (m *MockTenantPort) GetTenant(_ context.Context, scope, _ string) (domain.Tenant, error) {
	m.Scopes = append(m.Scopes, scope)
	return m.Tenant, m.TenantErr
}

func (m *MockTenantPort) GetUser(_ context.Context, scope, _ string) (domain.User, error) {
	m.Scopes = append(m.Scopes, scope)
	return m.User, m.UserErr
}
