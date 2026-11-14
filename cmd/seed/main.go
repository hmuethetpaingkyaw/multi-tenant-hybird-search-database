// Command seed creates dev tenants A, B, C (one user each) and prints a bearer token per user.
// It connects as the OWNER role (SEED_DATABASE_URL) because tenants are admin-managed;
// users are inserted inside a tenant context because FORCE RLS applies to the owner too.
// It is idempotent. Never point it at production.
package main

import (
	"context"
	"fmt"
	"multi-tenant-hybrid-search/domain"
	"multi-tenant-hybrid-search/driver/db"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"multi-tenant-hybrid-search/driver/auth"
)

type seedTenant struct{ Letter, TenantID, UserID string }

var tenants = []seedTenant{
	{"A", "00000000-0000-0000-0000-00000000000a", "10000000-0000-0000-0000-00000000000a"},
	{"B", "00000000-0000-0000-0000-00000000000b", "10000000-0000-0000-0000-00000000000b"},
	{"C", "00000000-0000-0000-0000-00000000000c", "10000000-0000-0000-0000-00000000000c"},
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}
}

func run() error {
	url, secret := os.Getenv("SEED_DATABASE_URL"), os.Getenv("JWT_SECRET")
	if url == "" {
		return fmt.Errorf("SEED_DATABASE_URL (owner role) is required")
	}
	authn, err := auth.New(secret)
	if err != nil {
		return fmt.Errorf("JWT_SECRET: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()

	for _, t := range tenants {
		if _, err := pool.Exec(ctx,
			`INSERT INTO tenants (id, name) VALUES ($1, $2) ON CONFLICT (id) DO NOTHING`,
			t.TenantID, "tenant-"+t.Letter); err != nil {
			return fmt.Errorf("tenant %s: %w", t.Letter, err)
		}
		err := db.WithTenantTx(ctx, pool, t.TenantID, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx,
				`INSERT INTO users (id, tenant_id, email) VALUES ($1, $2, $3) ON CONFLICT (id) DO NOTHING`,
				t.UserID, t.TenantID, "user-"+t.Letter+"@example.com")
			return err
		})
		if err != nil {
			return fmt.Errorf("user %s: %w", t.Letter, err)
		}
	}

	fmt.Println("# Dev tokens (valid 24h). Use: curl -H \"Authorization: Bearer $TOKEN_A\" localhost:8080/api/v1/me")
	for _, t := range tenants {
		tok, err := authn.Issue(domain.Identity{UserID: t.UserID, TenantID: t.TenantID}, 24*time.Hour)
		if err != nil {
			return err
		}
		fmt.Printf("TOKEN_%s=%s\n", t.Letter, tok)
	}
	return nil
}
