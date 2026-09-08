package auth

import (
	"context"
	"fmt"
	"time"

	"walletx-be/internal/middleware"
	database "walletx-be/internal/platform/database"
	sqlc "walletx-be/internal/platform/database/sqlc"
	"walletx-be/internal/platform/logger"
	apperrors "walletx-be/internal/shared/errors"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// UserRepository is the persisted data access for User aggregates.
type UserRepository interface {
	Create(ctx context.Context, user *User) error
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)
	FindByEmail(ctx context.Context, email string) (*User, error)
	FindByGoogleID(ctx context.Context, googleID string) (*User, error)
	FindByTelegramChatID(ctx context.Context, chatID string) (*User, error)
	Update(ctx context.Context, user *User) error
}

type userRepository struct {
	queries *sqlc.Queries
	logger  logger.Logger
}

func NewUserRepository(queries *sqlc.Queries, logger logger.Logger) UserRepository {
	return &userRepository{queries: queries, logger: logger}
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
			return apperrors.ErrDuplicate
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
		ID:             database.UUIDParam(user.ID),
		GoogleID:       user.GoogleID,
		Email:          user.Email,
		Name:           user.Name,
		Picture:        &picture,
		TelegramChatID: user.TelegramChatID,
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

func (r *userRepository) FindByTelegramChatID(ctx context.Context, chatID string) (*User, error) {
	row, err := r.queries.GetUserByTelegramChatID(ctx, &chatID)
	if err != nil {
		if database.IsNoRows(err) {
			return nil, apperrors.ErrNotFound
		}
		return nil, err
	}
	user := userFromRow(row)
	return &user, nil
}

func userFromRow(row sqlc.User) User {
	picture := ""
	if row.Picture != nil {
		picture = *row.Picture
	}

	return User{
		ID:             database.UUIDValue(row.ID),
		GoogleID:       row.GoogleID,
		Email:          row.Email,
		Name:           row.Name,
		Picture:        picture,
		TelegramChatID: row.TelegramChatID,
		CreatedAt:      database.TimeValue(row.CreatedAt),
		UpdatedAt:      database.TimeValue(row.UpdatedAt),
		DeletedAt:      database.TimePtr(row.DeletedAt),
	}
}

// ─── Token blacklist ─────────────────────────────────────────────────────────

type tokenBlacklistRepository struct {
	client *redis.Client
	logger logger.Logger
}

// NewTokenBlacklistRepository returns a Redis-backed token blacklist store used
// by both the auth service (on logout) and the JWT validator (on each request).
func NewTokenBlacklistRepository(client *redis.Client, logger logger.Logger) middleware.TokenBlacklistRepository {
	return &tokenBlacklistRepository{client: client, logger: logger}
}

func (r *tokenBlacklistRepository) BlacklistToken(ctx context.Context, jti string, ttl time.Duration) error {
	key := fmt.Sprintf("blacklist:%s", jti)
	if err := r.client.SetEx(ctx, key, "1", ttl).Err(); err != nil {
		r.logger.WithFields(map[string]interface{}{"jti": jti, "error": err}).Error("Failed to blacklist token in Redis")
		return err
	}

	r.logger.WithFields(map[string]interface{}{"jti": jti, "ttl": ttl}).Info("Token successfully blacklisted in Redis")
	return nil
}

func (r *tokenBlacklistRepository) IsTokenBlacklisted(ctx context.Context, jti string) (bool, error) {
	key := fmt.Sprintf("blacklist:%s", jti)
	val, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		r.logger.WithFields(map[string]interface{}{"jti": jti, "error": err}).Warn("Failed to check token blacklist status")
		return false, err
	}
	return val == "1", nil
}

type noOpBlacklistRepository struct{}

// NewNoOpBlacklistRepository returns a no-op blacklist store for when Redis is
// unavailable (reduces coupling — auth is still functional without blacklisting).
func NewNoOpBlacklistRepository() middleware.TokenBlacklistRepository {
	return &noOpBlacklistRepository{}
}

func (r *noOpBlacklistRepository) BlacklistToken(ctx context.Context, jti string, ttl time.Duration) error {
	return nil
}

func (r *noOpBlacklistRepository) IsTokenBlacklisted(ctx context.Context, jti string) (bool, error) {
	return false, nil
}
