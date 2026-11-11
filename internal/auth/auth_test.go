package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	domain "multi-tenant-hybrid-search/internal/domain/tenant"
)

const (
	secret = "0123456789abcdef0123456789abcdef"
	uid    = "11111111-1111-1111-1111-111111111111"
	tid    = "22222222-2222-2222-2222-222222222222"
)

func newAuth(t *testing.T, s string) *Authenticator {
	t.Helper()
	a, err := New(s)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func sign(t *testing.T, method jwt.SigningMethod, key any, c claims) string {
	t.Helper()
	s, err := jwt.NewWithClaims(method, c).SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func okClaims() claims {
	return claims{TenantID: tid, RegisteredClaims: jwt.RegisteredClaims{
		Subject: uid, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}
}

func TestRoundTrip(t *testing.T) {
	a := newAuth(t, secret)
	tok, err := a.Issue(domain.Identity{UserID: uid, TenantID: tid}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	id, err := a.Parse(tok)
	if err != nil || id.UserID != uid || id.TenantID != tid {
		t.Fatalf("got %+v err=%v", id, err)
	}
}

func TestRejectsBadTokens(t *testing.T) {
	a := newAuth(t, secret)
	expired := okClaims()
	expired.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute))
	noExp := okClaims()
	noExp.ExpiresAt = nil
	noTenant := okClaims()
	noTenant.TenantID = ""
	badTenant := okClaims()
	badTenant.TenantID = "not-a-uuid"
	badSub := okClaims()
	badSub.Subject = "x"

	cases := map[string]string{
		"empty":          "",
		"garbage":        "a.b.c",
		"expired":        sign(t, jwt.SigningMethodHS256, []byte(secret), expired),
		"no exp":         sign(t, jwt.SigningMethodHS256, []byte(secret), noExp),
		"wrong secret":   sign(t, jwt.SigningMethodHS256, []byte(strings.Repeat("x", 32)), okClaims()),
		"alg none":       sign(t, jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, okClaims()),
		"other HMAC alg": sign(t, jwt.SigningMethodHS512, []byte(secret), okClaims()),
		"missing tenant": sign(t, jwt.SigningMethodHS256, []byte(secret), noTenant),
		"invalid tenant": sign(t, jwt.SigningMethodHS256, []byte(secret), badTenant),
		"invalid sub":    sign(t, jwt.SigningMethodHS256, []byte(secret), badSub),
	}
	for name, tok := range cases {
		if _, err := a.Parse(tok); err != ErrInvalidToken {
			t.Errorf("%s: want ErrInvalidToken, got %v", name, err)
		}
	}
}

func TestShortSecretRejected(t *testing.T) {
	if _, err := New("short"); err == nil {
		t.Fatal("short secret must be rejected")
	}
}

func TestIssueRejectsBadIDs(t *testing.T) {
	if _, err := newAuth(t, secret).Issue(domain.Identity{UserID: "x", TenantID: tid}, time.Hour); err == nil {
		t.Fatal("expected error")
	}
}
