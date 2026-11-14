package tests

import (
	"context"
	"fmt"
	"multi-tenant-hybrid-search/driver/db"
	"multi-tenant-hybrid-search/tests/testenv"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func count(t *testing.T, tx pgx.Tx, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := tx.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return n
}

func asTenant(t *testing.T, e *testenv.Env, tenant string, fn func(tx pgx.Tx)) {
	t.Helper()
	if err := db.WithTenantTx(context.Background(), e.App, tenant, func(tx pgx.Tx) error { fn(tx); return nil }); err != nil {
		t.Fatal(err)
	}
}

// failing runs sql in its own tx as tenant and expects an error containing want.
func expectError(t *testing.T, e *testenv.Env, tenant, want, sql string, args ...any) {
	t.Helper()
	err := db.WithTenantTx(context.Background(), e.App, tenant, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), sql, args...)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("%s as %s: want error containing %q, got %v", sql, tenant[len(tenant)-1:], want, err)
	}
}

func TestRLSIsolation(t *testing.T) {
	e := testenv.Get(t)
	ctx := context.Background()
	testenv.SeedThreeTenants(t, e)

	t.Run("each tenant sees only its own rows", func(t *testing.T) {
		for _, ten := range []string{testenv.TenA, testenv.TenB, testenv.TenC} {
			asTenant(t, e, ten, func(tx pgx.Tx) {
				if n := count(t, tx, `SELECT count(*) FROM documents`); n != 2 {
					t.Errorf("documents visible = %d, want 2", n)
				}
				if n := count(t, tx, `SELECT count(*) FROM documents WHERE tenant_id <> $1`, ten); n != 0 {
					t.Errorf("saw %d foreign documents", n)
				}
				if n := count(t, tx, `SELECT count(*) FROM users`); n != 1 {
					t.Errorf("users visible = %d, want 1", n)
				}
				if n := count(t, tx, `SELECT count(*) FROM tenants`); n != 1 {
					t.Errorf("tenants visible = %d, want 1", n)
				}
			})
		}
	})

	t.Run("A cannot read B's document or embeddings by id", func(t *testing.T) {
		asTenant(t, e, testenv.TenA, func(tx pgx.Tx) {
			if n := count(t, tx, `SELECT count(*) FROM documents WHERE id = $1`, testenv.DocID(testenv.TenB, 0)); n != 0 {
				t.Errorf("A read B's document")
			}
			if n := count(t, tx, `SELECT count(*) FROM document_embeddings WHERE document_id = $1`, testenv.DocID(testenv.TenB, 0)); n != 0 {
				t.Errorf("A read B's embedding")
			}
			if n := count(t, tx, `SELECT count(*) FROM documents WHERE id = $1`, testenv.DocID(testenv.TenC, 0)); n != 0 {
				t.Errorf("A read C's document")
			}
		})
		asTenant(t, e, testenv.TenB, func(tx pgx.Tx) { // B vs C
			if n := count(t, tx, `SELECT count(*) FROM documents WHERE id = $1`, testenv.DocID(testenv.TenC, 0)); n != 0 {
				t.Errorf("B read C's document")
			}
		})
	})

	t.Run("A cannot update or delete B's document", func(t *testing.T) {
		asTenant(t, e, testenv.TenA, func(tx pgx.Tx) {
			tag, err := tx.Exec(ctx, `UPDATE documents SET title = 'hacked' WHERE id = $1`, testenv.DocID(testenv.TenB, 0))
			if err != nil || tag.RowsAffected() != 0 {
				t.Errorf("update: rows=%d err=%v, want 0 rows", tag.RowsAffected(), err)
			}
			tag, err = tx.Exec(ctx, `DELETE FROM documents WHERE id = $1`, testenv.DocID(testenv.TenB, 0))
			if err != nil || tag.RowsAffected() != 0 {
				t.Errorf("delete: rows=%d err=%v, want 0 rows", tag.RowsAffected(), err)
			}
			tag, err = tx.Exec(ctx, `DELETE FROM document_embeddings WHERE document_id = $1`, testenv.DocID(testenv.TenB, 0))
			if err != nil || tag.RowsAffected() != 0 {
				t.Errorf("embedding delete: rows=%d err=%v, want 0 rows", tag.RowsAffected(), err)
			}
		})
		var title string
		if err := e.Super.QueryRow(ctx, `SELECT title FROM documents WHERE id = $1`, testenv.DocID(testenv.TenB, 0)).Scan(&title); err != nil || title != "t" {
			t.Errorf("B's document was modified or removed: title=%q err=%v", title, err)
		}
	})

	t.Run("WITH CHECK blocks writing rows into another tenant", func(t *testing.T) {
		expectError(t, e, testenv.TenA, "row-level security", `INSERT INTO documents (tenant_id, title, content) VALUES ($1,'x','y')`, testenv.TenB)
		expectError(t, e, testenv.TenA, "row-level security", `UPDATE documents SET tenant_id = $1 WHERE id = $2`, testenv.TenB, testenv.DocID(testenv.TenA, 0))
		expectError(t, e, testenv.TenA, "row-level security", `INSERT INTO users (tenant_id, email) VALUES ($1,'x@y')`, testenv.TenB)
		expectError(t, e, testenv.TenA, "row-level security",
			`INSERT INTO document_embeddings (tenant_id, document_id, content, model, embedding) VALUES ($1,$2,'c','m',`+testenv.ZeroVec+`)`,
			testenv.TenB, testenv.DocID(testenv.TenB, 1))
	})

	t.Run("cross-tenant document/embedding relationship rejected", func(t *testing.T) {
		// Own tenant id pointing at B's document: B's document isn't visible/doesn't match (tenant_id, id).
		expectError(t, e, testenv.TenA, "foreign key",
			`INSERT INTO document_embeddings (tenant_id, document_id, content, model, embedding) VALUES ($1,$2,'c','m',`+testenv.ZeroVec+`)`,
			testenv.TenA, testenv.DocID(testenv.TenB, 1))
	})

	t.Run("app role cannot modify tenants", func(t *testing.T) {
		for _, q := range []string{`INSERT INTO tenants (name) VALUES ('evil')`, `UPDATE tenants SET name='x'`, `DELETE FROM tenants`} {
			expectError(t, e, testenv.TenA, "permission denied", q)
		}
	})

	t.Run("no tenant context returns nothing and writes fail", func(t *testing.T) {
		for _, tbl := range []string{"tenants", "users", "documents", "document_embeddings"} {
			var n int
			if err := e.App.QueryRow(ctx, `SELECT count(*) FROM `+tbl).Scan(&n); err != nil || n != 0 {
				t.Errorf("%s without context: n=%d err=%v, want 0 rows", tbl, n, err)
			}
		}
		if _, err := e.App.Exec(ctx, `INSERT INTO documents (tenant_id, title, content) VALUES ($1,'x','y')`, testenv.TenA); err == nil ||
			!strings.Contains(err.Error(), "row-level security") {
			t.Errorf("insert without context: want RLS error, got %v", err)
		}
		// An explicitly empty setting must behave the same.
		err := db.WithTenantTx(ctx, e.App, testenv.TenA, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `SELECT set_config('app.tenant_id', '', true)`); err != nil {
				return err
			}
			if n := count(t, tx, `SELECT count(*) FROM documents`); n != 0 {
				t.Errorf("empty tenant setting leaked %d rows", n)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("FORCE RLS applies to the table owner", func(t *testing.T) {
		for _, tbl := range []string{"users", "documents", "document_embeddings"} {
			var n int
			if err := e.Owner.QueryRow(ctx, `SELECT count(*) FROM `+tbl).Scan(&n); err != nil || n != 0 {
				t.Errorf("owner reading %s without context: n=%d err=%v, want 0", tbl, n, err)
			}
		}
	})
}

func smallPool(t *testing.T, e *testenv.Env, conns int) *pgxpool.Pool {
	t.Helper()
	p, err := pgxpool.New(context.Background(), fmt.Sprintf("%s&pool_max_conns=%d", e.AppURL, conns))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p
}

func TestTenantContextDoesNotLeakAcrossPooledConnections(t *testing.T) {
	e := testenv.Get(t)
	ctx := context.Background()
	testenv.SeedThreeTenants(t, e)
	pool := smallPool(t, e, 1) // one connection: every call reuses it

	plainCount := func() int {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM documents`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if err := db.WithTenantTx(ctx, pool, testenv.TenA, func(tx pgx.Tx) error {
		if n := count(t, tx, `SELECT count(*) FROM documents`); n != 2 {
			t.Errorf("tenant A sees %d, want 2", n)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if n := plainCount(); n != 0 {
		t.Errorf("after committed tenant tx, plain query on reused conn saw %d rows", n)
	}

	// A rolled-back tenant tx must not leave context behind either.
	_ = db.WithTenantTx(ctx, pool, testenv.TenB, func(tx pgx.Tx) error { return fmt.Errorf("boom") })
	if n := plainCount(); n != 0 {
		t.Errorf("after rolled-back tx, plain query saw %d rows", n)
	}
	var setting *string
	if err := pool.QueryRow(ctx, `SELECT nullif(current_setting('app.tenant_id', true), '')`).Scan(&setting); err != nil || setting != nil {
		t.Errorf("tenant setting persisted on pooled connection: %v (err %v)", setting, err)
	}
}

func TestConcurrentTenantsStayIsolated(t *testing.T) {
	e := testenv.Get(t)
	testenv.SeedThreeTenants(t, e)
	pool := smallPool(t, e, 4)
	tenants := []string{testenv.TenA, testenv.TenB, testenv.TenC}

	const goroutines, iterations = 16, 50
	errs := make(chan error, goroutines*iterations)
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			ctx := context.Background()
			for i := 0; i < iterations; i++ {
				ten := tenants[(g+i)%3]
				err := db.WithTenantTx(ctx, pool, ten, func(tx pgx.Tx) error {
					rows, err := tx.Query(ctx, `SELECT tenant_id::text FROM documents`)
					if err != nil {
						return err
					}
					ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
					if err != nil {
						return err
					}
					if len(ids) != 2 {
						return fmt.Errorf("tenant %s saw %d documents, want 2", ten, len(ids))
					}
					for _, id := range ids {
						if id != ten {
							return fmt.Errorf("tenant %s saw document of %s", ten, id)
						}
					}
					return nil
				})
				if err != nil {
					errs <- err
					return
				}
				// Interleave a context-free query on the shared pool: must see nothing.
				var n int
				if err := pool.QueryRow(ctx, `SELECT count(*) FROM documents`).Scan(&n); err != nil || n != 0 {
					errs <- fmt.Errorf("context-free query saw %d rows (err %v)", n, err)
					return
				}
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
