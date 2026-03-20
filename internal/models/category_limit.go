package models

import (
	"time"

	"github.com/google/uuid"
)

// CategoryLimit represents a spending budget cap for a given category.
// Uses hard deletes (no gorm.DeletedAt) to match the DB schema.
type CategoryLimit struct {
	ID          uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID      uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_user_category" json:"user_id"`
	CategoryID  uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_user_category" json:"category_id"`
	LimitAmount float64   `gorm:"type:numeric(15,2);not null" binding:"required,gt=0" json:"limit_amount"`

	IsActive bool `gorm:"default:true"    json:"is_active"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Associations
	Category Category `gorm:"foreignKey:CategoryID" json:"category,omitempty"`
}
