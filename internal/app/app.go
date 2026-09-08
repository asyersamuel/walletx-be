package app

import (
	"context"
	"fmt"
	"net/http"

	"walletx-be/configs"
	"walletx-be/internal/middleware"
	"walletx-be/internal/modules/auth"
	"walletx-be/internal/platform/cache"
	"walletx-be/internal/platform/database"
	platformlogger "walletx-be/internal/platform/logger"

	"github.com/redis/go-redis/v9"
)

// App is the assembled application instance returned to the entry point.
type App struct {
	Handler      http.Handler
	Config       *configs.Config
	Cleanup      func()
	ShutdownFunc func(ctx context.Context) error
}

// Run initialises the platform, wires every module, and returns a ready App.
func Run(cfg *configs.Config) (*App, error) {
	db, err := database.Init(cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	logger := platformlogger.NewLogger()

	var redisClient *redis.Client
	rdb, err := cache.InitRedis(cfg.Redis)
	if err != nil {
		fmt.Printf("Redis not connected, token blacklisting disabled: %v\n", err)
	} else {
		redisClient = rdb
	}

	var blacklistRepo middleware.TokenBlacklistRepository
	if redisClient != nil {
		blacklistRepo = auth.NewTokenBlacklistRepository(redisClient, logger)
	} else {
		blacklistRepo = auth.NewNoOpBlacklistRepository()
	}

	modules, err := buildModules(db, redisClient, logger, cfg, blacklistRepo)
	if err != nil {
		return nil, err
	}

	validator := middleware.NewJWTValidator(cfg.JWT.Secret, blacklistRepo)
	router := SetupRouter(modules, cfg, validator)

	cleanup := func() {
		if redisClient != nil {
			if err := redisClient.Close(); err != nil {
				fmt.Printf("Failed to close Redis connection: %v\n", err)
			}
		}
	}

	shutdownFunc := func(ctx context.Context) error {
		cleanup()
		sqlDB, err := db.DB()
		if err != nil {
			return fmt.Errorf("failed to get database instance: %w", err)
		}
		return sqlDB.Close()
	}

	return &App{
		Handler:      router,
		Config:       cfg,
		Cleanup:      cleanup,
		ShutdownFunc: shutdownFunc,
	}, nil
}
