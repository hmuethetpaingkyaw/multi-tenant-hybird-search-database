package tests

// API contract test: drives the whole stack (router, controllers/handlers, services, repositories, real Postgres,
// fake embedder, real JWTs) and compares status, body and the WWW-Authenticate header for every documented
// success and failure row. It exists to prove that restructuring the code does not change observable behavior.
// Only buildContractRouter (the wiring) may change when packages move; the cases below must not.

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"multi-tenant-hybrid-search/api/ollama/mock"
	"multi-tenant-hybrid-search/domain"
	"multi-tenant-hybrid-search/driver/db"
	"multi-tenant-hybrid-search/tests/testenv"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"multi-tenant-hybrid-search/driver/auth"
)

const contractSecret = "0123456789abcdef0123456789abcdef"

// buildContractRouter wires the production stack over the given pool. This is the only function that
// depends on package layout.
func buildContractRouter(pool *pgxpool.Pool, authn *auth.Authenticator, emb *mock_ollama.Fake, ready func(context.Context) error) http.Handler {
	return testenv.NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), pool, authn, emb, ready)
}

var (
	reUUID  = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	reTime  = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T[0-9:.]+(Z|[+-]\d{2}:\d{2})`)
	reScore = regexp.MustCompile(`"score":-?[0-9.e+-]+`)
)

func normalize(s string) string {
	s = reUUID.ReplaceAllString(s, "<uuid>")
	s = reTime.ReplaceAllString(s, "<time>")
	return reScore.ReplaceAllString(s, `"score":<n>`)
}

func TestAPIContract(t *testing.T) {
	e := testenv.Get(t)
	ctx := context.Background()
	testenv.ResetTenants(t, e)

	fake := &mock_ollama.Fake{}
	authn, err := auth.New(contractSecret)
	if err != nil {
		t.Fatal(err)
	}
	notReady := false
	router := buildContractRouter(e.App, authn, fake, func(context.Context) error {
		if notReady {
			return fmt.Errorf("db down")
		}
		return nil
	})

	// A user in tenant A for /me.
	userA := "10000000-0000-0000-0000-00000000000a"
	if err := db.WithTenantTx(ctx, e.App, testenv.TenA, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO users (id, tenant_id, email) VALUES ($1,$2,'user-a@example.com')`, userA, testenv.TenA)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	token := func(user, tenantID string) string {
		tok, err := authn.Issue(domain.Identity{UserID: user, TenantID: tenantID}, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	tokA, tokB := token(userA, testenv.TenA), token("10000000-0000-0000-0000-00000000000b", testenv.TenB)

	do := func(h http.Handler, method, path, body, tok string) (int, string, string) {
		var rd io.Reader
		if body != "" {
			rd = strings.NewReader(body)
		}
		req := httptest.NewRequest(method, path, rd)
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w.Code, strings.TrimSpace(w.Body.String()), w.Header().Get("WWW-Authenticate")
	}
	check := func(name, method, path, body, tok string, wantStatus int, wantBody string) {
		t.Helper()
		code, got, _ := do(router, method, path, body, tok)
		if code != wantStatus || normalize(got) != wantBody {
			t.Errorf("%s: %s %s\n  got  %d %s\n  want %d %s", name, method, path, code, normalize(got), wantStatus, wantBody)
		}
	}
	check401 := func(name, method, path, body, tok string) {
		t.Helper()
		code, got, hdr := do(router, method, path, body, tok)
		if code != 401 || got != `{"error":"unauthorized"}` || hdr != "Bearer" {
			t.Errorf("%s: got %d %s (WWW-Authenticate %q)", name, code, got, hdr)
		}
	}
	const (
		docs     = "/api/v1/documents"
		search   = "/api/v1/search/semantic"
		invalid  = `{"error":"invalid request body"}`
		notFound = `{"error":"not found"}`
	)

	// ---- health ----
	check("healthz", "GET", "/healthz", "", "", 200, `{"status":"ok"}`)
	check("readyz ready", "GET", "/readyz", "", "", 200, `{"status":"ready"}`)
	notReady = true
	check("readyz not ready", "GET", "/readyz", "", "", 503, `{"status":"not ready"}`)
	notReady = false

	// ---- /me ----
	check401("me no token", "GET", "/api/v1/me", "", "")
	check401("me garbage token", "GET", "/api/v1/me", "", "garbage")
	check("me ok", "GET", "/api/v1/me", "", tokA, 200,
		`{"tenant":{"id":"<uuid>","name":"tenant-a"},"user":{"email":"user-a@example.com","id":"<uuid>"}}`)
	check("me unknown user", "GET", "/api/v1/me", "", token("20000000-0000-0000-0000-000000000001", testenv.TenA), 404, notFound)

	// ---- documents: auth ----
	for _, r := range [][2]string{{"POST", docs}, {"GET", docs}, {"GET", docs + "/x"}, {"PUT", docs + "/x"}, {"DELETE", docs + "/x"}, {"POST", search}} {
		check401("no token "+r[0]+" "+r[1], r[0], r[1], `{"title":"t","content":"c"}`, "")
	}

	// ---- documents: empty list ----
	check("list empty", "GET", docs, "", tokA, 200, `{"documents":[],"limit":20,"offset":0}`)

	// ---- documents: create ----
	code, body, _ := do(router, "POST", docs, `{"title":"  Account Recovery Guide  ","content":"If you forgot your password, reset it by email."}`, tokA)
	if code != 201 || normalize(body) != `{"id":"<uuid>","title":"Account Recovery Guide","content":"If you forgot your password, reset it by email.","created_at":"<time>","updated_at":"<time>"}` {
		t.Fatalf("create: %d %s", code, normalize(body))
	}
	id := reUUID.FindString(body)
	check("create second", "POST", docs, `{"title":"Quarterly Sales Report","content":"Revenue grew in Europe."}`, tokA, 201,
		`{"id":"<uuid>","title":"Quarterly Sales Report","content":"Revenue grew in Europe.","created_at":"<time>","updated_at":"<time>"}`)

	// ---- documents: list / pagination ----
	item := func(title string) string {
		return `{"id":"<uuid>","title":"` + title + `","created_at":"<time>","updated_at":"<time>"}`
	}
	check("list newest first", "GET", docs, "", tokA, 200,
		`{"documents":[`+item("Quarterly Sales Report")+`,`+item("Account Recovery Guide")+`],"limit":20,"offset":0}`)
	check("list page", "GET", docs+"?limit=1&offset=1", "", tokA, 200, `{"documents":[`+item("Account Recovery Guide")+`],"limit":1,"offset":1}`)
	check("list clamp", "GET", docs+"?limit=1000", "", tokA, 200,
		`{"documents":[`+item("Quarterly Sales Report")+`,`+item("Account Recovery Guide")+`],"limit":100,"offset":0}`)
	check("list other tenant empty", "GET", docs, "", tokB, 200, `{"documents":[],"limit":20,"offset":0}`)
	for _, q := range []string{"limit=0", "limit=-3", "limit=abc"} {
		check("list bad "+q, "GET", docs+"?"+q, "", tokA, 400, `{"error":"limit must be a positive integer"}`)
	}
	for _, q := range []string{"offset=-1", "offset=x"} {
		check("list bad "+q, "GET", docs+"?"+q, "", tokA, 400, `{"error":"offset must be a non-negative integer"}`)
	}

	// ---- documents: get ----
	check("get own", "GET", docs+"/"+id, "", tokA, 200,
		`{"id":"<uuid>","title":"Account Recovery Guide","content":"If you forgot your password, reset it by email.","created_at":"<time>","updated_at":"<time>"}`)
	check("get other tenant", "GET", docs+"/"+id, "", tokB, 404, notFound)
	check("get missing", "GET", docs+"/00000000-0000-0000-0000-0000000000ff", "", tokA, 404, notFound)
	check("get bad uuid", "GET", docs+"/nope", "", tokA, 400, `{"error":"invalid document id"}`)

	// ---- documents: update ----
	check("put own", "PUT", docs+"/"+id, `{"title":"Renamed","content":"new body"}`, tokA, 200,
		`{"id":"<uuid>","title":"Renamed","content":"new body","created_at":"<time>","updated_at":"<time>"}`)
	check("put other tenant", "PUT", docs+"/"+id, `{"title":"hacked","content":"x"}`, tokB, 404, notFound)
	check("put bad uuid", "PUT", docs+"/nope", `{"title":"t","content":"c"}`, tokA, 400, `{"error":"invalid document id"}`)
	check("put invalid body", "PUT", docs+"/"+id, `{"title":"","content":"c"}`, tokA, 400, `{"error":"title is required"}`)
	check("still renamed after other tenant's attempt", "GET", docs+"/"+id, "", tokA, 200,
		`{"id":"<uuid>","title":"Renamed","content":"new body","created_at":"<time>","updated_at":"<time>"}`)

	// ---- documents: create/update validation and body rules ----
	long := func(n int) string { return strings.Repeat("a", n) }
	for _, c := range []struct {
		name, body string
		status     int
		want       string
	}{
		{"blank title", `{"title":"   ","content":"c"}`, 400, `{"error":"title is required"}`},
		{"missing title", `{"content":"c"}`, 400, `{"error":"title is required"}`},
		{"title 501", `{"title":"` + long(501) + `","content":"c"}`, 400, `{"error":"title must be at most 500 characters"}`},
		{"empty content", `{"title":"t","content":""}`, 400, `{"error":"content is required"}`},
		{"missing content", `{"title":"t"}`, 400, `{"error":"content is required"}`},
		{"content over 1 MiB", `{"title":"t","content":"` + long(1<<20+1) + `"}`, 400, `{"error":"content must be at most 1048576 bytes"}`},
		{"unknown field tenant_id", `{"title":"t","content":"c","tenant_id":"x"}`, 400, `{"error":"invalid request body: unknown field \"tenant_id\""}`},
		{"bad json", `{"title":`, 400, invalid},
		{"wrong type", `{"title":5,"content":"c"}`, 400, invalid},
		{"empty body", ``, 400, invalid},
		{"trailing data", `{"title":"t","content":"c"} {}`, 400, invalid},
		{"oversized body", `{"title":"t","content":"` + long(1<<20+70<<10) + `"}`, 413, `{"error":"request body too large"}`},
	} {
		check("POST "+c.name, "POST", docs, c.body, tokA, c.status, c.want)
	}

	// ---- search ----
	check("restore recovery doc", "PUT", docs+"/"+id, `{"title":"Account Recovery Guide","content":"If you forgot your password, reset it by email."}`, tokA, 200,
		`{"id":"<uuid>","title":"Account Recovery Guide","content":"If you forgot your password, reset it by email.","created_at":"<time>","updated_at":"<time>"}`)
	res := func(titles ...string) string {
		parts := make([]string, len(titles))
		for i, ti := range titles {
			parts[i] = `{"document_id":"<uuid>","title":"` + ti + `","score":<n>}`
		}
		return `{"results":[` + strings.Join(parts, ",") + `],"total":` + fmt.Sprint(len(titles)) + `}`
	}
	check("search relevant first", "POST", search, `{"query":"How can I recover my account password?"}`, tokA, 200,
		res("Account Recovery Guide", "Quarterly Sales Report"))
	check("search limit 1", "POST", search, `{"query":"How can I recover my account password?","limit":1}`, tokA, 200, res("Account Recovery Guide"))
	check("search other tenant empty", "POST", search, `{"query":"How can I recover my account password?"}`, tokB, 200, `{"results":[],"total":0}`)
	for _, c := range []struct {
		name, body string
		status     int
		want       string
	}{
		{"blank query", `{"query":"   "}`, 400, `{"error":"query is required"}`},
		{"missing query", `{}`, 400, `{"error":"query is required"}`},
		{"query 2001", `{"query":"` + long(2001) + `"}`, 400, `{"error":"query must be at most 2000 characters"}`},
		{"limit 0", `{"query":"x","limit":0}`, 400, `{"error":"limit must be a positive integer"}`},
		{"limit negative", `{"query":"x","limit":-1}`, 400, `{"error":"limit must be a positive integer"}`},
		{"limit wrong type", `{"query":"x","limit":"ten"}`, 400, invalid},
		{"unknown field tenant_id", `{"query":"x","tenant_id":"y"}`, 400, `{"error":"invalid request body: unknown field \"tenant_id\""}`},
		{"bad json", `{"query":`, 400, invalid},
		{"empty body", ``, 400, invalid},
		{"trailing data", `{"query":"x"} {}`, 400, invalid},
		{"oversized body", `{"query":"` + long(17<<10) + `"}`, 413, `{"error":"request body too large"}`},
	} {
		check("search "+c.name, "POST", search, c.body, tokA, c.status, c.want)
	}
	check("search clamps huge limit", "POST", search, `{"query":"recover account password","limit":100000}`, tokA, 200, res("Account Recovery Guide", "Quarterly Sales Report"))

	// ---- embedding failure: 503, nothing written, reads still work ----
	count := func() (int, int) {
		var d, em int
		_ = e.Super.QueryRow(ctx, `SELECT count(*) FROM documents`).Scan(&d)
		_ = e.Super.QueryRow(ctx, `SELECT count(*) FROM document_embeddings`).Scan(&em)
		return d, em
	}
	docsBefore, embsBefore := count()
	fake.Err = fmt.Errorf("connection refused")
	const unavailable = `{"error":"embedding service unavailable"}`
	check("create with embedding down", "POST", docs, `{"title":"x","content":"y"}`, tokA, 503, unavailable)
	check("update with embedding down", "PUT", docs+"/"+id, `{"title":"Changed","content":"changed"}`, tokA, 503, unavailable)
	check("search with embedding down", "POST", search, `{"query":"recover"}`, tokA, 503, unavailable)
	check("read still works", "GET", docs+"/"+id, "", tokA, 200,
		`{"id":"<uuid>","title":"Account Recovery Guide","content":"If you forgot your password, reset it by email.","created_at":"<time>","updated_at":"<time>"}`)
	fake.Err = nil
	if d, em := count(); d != docsBefore || em != embsBefore {
		t.Errorf("failed writes changed data: documents %d->%d embeddings %d->%d", docsBefore, d, embsBefore, em)
	}

	// ---- unexpected error: 500 with no internals ----
	dead, err := pgxpool.New(ctx, e.AppURL)
	if err != nil {
		t.Fatal(err)
	}
	dead.Close()
	broken := buildContractRouter(dead, authn, fake, func(context.Context) error { return nil })
	for _, r := range [][3]string{{"GET", docs, ""}, {"GET", docs + "/" + id, ""}, {"POST", docs, `{"title":"t","content":"c"}`}, {"POST", search, `{"query":"x"}`}} {
		code, got, _ := do(broken, r[0], r[1], r[2], tokA)
		if code != 500 || got != `{"error":"internal error"}` {
			t.Errorf("broken db %s %s = %d %s, want 500 generic body", r[0], r[1], code, got)
		}
	}

	// ---- delete ----
	check("delete other tenant", "DELETE", docs+"/"+id, "", tokB, 404, notFound)
	check("delete bad uuid", "DELETE", docs+"/nope", "", tokA, 400, `{"error":"invalid document id"}`)
	code, got, _ := do(router, "DELETE", docs+"/"+id, "", tokA)
	if code != 204 || got != "" {
		t.Errorf("delete own = %d %q, want 204 empty", code, got)
	}
	check("get after delete", "GET", docs+"/"+id, "", tokA, 404, notFound)
	check("delete again", "DELETE", docs+"/"+id, "", tokA, 404, notFound)
	var embLeft int
	_ = e.Super.QueryRow(ctx, `SELECT count(*) FROM document_embeddings WHERE document_id = $1`, id).Scan(&embLeft)
	if embLeft != 0 {
		t.Errorf("embedding left after delete: %d", embLeft)
	}
}
