package infrastructure

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	domain "walletx-be/internal/modules/transactions/contracts"
	"walletx-be/internal/platform/database"
	db "walletx-be/internal/platform/database/sqlc"
	apperrors "walletx-be/internal/shared/errors"
)

type CategoryQueries interface {
	ListCategoriesByUser(context.Context, db.ListCategoriesByUserParams) ([]db.Category, error)
}

type CategoryResolver struct {
	queries CategoryQueries
}

var _ domain.CategoryResolver = (*CategoryResolver)(nil)

func NewCategoryResolver(queries CategoryQueries) *CategoryResolver {
	return &CategoryResolver{queries: queries}
}

func (r *CategoryResolver) ResolveOwnedCategory(ctx context.Context, userID uuid.UUID, name string) (uuid.UUID, string, error) {
	name = strings.TrimSpace(name)
	if ctx == nil || userID == uuid.Nil || name == "" {
		return uuid.Nil, "", fmt.Errorf("invalid category lookup: %w", apperrors.ErrInvalidInput)
	}
	if err := ctx.Err(); err != nil {
		return uuid.Nil, "", err
	}
	if r == nil || r.queries == nil {
		return uuid.Nil, "", fmt.Errorf("category queries are not configured: %w", apperrors.ErrInternal)
	}
	rows, err := r.queries.ListCategoriesByUser(ctx, db.ListCategoriesByUserParams{UserID: database.UUIDParam(userID)})
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("resolve owned category: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return uuid.Nil, "", err
	}
	var match *db.Category
	for i := range rows {
		row := &rows[i]
		// The SQL query filters by owner. Retain that check at the adapter
		// boundary so an unexpected row can never become a foreign category.
		if database.UUIDValue(row.UserID) != userID || !strings.EqualFold(row.Name, name) {
			continue
		}
		if database.UUIDValue(row.ID) == uuid.Nil {
			return uuid.Nil, "", fmt.Errorf("invalid category row: %w", apperrors.ErrInternal)
		}
		if match != nil {
			return uuid.Nil, "", fmt.Errorf("ambiguous category name: %w", apperrors.ErrInvalidInput)
		}
		match = row
	}
	if match == nil {
		return uuid.Nil, "", apperrors.ErrNotFound
	}
	return database.UUIDValue(match.ID), match.Name, nil
}
