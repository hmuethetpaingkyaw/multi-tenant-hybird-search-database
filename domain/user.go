package domain

import "time"

type User struct {
	ID        string
	TenantID  string
	Email     string
	CreatedAt time.Time
}
