package budget

import (
	"time"

	"walletx-be/internal/modules/category"

	"github.com/google/uuid"
)

// CategoryLimit represents a spending budget cap for a given category.
// Uses hard deletes to match the database migration.
type CategoryLimit struct {
	ID          uuid.UUID `json:"id"`
	UserID      uuid.UUID `json:"user_id"`
	CategoryID  uuid.UUID `json:"category_id"`
	LimitAmount float64   `binding:"required,gt=0" json:"limit_amount"`
	Period      string    `json:"period"`

	IsActive bool `json:"is_active"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Associations
	Category category.Category `json:"category,omitempty"`
}

// LimitReport is the result of a real-time JOIN query that pairs each active
// category limit with the amount already spent in the current calendar month.
// It is produced by budget querying and consumed by the Telegram service.
type LimitReport struct {
	CategoryName string  `json:"category_name"`
	CategoryIcon string  `json:"category_icon"`
	LimitAmount  float64 `json:"limit_amount"`
	TotalSpent   float64 `json:"total_spent"`
}

// SummaryDTO is an aggregate row over the daily_expense_summary table grouped by category.
type SummaryDTO struct {
	CategoryID       *uuid.UUID `json:"category_id"`
	TotalAmount      float64    `json:"total_amount"`
	TransactionCount int64      `json:"transaction_count"`
}

// DailyTotalDTO is an aggregate row over the daily_expense_summary table grouped by day.
type DailyTotalDTO struct {
	Day         time.Time `json:"day"`
	TotalAmount float64   `json:"total_amount"`
}
