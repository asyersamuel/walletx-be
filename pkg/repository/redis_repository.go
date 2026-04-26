package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type TokenBlacklistRepository interface {
	BlacklistToken(ctx context.Context, jti string, ttl time.Duration) error
	IsTokenBlacklisted(ctx context.Context, jti string) (bool, error)
}

type tokenBlacklistRepository struct {
	client *redis.Client
}

func NewTokenBlacklistRepository(client *redis.Client) TokenBlacklistRepository {
	return &tokenBlacklistRepository{client: client}
}

func (r *tokenBlacklistRepository) BlacklistToken(ctx context.Context, jti string, ttl time.Duration) error {
	key := fmt.Sprintf("blacklist:%s", jti)
	return r.client.SetEx(ctx, key, "1", ttl).Err()
}

func (r *tokenBlacklistRepository) IsTokenBlacklisted(ctx context.Context, jti string) (bool, error) {
	key := fmt.Sprintf("blacklist:%s", jti)
	val, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return val == "1", nil
}

type noOpBlacklistRepository struct{}

func NewNoOpBlacklistRepository() TokenBlacklistRepository {
	return &noOpBlacklistRepository{}
}

func (r *noOpBlacklistRepository) BlacklistToken(ctx context.Context, jti string, ttl time.Duration) error {
	return nil
}

func (r *noOpBlacklistRepository) IsTokenBlacklisted(ctx context.Context, jti string) (bool, error) {
	return false, nil
}
