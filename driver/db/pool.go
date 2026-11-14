// Package database provides the pgx connection pool and startup checks.
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"multi-tenant-hybrid-search/config"
)

// NewPool builds a pgx pool from config and verifies connectivity.
// The pool must connect as the restricted app role.
func NewPool(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error) {
	pc, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("database: invalid DATABASE_URL: %w", err)
	}
	pc.MaxConns = cfg.DBMaxConns
	pc.MinConns = cfg.DBMinConns
	pc.MaxConnLifetime = cfg.DBMaxConnLifetime
	pc.HealthCheckPeriod = cfg.DBHealthCheck

	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return nil, fmt.Errorf("database: create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database: cannot reach database at %s:%d: %w",
			pc.ConnConfig.Host, pc.ConnConfig.Port, err)
	}
	return pool, nil
}
