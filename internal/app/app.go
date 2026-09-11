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

	blacklistRepo := auth.NewInMemoryBlacklistRepository()

	modules, err := buildModules(queries, appLogger, cfg, blacklistRepo)
	if err != nil {
		db.Close()
		return nil, err
	}

	validator := middleware.NewJWTValidator(cfg.JWT.Secret, blacklistRepo)
	router := SetupRouter(modules, cfg, validator, appLogger)

	cleanup := func() {}

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
