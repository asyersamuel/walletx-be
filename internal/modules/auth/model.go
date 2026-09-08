package auth

import (
	"time"

	"github.com/google/uuid"
)

// User represents the person registered via Google SSO.
type User struct {
	ID             uuid.UUID `json:"id"`
	GoogleID       string    `json:"google_id"`
	Email          string    `json:"email"` // Used for matching bank emails
	Name           string    `json:"name"`
	Picture        string    `json:"picture"`
	TelegramChatID *string   `json:"telegram_chat_id"`

	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"-"`
}
