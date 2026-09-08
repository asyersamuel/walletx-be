package recurring

import (
	"context"
	"errors"

	"walletx-be/internal/platform/logger"
	"walletx-be/internal/shared/errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Repository is the persisted data access for recurring configs.
type Repository interface {
	Create(ctx context.Context, config *Config) error
	List(ctx context.Context, userID uuid.UUID) ([]Config, error)
	GetByID(ctx context.Context, id, userID uuid.UUID) (*Config, error)
	Update(ctx context.Context, config *Config) error
	Delete(ctx context.Context, id, userID uuid.UUID) error
	GetDueConfigs(ctx context.Context) ([]Config, error)
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

func (r *repository) Create(ctx context.Context, config *Config) error {
	err := r.db.WithContext(ctx).Create(config).Error
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return apperrors.ErrDuplicate
	}
	return err
}

func (r *repository) List(ctx context.Context, userID uuid.UUID) ([]Config, error) {
	var configs []Config
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at DESC").Find(&configs).Error; err != nil {
		return nil, err
	}
	return configs, nil
}

func (r *repository) GetByID(ctx context.Context, id, userID uuid.UUID) (*Config, error) {
	var config Config
	err := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&config).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrNotFound
		}
		return nil, err
	}
	return &config, nil
}

func (r *repository) Update(ctx context.Context, config *Config) error {
	return r.db.WithContext(ctx).Save(config).Error
}

func (r *repository) Delete(ctx context.Context, id, userID uuid.UUID) error {
	result := r.db.WithContext(ctx).Unscoped().Where("id = ? AND user_id = ?", id, userID).Delete(&Config{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrNotFound
	}
	return nil
}

func (r *repository) GetDueConfigs(ctx context.Context) ([]Config, error) {
	var configs []Config
	if err := r.db.WithContext(ctx).Where("next_due_date <= NOW()").Find(&configs).Error; err != nil {
		return nil, err
	}
	return configs, nil
}
