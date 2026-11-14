// Package testenv is the shared integration-test environment: one migrated ParadeDB container per test
// run, the three tenants, and builders for the production stack. It is not imported by production code.
package testenv

import (
	"context"
	"github.com/golang-migrate/migrate/v4"
	"github.com/testcontainers/testcontainers-go"
	"path/filepath"
	"sync"
	"testing"

	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

const image = "paradedb/paradedb:0.26.6-pg17" // keep in sync with docker-compose.yml

// Env is one shared ParadeDB container for the whole package, migrated to the latest version.
type Env struct {
	Super, Owner, App *pgxpool.Pool // superuser (bypasses RLS), migration owner, restricted app role
	AppURL            string
	Migrate           func() *migrate.Migrate
}

var (
	once      sync.Once
	shared    *Env
	sharedErr error
	container *postgres.PostgresContainer
)

// Terminate stops the shared container; call it from TestMain after m.Run().
func Terminate() {
	if container != nil {
		_ = testcontainers.TerminateContainer(container)
	}
}

func Get(t *testing.T) *Env {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test needs Docker")
	}
	once.Do(func() { shared, sharedErr = start() })
	if sharedErr != nil {
		t.Fatalf("start test environment: %v", sharedErr)
	}
	return shared
}

func start() (*Env, error) {
	ctx := context.Background()
	initScript, _ := filepath.Abs("../docker/postgres/init/01-roles.sh")
	migDir, _ := filepath.Abs("../migrations")

	var err error
	container, err = postgres.Run(ctx, image,
		postgres.WithDatabase("search"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		postgres.WithInitScripts(initScript),
		testcontainers.WithEnv(map[string]string{"OWNER_PASSWORD": "owner_t", "APP_PASSWORD": "app_t"}),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		return nil, err
	}
	host, _ := container.Host(ctx)
	port, _ := container.MappedPort(ctx, "5432/tcp")
	url := func(user, pw, scheme string) string {
		return scheme + "://" + user + ":" + pw + "@" + host + ":" + port.Port() + "/search?sslmode=disable"
	}
	mk := func(user, pw string) (*pgxpool.Pool, error) { return pgxpool.New(ctx, url(user, pw, "postgres")) }

	e := &Env{AppURL: url("search_app", "app_t", "postgres")}
	if e.Super, err = mk("postgres", "postgres"); err != nil {
		return nil, err
	}
	if e.Owner, err = mk("search_owner", "owner_t"); err != nil {
		return nil, err
	}
	if e.App, err = mk("search_app", "app_t"); err != nil {
		return nil, err
	}
	e.Migrate = func() *migrate.Migrate {
		m, merr := migrate.New("file://"+migDir, url("search_owner", "owner_t", "pgx5"))
		if merr != nil {
			panic(merr)
		}
		return m
	}
	m := e.Migrate()
	defer m.Close()
	if err = m.Up(); err != nil {
		return nil, err
	}
	return e, nil
}
