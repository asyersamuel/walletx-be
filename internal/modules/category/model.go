package category

import (
	"time"

	"github.com/google/uuid"
)

// Category represents a user-defined expense category (e.g., Food, Transport).
type Category struct {
	ID     uuid.UUID `json:"id"`
	UserID uuid.UUID `json:"user_id"`
	Name   string    `json:"name"`
	Icon   string    `json:"icon"`

	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at"`
}
