package tests

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"multi-tenant-hybrid-search/internal/infrastructure/database"
)

const image = "paradedb/paradedb:0.26.6-pg17" // keep in sync with docker-compose.yml

type env struct {
	super, owner, app *pgxpool.Pool
	migrate           func() *migrate.Migrate
}

func setup(t *testing.T) *env {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test needs Docker")
	}
	ctx := context.Background()
	initScript, _ := filepath.Abs("../docker/postgres/init/01-roles.sh")
	migDir, _ := filepath.Abs("../migrations")

	ctr, err := postgres.Run(ctx, image,
		postgres.WithDatabase("search"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		postgres.WithInitScripts(initScript),
		testcontainers.WithEnv(map[string]string{"OWNER_PASSWORD": "owner_t", "APP_PASSWORD": "app_t"}),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start container: %v", err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(ctr) })

	host, _ := ctr.Host(ctx)
	port, _ := ctr.MappedPort(ctx, "5432/tcp")
	url := func(user, pw, scheme string) string {
		return scheme + "://" + user + ":" + pw + "@" + host + ":" + port.Port() + "/search?sslmode=disable"
	}
	pool := func(user, pw string) *pgxpool.Pool {
		p, err := pgxpool.New(ctx, url(user, pw, "postgres"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(p.Close)
		return p
	}
	return &env{
		super: pool("postgres", "postgres"),
		owner: pool("search_owner", "owner_t"),
		app:   pool("search_app", "app_t"),
		migrate: func() *migrate.Migrate {
			m, err := migrate.New("file://"+migDir, url("search_owner", "owner_t", "pgx5"))
			if err != nil {
				t.Fatal(err)
			}
			return m
		},
	}
}

func mustExec(t *testing.T, p *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := p.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func TestFoundation(t *testing.T) {
	e := setup(t)
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
		mustExec(t, e.app, `INSERT INTO tenants (name) VALUES ('t')`) // plain DML works pre-RLS
		mustExec(t, e.app, `DELETE FROM tenants`)
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
		mustExec(t, e.owner, `INSERT INTO tenants (id, name) VALUES ($1,'A'), ($2,'B')`, a, b)
		defer mustExec(t, e.owner, `DELETE FROM tenants`)
		mustExec(t, e.owner, `INSERT INTO documents (id, tenant_id, title, content) VALUES ($1,$2,'t','c')`, d, a)

		const ins = `INSERT INTO document_embeddings (tenant_id, document_id, content, model, embedding)
		             VALUES ($1, $2, 'c', 'm', array_fill(0, ARRAY[768])::vector)`
		if _, err := e.owner.Exec(ctx, ins, b, d); err == nil || !strings.Contains(err.Error(), "foreign key") {
			t.Errorf("cross-tenant embedding must violate FK, got: %v", err)
		}
		mustExec(t, e.owner, ins, a, d)
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
