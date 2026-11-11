// Package middleware holds Gin middleware.
package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	domain "multi-tenant-hybrid-search/internal/domain/tenant"
)

const identityKey = "identity"

// TokenParser verifies a bearer token and returns the caller's identity.
type TokenParser interface {
	Parse(token string) (domain.Identity, error)
}

// Auth requires a valid bearer token. The tenant comes only from the verified token.
// Failures return a generic 401 that does not reveal why verification failed.
func Auth(p TokenParser) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		scheme, token, ok := strings.Cut(h, " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(token) == "" {
			unauthorized(c)
			return
		}
		id, err := p.Parse(strings.TrimSpace(token))
		if err != nil {
			unauthorized(c)
			return
		}
		c.Set(identityKey, id)
		c.Next()
	}
}

// IdentityFrom returns the authenticated identity set by Auth.
func IdentityFrom(c *gin.Context) (domain.Identity, bool) {
	v, ok := c.Get(identityKey)
	if !ok {
		return domain.Identity{}, false
	}
	id, ok := v.(domain.Identity)
	return id, ok
}

func unauthorized(c *gin.Context) {
	c.Header("WWW-Authenticate", "Bearer")
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
}
