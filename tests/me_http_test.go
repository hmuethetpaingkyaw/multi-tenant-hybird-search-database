package tests

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appt "multi-tenant-hybrid-search/internal/application/tenant"
	"multi-tenant-hybrid-search/internal/auth"
	httpapi "multi-tenant-hybrid-search/internal/delivery/http"
	"multi-tenant-hybrid-search/internal/delivery/middleware"
	domain "multi-tenant-hybrid-search/internal/domain/tenant"
	"multi-tenant-hybrid-search/internal/infrastructure/database"
)

func TestMeEndpoint(t *testing.T) {
	e := getEnv(t)
	seedThreeTenants(t, e)
	authn, err := auth.New("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	router := httpapi.NewRouter(httpapi.Deps{
		Ready: func(ctx2 context.Context) error { return nil },
		Log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Auth:  middleware.Auth(authn),
		Me:    appt.NewService(database.NewTenantRepository(e.app)),
	})
	userA, userB := userIDOf(t, e, tenA), userIDOf(t, e, tenB)
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
	w := call("/api/v1/me", "Bearer "+token(userA, tenA))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"name":"tenant-a"`) || !strings.Contains(w.Body.String(), userA) {
		t.Errorf("valid A token = %d %s", w.Code, w.Body.String())
	}
	w = call("/api/v1/me", "Bearer "+token(userB, tenB))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"name":"tenant-b"`) {
		t.Errorf("valid B token = %d %s", w.Code, w.Body.String())
	}
	// Token claims tenant B but names tenant A's user: RLS hides the user.
	if w := call("/api/v1/me", "Bearer "+token(userA, tenB)); w.Code != http.StatusNotFound {
		t.Errorf("mismatched token = %d, want 404", w.Code)
	}
	// Probes stay unauthenticated.
	for _, p := range []string{"/healthz", "/readyz"} {
		if w := call(p, ""); w.Code != http.StatusOK {
			t.Errorf("%s = %d, want 200", p, w.Code)
		}
	}
}
