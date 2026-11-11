package tests

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"multi-tenant-hybrid-search/internal/infrastructure/database"
)

func mustExec(t *testing.T, p *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := p.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func TestFoundation(t *testing.T) {
	e := getEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	t.Run("migrations up/down/up", func(t *testing.T) {
		m := e.migrate()
		defer m.Close()
		for _, step := range []struct {
			name string
			fn   func() error
		}{{"up", m.Up}, {"down", m.Down}, {"up again", m.Up}} {
			if err := step.fn(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
				t.Fatalf("%s: %v", step.name, err)
			}
		}
	})

	t.Run("role attributes", func(t *testing.T) {
		for _, role := range []string{"search_owner", "search_app"} {
			var super, bypass bool
			err := e.super.QueryRow(ctx,
				`SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = $1`, role).Scan(&super, &bypass)
			if err != nil {
				t.Fatal(err)
			}
			if super || bypass {
				t.Errorf("%s must be neither superuser nor BYPASSRLS (super=%v bypass=%v)", role, super, bypass)
			}
		}
	})

	t.Run("app role privileges are limited", func(t *testing.T) {
		for _, q := range []string{`INSERT INTO tenants (name) VALUES ('t')`, `UPDATE tenants SET name = 'x'`, `DELETE FROM tenants`} {
			if _, err := e.app.Exec(ctx, q); err == nil {
				t.Errorf("app role must not run %q (tenants are admin-managed)", q)
			}
		}
		if _, err := e.app.Exec(ctx, `CREATE TABLE evil (i int)`); err == nil {
			t.Error("app role must not create tables")
		}
		if _, err := e.app.Exec(ctx, `DROP TABLE documents`); err == nil {
			t.Error("app role must not drop tables")
		}
	})

	t.Run("composite FK rejects cross-tenant embedding", func(t *testing.T) {
		const a, b, d = "00000000-0000-0000-0000-00000000000a", "00000000-0000-0000-0000-00000000000b",
			"00000000-0000-0000-0000-0000000000d1"
		mustExec(t, e.super, `DELETE FROM tenants`)
		mustExec(t, e.owner, `INSERT INTO tenants (id, name) VALUES ($1,'A'), ($2,'B')`, a, b)
		defer mustExec(t, e.super, `DELETE FROM tenants`)

		// Tenant A owns the document. Tenant B (valid RLS context for B) points at it: only the FK can stop this.
		err := database.WithTenantTx(ctx, e.app, a, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `INSERT INTO documents (id, tenant_id, title, content) VALUES ($1,$2,'t','c')`, d, a)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		const ins = `INSERT INTO document_embeddings (tenant_id, document_id, content, model, embedding)
		             VALUES ($1, $2, 'c', 'm', array_fill(0, ARRAY[768])::vector)`
		err = database.WithTenantTx(ctx, e.app, b, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, ins, b, d)
			return err
		})
		if err == nil || !strings.Contains(err.Error(), "foreign key") {
			t.Errorf("cross-tenant embedding must violate FK, got: %v", err)
		}
		err = database.WithTenantTx(ctx, e.app, a, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, ins, a, d)
			return err
		})
		if err != nil {
			t.Errorf("same-tenant embedding should succeed: %v", err)
		}
	})

	t.Run("extension check", func(t *testing.T) {
		if err := database.CheckExtensions(ctx, e.app); err != nil {
			t.Fatalf("extensions should be present: %v", err)
		}
		mustExec(t, e.super, `DROP EXTENSION pg_search`)
		err := database.CheckExtensions(ctx, e.app)
		if err == nil || !strings.Contains(err.Error(), "pg_search") {
			t.Errorf("want error naming pg_search, got: %v", err)
		}
		mustExec(t, e.super, `CREATE EXTENSION pg_search`)
	})
}
