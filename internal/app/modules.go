package app

import (
	"fmt"

	"walletx-be/configs"
	"walletx-be/internal/middleware"
	"walletx-be/internal/modules/auth"
	"walletx-be/internal/modules/budget"
	"walletx-be/internal/modules/category"
	"walletx-be/internal/modules/cron"
	"walletx-be/internal/modules/dashboard"
	"walletx-be/internal/modules/recurring"
	"walletx-be/internal/modules/telegram"
	"walletx-be/internal/modules/transaction"
	"walletx-be/internal/platform/cache"
	"walletx-be/internal/platform/logger"
	mailpkg "walletx-be/internal/platform/mail"
	"walletx-be/internal/platform/parser"

	"github.com/redis/go-redis/v9"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"gorm.io/gorm"
)

// Modules aggregates every built business module and exposes the wired handlers
// the router uses to register routes.
type Modules struct {
	Auth        *auth.Module
	Transaction *transaction.Module
	Category    *category.Module
	Budget      *budget.Module
	Recurring   *recurring.Module
	Dashboard   *dashboard.Module
	Telegram    *telegram.Module
	Cron        *cron.Module
}

// buildModules constructs all modules in dependency order and wires cross-module
// capabilities through narrow interfaces.
func buildModules(
	db *gorm.DB,
	redisClient *redis.Client,
	logger logger.Logger,
	cfg *configs.Config,
	blacklistRepo middleware.TokenBlacklistRepository,
) (*Modules, error) {
	// Platform capabilities
	emailParser := parser.NewGeminiEmailParser(logger, cfg.Gemini.APIKey)
	emailSvc := mailpkg.NewEmailService(cfg.SMTP)

	var dashCacheManager cache.DashboardCacheManager
	if redisClient != nil {
		dashCacheManager = cache.NewRedisDashboardCacheManager(redisClient, logger)
	} else {
		dashCacheManager = cache.NewNoOpDashboardCacheManager()
	}

	var cacheRepo cache.CacheRepository
	if redisClient != nil {
		cacheRepo = cache.NewCacheRepository(redisClient, logger)
	} else {
		cacheRepo = cache.NewNoOpCacheRepository()
	}

	oauthConfig := &oauth2.Config{
		ClientID:     cfg.OAuth.ClientID,
		ClientSecret: cfg.OAuth.ClientSecret,
		RedirectURL:  cfg.OAuth.RedirectURL,
		Scopes:       []string{"https://www.googleapis.com/auth/userinfo.email", "https://www.googleapis.com/auth/userinfo.profile"},
		Endpoint:     google.Endpoint,
	}

	// Build leaf modules first.
	authMod := auth.NewModule(db, logger, cfg.OAuth.ClientID, cfg.JWT.Secret, cfg.JWT.Expiration, blacklistRepo, oauthConfig)
	categoryMod := category.NewModule(db, logger)
	budgetMod := budget.NewModule(db, logger)

	transactionMod := transaction.NewModule(db, logger, authMod.Repository, categoryMod.Repository, emailParser, dashCacheManager)
	recurringMod := recurring.NewModule(db, logger, transactionMod.Repository, dashCacheManager)
	dashboardMod := dashboard.NewModule(budgetMod.Repository, budgetMod.SummaryRepository, cacheRepo, logger)

	// Telegram requires a bot token and a Redis client.
	if cfg.Telegram.BotToken == "" {
		return nil, fmt.Errorf("telegram bot token is required")
	}

	var bot *tgbotapi.BotAPI
	if cfg.Telegram.BotToken != "" {
		var err error
		bot, err = tgbotapi.NewBotAPI(cfg.Telegram.BotToken)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize telegram bot API: %w", err)
		}
	}

	if redisClient == nil {
		return nil, fmt.Errorf("redis client is required for telegram service")
	}

	telegramMod := telegram.NewModule(cfg, bot, redisClient, authMod.Repository, categoryMod.Repository, transactionMod.Repository, budgetMod.Repository, emailSvc)

	cronMod := cron.NewModule(db, logger, cfg.IMAP.Email, cfg.IMAP.Password, transactionMod.Processor, recurringMod.Service)

	return &Modules{
		Auth:        authMod,
		Transaction: transactionMod,
		Category:    categoryMod,
		Budget:      budgetMod,
		Recurring:   recurringMod,
		Dashboard:   dashboardMod,
		Telegram:    telegramMod,
		Cron:        cronMod,
	}, nil
}
