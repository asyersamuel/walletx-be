package category

import (
	sqlc "walletx-be/internal/platform/database/sqlc"
	"walletx-be/internal/platform/logger"
)

// Module exposes the category capability to the application wiring layer.
type Module struct {
	Handler    *Handler
	Service    Service
	Repository Repository
}

// NewModule wires category dependencies: repository → service → handler.
func NewModule(queries *sqlc.Queries, logger logger.Logger) *Module {
	repo := NewRepository(queries, logger)
	svc := NewService(repo, logger)

	return &Module{
		Handler:    NewHandler(svc),
		Service:    svc,
		Repository: repo,
	}
}
