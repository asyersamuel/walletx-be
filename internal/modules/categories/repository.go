package categories

import (
	"context"

	database "walletx-be/internal/platform/database"
	sqlc "walletx-be/internal/platform/database/sqlc"
	"walletx-be/internal/platform/logger"
	apperrors "walletx-be/internal/shared/errors"

	"github.com/google/uuid"
)

// CategoryRepository defines the persistence interface for categories.
// categoryType is always the normalized string ("expense" or "income").
// ListByUser accepts a *string filter: nil means return all types.
type CategoryRepository interface {
	Create(ctx context.Context, userID uuid.UUID, name, categoryType string) (*Category, error)
	ListByUser(ctx context.Context, userID uuid.UUID, categoryType *string) ([]Category, error)
	Update(ctx context.Context, userID, categoryID uuid.UUID, name, categoryType string) (*Category, error)
	Delete(ctx context.Context, userID, categoryID uuid.UUID) error
}

type categoryRepository struct {
	queries *sqlc.Queries
	logger  logger.Logger
}

func NewCategoryRepository(queries *sqlc.Queries, appLogger logger.Logger) CategoryRepository {
	return &categoryRepository{queries: queries, logger: appLogger}
}

func (r *categoryRepository) Create(ctx context.Context, userID uuid.UUID, name, categoryType string) (*Category, error) {
	row, err := r.queries.CreateCategory(ctx, sqlc.CreateCategoryParams{
		UserID: database.UUIDParam(userID),
		Name:   name,
		Type:   categoryType,
	})
	if err != nil {
		if database.IsUniqueViolation(err) {
			return nil, apperrors.ErrConflict
		}
		return nil, err
	}
	category := categoryFromRow(row)
	return &category, nil
}

func (r *categoryRepository) ListByUser(ctx context.Context, userID uuid.UUID, categoryType *string) ([]Category, error) {
	rows, err := r.queries.ListCategoriesByUser(ctx, sqlc.ListCategoriesByUserParams{
		UserID:       database.UUIDParam(userID),
		CategoryType: categoryType,
	})
	if err != nil {
		return nil, err
	}
	result := make([]Category, len(rows))
	for i, row := range rows {
		result[i] = categoryFromRow(row)
	}
	return result, nil
}

func (r *categoryRepository) Update(ctx context.Context, userID, categoryID uuid.UUID, name, categoryType string) (*Category, error) {
	row, err := r.queries.UpdateCategory(ctx, sqlc.UpdateCategoryParams{
		ID:     database.UUIDParam(categoryID),
		UserID: database.UUIDParam(userID),
		Name:   name,
		Type:   categoryType,
	})
	if err != nil {
		if database.IsNoRows(err) {
			return nil, apperrors.ErrNotFound
		}
		if database.IsUniqueViolation(err) {
			return nil, apperrors.ErrConflict
		}
		return nil, err
	}
	category := categoryFromRow(row)
	return &category, nil
}

func (r *categoryRepository) Delete(ctx context.Context, userID, categoryID uuid.UUID) error {
	rows, err := r.queries.DeleteCategory(ctx, sqlc.DeleteCategoryParams{
		ID:     database.UUIDParam(categoryID),
		UserID: database.UUIDParam(userID),
	})
	if err != nil {
		return err
	}
	if rows == 0 {
		return apperrors.ErrNotFound
	}
	return nil
}

// categoryFromRow maps a SQLC-generated Category row to the domain model.
func categoryFromRow(row sqlc.Category) Category {
	return Category{
		ID:        database.UUIDValue(row.ID),
		UserID:    database.UUIDValue(row.UserID),
		Name:      row.Name,
		Type:      row.Type,
		CreatedAt: database.TimeValue(row.CreatedAt),
		UpdatedAt: database.TimeValue(row.UpdatedAt),
	}
}
