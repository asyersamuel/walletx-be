package models

import (
	"time"

	"github.com/google/uuid"
)

// CategoryLimit represents a spending budget cap for a given category.
// Uses hard deletes (no gorm.DeletedAt) to match the DB schema.
type CategoryLimit struct {
	ID          uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID      uuid.UUID `gorm:"type:uuid;not null;index"                        json:"user_id"`
	CategoryID  uuid.UUID `gorm:"type:uuid;not null"                              json:"category_id"`
	LimitAmount float64   `gorm:"type:numeric(15,2);not null"                     json:"limit_amount"`

	// Period must be either "weekly" or "monthly"
	Period   string `gorm:"not null"        json:"period"`
	IsActive bool   `gorm:"default:true"    json:"is_active"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Associations
	Category Category `gorm:"foreignKey:CategoryID" json:"category,omitempty"`
}
