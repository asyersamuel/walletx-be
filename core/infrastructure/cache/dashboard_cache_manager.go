package cache

import (
	"context"
	"fmt"

	"walletx-be/core/ports"

	"github.com/redis/go-redis/v9"
)

type redisDashboardCacheManager struct {
	client *redis.Client
	logger ports.Logger
}

func NewRedisDashboardCacheManager(client *redis.Client, logger ports.Logger) ports.DashboardCacheManager {
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

type noOpDashboardCacheManager struct{}

func NewNoOpDashboardCacheManager() ports.DashboardCacheManager {
	return &noOpDashboardCacheManager{}
}

func (m *noOpDashboardCacheManager) InvalidateUserCache(ctx context.Context, userID string) error {
	return nil
}
