package cron

import (
	"walletx-be/internal/modules/transaction"
	sqlc "walletx-be/internal/platform/database/sqlc"
	"walletx-be/internal/platform/logger"
)

// Module exposes the cron capability to the application wiring layer.
type Module struct {
	Handler    *Handler
	IMAPSyncer IMAPSyncer
}

// NewModule wires the cron handler: health check + IMAP sync + recurring job.
func NewModule(
	queries *sqlc.Queries,
	logger logger.Logger,
	imapServer string,
	imapEmail string,
	imapPassword string,
	processor transaction.Processor,
	recurringProcessor RecurringProcessor,
) *Module {
	healthRepo := NewHealthRepository(queries)
	imapSyncer := NewIMAPService(imapServer, imapEmail, imapPassword, processor, logger)

	return &Module{
		Handler:    NewHandler(healthRepo, imapSyncer, recurringProcessor),
		IMAPSyncer: imapSyncer,
	}
}
