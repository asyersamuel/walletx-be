package dashboard

import (
	"walletx-be/internal/modules/budget"
	"walletx-be/internal/platform/cache"
	"walletx-be/internal/platform/logger"
)

// Module exposes the dashboard capability to the application wiring layer.
type Module struct {
	Handler *Handler
	Service Service
}

// NewModule wires dashboard dependencies: budget repository + summary + cache → service → handler.
func NewModule(
	budgetRepo budget.Repository,
	summaryRepo budget.SummaryRepository,
	cacheRepo cache.CacheRepository,
	logger logger.Logger,
) *Module {
	svc := NewService(budgetRepo, summaryRepo, cacheRepo, logger)

	return &Module{
		Handler: NewHandler(svc),
		Service: svc,
	}
}
