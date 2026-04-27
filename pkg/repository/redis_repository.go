package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
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
	err := r.client.SetEx(ctx, key, "1", ttl).Err()
	if err != nil {
		logrus.WithFields(logrus.Fields{"jti": jti, "error": err}).Error("Failed to blacklist token in Redis")
		return err
	}

	logrus.WithFields(logrus.Fields{"jti": jti, "ttl": ttl}).Info("Token successfully blacklisted in Redis")
	return nil
}

func (r *tokenBlacklistRepository) IsTokenBlacklisted(ctx context.Context, jti string) (bool, error) {
	key := fmt.Sprintf("blacklist:%s", jti)
	val, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		logrus.WithFields(logrus.Fields{"jti": jti, "error": err}).Warn("Failed to check token blacklist status")
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

// CacheRepository defines interface for generic caching operations
type CacheRepository interface {
	SetCache(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	GetCache(ctx context.Context, key string, dest interface{}) error
	DeleteCache(ctx context.Context, key string) error
}

type cacheRepository struct {
	client *redis.Client
}

func NewCacheRepository(client *redis.Client) CacheRepository {
	return &cacheRepository{client: client}
}

func (r *cacheRepository) SetCache(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	if r.client == nil {
		return nil // Redis not available, skip caching
	}

	data, err := json.Marshal(value)
	if err != nil {
		logrus.WithError(err).Error("Failed to marshal cache value")
		return fmt.Errorf("failed to marshal cache value: %w", err)
	}

	if err := r.client.SetEx(ctx, key, string(data), ttl).Err(); err != nil {
		logrus.WithFields(logrus.Fields{"key": key, "error": err}).Error("Failed to set cache in Redis")
		return err
	}

	logrus.WithFields(logrus.Fields{"key": key, "ttl": ttl}).Info("Successfully set data to Redis cache")
	return nil
}

func (r *cacheRepository) GetCache(ctx context.Context, key string, dest interface{}) error {
	if r.client == nil {
		return fmt.Errorf("redis not available")
	}

	val, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		logrus.WithField("key", key).Info("Redis cache miss")
		return fmt.Errorf("cache miss")
	}
	if err != nil {
		logrus.WithFields(logrus.Fields{"key": key, "error": err}).Warn("Failed to get cache from Redis")
		return err
	}

	if err := json.Unmarshal([]byte(val), dest); err != nil {
		logrus.WithError(err).Error("Failed to unmarshal cache data")
		return err
	}

	logrus.WithField("key", key).Info("Redis cache hit")
	return nil
}

func (r *cacheRepository) DeleteCache(ctx context.Context, key string) error {
	if r.client == nil {
		return nil // Redis not available, skip
	}

	if err := r.client.Del(ctx, key).Err(); err != nil {
		logrus.WithFields(logrus.Fields{"key": key, "error": err}).Error("Failed to invalidate cache from Redis")
		return err
	}

	logrus.WithField("key", key).Info("Redis cache invalidated successfully")
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
