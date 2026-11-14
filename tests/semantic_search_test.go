package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"multi-tenant-hybrid-search/api/ollama/mock"
	"multi-tenant-hybrid-search/domain"
	"multi-tenant-hybrid-search/repository"
	"multi-tenant-hybrid-search/tests/testenv"
	"multi-tenant-hybrid-search/usecase"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"multi-tenant-hybrid-search/driver/auth"
)

func searchSvc(e *testenv.Env, pool *pgxpool.Pool, f *mock_ollama.Fake) *usecase.SearchUsecase {
	return usecase.NewSearchUsecase(repository.NewSearchRepository(pool), f)
}

func mustCreate(t *testing.T, e *testenv.Env, f *mock_ollama.Fake, tenantID, title, content string) domain.Document {
	t.Helper()
	d, err := testenv.NewDocumentUsecase(e, f).Create(context.Background(), testenv.Ident(tenantID), domain.DocumentInput{Title: title, Content: content})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func titles(rs []domain.SearchResult) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Title
	}
	return out
}

func TestSemanticSearchRelevanceOrderLimitAndEmpty(t *testing.T) {
	e := testenv.Get(t)
	ctx := context.Background()
	testenv.ResetTenants(t, e)
	f := &mock_ollama.Fake{}
	mustCreate(t, e, f, testenv.TenA, "Quarterly sales report", "Revenue figures for the European region")
	mustCreate(t, e, f, testenv.TenA, "Account Recovery Guide", "How to recover your account: reset the password using your email")
	mustCreate(t, e, f, testenv.TenA, "Office parking policy", "Where employees may park")
	svc := searchSvc(e, e.App, f)

	res, err := svc.Semantic(ctx, testenv.Ident(testenv.TenA), "How can I recover my account?", nil)
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
	if res, err = svc.Semantic(ctx, testenv.Ident(testenv.TenA), "recover account", &one); err != nil || len(res) != 1 {
		t.Errorf("limit 1: %+v err=%v", res, err)
	}

	// A tenant with no embedded documents: empty, non-nil.
	res, err = svc.Semantic(ctx, testenv.Ident(testenv.TenC), "recover account", nil)
	if err != nil || res == nil || len(res) != 0 {
		t.Errorf("empty tenant: %v err=%v", res, err)
	}
}

func TestSemanticSearchNeverCrossesTenants(t *testing.T) {
	e := testenv.Get(t)
	ctx := context.Background()
	testenv.ResetTenants(t, e)
	f := &mock_ollama.Fake{}
	// A has the perfect match; B only has an unrelated document.
	aDoc := mustCreate(t, e, f, testenv.TenA, "Account Recovery Guide", "recover your account password reset")
	bDoc := mustCreate(t, e, f, testenv.TenB, "Cafeteria menu", "soup salad sandwiches")
	svc := searchSvc(e, e.App, f)

	resB, err := svc.Semantic(ctx, testenv.Ident(testenv.TenB), "Account Recovery Guide recover your account password reset", nil)
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
	resA, _ := svc.Semantic(ctx, testenv.Ident(testenv.TenA), "cafeteria soup salad", nil)
	for _, r := range resA {
		if r.DocumentID == bDoc.ID {
			t.Fatalf("tenant A received tenant B's document: %+v", resA)
		}
	}
}

func TestSemanticSearchEmbedderFailureSkipsDatabase(t *testing.T) {
	e := testenv.Get(t)
	testenv.ResetTenants(t, e)
	f := &mock_ollama.Fake{}
	svc := searchSvc(e, e.App, f)
	f.Err = fmt.Errorf("down")
	if _, err := svc.Semantic(context.Background(), testenv.Ident(testenv.TenA), "q", nil); err == nil {
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
	e := testenv.Get(t)
	ctx := context.Background()
	testenv.ResetTenants(t, e)
	f := &mock_ollama.Fake{}
	for i := 1; i <= 5; i++ {
		mustCreate(t, e, f, testenv.TenA, fmt.Sprintf("needle note %d", i), "small tenant document")
	}
	query := "needle haystack"
	// 300 tenant-B embeddings that are distinct but all nearer to the query than any of A's documents.
	for g := 1; g <= 300; g++ {
		var docID string
		if err := e.Super.QueryRow(ctx, `INSERT INTO documents (tenant_id, title, content) VALUES ($1, $2, 'c') RETURNING id::text`,
			testenv.TenB, fmt.Sprintf("noise %d", g)).Scan(&docID); err != nil {
			t.Fatal(err)
		}
		testenv.MustExec(t, e.Super, `INSERT INTO document_embeddings (tenant_id, document_id, content, model, embedding) VALUES ($1,$2,'noise','m',$3::vector)`,
			testenv.TenB, docID, testenv.VecText(mock_ollama.Vectorize(fmt.Sprintf("needle haystack noiseword%d", g))))
	}
	testenv.MustExec(t, e.Super, `ANALYZE document_embeddings`)

	pool, err := pgxpool.New(ctx, e.AppURL) // fresh pool: default role settings
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	limit := 5
	res, err := searchSvc(e, pool, f).Semantic(ctx, testenv.Ident(testenv.TenA), query, &limit)
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
	e := testenv.Get(t)
	testenv.ResetTenants(t, e)
	f := &mock_ollama.Fake{}
	authn, _ := auth.New("0123456789abcdef0123456789abcdef")
	router := testenv.NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), e.App, authn, f, func(context.Context) error { return nil })
	tokFor := func(tn string) string {
		tok, _ := authn.Issue(testenv.Ident(tn), time.Hour)
		return tok
	}
	do := func(path, body, tok string) (int, string) {
		req := httptest.NewRequest("POST", path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w.Code, w.Body.String()
	}

	if code, body := do("/api/v1/documents", `{"title":"Account Recovery Guide","content":"how to recover your account"}`, tokFor(testenv.TenA)); code != 201 {
		t.Fatalf("create = %d %s", code, body)
	}
	if code, _ := do("/api/v1/documents", `{"title":"Parking","content":"where to park"}`, tokFor(testenv.TenA)); code != 201 {
		t.Fatal("create 2")
	}

	code, body := do("/api/v1/search/semantic", `{"query":"How can I recover my account?"}`, tokFor(testenv.TenA))
	var out struct {
		Results []struct{ Title string } `json:"results"`
		Total   int
	}
	_ = json.Unmarshal([]byte(body), &out)
	if code != 200 || out.Total != 2 || out.Results[0].Title != "Account Recovery Guide" {
		t.Errorf("A search = %d %s", code, body)
	}
	// The same query as tenant B finds nothing of A's.
	code, body = do("/api/v1/search/semantic", `{"query":"How can I recover my account?"}`, tokFor(testenv.TenB))
	if code != 200 || strings.TrimSpace(body) != `{"results":[],"total":0}` {
		t.Errorf("B search = %d %s", code, body)
	}
}
