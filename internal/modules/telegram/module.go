package telegram

import (
	"walletx-be/configs"
	"walletx-be/internal/modules/auth"
	"walletx-be/internal/modules/budget"
	"walletx-be/internal/modules/category"
	"walletx-be/internal/modules/transaction"
	"walletx-be/internal/platform/mail"

	"github.com/redis/go-redis/v9"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Module exposes the Telegram capability to the application wiring layer.
type Module struct {
	Handler *Handler
	Service Service
}

// NewModule wires the Telegram service using the narrow cross-module
// capabilities satisfied by the concrete repositories of other modules.
func NewModule(
	cfg *configs.Config,
	bot *tgbotapi.BotAPI,
	rdb *redis.Client,
	userStore auth.UserRepository,
	categoryStore category.Repository,
	transactionStore transaction.Repository,
	budgetStore budget.Repository,
	emailSvc *mail.EmailService,
) *Module {
	svc := NewService(cfg, bot, rdb, userStore, categoryStore, transactionStore, budgetStore, emailSvc)

	return &Module{
		Handler: NewHandler(svc),
		Service: svc,
	}
}
