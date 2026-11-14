package testenv

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"multi-tenant-hybrid-search/domain"
	"multi-tenant-hybrid-search/driver/db"
)

// Tenant ids used across the integration tests.
const (
	TenA = "00000000-0000-0000-0000-00000000000a"
	TenB = "00000000-0000-0000-0000-00000000000b"
	TenC = "00000000-0000-0000-0000-00000000000c"
)

// ZeroVec is a 768-dimension zero vector literal for SQL.
const ZeroVec = `array_fill(0, ARRAY[768])::vector`

// DocID returns a stable document id for tenant t, index i.
func DocID(t string, i int) string {
	return fmt.Sprintf("0000000%d-0000-0000-0000-%s", i+1, t[24:])
}

func MustExec(t *testing.T, p *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := p.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

// VecText renders a vector the way pgvector parses it: "[0.1,0.2,...]".
func VecText(v []float32) string {
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

func UserIDOf(t *testing.T, e *Env, tenant string) string {
	t.Helper()
	var id string
	err := db.WithTenantTx(context.Background(), e.App, tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT id::text FROM users`).Scan(&id)
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// ResetTenants leaves tenants A, B, C with no users or documents.
func ResetTenants(t *testing.T, e *Env) {
	t.Helper()
	MustExec(t, e.Super, `DELETE FROM tenants`)
	for _, id := range []string{TenA, TenB, TenC} {
		MustExec(t, e.Owner, `INSERT INTO tenants (id, name) VALUES ($1, $2)`, id, "tenant-"+id[len(id)-1:])
	}
}

// SeedThreeTenants wipes data (superuser bypasses RLS) and creates tenants A, B, C,
// each with one user, two documents and one embedding, all written through the app role + RLS.
func SeedThreeTenants(t *testing.T, e *Env) {
	t.Helper()
	ctx := context.Background()
	MustExec(t, e.Super, `DELETE FROM tenants`)
	for _, id := range []string{TenA, TenB, TenC} {
		MustExec(t, e.Owner, `INSERT INTO tenants (id, name) VALUES ($1, $2)`, id, "tenant-"+id[len(id)-1:])
	}
	for _, id := range []string{TenA, TenB, TenC} {
		err := db.WithTenantTx(ctx, e.App, id, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `INSERT INTO users (tenant_id, email) VALUES ($1, $2)`, id, id[len(id)-1:]+"@example.com"); err != nil {
				return err
			}
			for i := 0; i < 2; i++ {
				if _, err := tx.Exec(ctx, `INSERT INTO documents (id, tenant_id, title, content) VALUES ($1,$2,'t','c')`,
					DocID(id, i), id); err != nil {
					return err
				}
			}
			_, err := tx.Exec(ctx, `INSERT INTO document_embeddings (tenant_id, document_id, content, model, embedding)
				VALUES ($1, $2, 'c', 'm', `+ZeroVec+`)`, id, DocID(id, 0))
			return err
		})
		if err != nil {
			t.Fatalf("seed tenant %s: %v", id, err)
		}
	}
}

// Ident is an identity for a user of the given tenant.
var Ident = func(tenantID string) domain.Identity {
	return domain.Identity{UserID: "10000000-0000-0000-0000-000000000001", TenantID: tenantID}
}
