package controller_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"multi-tenant-hybrid-search/apperror"
	"multi-tenant-hybrid-search/constants"
	"multi-tenant-hybrid-search/domain"
	"multi-tenant-hybrid-search/http/controller"
	"multi-tenant-hybrid-search/http/middleware"
	"multi-tenant-hybrid-search/http/router"
	"multi-tenant-hybrid-search/usecase"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"multi-tenant-hybrid-search/driver/auth"
)

const (
	testUID = "11111111-1111-1111-1111-111111111111"
	testTID = "22222222-2222-2222-2222-222222222222"
	docUUID = "33333333-3333-3333-3333-333333333333"
)

type fakeDocs struct {
	err        error
	gotIdent   domain.Identity
	gotInput   domain.DocumentInput
	gotID      string
	gotLim     *int
	gotOff     *int
	callCount  int
	listResult usecase.DocumentListResult
}

func (f *fakeDocs) doc() domain.Document {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	return domain.Document{ID: docUUID, Title: f.gotInput.Title, Content: f.gotInput.Content, CreatedAt: now, UpdatedAt: now}
}
func (f *fakeDocs) Create(_ context.Context, id domain.Identity, in domain.DocumentInput) (domain.Document, error) {
	f.callCount++
	f.gotIdent, f.gotInput = id, in
	return f.doc(), f.err
}
func (f *fakeDocs) Get(_ context.Context, id domain.Identity, docID string) (domain.Document, error) {
	f.callCount++
	f.gotIdent, f.gotID = id, docID
	return f.doc(), f.err
}
func (f *fakeDocs) List(_ context.Context, id domain.Identity, l, o *int) (usecase.DocumentListResult, error) {
	f.callCount++
	f.gotIdent, f.gotLim, f.gotOff = id, l, o
	return f.listResult, f.err
}
func (f *fakeDocs) Update(_ context.Context, id domain.Identity, docID string, in domain.DocumentInput) (domain.Document, error) {
	f.callCount++
	f.gotIdent, f.gotID, f.gotInput = id, docID, in
	return f.doc(), f.err
}
func (f *fakeDocs) Delete(_ context.Context, id domain.Identity, docID string) error {
	f.callCount++
	f.gotIdent, f.gotID = id, docID
	return f.err
}

type harness struct {
	router http.Handler
	svc    *fakeDocs
	token  string
	logs   *bytes.Buffer
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	authn, err := auth.New("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	tok, err := authn.Issue(domain.Identity{UserID: testUID, TenantID: testTID}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	svc := &fakeDocs{}
	logs := &bytes.Buffer{}
	log := slog.New(slog.NewTextHandler(logs, nil))
	r := router.New(log, middleware.Auth(authn), router.Controllers{Document: controller.NewDocumentController(svc)})
	return &harness{router: r, svc: svc, token: tok, logs: logs}
}

func (h *harness) do(method, path, body, token string) *httptest.ResponseRecorder {
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rd)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.router.ServeHTTP(w, req)
	return w
}

func errBody(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var b map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil {
		t.Fatalf("not a JSON error body: %q", w.Body.String())
	}
	return b["error"]
}

func TestHappyPaths(t *testing.T) {
	h := newHarness(t)
	const body = `{"title":"  Guide  ","content":"hello"}`

	w := h.do("POST", "/api/v1/documents", body, h.token)
	if w.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", w.Code, w.Body)
	}
	var created map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	for _, k := range []string{"id", "title", "content", "created_at", "updated_at"} {
		if _, ok := created[k]; !ok {
			t.Errorf("create response missing %q: %v", k, created)
		}
	}
	if _, leaked := created["tenant_id"]; leaked {
		t.Error("tenant_id must not be in responses")
	}
	if h.svc.gotIdent.TenantID != testTID || h.svc.gotInput.Title != "Guide" {
		t.Errorf("service got %+v %+v", h.svc.gotIdent, h.svc.gotInput)
	}

	// List: items have no content; empty result is [] not null; paging values come from the service.
	h.svc.listResult = usecase.DocumentListResult{Documents: nil, Limit: 20, Offset: 0}
	w = h.do("GET", "/api/v1/documents", "", h.token)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"documents":[]`) || !strings.Contains(w.Body.String(), `"limit":20`) {
		t.Errorf("empty list = %d %s", w.Code, w.Body)
	}
	h.svc.listResult = usecase.DocumentListResult{Documents: []domain.DocumentSummary{{ID: docUUID, Title: "t"}}, Limit: 100, Offset: 5}
	w = h.do("GET", "/api/v1/documents?limit=1000&offset=5", "", h.token)
	if w.Code != 200 || strings.Contains(w.Body.String(), `"content"`) || !strings.Contains(w.Body.String(), `"limit":100`) {
		t.Errorf("list = %d %s", w.Code, w.Body)
	}
	if h.svc.gotLim == nil || *h.svc.gotLim != 1000 || h.svc.gotOff == nil || *h.svc.gotOff != 5 {
		t.Errorf("query not forwarded: %v %v", h.svc.gotLim, h.svc.gotOff)
	}

	if w = h.do("GET", "/api/v1/documents/"+docUUID, "", h.token); w.Code != 200 || !strings.Contains(w.Body.String(), `"content"`) {
		t.Errorf("get = %d %s", w.Code, w.Body)
	}
	if w = h.do("PUT", "/api/v1/documents/"+docUUID, `{"title":"n","content":"b"}`, h.token); w.Code != 200 || h.svc.gotID != docUUID {
		t.Errorf("put = %d %s", w.Code, w.Body)
	}
	if w = h.do("DELETE", "/api/v1/documents/"+docUUID, "", h.token); w.Code != 204 || w.Body.Len() != 0 {
		t.Errorf("delete = %d %q", w.Code, w.Body)
	}
}

func TestUnauthenticatedOnEveryRoute(t *testing.T) {
	h := newHarness(t)
	routes := [][2]string{
		{"POST", "/api/v1/documents"}, {"GET", "/api/v1/documents"}, {"GET", "/api/v1/documents/" + docUUID},
		{"PUT", "/api/v1/documents/" + docUUID}, {"DELETE", "/api/v1/documents/" + docUUID},
	}
	for _, r := range routes {
		for _, tok := range []string{"", "garbage"} {
			w := h.do(r[0], r[1], `{"title":"t","content":"c"}`, tok)
			if w.Code != 401 || errBody(t, w) != "unauthorized" || w.Header().Get("WWW-Authenticate") != "Bearer" {
				t.Errorf("%s %s token=%q = %d %s", r[0], r[1], tok, w.Code, w.Body)
			}
		}
	}
	if h.svc.callCount != 0 {
		t.Errorf("service called %d times without auth", h.svc.callCount)
	}
}

func TestBadRequests(t *testing.T) {
	h := newHarness(t)
	cases := []struct {
		name, method, path, body string
		status                   int
		msg                      string
	}{
		{"unknown field tenant_id", "POST", "/api/v1/documents", `{"title":"t","content":"c","tenant_id":"x"}`, 400, `invalid request body: unknown field "tenant_id"`},
		{"unknown field on PUT", "PUT", "/api/v1/documents/" + docUUID, `{"title":"t","content":"c","id":"x"}`, 400, `invalid request body: unknown field "id"`},
		{"bad JSON", "POST", "/api/v1/documents", `{"title":`, 400, "invalid request body"},
		{"wrong type", "POST", "/api/v1/documents", `{"title":5,"content":"c"}`, 400, "invalid request body"},
		{"empty body", "POST", "/api/v1/documents", ``, 400, "invalid request body"},
		{"trailing data", "POST", "/api/v1/documents", `{"title":"t","content":"c"} {}`, 400, "invalid request body"},
		{"bad uuid get", "GET", "/api/v1/documents/nope", ``, 400, "invalid document id"},
		{"bad uuid put", "PUT", "/api/v1/documents/nope", `{"title":"t","content":"c"}`, 400, "invalid document id"},
		{"bad uuid delete", "DELETE", "/api/v1/documents/nope", ``, 400, "invalid document id"},
		{"limit not int", "GET", "/api/v1/documents?limit=abc", ``, 400, constants.MsgLimitInvalid},
		{"offset not int", "GET", "/api/v1/documents?offset=x", ``, 400, constants.MsgOffsetInvalid},
	}
	for _, c := range cases {
		before := h.svc.callCount
		w := h.do(c.method, c.path, c.body, h.token)
		if w.Code != c.status || errBody(t, w) != c.msg {
			t.Errorf("%s: %d %s, want %d %q", c.name, w.Code, w.Body, c.status, c.msg)
		}
		if h.svc.callCount != before {
			t.Errorf("%s: service was called", c.name)
		}
	}
}

func TestOversizedBodyIs413(t *testing.T) {
	h := newHarness(t)
	big := `{"title":"t","content":"` + strings.Repeat("a", constants.MaxDocumentBodyBytes) + `"}`
	w := h.do("POST", "/api/v1/documents", big, h.token)
	if w.Code != http.StatusRequestEntityTooLarge || errBody(t, w) != "request body too large" {
		t.Errorf("oversized = %d %.80s", w.Code, w.Body)
	}
	if h.svc.callCount != 0 {
		t.Error("service called for oversized body")
	}
}

func TestErrorMapping(t *testing.T) {
	h := newHarness(t)
	h.svc.err = apperror.ErrNotFound
	for _, r := range [][3]string{{"GET", "/api/v1/documents/" + docUUID, ""}, {"PUT", "/api/v1/documents/" + docUUID, `{"title":"t","content":"c"}`}, {"DELETE", "/api/v1/documents/" + docUUID, ""}} {
		if w := h.do(r[0], r[1], r[2], h.token); w.Code != 404 || errBody(t, w) != "not found" {
			t.Errorf("%s = %d %s", r[0], w.Code, w.Body)
		}
	}
	h.svc.err = apperror.NewValidationError("title is required")
	if w := h.do("POST", "/api/v1/documents", `{"title":"","content":"c"}`, h.token); w.Code != 400 || errBody(t, w) != "title is required" {
		t.Errorf("invalid = %d %s", w.Code, w.Body)
	}
	h.svc.err = errors.New("pq: connection refused to 10.0.0.5 password=secret")
	w := h.do("GET", "/api/v1/documents/"+docUUID, "", h.token)
	if w.Code != 500 || errBody(t, w) != "internal error" || strings.Contains(w.Body.String(), "secret") {
		t.Errorf("internal = %d %s", w.Code, w.Body)
	}
	if !strings.Contains(h.logs.String(), "connection refused") {
		t.Error("internal error must be logged for operators")
	}
}
