package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func get(t *testing.T, ready ReadyFunc, path string) *httptest.ResponseRecorder {
	t.Helper()
	r := NewRouter(ready, slog.New(slog.NewTextHandler(io.Discard, nil)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

func TestHealthzIgnoresDatabase(t *testing.T) {
	w := get(t, func(context.Context) error { return errors.New("db down") }, "/healthz")
	if w.Code != http.StatusOK {
		t.Fatalf("healthz = %d, want 200", w.Code)
	}
}

func TestReadyz(t *testing.T) {
	if w := get(t, func(context.Context) error { return nil }, "/readyz"); w.Code != http.StatusOK {
		t.Fatalf("ready = %d, want 200", w.Code)
	}
	w := get(t, func(context.Context) error { return errors.New("secret detail") }, "/readyz")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("not ready = %d, want 503", w.Code)
	}
	if strings.Contains(w.Body.String(), "secret detail") {
		t.Fatal("readyz must not leak error details")
	}
}
