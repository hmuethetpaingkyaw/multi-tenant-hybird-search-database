package tests

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	appt "multi-tenant-hybrid-search/internal/application/tenant"
	domain "multi-tenant-hybrid-search/internal/domain/tenant"
	"multi-tenant-hybrid-search/internal/infrastructure/database"
)

func userIDOf(t *testing.T, e *env, tenant string) string {
	t.Helper()
	var id string
	err := database.WithTenantTx(context.Background(), e.app, tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT id::text FROM users`).Scan(&id)
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestTenantRepositoryAndService(t *testing.T) {
	e := getEnv(t)
	ctx := context.Background()
	seedThreeTenants(t, e)
	repo := database.NewTenantRepository(e.app)
	svc := appt.NewService(repo)
	userA, userB := userIDOf(t, e, tenA), userIDOf(t, e, tenB)

	tn, err := repo.GetTenant(ctx, tenA, tenA)
	if err != nil || tn.ID != tenA || tn.Name != "tenant-a" {
		t.Fatalf("own tenant: %+v err=%v", tn, err)
	}
	if _, err := repo.GetTenant(ctx, tenA, tenB); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("other tenant's tenant row: want ErrNotFound, got %v", err)
	}
	u, err := repo.GetUser(ctx, tenA, userA)
	if err != nil || u.TenantID != tenA {
		t.Fatalf("own user: %+v err=%v", u, err)
	}
	if _, err := repo.GetUser(ctx, tenA, userB); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("other tenant's user: want ErrNotFound, got %v", err)
	}

	p, err := svc.Me(ctx, domain.Identity{UserID: userA, TenantID: tenA})
	if err != nil || p.Tenant.ID != tenA || p.User.ID != userA {
		t.Fatalf("Me: %+v err=%v", p, err)
	}
	// Tenant B's context with tenant A's user id: RLS hides the user.
	if _, err := svc.Me(ctx, domain.Identity{UserID: userA, TenantID: tenB}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("mismatched identity: want ErrNotFound, got %v", err)
	}
	if _, err := repo.GetTenant(ctx, "bad", tenA); !errors.Is(err, database.ErrInvalidTenant) {
		t.Errorf("invalid scope: want ErrInvalidTenant, got %v", err)
	}
}
