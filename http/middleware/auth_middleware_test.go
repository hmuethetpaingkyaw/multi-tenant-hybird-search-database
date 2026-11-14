package middleware_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"multi-tenant-hybrid-search/domain"
	"multi-tenant-hybrid-search/http/middleware"
)

type fakeParser struct {
	id  domain.Identity
	err error
	got string
}

func (f *fakeParser) Parse(token string) (domain.Identity, error) { f.got = token; return f.id, f.err }

func authRouter(p middleware.TokenParser) (*gin.Engine, *domain.Identity) {
	gin.SetMode(gin.ReleaseMode)
	var seen domain.Identity
	r := gin.New()
	r.Use(middleware.ErrorHandler(nil))
	r.GET("/x", middleware.Auth(p), func(c *gin.Context) {
		id, _ := middleware.IdentityFrom(c)
		seen = id
		c.Status(http.StatusNoContent)
	})
	return r, &seen
}

func TestAuthRejectsMissingOrMalformedHeaders(t *testing.T) {
	for name, header := range map[string]string{"missing": "", "wrong scheme": "Basic abc", "no token": "Bearer ", "only scheme": "Bearer"} {
		p := &fakeParser{}
		r, _ := authRouter(p)
		req := httptest.NewRequest("GET", "/x", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 401 || strings.TrimSpace(w.Body.String()) != `{"error":"unauthorized"}` || w.Header().Get("WWW-Authenticate") != "Bearer" {
			t.Errorf("%s: %d %s", name, w.Code, w.Body)
		}
		if p.got != "" {
			t.Errorf("%s: parser must not be called", name)
		}
	}
}

func TestAuthRejectsInvalidTokenWithGenericBody(t *testing.T) {
	r, _ := authRouter(&fakeParser{err: errors.New("signature is invalid: secret detail")})
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer abc")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 401 || strings.Contains(w.Body.String(), "signature") {
		t.Errorf("%d %s", w.Code, w.Body)
	}
}

func TestAuthStoresIdentityForHandlers(t *testing.T) {
	r, seen := authRouter(&fakeParser{id: domain.Identity{UserID: "u", TenantID: "t"}})
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "bearer  tok ") // scheme is case-insensitive, token is trimmed
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 204 || seen.TenantID != "t" || seen.UserID != "u" {
		t.Errorf("%d identity=%+v", w.Code, *seen)
	}
}
