package app

import (
	"walletx-be/configs"
	"walletx-be/internal/middleware"
	"walletx-be/internal/modules/auth"
	"walletx-be/internal/modules/users"
	sqlc "walletx-be/internal/platform/database/sqlc"
	"walletx-be/internal/platform/logger"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// Modules aggregates the business modules enabled by the application.
type Modules struct {
	Auth  *auth.Module
	Users *users.Module
}

// buildModules constructs the enabled modules and wires their dependencies.
func buildModules(
	pool *pgxpool.Pool,
	queries *sqlc.Queries,
	appLogger logger.Logger,
	cfg *configs.Config,
	blacklistRepo middleware.TokenBlacklistRepository,
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

	return &Modules{
		Auth:  authMod,
		Users: usersMod,
	}, nil
}
