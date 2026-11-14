// Package auth issues and verifies dev JWTs. The tenant is taken only from a verified token.
package auth

import (
	"errors"
	"fmt"
	"multi-tenant-hybrid-search/domain"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const MinSecretLen = 32

// ErrInvalidToken is returned for any token that fails verification (reason is deliberately not exposed).
var ErrInvalidToken = errors.New("auth: invalid token")

type claims struct {
	TenantID string `json:"tenant_id"`
	jwt.RegisteredClaims
}

type Authenticator struct{ secret []byte }

func New(secret string) (*Authenticator, error) {
	if len(secret) < MinSecretLen {
		return nil, fmt.Errorf("auth: secret must be at least %d bytes", MinSecretLen)
	}
	return &Authenticator{secret: []byte(secret)}, nil
}

// Issue signs an HS256 token for id valid for ttl.
func (a *Authenticator) Issue(id domain.Identity, ttl time.Duration) (string, error) {
	if !validUUID(id.UserID) || !validUUID(id.TenantID) {
		return "", errors.New("auth: user and tenant ids must be UUIDs")
	}
	now := time.Now()
	c := claims{
		TenantID: id.TenantID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   id.UserID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(a.secret)
}

// Parse verifies signature (HS256 only), expiry, and that sub and tenant_id are UUIDs.
func (a *Authenticator) Parse(token string) (domain.Identity, error) {
	var c claims
	p := jwt.NewParser(jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
	if _, err := p.ParseWithClaims(token, &c, func(*jwt.Token) (any, error) { return a.secret, nil }); err != nil {
		return domain.Identity{}, ErrInvalidToken
	}
	if !validUUID(c.Subject) || !validUUID(c.TenantID) {
		return domain.Identity{}, ErrInvalidToken
	}
	return domain.Identity{UserID: c.Subject, TenantID: c.TenantID}, nil
}

func validUUID(s string) bool {
	id, err := uuid.Parse(s)
	return err == nil && id != uuid.Nil
}
