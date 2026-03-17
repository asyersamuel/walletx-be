package repository

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SummaryDTO is a plain data-transfer object used to aggregate
// spending from the daily_expense_summary VIEW — it is NOT a GORM model.
type SummaryDTO struct {
	CategoryID       *uuid.UUID `json:"category_id"`
	TotalAmount      float64    `json:"total_amount"`
	TransactionCount int64      `json:"transaction_count"`
}

// SummaryRepository defines the contract for querying the daily_expense_summary VIEW
type SummaryRepository interface {
	// GetSummary returns aggregated spending for the user within the given period.
	// period accepted values: "weekly", "monthly"
	GetSummary(userID uuid.UUID, period string) ([]SummaryDTO, error)
}

type summaryRepository struct {
	db *gorm.DB
}

func NewSummaryRepository(db *gorm.DB) SummaryRepository {
	return &summaryRepository{db: db}
}

// GetSummary queries the daily_expense_summary VIEW using raw SQL.
// The VIEW already groups by user_id, category_id, and day, so we aggregate
// the SUM and COUNT from it for the requested period.
func (r *summaryRepository) GetSummary(userID uuid.UUID, period string) ([]SummaryDTO, error) {
	var startDate time.Time
	now := time.Now()

	switch period {
	case "weekly":
		// Start from Monday of the current week
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7 // Sunday → treat as 7 so Monday is -6
		}
		startDate = now.AddDate(0, 0, -(weekday - 1))
		startDate = time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, startDate.Location())
	default: // "monthly"
		startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	}

	query := `
		SELECT
			category_id,
			SUM(total_amount)      AS total_amount,
			SUM(transaction_count) AS transaction_count
		FROM daily_expense_summary
		WHERE user_id = ?
		  AND day >= ?
		GROUP BY category_id
	`

	var results []SummaryDTO
	if err := r.db.Raw(query, userID, startDate).Scan(&results).Error; err != nil {
		return nil, err
	}
	return results, nil
}
