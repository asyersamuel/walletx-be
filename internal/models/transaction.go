package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Transaction represents financial data extracted from bank emails
type Transaction struct {
	ID uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`

	// Belongs to User
	UserID uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`

	// Numeric type used for high precision currency (15 total digits, 2 decimal places)
	Amount          float64   `gorm:"type:numeric(15,2);not null" json:"amount"`
	Merchant        string    `gorm:"not null" json:"merchant"`
	TransactionDate time.Time `gorm:"not null" json:"transaction_date"`

	// IDEMPOTENCY KEY: Unique Email Message-ID to prevent duplicate records
	MessageID string `gorm:"uniqueIndex;not null" json:"message_id"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// Relation back to User for GORM Eager Loading (Preload)
	User User `gorm:"foreignKey:UserID" json:"-"`
}