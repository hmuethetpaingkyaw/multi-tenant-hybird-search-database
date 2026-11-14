package domain

// Identity is the authenticated caller. TenantID is derived from a verified credential,
// never from request input.
type Identity struct {
	UserID   string
	TenantID string
}
