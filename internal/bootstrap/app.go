package bootstrap

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"walletx-be/configs"
	dashboardcache "walletx-be/internal/infrastructure/cache"
	"walletx-be/internal/infrastructure/logger"
	emailparser "walletx-be/internal/infrastructure/parser"
	"walletx-be/internal/ports"
	repoCache "walletx-be/pkg/cache"
	"walletx-be/pkg/database"
	"walletx-be/pkg/handlers"
	"walletx-be/pkg/middleware"
	"walletx-be/pkg/repository"
	"walletx-be/pkg/router"
	"walletx-be/pkg/services"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

type App struct {
	Handler      http.Handler
	Cleanup      func()
	Config       *configs.Config
	ShutdownFunc func(ctx context.Context) error
}

func BuildApp(cfg *configs.Config) (*App, error) {
	db, err := database.Init(cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	redisClient, err := repoCache.InitRedis(cfg.Redis)
	if err != nil {
		fmt.Printf("Redis not connected, token blacklisting disabled: %v\n", err)
	}

	appLogger := logger.NewLogger()

	var dashboardCacheManager ports.DashboardCacheManager
	if redisClient != nil {
		dashboardCacheManager = dashboardcache.NewRedisDashboardCacheManager(redisClient, appLogger)
	} else {
		dashboardCacheManager = dashboardcache.NewNoOpDashboardCacheManager()
	}

	userRepo := repository.NewUserRepository(db, appLogger)
	transactionRepo := repository.NewTransactionRepository(db, appLogger)
	categoryRepo := repository.NewCategoryRepository(db, appLogger)
	budgetRepo := repository.NewBudgetRepository(db, appLogger)
	recurringRepo := repository.NewRecurringRepository(db, appLogger)
	summaryRepo := repository.NewSummaryRepository(db, appLogger)
	healthRepo := repository.NewHealthRepository(db)

	var blacklistRepo middleware.TokenBlacklistRepository
	var cacheRepo repository.CacheRepository
	if redisClient != nil {
		blacklistRepo = middlewareTokenBlacklistAdapter{repository.NewTokenBlacklistRepository(redisClient, appLogger)}
		cacheRepo = repository.NewCacheRepository(redisClient, appLogger)
	} else {
		blacklistRepo = middlewareNoOpBlacklistAdapter{}
		cacheRepo = repository.NewNoOpCacheRepository()
	}

	authService := services.NewAuthService(userRepo, cfg.OAuth.ClientID, cfg.JWT.Secret, cfg.JWT.Expiration, blacklistRepo)
	emailParser := emailparser.NewGeminiEmailParser(appLogger, cfg.Gemini.APIKey)
	txProcessor := services.NewTransactionProcessor(userRepo, transactionRepo, categoryRepo, emailParser, dashboardCacheManager, appLogger)
	txService := services.NewTransactionService(userRepo, transactionRepo, categoryRepo, txProcessor, dashboardCacheManager, appLogger)
	categoryService := services.NewCategoryService(categoryRepo, appLogger)
	budgetService := services.NewBudgetService(budgetRepo, summaryRepo, appLogger)
	recurringService := services.NewRecurringService(recurringRepo, transactionRepo, dashboardCacheManager, appLogger)
	dashboardService := services.NewDashboardService(budgetRepo, summaryRepo, cacheRepo, appLogger)

	oauthConfig := &oauth2.Config{
		ClientID:     cfg.OAuth.ClientID,
		ClientSecret: cfg.OAuth.ClientSecret,
		RedirectURL:  cfg.OAuth.RedirectURL,
		Scopes:       []string{"https://www.googleapis.com/auth/userinfo.email", "https://www.googleapis.com/auth/userinfo.profile"},
		Endpoint:     google.Endpoint,
	}
	authHandler := handlers.NewAuthHandler(authService, oauthConfig)
	txHandler := handlers.NewTransactionHandler(txService)
	categoryHandler := handlers.NewCategoryHandler(categoryService)
	budgetHandler := handlers.NewBudgetHandler(budgetService)
	recurringHandler := handlers.NewRecurringHandler(recurringService)
	dashboardHandler := handlers.NewDashboardHandler(dashboardService)

	imapService := services.NewIMAPService(cfg.IMAP.Email, cfg.IMAP.Password, txProcessor, appLogger)
	cronHandler := handlers.NewCronHandler(healthRepo, imapService, recurringService)

	r := router.SetupRouter(
		txHandler,
		authHandler,
		categoryHandler,
		budgetHandler,
		recurringHandler,
		dashboardHandler,
		cronHandler,
		cfg.JWT.Secret,
		cfg.App.DevMode,
		cfg.Media.StorageType,
		cfg.Media.BaseURL,
		cfg.Media.UploadDir,
		blacklistRepo,
		cfg.Cron.Secret,
	)

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
		Handler:      r,
		Cleanup:      cleanup,
		Config:       cfg,
		ShutdownFunc: shutdownFunc,
	}, nil
}

func LoadConfig() *configs.Config {
	return configs.Load()
}

type middlewareTokenBlacklistAdapter struct {
	repo repository.TokenBlacklistRepository
}

func (a middlewareTokenBlacklistAdapter) BlacklistToken(ctx context.Context, jti string, ttl time.Duration) error {
	return a.repo.BlacklistToken(ctx, jti, ttl)
}

func (a middlewareTokenBlacklistAdapter) IsTokenBlacklisted(ctx context.Context, jti string) (bool, error) {
	return a.repo.IsTokenBlacklisted(ctx, jti)
}

type middlewareNoOpBlacklistAdapter struct{}

func (a middlewareNoOpBlacklistAdapter) BlacklistToken(ctx context.Context, jti string, ttl time.Duration) error {
	return nil
}

func (a middlewareNoOpBlacklistAdapter) IsTokenBlacklisted(ctx context.Context, jti string) (bool, error) {
	return false, nil
}
