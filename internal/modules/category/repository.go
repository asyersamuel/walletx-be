package category

import (
	"context"

	database "walletx-be/internal/platform/database"
	sqlc "walletx-be/internal/platform/database/sqlc"
	"walletx-be/internal/platform/logger"
	apperrors "walletx-be/internal/shared/errors"

	"github.com/google/uuid"
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
	queries *sqlc.Queries
	logger  logger.Logger
}

func NewRepository(queries *sqlc.Queries, logger logger.Logger) Repository {
	return &repository{queries: queries, logger: logger}
}

func (r *repository) Create(ctx context.Context, category *Category) error {
	row, err := r.queries.CreateCategory(ctx, sqlc.CreateCategoryParams{
		UserID: database.UUIDParam(category.UserID),
		Name:   category.Name,
		Icon:   category.Icon,
	})
	if err != nil {
		if database.IsUniqueViolation(err) {
			return apperrors.ErrDuplicate
		}
		return err
	}
	*category = categoryFromRow(row)
	return nil
}

func (r *repository) List(ctx context.Context, userID uuid.UUID) ([]Category, error) {
	rows, err := r.queries.ListCategories(ctx, database.UUIDParam(userID))
	if err != nil {
		return nil, err
	}

	items := make([]Category, 0, len(rows))
	for _, row := range rows {
		items = append(items, categoryFromRow(row))
	}
	return items, nil
}

func (r *repository) GetByID(ctx context.Context, id, userID uuid.UUID) (*Category, error) {
	row, err := r.queries.GetCategoryByID(ctx, sqlc.GetCategoryByIDParams{
		ID:     database.UUIDParam(id),
		UserID: database.UUIDParam(userID),
	})
	if err != nil {
		if database.IsNoRows(err) {
			return nil, apperrors.ErrNotFound
		}
		return nil, err
	}
	category := categoryFromRow(row)
	return &category, nil
}

func (r *repository) Update(ctx context.Context, category *Category) error {
	row, err := r.queries.UpdateCategory(ctx, sqlc.UpdateCategoryParams{
		ID:     database.UUIDParam(category.ID),
		UserID: database.UUIDParam(category.UserID),
		Name:   category.Name,
		Icon:   category.Icon,
	})
	if err != nil {
		if database.IsNoRows(err) {
			return apperrors.ErrNotFound
		}
		if database.IsUniqueViolation(err) {
			return apperrors.ErrDuplicate
		}
		return err
	}
	*category = categoryFromRow(row)
	return nil
}

func (r *repository) Delete(ctx context.Context, id, userID uuid.UUID) error {
	rows, err := r.queries.SoftDeleteCategory(ctx, sqlc.SoftDeleteCategoryParams{
		ID:     database.UUIDParam(id),
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

func (r *repository) GetByName(ctx context.Context, name string, userID uuid.UUID) (*Category, error) {
	row, err := r.queries.GetCategoryByName(ctx, sqlc.GetCategoryByNameParams{
		Name:   name,
		UserID: database.UUIDParam(userID),
	})
	if err != nil {
		if database.IsNoRows(err) {
			return nil, apperrors.ErrNotFound
		}
		return nil, err
	}
	category := categoryFromRow(row)
	return &category, nil
}

func categoryFromRow(row sqlc.Category) Category {
	return Category{
		ID:        database.UUIDValue(row.ID),
		UserID:    database.UUIDValue(row.UserID),
		Name:      row.Name,
		Icon:      row.Icon,
		CreatedAt: database.TimeValue(row.CreatedAt),
		UpdatedAt: database.TimeValue(row.UpdatedAt),
		DeletedAt: database.TimePtr(row.DeletedAt),
	}
}
