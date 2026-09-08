package recurring

import (
	"time"

	"github.com/google/uuid"
)

// Config holds the schedule for a recurring transaction.
// Uses hard deletes to match the database migration.
type Config struct {
	ID         uuid.UUID  `json:"id"`
	UserID     uuid.UUID  `json:"user_id"`
	CategoryID *uuid.UUID `json:"category_id"`
	Amount     float64    `json:"amount"`

	// Frequency must be "weekly", "monthly", or "yearly"
	Frequency   string    `json:"frequency"`
	StartDate   time.Time `json:"start_date"`
	NextDueDate time.Time `json:"next_due_date"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
