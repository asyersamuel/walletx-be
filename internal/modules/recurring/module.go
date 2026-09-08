package recurring

import (
	"context"

	"walletx-be/internal/modules/transaction"
	"walletx-be/internal/platform/cache"
	"walletx-be/internal/platform/logger"

	"gorm.io/gorm"
)

// Module exposes the recurring capability to the application wiring layer.
type Module struct {
	Handler    *Handler
	Service    Service
	Repository Repository
}

// NewModule wires recurring dependencies: repository → service → handler.
// The TransactionCreator capability is satisfied by the transaction module.
func NewModule(
	db *gorm.DB,
	logger logger.Logger,
	txCreator transaction.Repository,
	cacheManager cache.DashboardCacheManager,
) *Module {
	repo := NewRepository(db, logger)
	txCreatorAdapter := txCreatorAdapter{txCreator}
	svc := NewService(repo, txCreatorAdapter, cacheManager, logger)

	return &Module{
		Handler:    NewHandler(svc),
		Service:    svc,
		Repository: repo,
	}
}

// txCreatorAdapter adapts the exported transaction.Repository to the narrow
// TransactionCreator capability required by the recurring processor.
type txCreatorAdapter struct {
	repo transaction.Repository
}

func (a txCreatorAdapter) Create(ctx context.Context, txn *transaction.Transaction) error {
	return a.repo.Create(ctx, txn)
}
