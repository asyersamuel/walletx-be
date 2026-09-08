package transaction

import (
	"context"

	"walletx-be/internal/modules/category"
	"walletx-be/internal/platform/cache"
	sqlc "walletx-be/internal/platform/database/sqlc"
	"walletx-be/internal/platform/logger"
	"walletx-be/internal/platform/parser"

	"github.com/google/uuid"
)

// Module exposes the transaction capability to the application wiring layer.
type Module struct {
	Handler    *Handler
	Service    Service
	Processor  Processor
	Repository Repository
}

// NewModule wires transaction dependencies: repository + processor → service → handler.
func NewModule(
	queries *sqlc.Queries,
	logger logger.Logger,
	userStore UserStore,
	categoryRepo category.Repository,
	emailParser parser.EmailParser,
	cacheManager cache.DashboardCacheManager,
) *Module {
	repo := NewRepository(queries, logger)
	categoryLookup := categoryLookupAdapter{categoryRepo}
	processor := NewProcessor(userStore, repo, categoryLookup, emailParser, cacheManager, logger)
	svc := NewService(userStore, repo, categoryLookup, processor, cacheManager, logger)

	return &Module{
		Handler:    NewHandler(svc),
		Service:    svc,
		Processor:  processor,
		Repository: repo,
	}
}

// categoryLookupAdapter adapts the exported category.Repository to the narrow
// CategoryLookup capability required within the transaction flow.
type categoryLookupAdapter struct {
	repo category.Repository
}

func (a categoryLookupAdapter) GetByName(ctx context.Context, name string, userID uuid.UUID) (*category.Category, error) {
	return a.repo.GetByName(ctx, name, userID)
}
