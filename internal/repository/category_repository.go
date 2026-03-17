package repository

import (
	"errors"

	"walletx-be/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CategoryRepository defines the data access contract for categories
type CategoryRepository interface {
	Create(category *models.Category) error
	List(userID uuid.UUID) ([]models.Category, error)
	GetByID(id, userID uuid.UUID) (*models.Category, error)
	Update(category *models.Category) error
	Delete(id, userID uuid.UUID) error
}

type categoryRepository struct {
	db *gorm.DB
}

func NewCategoryRepository(db *gorm.DB) CategoryRepository {
	return &categoryRepository{db: db}
}

func (r *categoryRepository) Create(category *models.Category) error {
	return r.db.Create(category).Error
}

func (r *categoryRepository) List(userID uuid.UUID) ([]models.Category, error) {
	var categories []models.Category
	if err := r.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&categories).Error; err != nil {
		return nil, err
	}
	return categories, nil
}

func (r *categoryRepository) GetByID(id, userID uuid.UUID) (*models.Category, error) {
	var category models.Category
	err := r.db.Where("id = ? AND user_id = ?", id, userID).First(&category).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, err
	}
	return &category, nil
}

func (r *categoryRepository) Update(category *models.Category) error {
	return r.db.Save(category).Error
}

func (r *categoryRepository) Delete(id, userID uuid.UUID) error {
	result := r.db.Where("id = ? AND user_id = ?", id, userID).Delete(&models.Category{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
