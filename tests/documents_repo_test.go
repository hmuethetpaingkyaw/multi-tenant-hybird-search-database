package tests

import (
	"context"
	"errors"
	"multi-tenant-hybrid-search/api/ollama/mock"
	"multi-tenant-hybrid-search/apperror"
	"multi-tenant-hybrid-search/domain"
	"multi-tenant-hybrid-search/driver/db"
	"multi-tenant-hybrid-search/repository"
	"multi-tenant-hybrid-search/tests/testenv"
	"testing"

	"github.com/jackc/pgx/v5"
)

func testEmb(in domain.DocumentInput) domain.DocumentEmbedding {
	return domain.DocumentEmbedding{Model: "fake-bow-768", Content: in.Title, Vector: mock_ollama.Vectorize(in.Title + " " + in.Content)}
}

func mkCreate(r repository.DocumentPort, ctx context.Context, scope string, in domain.DocumentInput) (domain.Document, error) {
	return r.Create(ctx, scope, in, testEmb(in))
}

func mkUpdate(r repository.DocumentPort, ctx context.Context, scope, id string, in domain.DocumentInput) (domain.Document, error) {
	return r.Update(ctx, scope, id, in, testEmb(in))
}

func TestDocumentRepositoryCRUDAndIsolation(t *testing.T) {
	e := testenv.Get(t)
	ctx := context.Background()
	testenv.ResetTenants(t, e)
	repo := repository.NewDocumentRepository(e.App)

	created, err := mkCreate(repo, ctx, testenv.TenA, domain.DocumentInput{Title: "T1", Content: "C1"})
	if err != nil || created.ID == "" || created.Title != "T1" || created.Content != "C1" {
		t.Fatalf("create: %+v err=%v", created, err)
	}
	var owner string
	if err := e.Super.QueryRow(ctx, `SELECT tenant_id::text FROM documents WHERE id = $1`, created.ID).Scan(&owner); err != nil || owner != testenv.TenA {
		t.Fatalf("document tenant = %q (err %v), want tenant A", owner, err)
	}
	got, err := repo.Get(ctx, testenv.TenA, created.ID)
	if err != nil || got.ID != created.ID || got.Content != "C1" {
		t.Fatalf("get: %+v err=%v", got, err)
	}

	updated, err := mkUpdate(repo, ctx, testenv.TenA, created.ID, domain.DocumentInput{Title: "T2", Content: "C2"})
	if err != nil || updated.Title != "T2" || updated.Content != "C2" ||
		!updated.CreatedAt.Equal(created.CreatedAt) || !updated.UpdatedAt.After(created.UpdatedAt) {
		t.Fatalf("update: %+v (created %+v) err=%v", updated, created, err)
	}

	// B and C cannot see, change or delete A's document; A's document stays unchanged.
	for _, other := range []string{testenv.TenB, testenv.TenC} {
		if _, err := repo.Get(ctx, other, created.ID); !errors.Is(err, apperror.ErrNotFound) {
			t.Errorf("get as other: %v", err)
		}
		if _, err := mkUpdate(repo, ctx, other, created.ID, domain.DocumentInput{Title: "hacked", Content: "x"}); !errors.Is(err, apperror.ErrNotFound) {
			t.Errorf("update as other: %v", err)
		}
		if err := repo.Delete(ctx, other, created.ID); !errors.Is(err, apperror.ErrNotFound) {
			t.Errorf("delete as other: %v", err)
		}
	}
	after, err := repo.Get(ctx, testenv.TenA, created.ID)
	if err != nil || after.Title != "T2" || after.Content != "C2" {
		t.Fatalf("A's document changed: %+v err=%v", after, err)
	}

	// Unknown and malformed ids are not-found, not errors.
	for _, id := range []string{"00000000-0000-0000-0000-0000000000ff", "not-a-uuid"} {
		if _, err := repo.Get(ctx, testenv.TenA, id); !errors.Is(err, apperror.ErrNotFound) {
			t.Errorf("get %q: %v", id, err)
		}
		if err := repo.Delete(ctx, testenv.TenA, id); !errors.Is(err, apperror.ErrNotFound) {
			t.Errorf("delete %q: %v", id, err)
		}
	}

	if err := repo.Delete(ctx, testenv.TenA, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Get(ctx, testenv.TenA, created.ID); !errors.Is(err, apperror.ErrNotFound) {
		t.Errorf("get after delete: %v", err)
	}
}

func TestDocumentRepositoryList(t *testing.T) {
	e := testenv.Get(t)
	ctx := context.Background()
	testenv.ResetTenants(t, e)
	repo := repository.NewDocumentRepository(e.App)

	empty, err := repo.List(ctx, testenv.TenA, 20, 0)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty list: %v err=%v (want non-nil empty slice)", empty, err)
	}

	for _, ti := range []string{"first", "second", "third"} {
		if _, err := mkCreate(repo, ctx, testenv.TenA, domain.DocumentInput{Title: ti, Content: "c"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := mkCreate(repo, ctx, testenv.TenB, domain.DocumentInput{Title: "B's", Content: "c"}); err != nil {
		t.Fatal(err)
	}

	all, err := repo.List(ctx, testenv.TenA, 20, 0)
	if err != nil || len(all) != 3 {
		t.Fatalf("A list: %+v err=%v", all, err)
	}
	for i, want := range []string{"third", "second", "first"} { // newest first
		if all[i].Title != want {
			t.Errorf("position %d = %q, want %q", i, all[i].Title, want)
		}
	}
	page, err := repo.List(ctx, testenv.TenA, 1, 1)
	if err != nil || len(page) != 1 || page[0].Title != "second" {
		t.Errorf("limit 1 offset 1: %+v err=%v", page, err)
	}
	bList, _ := repo.List(ctx, testenv.TenB, 20, 0)
	if len(bList) != 1 || bList[0].Title != "B's" {
		t.Errorf("B list: %+v", bList)
	}
}

func TestDeleteDocumentRemovesEmbeddings(t *testing.T) {
	e := testenv.Get(t)
	ctx := context.Background()
	testenv.ResetTenants(t, e)
	repo := repository.NewDocumentRepository(e.App)

	d, err := mkCreate(repo, ctx, testenv.TenA, domain.DocumentInput{Title: "t", Content: "c"})
	if err != nil {
		t.Fatal(err)
	}
	var before int
	if err := e.Super.QueryRow(ctx, `SELECT count(*) FROM document_embeddings WHERE document_id = $1`, d.ID).Scan(&before); err != nil || before != 1 {
		t.Fatalf("embedding rows before delete = %d (err %v), want 1", before, err)
	}
	if err := repo.Delete(ctx, testenv.TenA, d.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := e.Super.QueryRow(ctx, `SELECT count(*) FROM document_embeddings WHERE document_id = $1`, d.ID).Scan(&n); err != nil || n != 0 {
		t.Errorf("embeddings left after delete: %d (err %v)", n, err)
	}
}

func TestFailedWriteLeavesNoPartialRow(t *testing.T) {
	e := testenv.Get(t)
	ctx := context.Background()
	testenv.ResetTenants(t, e)
	err := db.WithTenantTx(ctx, e.App, testenv.TenA, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO documents (tenant_id, title, content) VALUES ($1,'t','c')`, testenv.TenA); err != nil {
			return err
		}
		return errors.New("boom")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	var n int
	if err := e.Super.QueryRow(ctx, `SELECT count(*) FROM documents`).Scan(&n); err != nil || n != 0 {
		t.Errorf("partial write persisted: %d rows (err %v)", n, err)
	}
}
