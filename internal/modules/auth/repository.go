package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"walletx-be/internal/middleware"
	"walletx-be/internal/platform/logger"
	"walletx-be/internal/shared/errors"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
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
	db     *gorm.DB
	logger logger.Logger
}

func NewUserRepository(db *gorm.DB, logger logger.Logger) UserRepository {
	return &userRepository{
		db:     db,
		logger: logger,
	}
}

func (r *userRepository) Create(ctx context.Context, user *User) error {
	err := r.db.WithContext(ctx).Create(user).Error
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return apperrors.ErrDuplicate
	}
	return err
}

func (r *userRepository) GetByID(ctx context.Context, id uuid.UUID) (*User, error) {
	var user User
	if err := r.db.WithContext(ctx).First(&user, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrNotFound
		}
		return nil, err
	}
	return &user, nil
}

func (r *userRepository) FindByEmail(ctx context.Context, email string) (*User, error) {
	var user User
	if err := r.db.WithContext(ctx).Where("email = ?", email).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrNotFound
		}
		return nil, err
	}
	return &user, nil
}

func (r *userRepository) Update(ctx context.Context, user *User) error {
	return r.db.WithContext(ctx).Save(user).Error
}

func (r *userRepository) FindByGoogleID(ctx context.Context, googleID string) (*User, error) {
	var user User
	err := r.db.WithContext(ctx).Where("google_id = ?", googleID).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

func (r *userRepository) FindByTelegramChatID(ctx context.Context, chatID string) (*User, error) {
	var user User
	err := r.db.WithContext(ctx).Where("telegram_chat_id = ?", chatID).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrNotFound
		}
		return nil, err
	}
	return &user, nil
}

// ─── Token blacklist ─────────────────────────────────────────────────────────

type tokenBlacklistRepository struct {
	client *redis.Client
	logger logger.Logger
}

// NewTokenBlacklistRepository returns a Redis-backed token blacklist store used
// by both the auth service (on logout) and the JWT validator (on each request).
func NewTokenBlacklistRepository(client *redis.Client, logger logger.Logger) middleware.TokenBlacklistRepository {
	return &tokenBlacklistRepository{
		client: client,
		logger: logger,
	}
}

func (r *tokenBlacklistRepository) BlacklistToken(ctx context.Context, jti string, ttl time.Duration) error {
	key := fmt.Sprintf("blacklist:%s", jti)
	err := r.client.SetEx(ctx, key, "1", ttl).Err()
	if err != nil {
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
