package auth

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// User represents the person registered via Google SSO.
type User struct {
	ID             uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	GoogleID       string    `gorm:"uniqueIndex;not null" json:"google_id"`
	Email          string    `gorm:"uniqueIndex;not null" json:"email"` // Used for matching bank emails
	Name           string    `gorm:"not null" json:"name"`
	Picture        string    `json:"picture"`
	TelegramChatID *string   `gorm:"uniqueIndex" json:"telegram_chat_id"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}
