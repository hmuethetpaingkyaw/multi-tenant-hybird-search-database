package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"multi-tenant-hybrid-search/internal/auth"
	"multi-tenant-hybrid-search/internal/delivery/middleware"
	"multi-tenant-hybrid-search/internal/domain/embedding"
	domain "multi-tenant-hybrid-search/internal/domain/search"
	"multi-tenant-hybrid-search/internal/domain/tenant"
)

type fakeSearch struct {
	res      []domain.Result
	err      error
	calls    int
	gotID    tenant.Identity
	gotQuery string
	gotLimit *int
}

func (f *fakeSearch) Semantic(_ context.Context, id tenant.Identity, q string, l *int) ([]domain.Result, error) {
	f.calls++
	f.gotID, f.gotQuery, f.gotLimit = id, q, l
	return f.res, f.err
}

func searchHarness(t *testing.T) (http.Handler, *fakeSearch, string, *bytes.Buffer) {
	t.Helper()
	authn, err := auth.New("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	tok, _ := authn.Issue(tenant.Identity{UserID: testUID, TenantID: testTID}, time.Hour)
	svc := &fakeSearch{}
	logs := &bytes.Buffer{}
	r := NewRouter(Deps{
		Ready:  func(context.Context) error { return nil },
		Log:    slog.New(slog.NewTextHandler(logs, nil)),
		Auth:   middleware.Auth(authn),
		Search: svc,
	})
	return r, svc, tok, logs
}

func post(h http.Handler, body, tok string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/api/v1/search/semantic", strings.NewReader(body))
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func msg(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var b map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil {
		t.Fatalf("not a JSON error body: %q", w.Body)
	}
	return b["error"]
}

func TestSemanticSearchSuccessShape(t *testing.T) {
	h, svc, tok, _ := searchHarness(t)
	svc.res = []domain.Result{{DocumentID: docUUID, Title: "Account Recovery Guide", Score: 0.9}}
	w := post(h, `{"query":"How can I recover my account?","limit":5}`, tok)
	if w.Code != 200 {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
	var out struct {
		Results []map[string]any `json:"results"`
		Total   int              `json:"total"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if out.Total != 1 || len(out.Results) != 1 || out.Results[0]["document_id"] != docUUID ||
		out.Results[0]["title"] != "Account Recovery Guide" || out.Results[0]["score"] != 0.9 || len(out.Results[0]) != 3 {
		t.Errorf("body: %s", w.Body)
	}
	if svc.gotID.TenantID != testTID || svc.gotQuery != "How can I recover my account?" || svc.gotLimit == nil || *svc.gotLimit != 5 {
		t.Errorf("service got %+v %q %v", svc.gotID, svc.gotQuery, svc.gotLimit)
	}
}

func TestSemanticSearchEmptyIsEmptyArray(t *testing.T) {
	h, svc, tok, _ := searchHarness(t)
	svc.res = nil
	w := post(h, `{"query":"x"}`, tok)
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"results":[],"total":0}` {
		t.Errorf("empty = %d %s", w.Code, w.Body)
	}
	if svc.gotLimit != nil {
		t.Error("absent limit must be passed as nil")
	}
}

func TestSemanticSearchRequiresAuth(t *testing.T) {
	h, svc, _, _ := searchHarness(t)
	for _, tok := range []string{"", "garbage"} {
		w := post(h, `{"query":"x"}`, tok)
		if w.Code != 401 || msg(t, w) != "unauthorized" || w.Header().Get("WWW-Authenticate") != "Bearer" {
			t.Errorf("token %q = %d %s", tok, w.Code, w.Body)
		}
	}
	if svc.calls != 0 {
		t.Error("service called without auth")
	}
}

func TestSemanticSearchBadRequests(t *testing.T) {
	h, svc, tok, _ := searchHarness(t)
	cases := []struct{ name, body, msg string }{
		{"unknown field tenant_id", `{"query":"x","tenant_id":"y"}`, `invalid request body: unknown field "tenant_id"`},
		{"bad JSON", `{"query":`, "invalid request body"},
		{"wrong type query", `{"query":5}`, "invalid request body"},
		{"wrong type limit", `{"query":"x","limit":"ten"}`, "invalid request body"},
		{"empty body", ``, "invalid request body"},
		{"trailing data", `{"query":"x"} {}`, "invalid request body"},
	}
	for _, c := range cases {
		w := post(h, c.body, tok)
		if w.Code != 400 || msg(t, w) != c.msg {
			t.Errorf("%s: %d %s, want 400 %q", c.name, w.Code, w.Body, c.msg)
		}
	}
	if svc.calls != 0 {
		t.Errorf("service called %d times for bad requests", svc.calls)
	}
}

func TestSemanticSearchOversizedBody413(t *testing.T) {
	h, svc, tok, _ := searchHarness(t)
	w := post(h, `{"query":"`+strings.Repeat("a", maxSearchBodyBytes)+`"}`, tok)
	if w.Code != http.StatusRequestEntityTooLarge || msg(t, w) != "request body too large" || svc.calls != 0 {
		t.Errorf("oversized = %d %.60s calls=%d", w.Code, w.Body, svc.calls)
	}
}

func TestSemanticSearchErrorMapping(t *testing.T) {
	h, svc, tok, logs := searchHarness(t)

	svc.err = domain.NewInvalid("query is required")
	if w := post(h, `{"query":""}`, tok); w.Code != 400 || msg(t, w) != "query is required" {
		t.Errorf("invalid = %d %s", w.Code, w.Body)
	}
	svc.err = domain.NewInvalid(domain.MsgLimit)
	if w := post(h, `{"query":"x","limit":0}`, tok); w.Code != 400 || msg(t, w) != domain.MsgLimit {
		t.Errorf("limit = %d %s", w.Code, w.Body)
	}

	svc.err = embedding.Unavailable(errors.New("dial tcp ollama:11434: connection refused"))
	w := post(h, `{"query":"x"}`, tok)
	if w.Code != 503 || msg(t, w) != "embedding service unavailable" || strings.Contains(w.Body.String(), "ollama") {
		t.Errorf("unavailable = %d %s", w.Code, w.Body)
	}
	if !strings.Contains(logs.String(), "connection refused") {
		t.Error("embedding failure cause must be logged")
	}

	svc.err = errors.New("pq: password=secret host=10.0.0.5")
	w = post(h, `{"query":"x"}`, tok)
	if w.Code != 500 || msg(t, w) != "internal error" || strings.Contains(w.Body.String(), "secret") {
		t.Errorf("internal = %d %s", w.Code, w.Body)
	}
	_ = io.Discard
}
