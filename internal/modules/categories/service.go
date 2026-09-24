package categories

import (
	"context"
	"fmt"
	"strings"

	"walletx-be/internal/platform/logger"
	apperrors "walletx-be/internal/shared/errors"

	"github.com/google/uuid"
)

// CategoryService defines the business logic interface for categories.
type CategoryService interface {
	Create(ctx context.Context, userID uuid.UUID, input CreateCategoryRequest) (*Category, error)
	List(ctx context.Context, userID uuid.UUID, categoryType *string) ([]Category, error)
	Update(ctx context.Context, userID, categoryID uuid.UUID, input UpdateCategoryRequest) (*Category, error)
	Delete(ctx context.Context, userID, categoryID uuid.UUID) error
}

type categoryService struct {
	repo   CategoryRepository
	logger logger.Logger
}

func NewCategoryService(repo CategoryRepository, appLogger logger.Logger) CategoryService {
	return &categoryService{repo: repo, logger: appLogger}
}

// Create validates the name and type before persisting a new category.
func (s *categoryService) Create(ctx context.Context, userID uuid.UUID, input CreateCategoryRequest) (*Category, error) {
	name, err := validateCategoryName(input.Name)
	if err != nil {
		return nil, err
	}

	categoryType, err := normalizeCategoryType(input.Type)
	if err != nil {
		return nil, err
	}

	return s.repo.Create(ctx, userID, name, categoryType)
}

// List returns all categories for the user, optionally filtered by type.
// A nil categoryType returns both expense and income categories.
func (s *categoryService) List(ctx context.Context, userID uuid.UUID, categoryType *string) ([]Category, error) {
	if categoryType != nil {
		normalized, err := normalizeCategoryType(*categoryType)
		if err != nil {
			return nil, err
		}
		return s.repo.ListByUser(ctx, userID, &normalized)
	}
	return s.repo.ListByUser(ctx, userID, nil)
}

// Update validates the new name and type, then delegates to the repository.
// Changing the type is allowed as long as the (userID, name, newType)
// combination is not already in use — enforced by the unique constraint.
func (s *categoryService) Update(ctx context.Context, userID, categoryID uuid.UUID, input UpdateCategoryRequest) (*Category, error) {
	name, err := validateCategoryName(input.Name)
	if err != nil {
		return nil, err
	}

	categoryType, err := normalizeCategoryType(input.Type)
	if err != nil {
		return nil, err
	}

	return s.repo.Update(ctx, userID, categoryID, name, categoryType)
}

// Delete removes a category by ID, enforcing ownership via userID.
func (s *categoryService) Delete(ctx context.Context, userID, categoryID uuid.UUID) error {
	return s.repo.Delete(ctx, userID, categoryID)
}

// validateCategoryName trims and validates a category name.
func validateCategoryName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", fmt.Errorf("%w: category name must not be empty", apperrors.ErrInvalidInput)
	}
	if len(name) > maxCategoryNameLength {
		return "", fmt.Errorf("%w: category name must not exceed %d characters", apperrors.ErrInvalidInput, maxCategoryNameLength)
	}
	return name, nil
}

// normalizeCategoryType trims, lowercases, and validates a category type.
// Only "expense" and "income" are accepted.
func normalizeCategoryType(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case CategoryTypeExpense, CategoryTypeIncome:
		return value, nil
	default:
		return "", fmt.Errorf("%w: category type must be 'expense' or 'income'", apperrors.ErrInvalidInput)
	}
}
