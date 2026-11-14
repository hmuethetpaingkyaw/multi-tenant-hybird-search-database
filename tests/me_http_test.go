package tests

import (
	"context"
	"io"
	"log/slog"
	"multi-tenant-hybrid-search/api/ollama/mock"
	"multi-tenant-hybrid-search/domain"
	"multi-tenant-hybrid-search/tests/testenv"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"multi-tenant-hybrid-search/driver/auth"
)

func TestMeEndpoint(t *testing.T) {
	e := testenv.Get(t)
	testenv.SeedThreeTenants(t, e)
	authn, err := auth.New("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	router := testenv.NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), e.App, authn, &mock_ollama.Fake{}, func(context.Context) error { return nil })
	userA, userB := testenv.UserIDOf(t, e, testenv.TenA), testenv.UserIDOf(t, e, testenv.TenB)
	token := func(user, tenant string) string {
		tok, err := authn.Issue(domain.Identity{UserID: user, TenantID: tenant}, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	call := func(path, authz string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if authz != "" {
			req.Header.Set("Authorization", authz)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	if w := call("/api/v1/me", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("no token = %d, want 401", w.Code)
	}
	if w := call("/api/v1/me", "Bearer garbage"); w.Code != http.StatusUnauthorized {
		t.Errorf("bad token = %d, want 401", w.Code)
	}
	if w := call("/api/v1/me", "Basic abc"); w.Code != http.StatusUnauthorized {
		t.Errorf("wrong scheme = %d, want 401", w.Code)
	}
	w := call("/api/v1/me", "Bearer "+token(userA, testenv.TenA))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"name":"tenant-a"`) || !strings.Contains(w.Body.String(), userA) {
		t.Errorf("valid A token = %d %s", w.Code, w.Body.String())
	}
	w = call("/api/v1/me", "Bearer "+token(userB, testenv.TenB))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"name":"tenant-b"`) {
		t.Errorf("valid B token = %d %s", w.Code, w.Body.String())
	}
	// Token claims tenant B but names tenant A's user: RLS hides the user.
	if w := call("/api/v1/me", "Bearer "+token(userA, testenv.TenB)); w.Code != http.StatusNotFound {
		t.Errorf("mismatched token = %d, want 404", w.Code)
	}
	// Probes stay unauthenticated.
	for _, p := range []string{"/healthz", "/readyz"} {
		if w := call(p, ""); w.Code != http.StatusOK {
			t.Errorf("%s = %d, want 200", p, w.Code)
		}
	}
}
