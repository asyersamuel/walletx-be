package repository

import (
	"errors"

	"walletx-be/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// BudgetRepository defines the data access contract for category_limits
type BudgetRepository interface {
	Create(limit *models.CategoryLimit) error
	List(userID uuid.UUID) ([]models.CategoryLimit, error)
	GetByID(id, userID uuid.UUID) (*models.CategoryLimit, error)
	Update(limit *models.CategoryLimit) error
	Delete(id, userID uuid.UUID) error
	// GetActiveByUserID returns only active limits — used by DashboardService
	GetActiveByUserID(userID uuid.UUID) ([]models.CategoryLimit, error)
	FindByCategory(userID, categoryID uuid.UUID) (*models.CategoryLimit, error)
	WithTx(tx *gorm.DB) BudgetRepository
}

type budgetRepository struct {
	db *gorm.DB
}

func NewBudgetRepository(db *gorm.DB) BudgetRepository {
	return &budgetRepository{db: db}
}

func (r *budgetRepository) Create(limit *models.CategoryLimit) error {
	return r.db.Create(limit).Error
}

func (r *budgetRepository) List(userID uuid.UUID) ([]models.CategoryLimit, error) {
	var limits []models.CategoryLimit
	if err := r.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&limits).Error; err != nil {
		return nil, err
	}
	return limits, nil
}

func (r *budgetRepository) GetByID(id, userID uuid.UUID) (*models.CategoryLimit, error) {
	var limit models.CategoryLimit
	err := r.db.Where("id = ? AND user_id = ?", id, userID).First(&limit).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, err
	}
	return &limit, nil
}

func (r *budgetRepository) Update(limit *models.CategoryLimit) error {
	return r.db.Save(limit).Error
}

func (r *budgetRepository) Delete(id, userID uuid.UUID) error {
	// Hard delete — CategoryLimit has no DeletedAt
	result := r.db.Unscoped().Where("id = ? AND user_id = ?", id, userID).Delete(&models.CategoryLimit{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *budgetRepository) GetActiveByUserID(userID uuid.UUID) ([]models.CategoryLimit, error) {
	var limits []models.CategoryLimit
	if err := r.db.Preload("Category").Where("user_id = ? AND is_active = true", userID).Find(&limits).Error; err != nil {
		return nil, err
	}
	return limits, nil
}

func (r *budgetRepository) FindByCategory(userID, categoryID uuid.UUID) (*models.CategoryLimit, error) {
	var limit models.CategoryLimit
	err := r.db.Where("user_id = ? AND category_id = ?", userID, categoryID).First(&limit).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gorm.ErrRecordNotFound
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
