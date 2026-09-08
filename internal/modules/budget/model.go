package budget

import (
	"time"

	"walletx-be/internal/modules/category"

	"github.com/google/uuid"
)

// CategoryLimit represents a spending budget cap for a given category.
// Uses hard deletes (no gorm.DeletedAt) to match the DB schema.
type CategoryLimit struct {
	ID          uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID      uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_user_category" json:"user_id"`
	CategoryID  uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_user_category" json:"category_id"`
	LimitAmount float64   `gorm:"type:numeric(15,2);not null" binding:"required,gt=0" json:"limit_amount"`
	Period      string    `gorm:"type:text;not null;default:'monthly'" json:"period"`

	IsActive bool `gorm:"default:true"    json:"is_active"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Associations
	Category category.Category `gorm:"foreignKey:CategoryID" json:"category,omitempty"`
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
