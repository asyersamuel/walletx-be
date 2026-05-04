package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"walletx-be/internal/ports"

	"github.com/redis/go-redis/v9"
)

type TokenBlacklistRepository interface {
	BlacklistToken(ctx context.Context, jti string, ttl time.Duration) error
	IsTokenBlacklisted(ctx context.Context, jti string) (bool, error)
}

type tokenBlacklistRepository struct {
	client *redis.Client
	logger ports.Logger
}

func NewTokenBlacklistRepository(client *redis.Client, logger ports.Logger) TokenBlacklistRepository {
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

func NewNoOpBlacklistRepository() TokenBlacklistRepository {
	return &noOpBlacklistRepository{}
}

func (r *noOpBlacklistRepository) BlacklistToken(ctx context.Context, jti string, ttl time.Duration) error {
	return nil
}

func (r *noOpBlacklistRepository) IsTokenBlacklisted(ctx context.Context, jti string) (bool, error) {
	return false, nil
}

type CacheRepository interface {
	SetCache(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	GetCache(ctx context.Context, key string, dest interface{}) error
	DeleteCache(ctx context.Context, key string) error
}

type cacheRepository struct {
	client *redis.Client
	logger ports.Logger
}

func NewCacheRepository(client *redis.Client, logger ports.Logger) CacheRepository {
	return &cacheRepository{
		client: client,
		logger: logger,
	}
}

func (r *cacheRepository) SetCache(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	if r.client == nil {
		return nil
	}

	data, err := json.Marshal(value)
	if err != nil {
		r.logger.WithError(err).Error("Failed to marshal cache value")
		return fmt.Errorf("failed to marshal cache value: %w", err)
	}

	if err := r.client.SetEx(ctx, key, string(data), ttl).Err(); err != nil {
		r.logger.WithFields(map[string]interface{}{"key": key, "error": err}).Error("Failed to set cache in Redis")
		return err
	}

	r.logger.WithFields(map[string]interface{}{"key": key, "ttl": ttl}).Info("Successfully set data to Redis cache")
	return nil
}

func (r *cacheRepository) GetCache(ctx context.Context, key string, dest interface{}) error {
	if r.client == nil {
		return fmt.Errorf("redis not available")
	}

	val, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		r.logger.WithField("key", key).Info("Redis cache miss")
		return fmt.Errorf("cache miss")
	}
	if err != nil {
		r.logger.WithFields(map[string]interface{}{"key": key, "error": err}).Warn("Failed to get cache from Redis")
		return err
	}

	if err := json.Unmarshal([]byte(val), dest); err != nil {
		r.logger.WithError(err).Error("Failed to unmarshal cache data")
		return err
	}

	r.logger.WithField("key", key).Info("Redis cache hit")
	return nil
}

func (r *cacheRepository) DeleteCache(ctx context.Context, key string) error {
	if r.client == nil {
		return nil
	}

	if err := r.client.Del(ctx, key).Err(); err != nil {
		r.logger.WithFields(map[string]interface{}{"key": key, "error": err}).Error("Failed to invalidate cache from Redis")
		return err
	}

	r.logger.WithField("key", key).Info("Redis cache invalidated successfully")
	return nil
}

// NoOp cache repository for when Redis is unavailable
type noOpCacheRepository struct{}

func NewNoOpCacheRepository() CacheRepository {
	return &noOpCacheRepository{}
}

func (r *noOpCacheRepository) SetCache(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	return nil
}

func (r *noOpCacheRepository) GetCache(ctx context.Context, key string, dest interface{}) error {
	return fmt.Errorf("cache miss")
}

func (r *noOpCacheRepository) DeleteCache(ctx context.Context, key string) error {
	return nil
}
