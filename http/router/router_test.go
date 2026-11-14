package router_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"multi-tenant-hybrid-search/http/controller"
	"multi-tenant-hybrid-search/http/router"
)

func TestRoutesOfNilControllersAreNotRegistered(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	r := router.New(log, nil, router.Controllers{})
	for _, p := range []string{"/healthz", "/readyz", "/api/v1/me", "/api/v1/documents", "/api/v1/search/semantic"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, p, nil))
		if w.Code != http.StatusNotFound {
			t.Errorf("%s = %d, want 404 when no controller is registered", p, w.Code)
		}
	}
}

func TestProbesAreRegisteredWithoutAuth(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	r := router.New(log, nil, router.Controllers{Health: controller.NewHealthController(func(context.Context) error { return nil }, log)})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if w.Code != 200 {
		t.Errorf("healthz = %d", w.Code)
	}
}
