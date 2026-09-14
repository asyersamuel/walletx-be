package users

import (
	"context"

	database "walletx-be/internal/platform/database"
	sqlc "walletx-be/internal/platform/database/sqlc"
	"walletx-be/internal/platform/logger"
	apperrors "walletx-be/internal/shared/errors"

	"github.com/google/uuid"
)

// UserRepository is the persisted data access for user profile operations.
type UserRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)
	UpdateProfile(ctx context.Context, id uuid.UUID, name string, picture *string) (*User, error)
	SoftDelete(ctx context.Context, id uuid.UUID) error
}

type userRepository struct {
	queries *sqlc.Queries
	logger  logger.Logger
}

func NewUserRepository(queries *sqlc.Queries, logger logger.Logger) UserRepository {
	return &userRepository{queries: queries, logger: logger}
}

func (r *userRepository) GetByID(ctx context.Context, id uuid.UUID) (*User, error) {
	row, err := r.queries.GetUserByID(ctx, database.UUIDParam(id))
	if err != nil {
		if database.IsNoRows(err) {
			return nil, apperrors.ErrNotFound
		}
		return nil, err
	}
	user := userFromRow(row)
	return &user, nil
}

func (r *userRepository) UpdateProfile(ctx context.Context, id uuid.UUID, name string, picture *string) (*User, error) {
	row, err := r.queries.UpdateUserProfile(ctx, sqlc.UpdateUserProfileParams{
		ID:      database.UUIDParam(id),
		Name:    name,
		Picture: picture,
	})
	if err != nil {
		if database.IsNoRows(err) {
			return nil, apperrors.ErrNotFound
		}
		return nil, err
	}
	user := userFromRow(row)
	return &user, nil
}

// SoftDelete marks the account as deleted. Deleting an already-deleted or
// unknown account affects zero rows and is treated as an idempotent success.
func (r *userRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	_, err := r.queries.SoftDeleteUser(ctx, database.UUIDParam(id))
	if err != nil {
		return err
	}
	return nil
}

func userFromRow(row sqlc.User) User {
	picture := ""
	if row.Picture != nil {
		picture = *row.Picture
	}

	return User{
		ID:        database.UUIDValue(row.ID),
		GoogleID:  row.GoogleID,
		Email:     row.Email,
		Name:      row.Name,
		Picture:   picture,
		CreatedAt: database.TimeValue(row.CreatedAt),
		UpdatedAt: database.TimeValue(row.UpdatedAt),
		DeletedAt: database.TimePtr(row.DeletedAt),
	}
}
