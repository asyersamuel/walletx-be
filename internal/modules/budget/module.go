package budget

import (
	"walletx-be/internal/platform/logger"

	"gorm.io/gorm"
)

// Module exposes the budget capability to the application wiring layer.
type Module struct {
	Handler          *Handler
	Service          Service
	Repository       Repository
	SummaryRepository SummaryRepository
}

// NewModule wires budget dependencies: repository → service → handler.
func NewModule(db *gorm.DB, logger logger.Logger) *Module {
	repo := NewRepository(db, logger)
	summaryRepo := NewSummaryRepository(db, logger)
	svc := NewService(repo, summaryRepo, logger)

	return &Module{
		Handler:           NewHandler(svc),
		Service:           svc,
		Repository:        repo,
		SummaryRepository: summaryRepo,
	}
}
