package controller_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"multi-tenant-hybrid-search/apperror"
	"multi-tenant-hybrid-search/domain"
	"multi-tenant-hybrid-search/driver/auth"
	"multi-tenant-hybrid-search/http/controller"
	"multi-tenant-hybrid-search/http/middleware"
	"multi-tenant-hybrid-search/http/router"
	"multi-tenant-hybrid-search/usecase"
)

type fakeTenant struct {
	profile usecase.TenantProfile
	err     error
	gotID   domain.Identity
}

func (f *fakeTenant) Me(_ context.Context, id domain.Identity) (usecase.TenantProfile, error) {
	f.gotID = id
	return f.profile, f.err
}

func meCall(t *testing.T, svc *fakeTenant, token string) (*httptest.ResponseRecorder, *bytes.Buffer) {
	t.Helper()
	authn, err := auth.New("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	logs := &bytes.Buffer{}
	log := slog.New(slog.NewTextHandler(logs, nil))
	r := router.New(log, middleware.Auth(authn), router.Controllers{Tenant: controller.NewTenantController(svc)})
	if token == "valid" {
		token, _ = authn.Issue(domain.Identity{UserID: testUID, TenantID: testTID}, time.Hour)
	}
	req := httptest.NewRequest("GET", "/api/v1/me", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w, logs
}

func TestMeSuccessShapeAndIdentity(t *testing.T) {
	svc := &fakeTenant{profile: usecase.TenantProfile{
		Tenant: domain.Tenant{ID: "t1", Name: "Acme"}, User: domain.User{ID: "u1", Email: "a@x.com"}}}
	w, _ := meCall(t, svc, "valid")
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"tenant":{"id":"t1","name":"Acme"},"user":{"email":"a@x.com","id":"u1"}}` {
		t.Errorf("me = %d %s", w.Code, w.Body)
	}
	if svc.gotID.TenantID != testTID || svc.gotID.UserID != testUID {
		t.Errorf("identity not taken from the token: %+v", svc.gotID)
	}
}

func TestMeRequiresAuth(t *testing.T) {
	for _, tok := range []string{"", "garbage"} {
		w, _ := meCall(t, &fakeTenant{}, tok)
		if w.Code != 401 || strings.TrimSpace(w.Body.String()) != `{"error":"unauthorized"}` || w.Header().Get("WWW-Authenticate") != "Bearer" {
			t.Errorf("token %q = %d %s", tok, w.Code, w.Body)
		}
	}
}

func TestMeErrorMapping(t *testing.T) {
	w, _ := meCall(t, &fakeTenant{err: apperror.ErrNotFound}, "valid")
	if w.Code != http.StatusNotFound || strings.TrimSpace(w.Body.String()) != `{"error":"not found"}` {
		t.Errorf("not found = %d %s", w.Code, w.Body)
	}
	w, logs := meCall(t, &fakeTenant{err: errors.New("pq: password=secret")}, "valid")
	if w.Code != 500 || strings.Contains(w.Body.String(), "secret") || !strings.Contains(logs.String(), "password=secret") {
		t.Errorf("internal = %d %s (cause must be logged, not returned)", w.Code, w.Body)
	}
}
