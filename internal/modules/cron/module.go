package cron

import (
	"walletx-be/internal/modules/transaction"
	"walletx-be/internal/platform/logger"

	"gorm.io/gorm"
)

// Module exposes the cron capability to the application wiring layer.
type Module struct {
	Handler    *Handler
	IMAPSyncer IMAPSyncer
}

// NewModule wires the cron handler: health check + IMAP sync + recurring job.
func NewModule(
	db *gorm.DB,
	logger logger.Logger,
	imapEmail string,
	imapPassword string,
	processor transaction.Processor,
	recurringProcessor RecurringProcessor,
) *Module {
	healthRepo := NewHealthRepository(db)
	imapSyncer := NewIMAPService(imapEmail, imapPassword, processor, logger)

	return &Module{
		Handler:    NewHandler(healthRepo, imapSyncer, recurringProcessor),
		IMAPSyncer: imapSyncer,
	}
}
