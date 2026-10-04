package infrastructure

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"walletx-be/internal/platform/database"
	db "walletx-be/internal/platform/database/sqlc"
	apperrors "walletx-be/internal/shared/errors"
)

type categoryQueries struct {
	list func(context.Context, db.ListCategoriesByUserParams) ([]db.Category, error)
}

func (q categoryQueries) ListCategoriesByUser(ctx context.Context, params db.ListCategoriesByUserParams) ([]db.Category, error) {
	return q.list(ctx, params)
}

func TestCategoryResolver(t *testing.T) {
	owner, categoryID := uuid.New(), uuid.New()
	owned := db.Category{ID: database.UUIDParam(categoryID), UserID: database.UUIDParam(owner), Name: "Coffee", Type: "expense"}
	foreign := db.Category{ID: database.UUIDParam(uuid.New()), UserID: database.UUIDParam(uuid.New()), Name: "Coffee"}
	cause := errors.New("database unavailable")
	for _, tc := range []struct {
		name string
		rows []db.Category
		err  error
		want error
	}{
		{"owned category", []db.Category{owned}, nil, nil},
		{"foreign row is ignored", []db.Category{foreign, owned}, nil, nil},
		{"foreign category only", []db.Category{foreign}, nil, apperrors.ErrNotFound},
		{"no categories", nil, nil, apperrors.ErrNotFound},
		{"different category name", []db.Category{{ID: database.UUIDParam(categoryID), UserID: database.UUIDParam(owner), Name: "Salary"}}, nil, apperrors.ErrNotFound},
		{"ambiguous name", []db.Category{owned, owned}, nil, apperrors.ErrInvalidInput},
		{"missing category ID", []db.Category{{UserID: database.UUIDParam(owner), Name: "Coffee"}}, nil, apperrors.ErrInternal},
		{"database failure", nil, cause, cause},
		{"database cancellation", nil, context.Canceled, context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			queries := categoryQueries{list: func(ctx context.Context, params db.ListCategoriesByUserParams) ([]db.Category, error) {
				require.Equal(t, database.UUIDParam(owner), params.UserID)
				require.Nil(t, params.CategoryType)
				return tc.rows, tc.err
			}}
			id, name, err := NewCategoryResolver(queries).ResolveOwnedCategory(context.Background(), owner, " coffee ")
			if tc.want != nil {
				require.ErrorIs(t, err, tc.want)
				require.Equal(t, uuid.Nil, id)
				require.Empty(t, name)
			} else {
				require.NoError(t, err)
				require.Equal(t, categoryID, id)
				require.Equal(t, "Coffee", name)
			}
		})
	}
}

func TestCategoryResolverInvalidInput(t *testing.T) {
	for _, tc := range []struct {
		ctx  context.Context
		id   uuid.UUID
		name string
	}{
		{nil, uuid.New(), "Coffee"},
		{context.Background(), uuid.Nil, "Coffee"},
		{context.Background(), uuid.New(), " \n"},
	} {
		_, _, err := NewCategoryResolver(categoryQueries{}).ResolveOwnedCategory(tc.ctx, tc.id, tc.name)
		require.ErrorIs(t, err, apperrors.ErrInvalidInput)
	}
	_, _, err := NewCategoryResolver(nil).ResolveOwnedCategory(context.Background(), uuid.New(), "Coffee")
	require.ErrorIs(t, err, apperrors.ErrInternal)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = NewCategoryResolver(categoryQueries{}).ResolveOwnedCategory(ctx, uuid.New(), "Coffee")
	require.ErrorIs(t, err, context.Canceled)
}
