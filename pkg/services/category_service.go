package services

import (
	"errors"

	"walletx-be/pkg/models"
	"walletx-be/pkg/repository"

	"github.com/google/uuid"
)

// CategoryService defines the business logic contract for categories
type CategoryService interface {
	CreateCategory(userID uuid.UUID, name, icon string) (*models.Category, error)
	ListCategories(userID uuid.UUID) ([]models.Category, error)
	GetCategoryByID(id, userID uuid.UUID) (*models.Category, error)
	UpdateCategory(id, userID uuid.UUID, name, icon string) (*models.Category, error)
	DeleteCategory(id, userID uuid.UUID) error
}

type categoryService struct {
	categoryRepo repository.CategoryRepository
}

func NewCategoryService(categoryRepo repository.CategoryRepository) CategoryService {
	return &categoryService{categoryRepo: categoryRepo}
}

func (s *categoryService) CreateCategory(userID uuid.UUID, name, icon string) (*models.Category, error) {
	if name == "" {
		return nil, errors.New("category name cannot be empty")
	}

	category := &models.Category{
		UserID: userID,
		Name:   name,
		Icon:   icon,
	}

	if err := s.categoryRepo.Create(category); err != nil {
		return nil, err
	}
	return category, nil
}

func (s *categoryService) ListCategories(userID uuid.UUID) ([]models.Category, error) {
	return s.categoryRepo.List(userID)
}

func (s *categoryService) GetCategoryByID(id, userID uuid.UUID) (*models.Category, error) {
	return s.categoryRepo.GetByID(id, userID)
}

func (s *categoryService) UpdateCategory(id, userID uuid.UUID, name, icon string) (*models.Category, error) {
	if name == "" {
		return nil, errors.New("category name cannot be empty")
	}

	category, err := s.categoryRepo.GetByID(id, userID)
	if err != nil {
		return nil, err
	}

	category.Name = name
	category.Icon = icon

	if err := s.categoryRepo.Update(category); err != nil {
		return nil, err
	}
	return category, nil
}

func (s *categoryService) DeleteCategory(id, userID uuid.UUID) error {
	return s.categoryRepo.Delete(id, userID)
}
