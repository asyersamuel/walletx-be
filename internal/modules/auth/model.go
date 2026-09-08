package auth

import (
	"time"

	"github.com/google/uuid"
)

// User represents the person registered via Google SSO.
type User struct {
	ID       uuid.UUID `json:"id"`
	GoogleID string    `json:"google_id"`
	Email    string    `json:"email"`
	Name     string    `json:"name"`
	Picture  string    `json:"picture"`

	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"-"`
}
