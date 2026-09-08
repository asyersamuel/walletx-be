package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"walletx-be/internal/platform/logger"

	"github.com/redis/go-redis/v9"
)

// CacheRepository is a generic Redis-backed key/value cache used by read-heavy
// modules such as dashboard. All methods degrade gracefully when Redis is nil.
type CacheRepository interface {
	SetCache(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	GetCache(ctx context.Context, key string, dest interface{}) error
	DeleteCache(ctx context.Context, key string) error
}

// DashboardCacheManager invalidates the `cache:dashboard:*` keys for a user
// whenever their financial data changes (new transaction, update, delete,
// recurring run). It is exposed as a capability to modules that mutate data.
type DashboardCacheManager interface {
	InvalidateUserCache(ctx context.Context, userID string) error
}

type cacheRepository struct {
	client *redis.Client
	logger logger.Logger
}

func NewCacheRepository(client *redis.Client, logger logger.Logger) CacheRepository {
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

// NewNoOpCacheRepository returns a no-op cache for when Redis is unavailable.
func NewNoOpCacheRepository() CacheRepository {
	return &noOpCacheRepository{}
}

type noOpCacheRepository struct{}

func (r *noOpCacheRepository) SetCache(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	return nil
}

func (r *noOpCacheRepository) GetCache(ctx context.Context, key string, dest interface{}) error {
	return fmt.Errorf("cache miss")
}

func (r *noOpCacheRepository) DeleteCache(ctx context.Context, key string) error {
	return nil
}

// ─── Dashboard cache manager ─────────────────────────────────────────────────

type redisDashboardCacheManager struct {
	client *redis.Client
	logger logger.Logger
}

func NewRedisDashboardCacheManager(client *redis.Client, logger logger.Logger) DashboardCacheManager {
	return &redisDashboardCacheManager{
		client: client,
		logger: logger,
	}
}

func (m *redisDashboardCacheManager) InvalidateUserCache(ctx context.Context, userID string) error {
	if m.client == nil {
		return nil
	}

	pattern := fmt.Sprintf("cache:dashboard:*:%s*", userID)
	cursor := uint64(0)
	const count = 100

	for {
		keys, nextCursor, err := m.client.Scan(ctx, cursor, pattern, count).Result()
		if err != nil {
			m.logger.WithError(err).Warn("Failed to scan cache keys for invalidation")
			break
		}

		if len(keys) > 0 {
			if err := m.client.Del(ctx, keys...).Err(); err != nil {
				m.logger.WithError(err).Warn("Failed to delete cache keys")
			} else {
				m.logger.WithField("deleted_count", len(keys)).Info("Dashboard cache invalidated")
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return nil
}

func NewNoOpDashboardCacheManager() DashboardCacheManager {
	return &noOpDashboardCacheManager{}
}

type noOpDashboardCacheManager struct{}

func (m *noOpDashboardCacheManager) InvalidateUserCache(ctx context.Context, userID string) error {
	return nil
}
