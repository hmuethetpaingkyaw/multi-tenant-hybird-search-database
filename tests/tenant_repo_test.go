package tests

import (
	"context"
	"errors"
	"multi-tenant-hybrid-search/apperror"
	"multi-tenant-hybrid-search/domain"
	"multi-tenant-hybrid-search/driver/db"
	"multi-tenant-hybrid-search/repository"
	"multi-tenant-hybrid-search/tests/testenv"
	"testing"

	"multi-tenant-hybrid-search/usecase"
)

func TestTenantRepositoryAndUsecase(t *testing.T) {
	e := testenv.Get(t)
	ctx := context.Background()
	testenv.SeedThreeTenants(t, e)
	repo := repository.NewTenantRepository(e.App)
	svc := usecase.NewTenantUsecase(repo)
	userA, userB := testenv.UserIDOf(t, e, testenv.TenA), testenv.UserIDOf(t, e, testenv.TenB)

	tn, err := repo.GetTenant(ctx, testenv.TenA, testenv.TenA)
	if err != nil || tn.ID != testenv.TenA || tn.Name != "tenant-a" {
		t.Fatalf("own tenant: %+v err=%v", tn, err)
	}
	if _, err := repo.GetTenant(ctx, testenv.TenA, testenv.TenB); !errors.Is(err, apperror.ErrNotFound) {
		t.Errorf("other tenant's tenant row: want ErrNotFound, got %v", err)
	}
	u, err := repo.GetUser(ctx, testenv.TenA, userA)
	if err != nil || u.TenantID != testenv.TenA {
		t.Fatalf("own user: %+v err=%v", u, err)
	}
	if _, err := repo.GetUser(ctx, testenv.TenA, userB); !errors.Is(err, apperror.ErrNotFound) {
		t.Errorf("other tenant's user: want ErrNotFound, got %v", err)
	}

	p, err := svc.Me(ctx, domain.Identity{UserID: userA, TenantID: testenv.TenA})
	if err != nil || p.Tenant.ID != testenv.TenA || p.User.ID != userA {
		t.Fatalf("Me: %+v err=%v", p, err)
	}
	// Tenant B's context with tenant A's user id: RLS hides the user.
	if _, err := svc.Me(ctx, domain.Identity{UserID: userA, TenantID: testenv.TenB}); !errors.Is(err, apperror.ErrNotFound) {
		t.Errorf("mismatched identity: want ErrNotFound, got %v", err)
	}
	if _, err := repo.GetTenant(ctx, "bad", testenv.TenA); !errors.Is(err, db.ErrInvalidTenant) {
		t.Errorf("invalid scope: want ErrInvalidTenant, got %v", err)
	}
}
