// Package domain holds the plain entities of the application. It imports nothing but constants.
package domain

import "time"

type Tenant struct {
	ID        string
	Name      string
	CreatedAt time.Time
}
