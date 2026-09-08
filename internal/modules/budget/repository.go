package budget

import (
	"context"
	"errors"
	"time"

	"walletx-be/internal/platform/logger"
	"walletx-be/internal/shared/errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Repository is the persisted data access for budget limits.
type Repository interface {
	Create(ctx context.Context, limit *CategoryLimit) error
	List(ctx context.Context, userID uuid.UUID) ([]CategoryLimit, error)
	GetByID(ctx context.Context, id, userID uuid.UUID) (*CategoryLimit, error)
	Update(ctx context.Context, limit *CategoryLimit) error
	Delete(ctx context.Context, id, userID uuid.UUID) error
	GetActiveByUserID(ctx context.Context, userID uuid.UUID) ([]CategoryLimit, error)
	FindByCategory(ctx context.Context, userID, categoryID uuid.UUID) (*CategoryLimit, error)
	GetLimitReportByUserID(ctx context.Context, userID uuid.UUID) ([]LimitReport, error)
	WithTx(tx *gorm.DB) Repository
}

// SummaryRepository provides read-only aggregates over the daily_expense_summary
// table. It is shared by the budget progress calculation and the dashboard.
type SummaryRepository interface {
	GetSummary(ctx context.Context, userID uuid.UUID, period string) ([]SummaryDTO, error)
	GetDailyTotal(ctx context.Context, userID uuid.UUID, month int, year int) ([]DailyTotalDTO, error)
	GetSpendingByCategoryAndDateRange(ctx context.Context, userID uuid.UUID, startDate time.Time, endDate time.Time) ([]SummaryDTO, error)
}

type repository struct {
	db     *gorm.DB
	logger logger.Logger
}

func NewRepository(db *gorm.DB, logger logger.Logger) Repository {
	return &repository{
		db:     db,
		logger: logger,
	}
}

func (r *repository) Create(ctx context.Context, limit *CategoryLimit) error {
	err := r.db.WithContext(ctx).Create(limit).Error
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return apperrors.ErrDuplicate
	}
	return err
}

func (r *repository) List(ctx context.Context, userID uuid.UUID) ([]CategoryLimit, error) {
	var limits []CategoryLimit
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at DESC").Find(&limits).Error; err != nil {
		return nil, err
	}
	return limits, nil
}

func (r *repository) GetByID(ctx context.Context, id, userID uuid.UUID) (*CategoryLimit, error) {
	var limit CategoryLimit
	err := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&limit).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrNotFound
		}
		return nil, err
	}
	return &limit, nil
}

func (r *repository) Update(ctx context.Context, limit *CategoryLimit) error {
	return r.db.WithContext(ctx).Save(limit).Error
}

func (r *repository) Delete(ctx context.Context, id, userID uuid.UUID) error {
	result := r.db.WithContext(ctx).Unscoped().Where("id = ? AND user_id = ?", id, userID).Delete(&CategoryLimit{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrNotFound
	}
	return nil
}

func (r *repository) GetActiveByUserID(ctx context.Context, userID uuid.UUID) ([]CategoryLimit, error) {
	var limits []CategoryLimit
	if err := r.db.WithContext(ctx).Preload("Category").Where("user_id = ? AND is_active = true", userID).Find(&limits).Error; err != nil {
		return nil, err
	}
	return limits, nil
}

func (r *repository) FindByCategory(ctx context.Context, userID, categoryID uuid.UUID) (*CategoryLimit, error) {
	var limit CategoryLimit
	err := r.db.WithContext(ctx).Where("user_id = ? AND category_id = ?", userID, categoryID).First(&limit).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrNotFound
		}
		return nil, err
	}
	return &limit, nil
}

func (r *repository) GetLimitReportByUserID(ctx context.Context, userID uuid.UUID) ([]LimitReport, error) {
	var results []LimitReport

	// Single efficient query: JOIN category_limits + categories, then
	// LEFT JOIN transactions scoped to the current calendar month only.
	// DATE_TRUNC('month', ...) on both sides ensures we compare month-boundaries
	// without application-level date arithmetic, keeping timezone handling
	// consistent with the PostgreSQL server's clock.
	query := `
		SELECT
			c.name        AS category_name,
			c.icon        AS category_icon,
			cl.limit_amount,
			COALESCE(SUM(t.amount), 0) AS total_spent
		FROM category_limits cl
		JOIN categories c ON cl.category_id = c.id
		LEFT JOIN transactions t
			ON  t.category_id = cl.category_id
			AND t.user_id    = cl.user_id
			AND t.deleted_at IS NULL
			AND DATE_TRUNC('month', t.transaction_date) = DATE_TRUNC('month', CURRENT_DATE)
		WHERE cl.user_id   = ?
		  AND cl.is_active = true
		  AND c.deleted_at IS NULL
		GROUP BY c.id, c.name, c.icon, cl.id, cl.limit_amount
		ORDER BY c.name ASC
	`

	if err := r.db.WithContext(ctx).Raw(query, userID).Scan(&results).Error; err != nil {
		return nil, err
	}
	return results, nil
}

func (r *repository) WithTx(tx *gorm.DB) Repository {
	if tx == nil {
		return r
	}
	return &repository{db: tx}
}

// ─── Summary repository ───────────────────────────────────────────────────────

type summaryRepository struct {
	db     *gorm.DB
	logger logger.Logger
}

func NewSummaryRepository(db *gorm.DB, logger logger.Logger) SummaryRepository {
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
