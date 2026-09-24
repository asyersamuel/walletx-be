package auth

import (
	"walletx-be/internal/middleware"
	sqlc "walletx-be/internal/platform/database/sqlc"
	"walletx-be/internal/platform/logger"

	"github.com/jackc/pgx/v5/pgxpool"
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
	pool *pgxpool.Pool,
	queries *sqlc.Queries,
	appLogger logger.Logger,
	oauthClientID string,
	jwtSecret string,
	jwtExpiration int,
	blacklistRepo middleware.TokenBlacklistRepository,
	oauthConfig *oauth2.Config,
) *Module {
	userRepo := NewUserRepository(pool, queries, appLogger)
	svc := NewService(userRepo, NewGoogleTokenVerifier(oauthClientID), appLogger, jwtSecret, jwtExpiration, blacklistRepo)

	return &Module{
		Handler:    NewHandler(svc, appLogger, oauthConfig),
		Service:    svc,
		Repository: userRepo,
	}
}
