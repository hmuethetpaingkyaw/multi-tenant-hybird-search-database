package database

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	domain "multi-tenant-hybrid-search/internal/domain/search"
)

// SearchRepository implements domain.Repository. Tenant isolation comes from RLS on both tables.
type SearchRepository struct{ pool *pgxpool.Pool }

func NewSearchRepository(pool *pgxpool.Pool) *SearchRepository { return &SearchRepository{pool: pool} }

// SemanticSQL finds the nearest chunks first (so the HNSW index can serve the ORDER BY ... LIMIT),
// then joins documents for titles. relaxed_order iterative scans may return slightly out-of-order
// rows, hence the final ORDER BY on distance.
const SemanticSQL = `
WITH nearest AS MATERIALIZED (
    SELECT e.document_id, e.embedding <=> $1::vector AS distance
    FROM document_embeddings e
    ORDER BY e.embedding <=> $1::vector
    LIMIT $2
)
SELECT d.id::text, d.title, 1 - n.distance
FROM nearest n JOIN documents d ON d.id = n.document_id
ORDER BY n.distance, d.id`

func (r *SearchRepository) Semantic(ctx context.Context, scope string, queryVector []float32, limit int) ([]domain.Result, error) {
	out := []domain.Result{}
	err := WithTenantTx(ctx, r.pool, scope, func(tx pgx.Tx) error {
		// With a global HNSW index and RLS filtering afterwards, a plain index scan stops after
		// hnsw.ef_search candidates, which can all belong to other tenants. Iterative scan keeps going.
		if _, err := tx.Exec(ctx, `SELECT set_config('hnsw.iterative_scan', 'relaxed_order', true)`); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, SemanticSQL, vectorLiteral(queryVector), limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var res domain.Result
			if err := rows.Scan(&res.DocumentID, &res.Title, &res.Score); err != nil {
				return err
			}
			out = append(out, res)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
