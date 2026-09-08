package recurring

import (
	"time"

	"github.com/google/uuid"
)

// Config holds the schedule for a recurring transaction.
// Uses hard deletes (no gorm.DeletedAt) to match the DB schema.
type Config struct {
	ID         uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID     uuid.UUID `gorm:"type:uuid;not null;index"                        json:"user_id"`
	CategoryID uuid.UUID `gorm:"type:uuid;not null"                              json:"category_id"`
	Amount     float64   `gorm:"type:numeric(15,2);not null"                     json:"amount"`

	// Frequency must be "weekly", "monthly", or "yearly"
	Frequency   string    `gorm:"not null"      json:"frequency"`
	StartDate   time.Time `gorm:"not null"      json:"start_date"`
	NextDueDate time.Time `gorm:"not null"      json:"next_due_date"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
