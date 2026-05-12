package repository

import (
	"context"
	"errors"

	"walletx-be/core/domain"
	"walletx-be/core/ports"
	"walletx-be/pkg/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type RecurringRepository interface {
	Create(ctx context.Context, config *models.RecurringConfig) error
	List(ctx context.Context, userID uuid.UUID) ([]models.RecurringConfig, error)
	GetByID(ctx context.Context, id, userID uuid.UUID) (*models.RecurringConfig, error)
	Update(ctx context.Context, config *models.RecurringConfig) error
	Delete(ctx context.Context, id, userID uuid.UUID) error
	GetDueConfigs(ctx context.Context) ([]models.RecurringConfig, error)
}

type recurringRepository struct {
	db     *gorm.DB
	logger ports.Logger
}

func NewRecurringRepository(db *gorm.DB, logger ports.Logger) RecurringRepository {
	return &recurringRepository{
		db:     db,
		logger: logger,
	}
}

func (r *recurringRepository) Create(ctx context.Context, config *models.RecurringConfig) error {
	err := r.db.WithContext(ctx).Create(config).Error
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return domain.ErrDuplicate
	}
	return err
}

func (r *recurringRepository) List(ctx context.Context, userID uuid.UUID) ([]models.RecurringConfig, error) {
	var configs []models.RecurringConfig
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at DESC").Find(&configs).Error; err != nil {
		return nil, err
	}
	return configs, nil
}

func (r *recurringRepository) GetByID(ctx context.Context, id, userID uuid.UUID) (*models.RecurringConfig, error) {
	var config models.RecurringConfig
	err := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&config).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &config, nil
}

func (r *recurringRepository) Update(ctx context.Context, config *models.RecurringConfig) error {
	return r.db.WithContext(ctx).Save(config).Error
}

func (r *recurringRepository) Delete(ctx context.Context, id, userID uuid.UUID) error {
	result := r.db.WithContext(ctx).Unscoped().Where("id = ? AND user_id = ?", id, userID).Delete(&models.RecurringConfig{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *recurringRepository) GetDueConfigs(ctx context.Context) ([]models.RecurringConfig, error) {
	var configs []models.RecurringConfig
	if err := r.db.WithContext(ctx).Where("next_due_date <= NOW()").Find(&configs).Error; err != nil {
		return nil, err
	}
	return configs, nil
}
