// Package middleware holds the Gin middleware.
package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"

	"multi-tenant-hybrid-search/apperror"
	"multi-tenant-hybrid-search/domain"
)

const identityKey = "identity"

// TokenParser verifies a bearer token and returns the caller's identity.
type TokenParser interface {
	Parse(token string) (domain.Identity, error)
}

// Auth requires a valid bearer token. The tenant comes only from the verified token.
// Failures return a generic 401 that does not reveal why verification failed; the response is
// written by ErrorHandler.
func Auth(p TokenParser) gin.HandlerFunc {
	return func(c *gin.Context) {
		scheme, token, ok := strings.Cut(c.GetHeader("Authorization"), " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(token) == "" {
			_ = c.Error(apperror.Unauthorized())
			c.Abort()
			return
		}
		id, err := p.Parse(strings.TrimSpace(token))
		if err != nil {
			_ = c.Error(apperror.Unauthorized())
			c.Abort()
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
