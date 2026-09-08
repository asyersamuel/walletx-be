package category

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Category represents a user-defined expense category (e.g., Food, Transport).
type Category struct {
	ID     uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID uuid.UUID `gorm:"type:uuid;not null;index"                        json:"user_id"`
	Name   string    `gorm:"not null"                                        json:"name"`
	Icon   string    `gorm:"default:''"                                      json:"icon"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}
