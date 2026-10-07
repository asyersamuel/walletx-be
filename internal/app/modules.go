package app

import (
	"fmt"
	"net/http"
	"walletx-be/configs"
	"walletx-be/internal/middleware"
	"walletx-be/internal/modules/ai"
	aihttp "walletx-be/internal/modules/ai/delivery/http"
	aipostgres "walletx-be/internal/modules/ai/repository/postgres"
	"walletx-be/internal/modules/auth"
	"walletx-be/internal/modules/categories"
	"walletx-be/internal/modules/transactions"
	"walletx-be/internal/modules/transactions/infrastructure"
	"walletx-be/internal/modules/users"
	sqlc "walletx-be/internal/platform/database/sqlc"
	"walletx-be/internal/platform/logger"
	"walletx-be/internal/platform/storage"
	apperrors "walletx-be/internal/shared/errors"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// Modules aggregates the business modules enabled by the application.
type Modules struct {
	Auth           *auth.Module
	Users          *users.Module
	Categories     *categories.Module
	Transactions   *transactions.Module
	AI             *aihttp.Handler
	ReceiptStorage transactions.ReceiptStorage
}

// buildModules constructs the enabled modules and wires their dependencies.
func buildModules(
	pool *pgxpool.Pool,
	queries *sqlc.Queries,
	appLogger logger.Logger,
	cfg *configs.Config,
	blacklistRepo middleware.TokenBlacklistRepository,
	embeddingProvider transactions.EmbeddingProvider,
	aiProvider ai.AIChatProvider,
	toolGenerator infrastructure.LLMGenerator,
) (*Modules, error) {
	oauthConfig := &oauth2.Config{
		ClientID:     cfg.OAuth.ClientID,
		ClientSecret: cfg.OAuth.ClientSecret,
		RedirectURL:  cfg.OAuth.RedirectURL,
		Scopes:       []string{"https://www.googleapis.com/auth/userinfo.email", "https://www.googleapis.com/auth/userinfo.profile"},
		Endpoint:     google.Endpoint,
	}

	authMod := auth.NewModule(
		pool,
		queries,
		appLogger,
		cfg.OAuth.ClientID,
		cfg.JWT.Secret,
		cfg.JWT.Expiration,
		blacklistRepo,
		oauthConfig,
	)

	usersMod := users.NewModule(queries, appLogger, authMod.Service)
	categoriesMod := categories.NewModule(queries, appLogger)
	aiRepository := aipostgres.NewRepository(queries)
	aiService := ai.NewAIService(aiProvider, aiRepository, aiRepository)
	receiptStorage, err := buildReceiptStorage(cfg.ReceiptStorage)
	if err != nil {
		return nil, fmt.Errorf("initialize receipt storage: %w", err)
	}
	transactionsMod, err := transactions.NewModule(queries, appLogger, embeddingProvider, toolGenerator, aiService, aiRepository, receiptStorage, cfg.Receipt.ExtractionTimeout)
	if err != nil {
		return nil, fmt.Errorf("initialize transactions module: %w", err)
	}
	aiHandler := aihttp.NewHandler(aiService, appLogger)

	return &Modules{
		Auth:           authMod,
		Users:          usersMod,
		Categories:     categoriesMod,
		Transactions:   transactionsMod,
		AI:             aiHandler,
		ReceiptStorage: receiptStorage,
	}, nil
}

func buildReceiptStorage(cfg configs.ReceiptStorageConfig) (transactions.ReceiptStorage, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	switch cfg.Provider {
	case "local":
		return storage.NewLocal(cfg.Directory)
	case "supabase":
		client := &http.Client{Timeout: cfg.UploadTimeout}
		store, err := storage.NewSupabase(cfg.SupabaseURL, cfg.SupabaseServiceRoleKey, client)
		if err != nil {
			return nil, err
		}
		return store, nil
	default:
		return nil, fmt.Errorf("unsupported receipt storage provider: %w", apperrors.ErrInvalidInput)
	}
}
