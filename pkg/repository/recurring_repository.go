package repository

import (
	"errors"

	"walletx-be/pkg/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// RecurringRepository defines the data access contract for recurring_configs
type RecurringRepository interface {
	Create(config *models.RecurringConfig) error
	List(userID uuid.UUID) ([]models.RecurringConfig, error)
	GetByID(id, userID uuid.UUID) (*models.RecurringConfig, error)
	Update(config *models.RecurringConfig) error
	Delete(id, userID uuid.UUID) error
	// GetDueConfigs returns all configs where next_due_date is today or in the past
	GetDueConfigs() ([]models.RecurringConfig, error)
}

type recurringRepository struct {
	db *gorm.DB
}

func NewRecurringRepository(db *gorm.DB) RecurringRepository {
	return &recurringRepository{db: db}
}

func (r *recurringRepository) Create(config *models.RecurringConfig) error {
	return r.db.Create(config).Error
}

func (r *recurringRepository) List(userID uuid.UUID) ([]models.RecurringConfig, error) {
	var configs []models.RecurringConfig
	if err := r.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&configs).Error; err != nil {
		return nil, err
	}
	return configs, nil
}

func (r *recurringRepository) GetByID(id, userID uuid.UUID) (*models.RecurringConfig, error) {
	var config models.RecurringConfig
	err := r.db.Where("id = ? AND user_id = ?", id, userID).First(&config).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, err
	}
	return &config, nil
}

func (r *recurringRepository) Update(config *models.RecurringConfig) error {
	return r.db.Save(config).Error
}

func (r *recurringRepository) Delete(id, userID uuid.UUID) error {
	// Hard delete — RecurringConfig has no DeletedAt
	result := r.db.Unscoped().Where("id = ? AND user_id = ?", id, userID).Delete(&models.RecurringConfig{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *recurringRepository) GetDueConfigs() ([]models.RecurringConfig, error) {
	var configs []models.RecurringConfig
	// Find all configs where the next due date is today or overdue
	if err := r.db.Where("next_due_date <= NOW()").Find(&configs).Error; err != nil {
		return nil, err
	}
	return configs, nil
}
