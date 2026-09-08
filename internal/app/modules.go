package app

import (
	"walletx-be/configs"
	"walletx-be/internal/middleware"
	"walletx-be/internal/modules/auth"
	sqlc "walletx-be/internal/platform/database/sqlc"
	"walletx-be/internal/platform/logger"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// Modules aggregates the business modules enabled by the application.
type Modules struct {
	Auth *auth.Module
}

// buildModules constructs the enabled modules and wires their dependencies.
func buildModules(
	queries *sqlc.Queries,
	logger logger.Logger,
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
		queries,
		logger,
		cfg.OAuth.ClientID,
		cfg.JWT.Secret,
		cfg.JWT.Expiration,
		blacklistRepo,
		oauthConfig,
	)

	return &Modules{
		Auth: authMod,
	}, nil
}
