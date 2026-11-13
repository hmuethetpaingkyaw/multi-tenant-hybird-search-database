package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	appsearch "multi-tenant-hybrid-search/internal/application/search"
	"multi-tenant-hybrid-search/internal/auth"
	httpapi "multi-tenant-hybrid-search/internal/delivery/http"
	"multi-tenant-hybrid-search/internal/delivery/middleware"
	docdomain "multi-tenant-hybrid-search/internal/domain/document"
	"multi-tenant-hybrid-search/internal/domain/search"
	"multi-tenant-hybrid-search/internal/infrastructure/database"
	"multi-tenant-hybrid-search/internal/infrastructure/embedding"
)

func searchSvc(e *env, pool *pgxpool.Pool, f *embedding.Fake) *appsearch.Service {
	return appsearch.NewService(database.NewSearchRepository(pool), f)
}

func mustCreate(t *testing.T, e *env, f *embedding.Fake, tenantID, title, content string) docdomain.Document {
	t.Helper()
	d, err := newDocService(e, f).Create(context.Background(), ident(tenantID), docdomain.Input{Title: title, Content: content})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func titles(rs []search.Result) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Title
	}
	return out
}

func TestSemanticSearchRelevanceOrderLimitAndEmpty(t *testing.T) {
	e := getEnv(t)
	ctx := context.Background()
	resetTenants(t, e)
	f := &embedding.Fake{}
	mustCreate(t, e, f, tenA, "Quarterly sales report", "Revenue figures for the European region")
	mustCreate(t, e, f, tenA, "Account Recovery Guide", "How to recover your account: reset the password using your email")
	mustCreate(t, e, f, tenA, "Office parking policy", "Where employees may park")
	svc := searchSvc(e, e.app, f)

	res, err := svc.Semantic(ctx, ident(tenA), "How can I recover my account?", nil)
	if err != nil || len(res) != 3 {
		t.Fatalf("results: %+v err=%v", res, err)
	}
	if res[0].Title != "Account Recovery Guide" {
		t.Errorf("most relevant first: got %v", titles(res))
	}
	for i := 1; i < len(res); i++ {
		if res[i].Score > res[i-1].Score {
			t.Errorf("scores must be descending: %+v", res)
		}
	}
	if res[0].DocumentID == "" || res[0].Score <= 0 || res[0].Score > 1.0001 {
		t.Errorf("bad first result: %+v", res[0])
	}

	one := 1
	if res, err = svc.Semantic(ctx, ident(tenA), "recover account", &one); err != nil || len(res) != 1 {
		t.Errorf("limit 1: %+v err=%v", res, err)
	}

	// A tenant with no embedded documents: empty, non-nil.
	res, err = svc.Semantic(ctx, ident(tenC), "recover account", nil)
	if err != nil || res == nil || len(res) != 0 {
		t.Errorf("empty tenant: %v err=%v", res, err)
	}
}

func TestSemanticSearchNeverCrossesTenants(t *testing.T) {
	e := getEnv(t)
	ctx := context.Background()
	resetTenants(t, e)
	f := &embedding.Fake{}
	// A has the perfect match; B only has an unrelated document.
	aDoc := mustCreate(t, e, f, tenA, "Account Recovery Guide", "recover your account password reset")
	bDoc := mustCreate(t, e, f, tenB, "Cafeteria menu", "soup salad sandwiches")
	svc := searchSvc(e, e.app, f)

	resB, err := svc.Semantic(ctx, ident(tenB), "Account Recovery Guide recover your account password reset", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range resB {
		if r.DocumentID == aDoc.ID {
			t.Fatalf("tenant B received tenant A's document: %+v", resB)
		}
	}
	if len(resB) != 1 || resB[0].DocumentID != bDoc.ID {
		t.Errorf("B should see only its own document, got %+v", resB)
	}
	resA, _ := svc.Semantic(ctx, ident(tenA), "cafeteria soup salad", nil)
	for _, r := range resA {
		if r.DocumentID == bDoc.ID {
			t.Fatalf("tenant A received tenant B's document: %+v", resA)
		}
	}
}

func TestSemanticSearchEmbedderFailureSkipsDatabase(t *testing.T) {
	e := getEnv(t)
	resetTenants(t, e)
	f := &embedding.Fake{}
	svc := searchSvc(e, e.app, f)
	f.Err = fmt.Errorf("down")
	if _, err := svc.Semantic(context.Background(), ident(tenA), "q", nil); err == nil {
		t.Fatal("expected error")
	}
}

// A small tenant surrounded by a larger tenant's nearer vectors must still get its full result set.
// This runs with the planner's natural choices (tenant_id btree index present, no forced settings): for a
// selective tenant filter it picks the tenant_id index plus an exact sort, which is correct.
//
// KNOWN LIMITATION (documented in the README): if the planner were forced onto the global HNSW index for such a
// tenant, RLS filtering after the graph scan can return 0 rows even with hnsw.iterative_scan, because the small
// tenant's nodes may be unreachable from the graph's entry point (reproduced during development).
func TestSemanticSearchSmallTenantAmongNearerNoise(t *testing.T) {
	e := getEnv(t)
	ctx := context.Background()
	resetTenants(t, e)
	f := &embedding.Fake{}
	for i := 1; i <= 5; i++ {
		mustCreate(t, e, f, tenA, fmt.Sprintf("needle note %d", i), "small tenant document")
	}
	query := "needle haystack"
	// 300 tenant-B embeddings that are distinct but all nearer to the query than any of A's documents.
	for g := 1; g <= 300; g++ {
		var docID string
		if err := e.super.QueryRow(ctx, `INSERT INTO documents (tenant_id, title, content) VALUES ($1, $2, 'c') RETURNING id::text`,
			tenB, fmt.Sprintf("noise %d", g)).Scan(&docID); err != nil {
			t.Fatal(err)
		}
		mustExec(t, e.super, `INSERT INTO document_embeddings (tenant_id, document_id, content, model, embedding) VALUES ($1,$2,'noise','m',$3::vector)`,
			tenB, docID, vecText(embedding.Vectorize(fmt.Sprintf("needle haystack noiseword%d", g))))
	}
	mustExec(t, e.super, `ANALYZE document_embeddings`)

	pool, err := pgxpool.New(ctx, e.appURL) // fresh pool: default role settings
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	limit := 5
	res, err := searchSvc(e, pool, f).Semantic(ctx, ident(tenA), query, &limit)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 5 {
		t.Fatalf("small tenant got %d of 5 results among nearer noise: %v", len(res), titles(res))
	}
	for _, r := range res {
		if !strings.HasPrefix(r.Title, "needle note") {
			t.Errorf("unexpected result %q", r.Title)
		}
	}
}

func TestSemanticSearchOverHTTPFullStack(t *testing.T) {
	e := getEnv(t)
	resetTenants(t, e)
	f := &embedding.Fake{}
	authn, _ := auth.New("0123456789abcdef0123456789abcdef")
	docs := newDocService(e, f)
	router := httpapi.NewRouter(httpapi.Deps{
		Ready:     func(context.Context) error { return nil },
		Log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		Auth:      middleware.Auth(authn),
		Documents: docs,
		Search:    searchSvc(e, e.app, f),
	})
	tokFor := func(tn string) string {
		tok, _ := authn.Issue(ident(tn), time.Hour)
		return tok
	}
	do := func(path, body, tok string) (int, string) {
		req := httptest.NewRequest("POST", path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w.Code, w.Body.String()
	}

	if code, body := do("/api/v1/documents", `{"title":"Account Recovery Guide","content":"how to recover your account"}`, tokFor(tenA)); code != 201 {
		t.Fatalf("create = %d %s", code, body)
	}
	if code, _ := do("/api/v1/documents", `{"title":"Parking","content":"where to park"}`, tokFor(tenA)); code != 201 {
		t.Fatal("create 2")
	}

	code, body := do("/api/v1/search/semantic", `{"query":"How can I recover my account?"}`, tokFor(tenA))
	var out struct {
		Results []struct{ Title string } `json:"results"`
		Total   int
	}
	_ = json.Unmarshal([]byte(body), &out)
	if code != 200 || out.Total != 2 || out.Results[0].Title != "Account Recovery Guide" {
		t.Errorf("A search = %d %s", code, body)
	}
	// The same query as tenant B finds nothing of A's.
	code, body = do("/api/v1/search/semantic", `{"query":"How can I recover my account?"}`, tokFor(tenB))
	if code != 200 || strings.TrimSpace(body) != `{"results":[],"total":0}` {
		t.Errorf("B search = %d %s", code, body)
	}
}
