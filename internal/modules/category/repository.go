package category

import (
	"context"
	"errors"

	"walletx-be/internal/platform/logger"
	"walletx-be/internal/shared/errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Repository is the persisted data access for Category aggregates.
type Repository interface {
	Create(ctx context.Context, category *Category) error
	List(ctx context.Context, userID uuid.UUID) ([]Category, error)
	GetByID(ctx context.Context, id, userID uuid.UUID) (*Category, error)
	Update(ctx context.Context, category *Category) error
	Delete(ctx context.Context, id, userID uuid.UUID) error
	GetByName(ctx context.Context, name string, userID uuid.UUID) (*Category, error)
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

func (r *repository) Create(ctx context.Context, category *Category) error {
	err := r.db.WithContext(ctx).Create(category).Error
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return apperrors.ErrDuplicate
	}
	return err
}

func (r *repository) List(ctx context.Context, userID uuid.UUID) ([]Category, error) {
	var categories []Category
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at DESC").Find(&categories).Error; err != nil {
		return nil, err
	}
	return categories, nil
}

func (r *repository) GetByID(ctx context.Context, id, userID uuid.UUID) (*Category, error) {
	var category Category
	err := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&category).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrNotFound
		}
		return nil, err
	}
	return &category, nil
}

func (r *repository) Update(ctx context.Context, category *Category) error {
	return r.db.WithContext(ctx).Save(category).Error
}

func (r *repository) Delete(ctx context.Context, id, userID uuid.UUID) error {
	result := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).Delete(&Category{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrNotFound
	}
	return nil
}

func (r *repository) GetByName(ctx context.Context, name string, userID uuid.UUID) (*Category, error) {
	var category Category
	err := r.db.WithContext(ctx).Where("name ILIKE ? AND user_id = ?", name, userID).First(&category).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrNotFound
		}
		return nil, err
	}
	return &category, nil
}
