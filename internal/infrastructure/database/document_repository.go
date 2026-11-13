package database

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	domain "multi-tenant-hybrid-search/internal/domain/document"
	"multi-tenant-hybrid-search/internal/domain/embedding"
)

// DocumentRepository implements domain.Repository. Isolation comes from RLS (via WithTenantTx),
// not from SQL filters: rows are addressed by id only, and tenant_id on INSERT is the caller's own
// tenant (RLS WITH CHECK is the backstop).
type DocumentRepository struct{ pool *pgxpool.Pool }

func NewDocumentRepository(pool *pgxpool.Pool) *DocumentRepository {
	return &DocumentRepository{pool: pool}
}

const docColumns = `id::text, title, content, created_at, updated_at`

func scanDoc(row pgx.Row, d *domain.Document) error {
	return row.Scan(&d.ID, &d.Title, &d.Content, &d.CreatedAt, &d.UpdatedAt)
}

// Create stores the document and its embedding in one transaction (the embedding was computed beforehand,
// never inside the transaction).
func (r *DocumentRepository) Create(ctx context.Context, scope string, in domain.Input, emb embedding.Embedding) (domain.Document, error) {
	var d domain.Document
	err := WithTenantTx(ctx, r.pool, scope, func(tx pgx.Tx) error {
		if err := scanDoc(tx.QueryRow(ctx,
			`INSERT INTO documents (tenant_id, title, content) VALUES ($1, $2, $3) RETURNING `+docColumns,
			scope, in.Title, in.Content), &d); err != nil {
			return err
		}
		return upsertEmbedding(ctx, tx, scope, d.ID, emb)
	})
	return d, err
}

func (r *DocumentRepository) Get(ctx context.Context, scope, id string) (domain.Document, error) {
	var d domain.Document
	if !validID(id) {
		return d, domain.ErrNotFound
	}
	err := WithTenantTx(ctx, r.pool, scope, func(tx pgx.Tx) error {
		return scanDoc(tx.QueryRow(ctx, `SELECT `+docColumns+` FROM documents WHERE id = $1`, id), &d)
	})
	return d, mapDocNotFound(err)
}

func (r *DocumentRepository) List(ctx context.Context, scope string, limit, offset int) ([]domain.Summary, error) {
	out := []domain.Summary{}
	err := WithTenantTx(ctx, r.pool, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT id::text, title, created_at, updated_at FROM documents
			 ORDER BY created_at DESC, id LIMIT $1 OFFSET $2`, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var s domain.Summary
			if err := rows.Scan(&s.ID, &s.Title, &s.CreatedAt, &s.UpdatedAt); err != nil {
				return err
			}
			out = append(out, s)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Update replaces the document and its embedding in one transaction.
func (r *DocumentRepository) Update(ctx context.Context, scope, id string, in domain.Input, emb embedding.Embedding) (domain.Document, error) {
	var d domain.Document
	if !validID(id) {
		return d, domain.ErrNotFound
	}
	err := WithTenantTx(ctx, r.pool, scope, func(tx pgx.Tx) error {
		if err := scanDoc(tx.QueryRow(ctx,
			`UPDATE documents SET title = $2, content = $3, updated_at = now() WHERE id = $1 RETURNING `+docColumns,
			id, in.Title, in.Content), &d); err != nil {
			return err
		}
		return upsertEmbedding(ctx, tx, scope, d.ID, emb)
	})
	return d, mapDocNotFound(err)
}

// upsertEmbedding writes chunk 0 (one chunk per document for now). tenant_id is the caller's tenant;
// the composite FK and RLS WITH CHECK are the backstops.
func upsertEmbedding(ctx context.Context, tx pgx.Tx, scope, docID string, emb embedding.Embedding) error {
	_, err := tx.Exec(ctx, `INSERT INTO document_embeddings (tenant_id, document_id, chunk_index, content, model, embedding)
		VALUES ($1, $2, 0, $3, $4, $5::vector)
		ON CONFLICT (document_id, chunk_index) DO UPDATE
		SET content = EXCLUDED.content, model = EXCLUDED.model, embedding = EXCLUDED.embedding, created_at = now()`,
		scope, docID, emb.Content, emb.Model, vectorLiteral(emb.Vector))
	return err
}

// vectorLiteral renders a vector in pgvector's text format, "[0.1,0.2,...]", so no extra driver is needed.
func vectorLiteral(v []float32) string {
	var b strings.Builder
	b.Grow(len(v) * 10)
	b.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(x), 'g', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}

func (r *DocumentRepository) Delete(ctx context.Context, scope, id string) error {
	if !validID(id) {
		return domain.ErrNotFound
	}
	return WithTenantTx(ctx, r.pool, scope, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM documents WHERE id = $1`, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

func validID(id string) bool {
	_, err := uuid.Parse(id)
	return err == nil
}

func mapDocNotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	return err
}
