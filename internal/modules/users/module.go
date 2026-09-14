package users

import (
	sqlc "walletx-be/internal/platform/database/sqlc"
	"walletx-be/internal/platform/logger"
)

// Module exposes the users capability to the application wiring layer.
type Module struct {
	Handler    *Handler
	Service    Service
	Repository UserRepository
}

// NewModule wires users dependencies: repository → service → handler.
func NewModule(queries *sqlc.Queries, logger logger.Logger, revoker TokenRevoker) *Module {
	repo := NewUserRepository(queries, logger)
	svc := NewService(repo, revoker, logger)

	return &Module{
		Handler:    NewHandler(svc, logger),
		Service:    svc,
		Repository: repo,
	}
}
