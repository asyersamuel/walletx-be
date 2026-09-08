package auth

import (
	"walletx-be/internal/middleware"
	sqlc "walletx-be/internal/platform/database/sqlc"
	"walletx-be/internal/platform/logger"

	"golang.org/x/oauth2"
)

// Module exposes the auth capability to the application wiring layer.
type Module struct {
	Handler    *Handler
	Service    Service
	Repository UserRepository
}

// NewModule wires auth dependencies: repository → service → handler.
func NewModule(
	queries *sqlc.Queries,
	logger logger.Logger,
	oauthClientID string,
	jwtSecret string,
	jwtExpiration int,
	blacklistRepo middleware.TokenBlacklistRepository,
	oauthConfig *oauth2.Config,
) *Module {
	userRepo := NewUserRepository(queries, logger)
	svc := NewService(userRepo, oauthClientID, jwtSecret, jwtExpiration, blacklistRepo)

	return &Module{
		Handler:    NewHandler(svc, oauthConfig),
		Service:    svc,
		Repository: userRepo,
	}
}
