package repository

import (
	"context"
	"time"

	"walletx-be/core/ports"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type SummaryDTO struct {
	CategoryID       *uuid.UUID `json:"category_id"`
	TotalAmount      float64    `json:"total_amount"`
	TransactionCount int64      `json:"transaction_count"`
}

type DailyTotalDTO struct {
	Day         time.Time `json:"day"`
	TotalAmount float64   `json:"total_amount"`
}

type SummaryRepository interface {
	GetSummary(ctx context.Context, userID uuid.UUID, period string) ([]SummaryDTO, error)
	GetDailyTotal(ctx context.Context, userID uuid.UUID, month int, year int) ([]DailyTotalDTO, error)
	GetSpendingByCategoryAndDateRange(ctx context.Context, userID uuid.UUID, startDate time.Time, endDate time.Time) ([]SummaryDTO, error)
}

type summaryRepository struct {
	db     *gorm.DB
	logger ports.Logger
}

func NewSummaryRepository(db *gorm.DB, logger ports.Logger) SummaryRepository {
	return &summaryRepository{
		db:     db,
		logger: logger,
	}
}

func (r *summaryRepository) GetSummary(ctx context.Context, userID uuid.UUID, period string) ([]SummaryDTO, error) {
	var startDate time.Time
	now := time.Now()

	switch period {
	case "weekly":
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		startDate = now.AddDate(0, 0, -(weekday - 1))
		startDate = time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, startDate.Location())
	default:
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
	if err := r.db.WithContext(ctx).Raw(query, userID, startDate).Scan(&results).Error; err != nil {
		return nil, err
	}
	return results, nil
}

func (r *summaryRepository) GetSpendingByCategoryAndDateRange(ctx context.Context, userID uuid.UUID, startDate time.Time, endDate time.Time) ([]SummaryDTO, error) {
	query := `
		SELECT
			category_id,
			SUM(total_amount)      AS total_amount,
			SUM(transaction_count) AS transaction_count
		FROM daily_expense_summary
		WHERE user_id = ?
		  AND day >= ?
		  AND day <= ?
		GROUP BY category_id
	`
	var results []SummaryDTO
	if err := r.db.WithContext(ctx).Raw(query, userID, startDate, endDate).Scan(&results).Error; err != nil {
		return nil, err
	}
	return results, nil
}

func (r *summaryRepository) GetDailyTotal(ctx context.Context, userID uuid.UUID, month int, year int) ([]DailyTotalDTO, error) {
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
	if err := r.db.WithContext(ctx).Raw(query, userID, startDate, endDate).Scan(&results).Error; err != nil {
		return nil, err
	}
	return results, nil
}
