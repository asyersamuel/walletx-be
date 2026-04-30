package repository

import (
	"context"
	"errors"

	"walletx-be/internal/domain"
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

func (r *budgetRepository) WithTx(tx *gorm.DB) BudgetRepository {
	if tx == nil {
		return r
	}
	return &budgetRepository{db: tx}
}
