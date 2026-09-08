package budget

import (
	sqlc "walletx-be/internal/platform/database/sqlc"
	"walletx-be/internal/platform/logger"
)

// Module exposes the budget capability to the application wiring layer.
type Module struct {
	Handler           *Handler
	Service           Service
	Repository        Repository
	SummaryRepository SummaryRepository
}

// NewModule wires budget dependencies: repository → service → handler.
func NewModule(queries *sqlc.Queries, logger logger.Logger) *Module {
	repo := NewRepository(queries, logger)
	summaryRepo := NewSummaryRepository(queries, logger)
	svc := NewService(repo, summaryRepo, logger)

	return &Module{
		Handler:           NewHandler(svc),
		Service:           svc,
		Repository:        repo,
		SummaryRepository: summaryRepo,
	}
}
