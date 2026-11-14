package repository

import (
	"context"
	"errors"
	"multi-tenant-hybrid-search/apperror"
	"multi-tenant-hybrid-search/domain"
	"multi-tenant-hybrid-search/driver/db"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TenantPort reads tenant data. scopeTenantID is the RLS context the query runs under;
// implementations must not add their own tenant filters.
type TenantPort interface {
	GetTenant(ctx context.Context, scopeTenantID, id string) (domain.Tenant, error)
	GetUser(ctx context.Context, scopeTenantID, id string) (domain.User, error)
}

// TenantRepository implements TenantPort. Isolation comes from RLS, not from SQL filters.
type TenantRepository struct{ pool *pgxpool.Pool }

func NewTenantRepository(pool *pgxpool.Pool) TenantPort { return &TenantRepository{pool: pool} }

func (r *TenantRepository) GetTenant(ctx context.Context, scopeTenantID, id string) (domain.Tenant, error) {
	var t domain.Tenant
	err := db.WithTenantTx(ctx, r.pool, scopeTenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id::text, name, created_at FROM tenants WHERE id = $1`, id).
			Scan(&t.ID, &t.Name, &t.CreatedAt)
	})
	return t, mapNotFound(err)
}

func (r *TenantRepository) GetUser(ctx context.Context, scopeTenantID, id string) (domain.User, error) {
	var u domain.User
	err := db.WithTenantTx(ctx, r.pool, scopeTenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id::text, tenant_id::text, email, created_at FROM users WHERE id = $1`, id).
			Scan(&u.ID, &u.TenantID, &u.Email, &u.CreatedAt)
	})
	return u, mapNotFound(err)
}

func mapNotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperror.ErrNotFound
	}
	return err
}
