package auth

import (
	"context"

	database "walletx-be/internal/platform/database"
	sqlc "walletx-be/internal/platform/database/sqlc"
	"walletx-be/internal/platform/logger"
	apperrors "walletx-be/internal/shared/errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UserRepository is the persisted data access for User aggregates.
type UserRepository interface {
	Create(ctx context.Context, user *User) error
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)
	FindByEmail(ctx context.Context, email string) (*User, error)
	FindByGoogleID(ctx context.Context, googleID string) (*User, error)
	FindByGoogleIDAny(ctx context.Context, googleID string) (*User, error)
	Update(ctx context.Context, user *User) error
	UpdateGoogleProfile(ctx context.Context, userID uuid.UUID, name, picture string) (*User, error)
	CreateWithDefaultCategories(ctx context.Context, user *User) error
}

type userRepository struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
	logger  logger.Logger
}

func NewUserRepository(pool *pgxpool.Pool, queries *sqlc.Queries, appLogger logger.Logger) UserRepository {
	return &userRepository{pool: pool, queries: queries, logger: appLogger}
}

func (r *userRepository) Create(ctx context.Context, user *User) error {
	picture := user.Picture
	row, err := r.queries.CreateUser(ctx, sqlc.CreateUserParams{
		GoogleID: user.GoogleID,
		Email:    user.Email,
		Name:     user.Name,
		Picture:  &picture,
	})
	if err != nil {
		if database.IsUniqueViolation(err) {
			return apperrors.ErrConflict
		}
		return err
	}
	*user = userFromRow(row)
	return nil
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

func (r *userRepository) FindByEmail(ctx context.Context, email string) (*User, error) {
	row, err := r.queries.GetUserByEmail(ctx, email)
	if err != nil {
		if database.IsNoRows(err) {
			return nil, apperrors.ErrNotFound
		}
		return nil, err
	}
	user := userFromRow(row)
	return &user, nil
}

func (r *userRepository) Update(ctx context.Context, user *User) error {
	picture := user.Picture
	row, err := r.queries.UpdateUser(ctx, sqlc.UpdateUserParams{
		ID:       database.UUIDParam(user.ID),
		GoogleID: user.GoogleID,
		Email:    user.Email,
		Name:     user.Name,
		Picture:  &picture,
	})
	if err != nil {
		if database.IsNoRows(err) {
			return apperrors.ErrNotFound
		}
		if database.IsUniqueViolation(err) {
			return apperrors.ErrConflict
		}
		return err
	}
	*user = userFromRow(row)
	return nil
}

func (r *userRepository) FindByGoogleID(ctx context.Context, googleID string) (*User, error) {
	row, err := r.queries.GetUserByGoogleID(ctx, googleID)
	if err != nil {
		if database.IsNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	user := userFromRow(row)
	return &user, nil
}

func (r *userRepository) FindByGoogleIDAny(ctx context.Context, googleID string) (*User, error) {
	row, err := r.queries.GetUserByGoogleIDAny(ctx, googleID)
	if err != nil {
		if database.IsNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	user := userFromRow(row)
	return &user, nil
}

func (r *userRepository) UpdateGoogleProfile(ctx context.Context, userID uuid.UUID, name, picture string) (*User, error) {
	row, err := r.queries.UpdateUserGoogleProfile(ctx, sqlc.UpdateUserGoogleProfileParams{
		ID:      database.UUIDParam(userID),
		Name:    name,
		Picture: &picture,
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

// CreateWithDefaultCategories inserts a new user inside a database transaction.
// Default categories will be inserted in the same transaction once the categories
// schema is available (currently BLOCKED pending product decision).
func (r *userRepository) CreateWithDefaultCategories(ctx context.Context, user *User) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	qtx := r.queries.WithTx(tx)

	picture := user.Picture
	row, err := qtx.CreateUser(ctx, sqlc.CreateUserParams{
		GoogleID: user.GoogleID,
		Email:    user.Email,
		Name:     user.Name,
		Picture:  &picture,
	})
	if err != nil {
		if database.IsUniqueViolation(err) {
			return apperrors.ErrConflict
		}
		return err
	}

	// TODO: insert default categories here once the categories table exists.
	// The transaction is already open; add qtx.CreateCategory calls before Commit.

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	*user = userFromRow(row)
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
