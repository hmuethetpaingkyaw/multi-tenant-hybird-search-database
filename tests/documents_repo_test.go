package tests

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	domain "multi-tenant-hybrid-search/internal/domain/document"
	"multi-tenant-hybrid-search/internal/domain/embedding"
	"multi-tenant-hybrid-search/internal/infrastructure/database"
	infraemb "multi-tenant-hybrid-search/internal/infrastructure/embedding"
)

func testEmb(in domain.Input) embedding.Embedding {
	return embedding.Embedding{Model: "fake-bow-768", Content: in.Title, Vector: infraemb.Vectorize(in.Title + " " + in.Content)}
}

func mkCreate(r *database.DocumentRepository, ctx context.Context, scope string, in domain.Input) (domain.Document, error) {
	return r.Create(ctx, scope, in, testEmb(in))
}

func mkUpdate(r *database.DocumentRepository, ctx context.Context, scope, id string, in domain.Input) (domain.Document, error) {
	return r.Update(ctx, scope, id, in, testEmb(in))
}

// resetTenants leaves tenants A, B, C with no users or documents.
func resetTenants(t *testing.T, e *env) {
	t.Helper()
	mustExec(t, e.super, `DELETE FROM tenants`)
	for _, id := range []string{tenA, tenB, tenC} {
		mustExec(t, e.owner, `INSERT INTO tenants (id, name) VALUES ($1, $2)`, id, "tenant-"+id[len(id)-1:])
	}
}

func TestDocumentRepositoryCRUDAndIsolation(t *testing.T) {
	e := getEnv(t)
	ctx := context.Background()
	resetTenants(t, e)
	repo := database.NewDocumentRepository(e.app)

	created, err := mkCreate(repo, ctx, tenA, domain.Input{Title: "T1", Content: "C1"})
	if err != nil || created.ID == "" || created.Title != "T1" || created.Content != "C1" {
		t.Fatalf("create: %+v err=%v", created, err)
	}
	var owner string
	if err := e.super.QueryRow(ctx, `SELECT tenant_id::text FROM documents WHERE id = $1`, created.ID).Scan(&owner); err != nil || owner != tenA {
		t.Fatalf("document tenant = %q (err %v), want tenant A", owner, err)
	}
	got, err := repo.Get(ctx, tenA, created.ID)
	if err != nil || got.ID != created.ID || got.Content != "C1" {
		t.Fatalf("get: %+v err=%v", got, err)
	}

	updated, err := mkUpdate(repo, ctx, tenA, created.ID, domain.Input{Title: "T2", Content: "C2"})
	if err != nil || updated.Title != "T2" || updated.Content != "C2" ||
		!updated.CreatedAt.Equal(created.CreatedAt) || !updated.UpdatedAt.After(created.UpdatedAt) {
		t.Fatalf("update: %+v (created %+v) err=%v", updated, created, err)
	}

	// B and C cannot see, change or delete A's document; A's document stays unchanged.
	for _, other := range []string{tenB, tenC} {
		if _, err := repo.Get(ctx, other, created.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("get as other: %v", err)
		}
		if _, err := mkUpdate(repo, ctx, other, created.ID, domain.Input{Title: "hacked", Content: "x"}); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("update as other: %v", err)
		}
		if err := repo.Delete(ctx, other, created.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("delete as other: %v", err)
		}
	}
	after, err := repo.Get(ctx, tenA, created.ID)
	if err != nil || after.Title != "T2" || after.Content != "C2" {
		t.Fatalf("A's document changed: %+v err=%v", after, err)
	}

	// Unknown and malformed ids are not-found, not errors.
	for _, id := range []string{"00000000-0000-0000-0000-0000000000ff", "not-a-uuid"} {
		if _, err := repo.Get(ctx, tenA, id); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("get %q: %v", id, err)
		}
		if err := repo.Delete(ctx, tenA, id); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("delete %q: %v", id, err)
		}
	}

	if err := repo.Delete(ctx, tenA, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Get(ctx, tenA, created.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("get after delete: %v", err)
	}
}

func TestDocumentRepositoryList(t *testing.T) {
	e := getEnv(t)
	ctx := context.Background()
	resetTenants(t, e)
	repo := database.NewDocumentRepository(e.app)

	empty, err := repo.List(ctx, tenA, 20, 0)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty list: %v err=%v (want non-nil empty slice)", empty, err)
	}

	for _, ti := range []string{"first", "second", "third"} {
		if _, err := mkCreate(repo, ctx, tenA, domain.Input{Title: ti, Content: "c"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := mkCreate(repo, ctx, tenB, domain.Input{Title: "B's", Content: "c"}); err != nil {
		t.Fatal(err)
	}

	all, err := repo.List(ctx, tenA, 20, 0)
	if err != nil || len(all) != 3 {
		t.Fatalf("A list: %+v err=%v", all, err)
	}
	for i, want := range []string{"third", "second", "first"} { // newest first
		if all[i].Title != want {
			t.Errorf("position %d = %q, want %q", i, all[i].Title, want)
		}
	}
	page, err := repo.List(ctx, tenA, 1, 1)
	if err != nil || len(page) != 1 || page[0].Title != "second" {
		t.Errorf("limit 1 offset 1: %+v err=%v", page, err)
	}
	bList, _ := repo.List(ctx, tenB, 20, 0)
	if len(bList) != 1 || bList[0].Title != "B's" {
		t.Errorf("B list: %+v", bList)
	}
}

func TestDeleteDocumentRemovesEmbeddings(t *testing.T) {
	e := getEnv(t)
	ctx := context.Background()
	resetTenants(t, e)
	repo := database.NewDocumentRepository(e.app)

	d, err := mkCreate(repo, ctx, tenA, domain.Input{Title: "t", Content: "c"})
	if err != nil {
		t.Fatal(err)
	}
	var before int
	if err := e.super.QueryRow(ctx, `SELECT count(*) FROM document_embeddings WHERE document_id = $1`, d.ID).Scan(&before); err != nil || before != 1 {
		t.Fatalf("embedding rows before delete = %d (err %v), want 1", before, err)
	}
	if err := repo.Delete(ctx, tenA, d.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := e.super.QueryRow(ctx, `SELECT count(*) FROM document_embeddings WHERE document_id = $1`, d.ID).Scan(&n); err != nil || n != 0 {
		t.Errorf("embeddings left after delete: %d (err %v)", n, err)
	}
}

func TestFailedWriteLeavesNoPartialRow(t *testing.T) {
	e := getEnv(t)
	ctx := context.Background()
	resetTenants(t, e)
	err := database.WithTenantTx(ctx, e.app, tenA, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO documents (tenant_id, title, content) VALUES ($1,'t','c')`, tenA); err != nil {
			return err
		}
		return errors.New("boom")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	var n int
	if err := e.super.QueryRow(ctx, `SELECT count(*) FROM documents`).Scan(&n); err != nil || n != 0 {
		t.Errorf("partial write persisted: %d rows (err %v)", n, err)
	}
}
