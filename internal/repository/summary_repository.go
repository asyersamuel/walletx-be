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

// DailyTotalDTO holds the aggregated spending for a specific day.
type DailyTotalDTO struct {
	Day         time.Time `json:"day"`
	TotalAmount float64   `json:"total_amount"`
}

// SummaryRepository defines the contract for querying the daily_expense_summary VIEW
type SummaryRepository interface {
	// GetSummary returns aggregated spending for the user within the given period.
	// period accepted values: "weekly", "monthly"
	GetSummary(userID uuid.UUID, period string) ([]SummaryDTO, error)
	GetDailyTotal(userID uuid.UUID, month int, year int) ([]DailyTotalDTO, error)
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

// GetDailyTotal queries the daily_expense_summary VIEW grouped by day
// to get the total spending for each day in a specific month and year.
func (r *summaryRepository) GetDailyTotal(userID uuid.UUID, month int, year int) ([]DailyTotalDTO, error) {
	startDate := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local)
	endDate := startDate.AddDate(0, 1, 0)

	query := `
		SELECT
			day,
			SUM(total_amount) AS total_amount
		FROM daily_expense_summary
		WHERE user_id = ?
		  AND day >= ?
		  AND day < ?
		GROUP BY day
		ORDER BY day ASC
	`

	var results []DailyTotalDTO
	if err := r.db.Raw(query, userID, startDate, endDate).Scan(&results).Error; err != nil {
		return nil, err
	}
	return results, nil
}
