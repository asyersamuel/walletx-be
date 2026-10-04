package app

import (
	"context"
	"fmt"
	"net/http"

	"walletx-be/configs"
	"walletx-be/internal/middleware"
	"walletx-be/internal/modules/auth"
	"walletx-be/internal/platform/database"
	sqlc "walletx-be/internal/platform/database/sqlc"
	"walletx-be/internal/platform/gemini"
	platformlogger "walletx-be/internal/platform/logger"
)

// App is the assembled application instance returned to the entry point.
type App struct {
	Handler      http.Handler
	Config       *configs.Config
	Cleanup      func()
	ShutdownFunc func(ctx context.Context) error
}

// Run initialises the platform, wires the enabled modules, and returns a ready App.
func Run(cfg *configs.Config) (*App, error) {
	appLogger := platformlogger.NewLogger()

	db, err := database.Init(cfg.Database.URL, appLogger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}
	queries := sqlc.New(db)
	embeddingProvider, err := gemini.NewEmbeddingClient(cfg.AI.GeminiAPIKey)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize Gemini embedding client: %w", err)
	}

	blacklistRepo := auth.NewInMemoryBlacklistRepository()

	chatModel := cfg.AI.ChatModel
	if chatModel == "" {
		chatModel = configs.DefaultChatModel
	}
	modules, err := buildModules(db, queries, appLogger, cfg, blacklistRepo, embeddingProvider, embeddingProvider.ChatProvider(chatModel), embeddingProvider.TransactionLLM(chatModel))
	if err != nil {
		embeddingProvider.Close()
		db.Close()
		return nil, err
	}

	validator := middleware.NewJWTValidator(cfg.JWT.Secret, blacklistRepo)
	router := SetupRouter(modules, cfg, validator, appLogger)

	cleanup := func() {
		if err := embeddingProvider.Close(); err != nil {
			appLogger.WithError(err).Error("failed to close Gemini embedding client")
		}
	}

	shutdownFunc := func(ctx context.Context) error {
		db.Close()
		return nil
	}

	return &App{
		Handler:      router,
		Config:       cfg,
		Cleanup:      cleanup,
		ShutdownFunc: shutdownFunc,
	}, nil
}
