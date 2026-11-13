package tests

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appdoc "multi-tenant-hybrid-search/internal/application/document"
	"multi-tenant-hybrid-search/internal/auth"
	httpapi "multi-tenant-hybrid-search/internal/delivery/http"
	"multi-tenant-hybrid-search/internal/delivery/middleware"
	"multi-tenant-hybrid-search/internal/domain/tenant"
	"multi-tenant-hybrid-search/internal/infrastructure/database"
)

func TestDocumentsHTTPFullStackIsolation(t *testing.T) {
	e := getEnv(t)
	ctx := context.Background()
	resetTenants(t, e)

	authn, err := auth.New("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	router := httpapi.NewRouter(httpapi.Deps{
		Ready:     func(context.Context) error { return nil },
		Log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		Auth:      middleware.Auth(authn),
		Documents: appdoc.NewService(database.NewDocumentRepository(e.app)),
	})
	token := func(tenantID string) string {
		tok, err := authn.Issue(tenant.Identity{UserID: "10000000-0000-0000-0000-000000000001", TenantID: tenantID}, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	tokA, tokB := token(tenA), token(tenB)
	call := func(method, path, body, tok string) (int, string) {
		var rd io.Reader
		if body != "" {
			rd = strings.NewReader(body)
		}
		req := httptest.NewRequest(method, path, rd)
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w.Code, w.Body.String()
	}
	docCount := func() int {
		var n int
		if err := e.super.QueryRow(ctx, `SELECT count(*) FROM documents`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	// H1/H5: A creates and reads its document; it is stored under A's tenant.
	code, body := call("POST", "/api/v1/documents", `{"title":"Account Recovery Guide","content":"steps..."}`, tokA)
	if code != http.StatusCreated {
		t.Fatalf("create = %d %s", code, body)
	}
	var created struct{ ID, Title, Content string }
	_ = json.Unmarshal([]byte(body), &created)
	if created.ID == "" || created.Title != "Account Recovery Guide" {
		t.Fatalf("create body: %s", body)
	}
	var owner string
	if err := e.super.QueryRow(ctx, `SELECT tenant_id::text FROM documents WHERE id = $1`, created.ID).Scan(&owner); err != nil || owner != tenA {
		t.Fatalf("stored tenant = %q (err %v), want A", owner, err)
	}
	if code, body = call("GET", "/api/v1/documents/"+created.ID, "", tokA); code != 200 || !strings.Contains(body, "steps...") {
		t.Errorf("A get = %d %s", code, body)
	}

	// F10: B cannot GET/PUT/DELETE A's document; it looks exactly like a missing one.
	missing := "00000000-0000-0000-0000-0000000000ff"
	_, missingBody := call("GET", "/api/v1/documents/"+missing, "", tokB)
	for _, r := range [][3]string{
		{"GET", "/api/v1/documents/" + created.ID, ""},
		{"PUT", "/api/v1/documents/" + created.ID, `{"title":"hacked","content":"x"}`},
		{"DELETE", "/api/v1/documents/" + created.ID, ""},
	} {
		code, body := call(r[0], r[1], r[2], tokB)
		if code != 404 || body != missingBody {
			t.Errorf("B %s A's doc = %d %s (missing doc gives %s)", r[0], code, body, missingBody)
		}
	}
	if code, body = call("GET", "/api/v1/documents/"+created.ID, "", tokA); code != 200 || !strings.Contains(body, "Account Recovery Guide") {
		t.Errorf("A's document changed after B's attempts: %d %s", code, body)
	}

	// H8: B's list excludes A's document; A's list includes it (no content in items).
	if code, body = call("GET", "/api/v1/documents", "", tokB); code != 200 || !strings.Contains(body, `"documents":[]`) {
		t.Errorf("B list = %d %s", code, body)
	}
	if code, body = call("GET", "/api/v1/documents", "", tokA); code != 200 || !strings.Contains(body, created.ID) || strings.Contains(body, "steps...") {
		t.Errorf("A list = %d %s", code, body)
	}

	// F8: a body naming tenant_id is rejected and creates nothing.
	before := docCount()
	code, body = call("POST", "/api/v1/documents", `{"title":"t","content":"c","tenant_id":"`+tenB+`"}`, tokA)
	if code != 400 || !strings.Contains(body, `unknown field`) || docCount() != before {
		t.Errorf("tenant_id body = %d %s (docs %d -> %d)", code, body, before, docCount())
	}

	// H6/H7: A updates then deletes; afterwards it is gone.
	if code, body = call("PUT", "/api/v1/documents/"+created.ID, `{"title":"Renamed","content":"new"}`, tokA); code != 200 || !strings.Contains(body, "Renamed") {
		t.Errorf("A put = %d %s", code, body)
	}
	if code, _ = call("DELETE", "/api/v1/documents/"+created.ID, "", tokA); code != 204 {
		t.Errorf("A delete = %d", code)
	}
	if code, _ = call("GET", "/api/v1/documents/"+created.ID, "", tokA); code != 404 {
		t.Errorf("A get after delete = %d", code)
	}

	// F14: a validly signed token for a tenant that does not exist cannot create anything.
	before = docCount()
	code, body = call("POST", "/api/v1/documents", `{"title":"t","content":"c"}`, token("00000000-0000-0000-0000-0000000000ee"))
	if code != 500 || !strings.Contains(body, `"internal error"`) || docCount() != before {
		t.Errorf("unknown tenant create = %d %s (docs %d -> %d)", code, body, before, docCount())
	}
}
