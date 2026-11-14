package usecase

import (
	"context"
	"errors"
	"testing"

	"multi-tenant-hybrid-search/apperror"
	"multi-tenant-hybrid-search/domain"
	"multi-tenant-hybrid-search/repository/mock"
)

func TestMeReturnsTenantAndUserScopedToIdentity(t *testing.T) {
	m := &mock_repository.MockTenantPort{Tenant: domain.Tenant{ID: ident.TenantID, Name: "A"}, User: domain.User{ID: "u", Email: "a@x"}}
	p, err := NewTenantUsecase(m).Me(context.Background(), ident)
	if err != nil || p.Tenant.Name != "A" || p.User.Email != "a@x" {
		t.Fatalf("profile=%+v err=%v", p, err)
	}
	if len(m.Scopes) != 2 || m.Scopes[0] != ident.TenantID || m.Scopes[1] != ident.TenantID {
		t.Errorf("scopes = %v", m.Scopes)
	}
}

func TestMeNotFoundWhenTenantOrUserInvisible(t *testing.T) {
	for name, m := range map[string]*mock_repository.MockTenantPort{
		"tenant": {TenantErr: apperror.ErrNotFound},
		"user":   {UserErr: apperror.ErrNotFound},
	} {
		if _, err := NewTenantUsecase(m).Me(context.Background(), ident); !errors.Is(err, apperror.ErrNotFound) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
