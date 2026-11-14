package middleware_test

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"multi-tenant-hybrid-search/apperror"
	"multi-tenant-hybrid-search/http/middleware"
)

// run serves one request to a handler that fails with err and returns the response and the log output.
func run(t *testing.T, handler gin.HandlerFunc) (*httptest.ResponseRecorder, string) {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)
	logs := &bytes.Buffer{}
	r := gin.New()
	r.Use(middleware.ErrorHandler(slog.New(slog.NewTextHandler(logs, nil))))
	r.GET("/x", handler)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/x", nil))
	return w, logs.String()
}

func failWith(err error) gin.HandlerFunc {
	return func(c *gin.Context) { _ = c.Error(err) }
}

func TestErrorMapping(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		body   string
		logged bool
	}{
		{"app error", apperror.BadRequest("invalid document id", errors.New("cause")), 400, `{"error":"invalid document id"}`, false},
		{"app error 413", apperror.PayloadTooLarge(nil), 413, `{"error":"request body too large"}`, false},
		{"validation", apperror.NewValidationError("title is required"), 400, `{"error":"title is required"}`, false},
		{"wrapped validation", fmt.Errorf("ctx: %w", apperror.NewValidationError("query is required")), 400, `{"error":"query is required"}`, false},
		{"not found", apperror.ErrNotFound, 404, `{"error":"not found"}`, false},
		{"wrapped not found", fmt.Errorf("repo: %w", apperror.ErrNotFound), 404, `{"error":"not found"}`, false},
		{"embedding unavailable", apperror.Unavailable(errors.New("dial tcp: refused")), 503, `{"error":"embedding service unavailable"}`, true},
		{"unknown", errors.New("pq: password=secret"), 500, `{"error":"internal error"}`, true},
		{"5xx app error", &apperror.AppError{Code: 502, Message: "bad gateway", Err: errors.New("upstream")}, 502, `{"error":"bad gateway"}`, true},
	}
	for _, c := range cases {
		w, logs := run(t, failWith(c.err))
		if w.Code != c.status || strings.TrimSpace(w.Body.String()) != c.body {
			t.Errorf("%s: got %d %s, want %d %s", c.name, w.Code, w.Body, c.status, c.body)
		}
		if c.logged != (logs != "") {
			t.Errorf("%s: logged=%v, want %v (%q)", c.name, logs != "", c.logged, logs)
		}
		if c.logged && !strings.Contains(logs, "level=ERROR") {
			t.Errorf("%s: want ERROR level: %s", c.name, logs)
		}
	}
}

func TestCauseIsLoggedButNeverReturned(t *testing.T) {
	w, logs := run(t, failWith(errors.New("pq: password=secret host=10.0.0.5")))
	if strings.Contains(w.Body.String(), "secret") || strings.Contains(w.Body.String(), "10.0.0.5") {
		t.Errorf("internal details leaked: %s", w.Body)
	}
	if !strings.Contains(logs, "password=secret") || !strings.Contains(logs, "method=GET") || !strings.Contains(logs, "path=/x") {
		t.Errorf("log must carry method, path and cause: %s", logs)
	}
}

func TestHeadersFromAppErrorAreSent(t *testing.T) {
	w, _ := run(t, failWith(apperror.Unauthorized()))
	if w.Code != 401 || w.Header().Get("WWW-Authenticate") != "Bearer" {
		t.Errorf("got %d, header %q", w.Code, w.Header().Get("WWW-Authenticate"))
	}
}

func TestNoErrorLeavesResponseUntouched(t *testing.T) {
	w, logs := run(t, func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	if w.Code != 200 || logs != "" {
		t.Errorf("got %d, logs %q", w.Code, logs)
	}
}

func TestAlreadyWrittenResponseIsNotOverwritten(t *testing.T) {
	w, _ := run(t, func(c *gin.Context) {
		c.JSON(http.StatusCreated, gin.H{"ok": true})
		_ = c.Error(errors.New("late error"))
	})
	if w.Code != http.StatusCreated {
		t.Errorf("status overwritten: %d", w.Code)
	}
}
