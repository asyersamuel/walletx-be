package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"walletx-be/configs"
	"walletx-be/internal/modules/auth"
	"walletx-be/internal/modules/budget"
	"walletx-be/internal/modules/category"
	"walletx-be/internal/modules/transaction"
	"walletx-be/internal/platform/mail"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// ─── Redis key prefixes ───────────────────────────────────────────────────────

const (
	keyPrefixTgState     = "tg_state:"     // tg_state:<chatID>
	keyPrefixTgVerify    = "tg_verify:"    // tg_verify:<token>
	keyPrefixTgPendingTx = "tg_ptx:"       // tg_ptx:<shortKey>
	keyPrefixTgStateCat  = "tg_stat_cat:"  // tg_stat_cat:<chatID>
	ttlVerify            = 15 * time.Minute
	ttlPendingTx         = 5 * time.Minute
	ttlState             = 5 * time.Minute
	ttlStateCat          = 5 * time.Minute
)

// ─── Narrow cross-module capabilities ─────────────────────────────────────────

// UserStore is the subset of the auth module's user repository the Telegram
// flow relies on.
type UserStore interface {
	FindByEmail(ctx context.Context, email string) (*auth.User, error)
	FindByTelegramChatID(ctx context.Context, chatID string) (*auth.User, error)
	Update(ctx context.Context, user *auth.User) error
}

// CategoryStore is the subset of the category module's repository used here.
type CategoryStore interface {
	List(ctx context.Context, userID uuid.UUID) ([]category.Category, error)
	Create(ctx context.Context, cat *category.Category) error
}

// TransactionStore is the subset of the transaction module's repository used here.
type TransactionStore interface {
	Create(ctx context.Context, tx *transaction.Transaction) error
	GetTotalSpentByCategoryThisMonth(ctx context.Context, userID, categoryID uuid.UUID, month, year int) (float64, error)
}

// BudgetStore is the subset of the budget module's repository used here.
type BudgetStore interface {
	Create(ctx context.Context, limit *budget.CategoryLimit) error
	FindByCategory(ctx context.Context, userID, categoryID uuid.UUID) (*budget.CategoryLimit, error)
	GetLimitReportByUserID(ctx context.Context, userID uuid.UUID) ([]budget.LimitReport, error)
}

// ─── Service interface ────────────────────────────────────────────────────────

// Service is the Telegram bot orchestration capability.
type Service interface {
	HandleWebhook(update tgbotapi.Update) error
	VerifyToken(token string) error
}

type service struct {
	bot             *tgbotapi.BotAPI
	redis           *redis.Client
	userStore       UserStore
	categoryStore   CategoryStore
	transactionStore TransactionStore
	budgetStore     BudgetStore
	emailSvc        *mail.EmailService
	appConfig       configs.AppConfig
}

func NewService(
	cfg *configs.Config,
	bot *tgbotapi.BotAPI,
	rdb *redis.Client,
	userStore UserStore,
	categoryStore CategoryStore,
	transactionStore TransactionStore,
	budgetStore BudgetStore,
	emailSvc *mail.EmailService,
) Service {
	if bot != nil {
		commands := tgbotapi.NewSetMyCommands(
			tgbotapi.BotCommand{Command: "limit", Description: "Cek sisa budget bulan ini"},
			tgbotapi.BotCommand{Command: "tambah_category", Description: "Buat kategori & limit baru"},
			tgbotapi.BotCommand{Command: "help", Description: "Bantuan penggunaan"},
		)
		if _, err := bot.Request(commands); err != nil {
			logrus.WithError(err).Warn("Failed to set telegram bot commands")
		}
	}

	return &service{
		bot:             bot,
		redis:           rdb,
		userStore:       userStore,
		categoryStore:   categoryStore,
		transactionStore: transactionStore,
		budgetStore:     budgetStore,
		emailSvc:        emailSvc,
		appConfig:       cfg.App,
	}
}

// HandleWebhook is the main entry point for all Telegram updates.
func (s *service) HandleWebhook(update tgbotapi.Update) error {
	if s.bot == nil || s.redis == nil {
		return fmt.Errorf("service not properly initialized (bot or redis is nil)")
	}

	// ── Check for CallbackQuery ───────────────────────────────────────────────
	if update.CallbackQuery != nil {
		return s.HandleCallbackQuery(update.CallbackQuery)
	}

	// ── Ignore non-message updates ────────────────────────────────────────────
	if update.Message == nil {
		return nil
	}

	chatID := update.Message.Chat.ID
	text := update.Message.Text
	ctx := context.Background()

	// ── /start command → account-linking flow ─────────────────────────────────
	if text == "/start" {
		stateKey := keyPrefixTgState + strconv.FormatInt(chatID, 10)
		if err := s.redis.Set(ctx, stateKey, "awaiting_email", ttlState).Err(); err != nil {
			logrus.WithError(err).Error("Failed to set redis state")
			return err
		}
		s.sendMessage(chatID, "Halo! Silakan ketik email yang terdaftar di akun WalletX Anda.")
		return nil
	}

	// ── /limit command → real-time budget report ───────────────────────────────
	if text == "/limit" {
		return s.handleLimitCommand(ctx, chatID)
	}

	// ── /help command → instructions ──────────────────────────────────────────
	if text == "/help" {
		replyText := "<b>📊 Daftar Perintah (Commands)</b>\n" +
			"🔹 /limit — Cek sisa budget kategori bulan ini.\n" +
			"🔹 /tambah_category — Buat kategori baru & set limit.\n" +
			"🔹 /help — Bantuan instruksi penggunaan.\n\n" +
			"Catat transaksi harian kamu dengan mengetik <code>Nama Barang Nominal</code>. Contoh: <code>Kopi 15k</code>"
		msg := tgbotapi.NewMessage(chatID, replyText)
		msg.ParseMode = "HTML"
		if s.bot != nil {
			if _, err := s.bot.Send(msg); err != nil {
				logrus.WithError(err).Error("Failed to send help message")
			}
		}
		return nil
	}

	// ── /tambah_category command → start conversational category creation ──────
	if strings.HasPrefix(text, "/tambah_category") {
		return s.handleTambahCategoryCommand(ctx, chatID, text)
	}

	// ── Check if user is in the category-creation AWAITING_LIMIT state ─────────
	catState, err := s.getCategoryState(ctx, chatID)
	if err != nil {
		logrus.WithError(err).Error("Failed to read category state from Redis")
	}
	if catState != nil {
		return s.handleCategoryLimitReply(ctx, chatID, text, catState)
	}

	// ── Check if we are mid-flow for account linking ──────────────────────────
	stateKey := keyPrefixTgState + strconv.FormatInt(chatID, 10)
	state, err := s.redis.Get(ctx, stateKey).Result()
	if err != nil && err != redis.Nil {
		logrus.WithError(err).Error("Failed to get redis state")
		return err
	}

	if state == "awaiting_email" {
		return s.handleAccountLinking(ctx, chatID, text, stateKey)
	}

	// ── Try to parse as a Quick Transaction message ───────────────────────────
	if parsed := ParseTransactionMessage(text); parsed != nil {
		return s.handleQuickTransaction(ctx, chatID, parsed)
	}

	// ── Fallback ──────────────────────────────────────────────────────────────
	s.sendMessage(chatID, "Ketik transaksi kamu, contoh: \"Makan warteg 20k\", atau gunakan /tambah_category untuk membuat kategori baru.")
	return nil
}

// HandleCallbackQuery intercepts inline keyboard clicks.
func (s *service) HandleCallbackQuery(query *tgbotapi.CallbackQuery) error {
	ctx := context.Background()
	chatID := query.Message.Chat.ID
	messageID := query.Message.MessageID

	data := query.Data
	parts := strings.Split(data, ":")
	if len(parts) < 2 {
		return nil
	}

	if parts[0] == "tx_new" {
		redisKeyStr := parts[1]
		if err := s.saveCategoryState(ctx, chatID, CategoryState{
			Step:         "AWAITING_NAME",
			PendingTxKey: redisKeyStr,
		}); err != nil {
			logrus.WithError(err).Error("Failed to save state for new category via inline button")
			s.bot.Request(tgbotapi.NewCallbackWithAlert(query.ID, "Gagal memproses. Coba lagi nanti."))
			return err
		}
		s.bot.Request(tgbotapi.NewCallback(query.ID, ""))

		replyText := "Oke! Apa nama kategori baru yang ingin kamu buat? (Contoh: Jajan / Makan)"
		editMsg := tgbotapi.NewEditMessageTextAndMarkup(chatID, messageID, replyText, tgbotapi.InlineKeyboardMarkup{InlineKeyboard: make([][]tgbotapi.InlineKeyboardButton, 0)})
		editMsg.ParseMode = "HTML"
		if _, err := s.bot.Send(editMsg); err != nil {
			logrus.WithError(err).Error("Failed to edit telegram message")
		}
		return nil
	}

	if parts[0] != "tx_cat" || len(parts) != 3 {
		return nil
	}

	redisKeyStr := parts[1]
	categoryIDStr := parts[2]

	var categoryID *uuid.UUID
	if categoryIDStr != "none" {
		parsedID, err := uuid.Parse(categoryIDStr)
		if err != nil {
			return err
		}
		categoryID = &parsedID
	}

	fullRedisKey := keyPrefixTgPendingTx + redisKeyStr
	dataStr, err := s.redis.Get(ctx, fullRedisKey).Result()
	if err == redis.Nil {
		callback := tgbotapi.NewCallbackWithAlert(query.ID, "Waktu habis, silakan ketik ulang transaksinya.")
		s.bot.Request(callback)
		return nil
	} else if err != nil {
		return err
	}

	s.bot.Request(tgbotapi.NewCallback(query.ID, ""))

	var pending PendingTxData
	if err := json.Unmarshal([]byte(dataStr), &pending); err != nil {
		return err
	}

	chatIDStr := strconv.FormatInt(chatID, 10)
	user, err := s.userStore.FindByTelegramChatID(ctx, chatIDStr)
	if err != nil {
		return err
	}

	tx := &transaction.Transaction{
		UserID:          user.ID,
		CategoryID:      categoryID,
		Amount:          pending.Amount,
		Merchant:        pending.Merchant,
		TransactionDate: time.Now(),
	}

	if err := s.transactionStore.Create(ctx, tx); err != nil {
		return err
	}

	s.redis.Del(ctx, fullRedisKey)

	replyText := fmt.Sprintf("✅ <b>%s</b> untuk <b>%s</b> berhasil dicatat.", formatRupiah(pending.Amount), pending.Merchant)

	if categoryID != nil {
		limit, err := s.budgetStore.FindByCategory(ctx, user.ID, *categoryID)
		if err == nil && limit != nil {
			now := time.Now()
			totalSpent, err := s.transactionStore.GetTotalSpentByCategoryThisMonth(ctx, user.ID, *categoryID, int(now.Month()), now.Year())
			if err == nil {
				remaining := limit.LimitAmount - totalSpent
				if remaining >= 0 {
					replyText += fmt.Sprintf("\n\nSisa budget kategori ini: %s 🟢", formatRupiah(remaining))
				} else {
					replyText += fmt.Sprintf("\n\n⚠️ OVER BUDGET! Kamu melebihi limit sebesar %s 🔴", formatRupiah(abs(remaining)))
				}
			}
		}
	} else {
		replyText += " (Tanpa Kategori)"
	}

	editMsg := tgbotapi.NewEditMessageTextAndMarkup(chatID, messageID, replyText, tgbotapi.InlineKeyboardMarkup{InlineKeyboard: make([][]tgbotapi.InlineKeyboardButton, 0)})
	editMsg.ParseMode = "HTML"
	if _, err := s.bot.Send(editMsg); err != nil {
		logrus.WithError(err).Error("Failed to edit telegram message")
	}

	return nil
}

// VerifyToken handles the one-time email verification link.
func (s *service) VerifyToken(token string) error {
	if s.redis == nil {
		return fmt.Errorf("redis client is nil")
	}

	ctx := context.Background()
	verifyKey := keyPrefixTgVerify + token

	dataStr, err := s.redis.Get(ctx, verifyKey).Result()
	if err == redis.Nil {
		return fmt.Errorf("token invalid or expired")
	} else if err != nil {
		return fmt.Errorf("failed to check token: %w", err)
	}

	var data VerifyData
	if err := json.Unmarshal([]byte(dataStr), &data); err != nil {
		return fmt.Errorf("failed to parse verification data: %w", err)
	}

	user, err := s.userStore.FindByEmail(ctx, data.Email)
	if err != nil || user == nil {
		return fmt.Errorf("user not found")
	}

	chatIDStr := strconv.FormatInt(data.ChatID, 10)
	user.TelegramChatID = &chatIDStr
	if err := s.userStore.Update(ctx, user); err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	s.redis.Del(ctx, verifyKey)

	replyText := "<b>🎉 Selamat! Akun Telegram Anda berhasil terhubung dengan WalletX.</b>\n\n" +
		"Sekarang Anda bisa mengelola keuangan langsung dari sini. Berikut adalah cara pakainya:\n\n" +
		"<b>📝 Catat Transaksi Langsung</b>\n" +
		"Cukup ketik: <code>Nama Barang [spasi] Nominal</code>\n" +
		"Contoh: <code>Kopi susu 15k</code> atau <code>Beli bensin 50.000</code>\n\n" +
		"<b>📊 Daftar Perintah (Commands)</b>\n" +
		"🔹 /limit — Cek sisa budget kategori bulan ini.\n" +
		"🔹 /tambah_category — Buat kategori baru & set limit.\n" +
		"🔹 /help — Bantuan instruksi penggunaan.\n\n" +
		"Silakan coba ketik transaksi pertama Anda sekarang! 🚀"

	msg := tgbotapi.NewMessage(data.ChatID, replyText)
	msg.ParseMode = "HTML"
	if s.bot != nil {
		if _, err := s.bot.Send(msg); err != nil {
			logrus.WithError(err).Error("Failed to send onboarding message")
		}
	}

	return nil
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
