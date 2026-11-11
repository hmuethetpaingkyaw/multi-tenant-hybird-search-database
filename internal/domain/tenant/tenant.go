// Package tenant holds the tenant and user domain types.
package tenant

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound means the row does not exist or is not visible to the caller's tenant.
// The two cases are deliberately indistinguishable.
var ErrNotFound = errors.New("not found")

type Tenant struct {
	ID        string
	Name      string
	CreatedAt time.Time
}

type User struct {
	ID        string
	TenantID  string
	Email     string
	CreatedAt time.Time
}

// Identity is the authenticated caller. TenantID is derived from a verified credential,
// never from request input.
type Identity struct {
	UserID   string
	TenantID string
}

// Repository reads tenant data. scopeTenantID is the RLS context the query runs under;
// implementations must not add their own tenant filters.
type Repository interface {
	GetTenant(ctx context.Context, scopeTenantID, id string) (Tenant, error)
	GetUser(ctx context.Context, scopeTenantID, id string) (User, error)
}
