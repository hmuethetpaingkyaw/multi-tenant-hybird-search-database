package tests

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

const image = "paradedb/paradedb:0.26.6-pg17" // keep in sync with docker-compose.yml

// env is one shared ParadeDB container for the whole package, migrated to the latest version.
type env struct {
	super, owner, app *pgxpool.Pool
	appURL            string
	migrate           func() *migrate.Migrate
}

var (
	once      sync.Once
	shared    *env
	sharedErr error
	container *postgres.PostgresContainer
)

func TestMain(m *testing.M) {
	code := m.Run()
	if container != nil {
		_ = testcontainers.TerminateContainer(container)
	}
	os.Exit(code)
}

func getEnv(t *testing.T) *env {
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

func start() (*env, error) {
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

	e := &env{appURL: url("search_app", "app_t", "postgres")}
	if e.super, err = mk("postgres", "postgres"); err != nil {
		return nil, err
	}
	if e.owner, err = mk("search_owner", "owner_t"); err != nil {
		return nil, err
	}
	if e.app, err = mk("search_app", "app_t"); err != nil {
		return nil, err
	}
	e.migrate = func() *migrate.Migrate {
		m, merr := migrate.New("file://"+migDir, url("search_owner", "owner_t", "pgx5"))
		if merr != nil {
			panic(merr)
		}
		return m
	}
	m := e.migrate()
	defer m.Close()
	if err = m.Up(); err != nil {
		return nil, err
	}
	return e, nil
}
