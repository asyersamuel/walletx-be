package categories

import (
	sqlc "walletx-be/internal/platform/database/sqlc"
	"walletx-be/internal/platform/logger"
)

type Module struct {
	Handler    *Handler
	Service    CategoryService
	Repository CategoryRepository
}

func NewModule(queries *sqlc.Queries, appLogger logger.Logger) *Module {
	repo := NewCategoryRepository(queries, appLogger)
	svc := NewCategoryService(repo, appLogger)

	return &Module{
		Handler:    NewHandler(svc, appLogger),
		Service:    svc,
		Repository: repo,
	}
}
