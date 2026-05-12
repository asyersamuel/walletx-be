package services

import (
	"context"
	"errors"

	"walletx-be/core/ports"
	"walletx-be/pkg/models"
	"walletx-be/pkg/repository"

	"github.com/google/uuid"
)

type CategoryService interface {
	CreateCategory(ctx context.Context, userID uuid.UUID, name, icon string) (*models.Category, error)
	ListCategories(ctx context.Context, userID uuid.UUID) ([]models.Category, error)
	GetCategoryByID(ctx context.Context, id, userID uuid.UUID) (*models.Category, error)
	UpdateCategory(ctx context.Context, id, userID uuid.UUID, name, icon string) (*models.Category, error)
	DeleteCategory(ctx context.Context, id, userID uuid.UUID) error
}

type categoryService struct {
	categoryRepo repository.CategoryRepository
	logger       ports.Logger
}

func NewCategoryService(categoryRepo repository.CategoryRepository, logger ports.Logger) CategoryService {
	return &categoryService{
		categoryRepo: categoryRepo,
		logger:       logger,
	}
}

func (s *categoryService) CreateCategory(ctx context.Context, userID uuid.UUID, name, icon string) (*models.Category, error) {
	if name == "" {
		return nil, errors.New("category name cannot be empty")
	}

	category := &models.Category{
		UserID: userID,
		Name:   name,
		Icon:   icon,
	}

	if err := s.categoryRepo.Create(ctx, category); err != nil {
		return nil, err
	}
	return category, nil
}

func (s *categoryService) ListCategories(ctx context.Context, userID uuid.UUID) ([]models.Category, error) {
	return s.categoryRepo.List(ctx, userID)
}

func (s *categoryService) GetCategoryByID(ctx context.Context, id, userID uuid.UUID) (*models.Category, error) {
	return s.categoryRepo.GetByID(ctx, id, userID)
}

func (s *categoryService) UpdateCategory(ctx context.Context, id, userID uuid.UUID, name, icon string) (*models.Category, error) {
	if name == "" {
		return nil, errors.New("category name cannot be empty")
	}

	category, err := s.categoryRepo.GetByID(ctx, id, userID)
	if err != nil {
		return nil, err
	}

	category.Name = name
	category.Icon = icon

	if err := s.categoryRepo.Update(ctx, category); err != nil {
		return nil, err
	}
	return category, nil
}

func (s *categoryService) DeleteCategory(ctx context.Context, id, userID uuid.UUID) error {
	return s.categoryRepo.Delete(ctx, id, userID)
}
