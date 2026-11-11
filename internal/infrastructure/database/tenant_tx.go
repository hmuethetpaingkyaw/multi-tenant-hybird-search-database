package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// TenantSetting is the transaction-local setting read by the RLS helper app_current_tenant().
const TenantSetting = "app.tenant_id"

// Beginner is satisfied by *pgxpool.Pool.
type Beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// ErrInvalidTenant is returned when the tenant id is not a usable UUID.
var ErrInvalidTenant = errors.New("database: invalid tenant id")

// WithTenantTx runs fn inside a transaction whose RLS tenant context is tenantID.
//
// The context is set with set_config(..., is_local=true), so it ends with the
// transaction and can never leak to the next user of the pooled connection.
// This is the only way application code should reach tenant-owned tables.
// The tenant id must come from authenticated context, never from client input.
func WithTenantTx(ctx context.Context, db Beginner, tenantID string, fn func(pgx.Tx) error) (err error) {
	id, perr := uuid.Parse(tenantID)
	if perr != nil || id == uuid.Nil {
		return ErrInvalidTenant
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("database: begin tx: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
		}
	}()

	if _, err = tx.Exec(ctx, `SELECT set_config($1, $2, true)`, TenantSetting, id.String()); err != nil {
		return fmt.Errorf("database: set tenant context: %w", err)
	}
	if err = fn(tx); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("database: commit: %w", err)
	}
	return nil
}
