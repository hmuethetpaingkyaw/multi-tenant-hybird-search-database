package tests

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	appdoc "multi-tenant-hybrid-search/internal/application/document"
	"multi-tenant-hybrid-search/internal/auth"
	httpapi "multi-tenant-hybrid-search/internal/delivery/http"
	"multi-tenant-hybrid-search/internal/delivery/middleware"
	docdomain "multi-tenant-hybrid-search/internal/domain/document"
	embdomain "multi-tenant-hybrid-search/internal/domain/embedding"
	"multi-tenant-hybrid-search/internal/domain/tenant"
	"multi-tenant-hybrid-search/internal/infrastructure/database"
	"multi-tenant-hybrid-search/internal/infrastructure/embedding"
)

// vecText renders a vector the way pgvector parses it: "[0.1,0.2,...]".
func vecText(v []float32) string {
	var b strings.Builder
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

func TestHNSWIndexIsUsedForCosineSearch(t *testing.T) {
	e := getEnv(t)
	ctx := context.Background()
	resetTenants(t, e)

	var exists bool
	if err := e.super.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'document_embeddings_embedding_hnsw_idx')`).Scan(&exists); err != nil || !exists {
		t.Fatalf("hnsw index missing (err %v)", err)
	}

	err := database.WithTenantTx(ctx, e.app, tenA, func(tx pgx.Tx) error {
		for _, text := range []string{"alpha beta", "gamma delta", "epsilon zeta"} {
			var docID string
			if err := tx.QueryRow(ctx, `INSERT INTO documents (tenant_id, title, content) VALUES ($1,$2,'c') RETURNING id::text`, tenA, text).Scan(&docID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO document_embeddings (tenant_id, document_id, content, model, embedding)
				VALUES ($1,$2,$3,'m',$4::vector)`, tenA, docID, text, vecText(embedding.Vectorize(text))); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// The index must be valid for the cosine operator. Checked as superuser (no RLS qual) with scans
	// that would bypass it disabled; how the planner chooses under RLS at scale is Phase 6 analysis
	// (for tiny tables it prefers the tenant_id index plus a sort).
	tx, err := e.super.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	for _, set := range []string{`SET LOCAL enable_seqscan = off`, `SET LOCAL enable_bitmapscan = off`} {
		if _, err := tx.Exec(ctx, set); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := tx.Query(ctx, `EXPLAIN SELECT document_id FROM document_embeddings ORDER BY embedding <=> '`+
		vecText(embedding.Vectorize("alpha"))+`'::vector LIMIT 5`, pgx.QueryExecModeSimpleProtocol)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(plan, "\n"), "document_embeddings_embedding_hnsw_idx") {
		t.Errorf("plan does not use the HNSW index:\n%s", strings.Join(plan, "\n"))
	}
}

// ---- write path (step 4) ----

type embRow struct {
	Tenant, Model, Content string
	Dims                   int
	Vec                    string
}

func embRows(t *testing.T, e *env, docID string) []embRow {
	t.Helper()
	rows, err := e.super.Query(context.Background(),
		`SELECT tenant_id::text, model, content, vector_dims(embedding), embedding::text
		 FROM document_embeddings WHERE document_id = $1`, docID)
	if err != nil {
		t.Fatal(err)
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (embRow, error) {
		var x embRow
		return x, r.Scan(&x.Tenant, &x.Model, &x.Content, &x.Dims, &x.Vec)
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func newDocService(e *env, emb *embedding.Fake) *appdoc.Service {
	return appdoc.NewService(database.NewDocumentRepository(e.app), emb)
}

var ident = func(tenantID string) tenant.Identity {
	return tenant.Identity{UserID: "10000000-0000-0000-0000-000000000001", TenantID: tenantID}
}

func TestEmbeddingWritePath(t *testing.T) {
	e := getEnv(t)
	ctx := context.Background()
	resetTenants(t, e)
	fake := &embedding.Fake{}
	svc := newDocService(e, fake)

	d, err := svc.Create(ctx, ident(tenA), docdomain.Input{Title: "Account Recovery", Content: "Reset your password by email"})
	if err != nil {
		t.Fatal(err)
	}
	rows := embRows(t, e, d.ID)
	if len(rows) != 1 || rows[0].Tenant != tenA || rows[0].Model != fake.Model() || rows[0].Dims != 768 ||
		rows[0].Content != "Account Recovery\n\nReset your password by email" {
		t.Fatalf("create embedding rows: %+v", rows)
	}
	first := rows[0].Vec

	// Update replaces the single row.
	if _, err := svc.Update(ctx, ident(tenA), d.ID, docdomain.Input{Title: "Billing", Content: "Invoices and refunds"}); err != nil {
		t.Fatal(err)
	}
	rows = embRows(t, e, d.ID)
	if len(rows) != 1 || rows[0].Content != "Billing\n\nInvoices and refunds" || rows[0].Vec == first {
		t.Fatalf("update embedding rows: %+v", rows)
	}

	// Delete removes it.
	if err := svc.Delete(ctx, ident(tenA), d.ID); err != nil {
		t.Fatal(err)
	}
	if rows = embRows(t, e, d.ID); len(rows) != 0 {
		t.Fatalf("embedding left after delete: %+v", rows)
	}
}

func TestLongDocumentEmbedsTruncatedChunkButKeepsFullContent(t *testing.T) {
	e := getEnv(t)
	ctx := context.Background()
	resetTenants(t, e)
	long := strings.Repeat("é", 9000) // multibyte: truncation counts characters, not bytes
	d, err := newDocService(e, &embedding.Fake{}).Create(ctx, ident(tenA), docdomain.Input{Title: "T", Content: long})
	if err != nil {
		t.Fatal(err)
	}
	rows := embRows(t, e, d.ID)
	if len(rows) != 1 || len([]rune(rows[0].Content)) != appdoc.MaxEmbedChars {
		t.Fatalf("chunk length = %d, want %d", len([]rune(rows[0].Content)), appdoc.MaxEmbedChars)
	}
	var contentLen int
	if err := e.super.QueryRow(ctx, `SELECT char_length(content) FROM documents WHERE id = $1`, d.ID).Scan(&contentLen); err != nil || contentLen != 9000 {
		t.Errorf("stored document content length = %d (err %v), want 9000", contentLen, err)
	}
}

func TestEmbeddingFailureWritesNothingAndLeavesUpdatesUnchanged(t *testing.T) {
	e := getEnv(t)
	ctx := context.Background()
	resetTenants(t, e)
	fake := &embedding.Fake{}
	svc := newDocService(e, fake)

	// A document that exists and is embedded.
	d, err := svc.Create(ctx, ident(tenA), docdomain.Input{Title: "Original", Content: "original body"})
	if err != nil {
		t.Fatal(err)
	}
	before := embRows(t, e, d.ID)

	fake.Err = errors.New("connection refused") // plain error: the service must still report ErrUnavailable
	if _, err := svc.Create(ctx, ident(tenA), docdomain.Input{Title: "New", Content: "new body"}); !errors.Is(err, embdomain.ErrUnavailable) {
		t.Errorf("create with failing embedder: %v", err)
	}
	var docs, embs int
	_ = e.super.QueryRow(ctx, `SELECT count(*) FROM documents`).Scan(&docs)
	_ = e.super.QueryRow(ctx, `SELECT count(*) FROM document_embeddings`).Scan(&embs)
	if docs != 1 || embs != 1 {
		t.Errorf("failed create left rows: documents=%d embeddings=%d, want 1/1", docs, embs)
	}

	if _, err := svc.Update(ctx, ident(tenA), d.ID, docdomain.Input{Title: "Changed", Content: "changed body"}); !errors.Is(err, embdomain.ErrUnavailable) {
		t.Errorf("update with failing embedder: %v", err)
	}
	got, err := database.NewDocumentRepository(e.app).Get(ctx, tenA, d.ID)
	if err != nil || got.Title != "Original" || got.Content != "original body" {
		t.Errorf("document changed by failed update: %+v err=%v", got, err)
	}
	if after := embRows(t, e, d.ID); len(after) != 1 || after[0] != before[0] {
		t.Errorf("embedding changed by failed update: %+v vs %+v", after, before)
	}
}

func TestEmbeddingFailureIs503OverHTTPAndLogged(t *testing.T) {
	e := getEnv(t)
	resetTenants(t, e)
	fake := &embedding.Fake{Err: errors.New("dial tcp ollama:11434: connection refused")}
	authn, _ := auth.New("0123456789abcdef0123456789abcdef")
	logs := &bytes.Buffer{}
	router := httpapi.NewRouter(httpapi.Deps{
		Ready:     func(context.Context) error { return nil },
		Log:       slog.New(slog.NewTextHandler(logs, nil)),
		Auth:      middleware.Auth(authn),
		Documents: newDocService(e, fake),
	})
	tok, _ := authn.Issue(ident(tenA), time.Hour)
	req := httptest.NewRequest("POST", "/api/v1/documents", strings.NewReader(`{"title":"t","content":"c"}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 503 || strings.TrimSpace(w.Body.String()) != `{"error":"embedding service unavailable"}` {
		t.Errorf("response = %d %s", w.Code, w.Body)
	}
	if !strings.Contains(logs.String(), "connection refused") {
		t.Errorf("cause must be logged for operators: %s", logs)
	}
	var n int
	_ = e.super.QueryRow(context.Background(), `SELECT count(*) FROM documents`).Scan(&n)
	if n != 0 {
		t.Errorf("document written despite embedding failure: %d", n)
	}
}
