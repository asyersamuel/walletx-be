package categories

import (
	"time"

	"github.com/google/uuid"
)

const (
	CategoryTypeExpense   = "expense"
	CategoryTypeIncome    = "income"
	maxCategoryNameLength = 100
)

type Category struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
