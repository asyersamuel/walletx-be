package category

import (
	"walletx-be/internal/platform/logger"

	"gorm.io/gorm"
)

// Module exposes the category capability to the application wiring layer.
type Module struct {
	Handler    *Handler
	Service    Service
	Repository Repository
}

// NewModule wires category dependencies: repository → service → handler.
func NewModule(db *gorm.DB, logger logger.Logger) *Module {
	repo := NewRepository(db, logger)
	svc := NewService(repo, logger)

	return &Module{
		Handler:    NewHandler(svc),
		Service:    svc,
		Repository: repo,
	}
}
