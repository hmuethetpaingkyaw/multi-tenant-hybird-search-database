package response

import "multi-tenant-hybrid-search/domain"

type TenantDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// UserDTO keeps email before id: the JSON key order is part of the byte-for-byte API contract.
type UserDTO struct {
	Email string `json:"email"`
	ID    string `json:"id"`
}

// MeResponse is the body of GET /api/v1/me.
type MeResponse struct {
	Tenant TenantDTO `json:"tenant"`
	User   UserDTO   `json:"user"`
}

func ToMeResponse(t domain.Tenant, u domain.User) MeResponse {
	return MeResponse{Tenant: TenantDTO{ID: t.ID, Name: t.Name}, User: UserDTO{Email: u.Email, ID: u.ID}}
}
