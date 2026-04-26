package cache

import (
	"context"
	"fmt"
	"time"

	"walletx-be/configs"

	"github.com/redis/go-redis/v9"
)

func InitRedis(cfg configs.RedisConfig) (*redis.Client, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("REDIS_URL is not set")
	}

	opt, err := redis.ParseURL(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Redis URL: %w", err)
	}

	// Timeout settings for Vercel Serverless environment
	opt.DialTimeout = 5 * time.Second
	opt.ReadTimeout = 3 * time.Second

	client := redis.NewClient(opt)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return client, nil
}
