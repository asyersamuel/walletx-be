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

type CategoryRepository interface {
	Create(ctx context.Context, category *models.Category) error
	List(ctx context.Context, userID uuid.UUID) ([]models.Category, error)
	GetByID(ctx context.Context, id, userID uuid.UUID) (*models.Category, error)
	Update(ctx context.Context, category *models.Category) error
	Delete(ctx context.Context, id, userID uuid.UUID) error
	GetByName(ctx context.Context, name string, userID uuid.UUID) (*models.Category, error)
}

type categoryRepository struct {
	db     *gorm.DB
	logger ports.Logger
}

func NewCategoryRepository(db *gorm.DB, logger ports.Logger) CategoryRepository {
	return &categoryRepository{
		db:     db,
		logger: logger,
	}
}

func (r *categoryRepository) Create(ctx context.Context, category *models.Category) error {
	err := r.db.WithContext(ctx).Create(category).Error
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return domain.ErrDuplicate
	}
	return err
}

func (r *categoryRepository) List(ctx context.Context, userID uuid.UUID) ([]models.Category, error) {
	var categories []models.Category
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at DESC").Find(&categories).Error; err != nil {
		return nil, err
	}
	return categories, nil
}

func (r *categoryRepository) GetByID(ctx context.Context, id, userID uuid.UUID) (*models.Category, error) {
	var category models.Category
	err := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&category).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &category, nil
}

func (r *categoryRepository) Update(ctx context.Context, category *models.Category) error {
	return r.db.WithContext(ctx).Save(category).Error
}

func (r *categoryRepository) Delete(ctx context.Context, id, userID uuid.UUID) error {
	result := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).Delete(&models.Category{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *categoryRepository) GetByName(ctx context.Context, name string, userID uuid.UUID) (*models.Category, error) {
	var category models.Category
	err := r.db.WithContext(ctx).Where("name ILIKE ? AND user_id = ?", name, userID).First(&category).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &category, nil
}
