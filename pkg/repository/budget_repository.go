package repository

import (
	"context"
	"errors"

	"walletx-be/internal/domain"
	"walletx-be/internal/domain/report"
	"walletx-be/internal/ports"
	"walletx-be/pkg/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type BudgetRepository interface {
	Create(ctx context.Context, limit *models.CategoryLimit) error
	List(ctx context.Context, userID uuid.UUID) ([]models.CategoryLimit, error)
	GetByID(ctx context.Context, id, userID uuid.UUID) (*models.CategoryLimit, error)
	Update(ctx context.Context, limit *models.CategoryLimit) error
	Delete(ctx context.Context, id, userID uuid.UUID) error
	GetActiveByUserID(ctx context.Context, userID uuid.UUID) ([]models.CategoryLimit, error)
	FindByCategory(ctx context.Context, userID, categoryID uuid.UUID) (*models.CategoryLimit, error)
	GetLimitReportByUserID(ctx context.Context, userID uuid.UUID) ([]report.LimitReport, error)
	WithTx(tx *gorm.DB) BudgetRepository
}

type budgetRepository struct {
	db     *gorm.DB
	logger ports.Logger
}

func NewBudgetRepository(db *gorm.DB, logger ports.Logger) BudgetRepository {
	return &budgetRepository{
		db:     db,
		logger: logger,
	}
}

func (r *budgetRepository) Create(ctx context.Context, limit *models.CategoryLimit) error {
	err := r.db.WithContext(ctx).Create(limit).Error
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return domain.ErrDuplicate
	}
	return err
}

func (r *budgetRepository) List(ctx context.Context, userID uuid.UUID) ([]models.CategoryLimit, error) {
	var limits []models.CategoryLimit
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at DESC").Find(&limits).Error; err != nil {
		return nil, err
	}
	return limits, nil
}

func (r *budgetRepository) GetByID(ctx context.Context, id, userID uuid.UUID) (*models.CategoryLimit, error) {
	var limit models.CategoryLimit
	err := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&limit).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &limit, nil
}

func (r *budgetRepository) Update(ctx context.Context, limit *models.CategoryLimit) error {
	return r.db.WithContext(ctx).Save(limit).Error
}

func (r *budgetRepository) Delete(ctx context.Context, id, userID uuid.UUID) error {
	result := r.db.WithContext(ctx).Unscoped().Where("id = ? AND user_id = ?", id, userID).Delete(&models.CategoryLimit{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *budgetRepository) GetActiveByUserID(ctx context.Context, userID uuid.UUID) ([]models.CategoryLimit, error) {
	var limits []models.CategoryLimit
	if err := r.db.WithContext(ctx).Preload("Category").Where("user_id = ? AND is_active = true", userID).Find(&limits).Error; err != nil {
		return nil, err
	}
	return limits, nil
}

func (r *budgetRepository) FindByCategory(ctx context.Context, userID, categoryID uuid.UUID) (*models.CategoryLimit, error) {
	var limit models.CategoryLimit
	err := r.db.WithContext(ctx).Where("user_id = ? AND category_id = ?", userID, categoryID).First(&limit).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &limit, nil
}

func (r *budgetRepository) GetLimitReportByUserID(ctx context.Context, userID uuid.UUID) ([]report.LimitReport, error) {
	var results []report.LimitReport

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

func (r *budgetRepository) WithTx(tx *gorm.DB) BudgetRepository {
	if tx == nil {
		return r
	}
	return &budgetRepository{db: tx}
}
