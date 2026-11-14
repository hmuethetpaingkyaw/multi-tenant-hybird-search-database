package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// RequiredExtensions must be installed for search to work.
var RequiredExtensions = []string{"vector", "pg_search"}

// Querier is satisfied by *pgxpool.Pool and pgx.Tx.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// CheckExtensions returns an error naming every required extension that is not installed.
func CheckExtensions(ctx context.Context, q Querier) error {
	rows, err := q.Query(ctx, `SELECT extname FROM pg_extension WHERE extname = ANY($1)`, RequiredExtensions)
	if err != nil {
		return fmt.Errorf("database: list extensions: %w", err)
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return fmt.Errorf("database: list extensions: %w", err)
	}
	have := make(map[string]bool, len(names))
	for _, n := range names {
		have[n] = true
	}
	var missing []string
	for _, n := range RequiredExtensions {
		if !have[n] {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("database: missing required extension(s): %s", strings.Join(missing, ", "))
	}
	return nil
}
