package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/mail"
	"strconv"
	"time"

	"walletx-be/configs"
	"walletx-be/pkg/models"
	"walletx-be/pkg/repository"
	"walletx-be/pkg/utils"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// ─── Redis key prefixes ───────────────────────────────────────────────────────

const (
	keyPrefixTgState    = "tg_state:"   // tg_state:<chatID>
	keyPrefixTgVerify   = "tg_verify:"  // tg_verify:<token>
	keyPrefixTgPendingTx = "tg_ptx:"   // tg_ptx:<shortKey>
	ttlVerify           = 15 * time.Minute
	ttlPendingTx        = 5 * time.Minute
	ttlState            = 5 * time.Minute
)

// ─── Interface ────────────────────────────────────────────────────────────────

type TelegramService interface {
	HandleWebhook(update tgbotapi.Update) error
	VerifyToken(token string) error
}

// ─── DTOs ─────────────────────────────────────────────────────────────────────

// VerifyData is stored in Redis during the account-linking email flow.
type VerifyData struct {
	Email  string `json:"email"`
	ChatID int64  `json:"chat_id"`
}

// PendingTxData is stored in Redis while the user picks a category.
type PendingTxData struct {
	Merchant string  `json:"merchant"`
	Amount   float64 `json:"amount"`
	ChatID   int64   `json:"chat_id"`
}

// ─── Implementation ───────────────────────────────────────────────────────────

type telegramService struct {
	bot             *tgbotapi.BotAPI
	redis           *redis.Client
	userRepo        repository.UserRepository
	categoryRepo    repository.CategoryRepository
	transactionRepo repository.TransactionRepository
	emailSvc        *utils.EmailService
	appConfig       configs.AppConfig
}

func NewTelegramService(
	cfg *configs.Config,
	bot *tgbotapi.BotAPI,
	rdb *redis.Client,
	userRepo repository.UserRepository,
	categoryRepo repository.CategoryRepository,
	transactionRepo repository.TransactionRepository,
	emailSvc *utils.EmailService,
) TelegramService {
	return &telegramService{
		bot:             bot,
		redis:           rdb,
		userRepo:        userRepo,
		categoryRepo:    categoryRepo,
		transactionRepo: transactionRepo,
		emailSvc:        emailSvc,
		appConfig:       cfg.App,
	}
}

// ─── Private helpers ──────────────────────────────────────────────────────────

func (s *telegramService) sendMessage(chatID int64, text string) {
	if s.bot == nil {
		logrus.Warn("Telegram Bot is not initialized, cannot send message")
		return
	}
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := s.bot.Send(msg); err != nil {
		logrus.WithError(err).Error("Failed to send Telegram message")
	}
}

func (s *telegramService) sendMessageWithKeyboard(chatID int64, text string, keyboard tgbotapi.InlineKeyboardMarkup) {
	if s.bot == nil {
		logrus.Warn("Telegram Bot is not initialized, cannot send message")
		return
	}
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = keyboard
	if _, err := s.bot.Send(msg); err != nil {
		logrus.WithError(err).Error("Failed to send Telegram message with keyboard")
	}
}

// shortKey generates an 8-character random key safe for use inside callback_data.
// callback_data is capped at 64 bytes by Telegram, so we keep this compact.
func shortKey() string {
	id := uuid.New().String() // e.g. "550e8400-e29b-41d4-a716-446655440000"
	// Use first 8 hex characters (sufficient entropy for a 5-minute TTL cache).
	return id[:8]
}

// buildCategoryKeyboard constructs an InlineKeyboardMarkup from a category list.
// Each button's callback_data follows the format: "tx_cat:<redisKey>:<categoryID>"
// Maximum callback_data length allowed by Telegram: 64 bytes.
// "tx_cat:" (7) + 8-char key + ":" (1) + 36-char UUID = 52 bytes — well within limit.
func buildCategoryKeyboard(categories []models.Category, redisKey string) tgbotapi.InlineKeyboardMarkup {
	const maxButtonsPerRow = 2
	var rows [][]tgbotapi.InlineKeyboardButton

	for i := 0; i < len(categories); i += maxButtonsPerRow {
		end := i + maxButtonsPerRow
		if end > len(categories) {
			end = len(categories)
		}

		var row []tgbotapi.InlineKeyboardButton
		for _, cat := range categories[i:end] {
			callbackData := fmt.Sprintf("tx_cat:%s:%s", redisKey, cat.ID.String())
			label := cat.Name
			if cat.Icon != "" {
				label = cat.Icon + " " + cat.Name
			}
			btn := tgbotapi.NewInlineKeyboardButtonData(label, callbackData)
			row = append(row, btn)
		}
		rows = append(rows, row)
	}

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// formatRupiah formats a float64 as Indonesian Rupiah without decimals.
func formatRupiah(amount float64) string {
	amountInt := int64(amount)
	s := strconv.FormatInt(amountInt, 10)

	// Insert thousands separators (dots for ID locale).
	n := len(s)
	var result []byte
	for i, c := range s {
		if i > 0 && (n-i)%3 == 0 {
			result = append(result, '.')
		}
		result = append(result, byte(c))
	}
	return "Rp " + string(result)
}

// ─── handleQuickTransaction deals with the Phase 1 "record transaction" flow ──

func (s *telegramService) handleQuickTransaction(ctx context.Context, chatID int64, parsed *utils.ParsedTransaction) error {
	// 1. Look up the linked user by telegram_chat_id.
	chatIDStr := strconv.FormatInt(chatID, 10)
	user, err := s.userRepo.FindByTelegramChatID(ctx, chatIDStr)
	if err != nil {
		s.sendMessage(chatID, "⚠️ Akun kamu belum terhubung dengan WalletX. Kirim /start untuk menghubungkan.")
		return nil
	}

	// 2. Fetch user's categories.
	categories, err := s.categoryRepo.List(ctx, user.ID)
	if err != nil {
		logrus.WithError(err).Error("Failed to fetch user categories")
		s.sendMessage(chatID, "❌ Gagal mengambil data kategori. Coba lagi nanti.")
		return err
	}

	// Condition B: If user has no categories, insert directly.
	if len(categories) == 0 {
		tx := &models.Transaction{
			UserID:          user.ID,
			CategoryID:      nil,
			Amount:          parsed.Amount,
			Merchant:        parsed.Merchant,
			TransactionDate: time.Now(),
		}

		if err := s.transactionRepo.Create(ctx, tx); err != nil {
			logrus.WithError(err).Error("Failed to insert quick transaction without category")
			s.sendMessage(chatID, "❌ Gagal mencatat transaksi. Coba lagi nanti.")
			return err
		}

		replyText := fmt.Sprintf(
			"✅ %s untuk *%s* berhasil dicatat (Tanpa Kategori).\n\nSepertinya kamu belum membuat kategori pengeluaran nih, silakan buat dulu di aplikasi WalletX ya!",
			formatRupiah(parsed.Amount),
			parsed.Merchant,
		)
		msg := tgbotapi.NewMessage(chatID, replyText)
		msg.ParseMode = tgbotapi.ModeMarkdown
		if s.bot != nil {
			if _, err := s.bot.Send(msg); err != nil {
				logrus.WithError(err).Error("Failed to send direct insertion reply")
			}
		}
		return nil
	}

	// Condition A: User has categories. Cache to Redis and send keyboard.
	key := shortKey()
	redisKey := keyPrefixTgPendingTx + key

	pendingData := PendingTxData{
		Merchant: parsed.Merchant,
		Amount:   parsed.Amount,
		ChatID:   chatID,
	}
	dataBytes, _ := json.Marshal(pendingData)
	if err := s.redis.Set(ctx, redisKey, dataBytes, ttlPendingTx).Err(); err != nil {
		logrus.WithError(err).Error("Failed to cache pending transaction in Redis")
		s.sendMessage(chatID, "❌ Gagal menyimpan data sementara. Coba lagi.")
		return err
	}

	// Build inline keyboard and send reply.
	keyboard := buildCategoryKeyboard(categories, key)
	replyText := fmt.Sprintf(
		"✅ Sip! %s buat *%s*.\nMasuk kategori mana?",
		formatRupiah(parsed.Amount),
		parsed.Merchant,
	)

	msg := tgbotapi.NewMessage(chatID, replyText)
	msg.ParseMode = tgbotapi.ModeMarkdown
	msg.ReplyMarkup = keyboard
	if s.bot != nil {
		if _, err := s.bot.Send(msg); err != nil {
			logrus.WithError(err).Error("Failed to send category keyboard")
			return err
		}
	}

	return nil
}

// ─── HandleWebhook is the main entry point for all Telegram updates ───────────

func (s *telegramService) HandleWebhook(update tgbotapi.Update) error {
	if s.bot == nil || s.redis == nil {
		return fmt.Errorf("service not properly initialized (bot or redis is nil)")
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
	if parsed := utils.ParseTransactionMessage(text); parsed != nil {
		return s.handleQuickTransaction(ctx, chatID, parsed)
	}

	// ── Fallback ──────────────────────────────────────────────────────────────
	s.sendMessage(chatID, "Ketik transaksi kamu, contoh: \"Makan warteg 20k\" atau ketik /start untuk menghubungkan akun.")
	return nil
}

// handleAccountLinking handles the email-based account-linking sub-flow.
func (s *telegramService) handleAccountLinking(ctx context.Context, chatID int64, text, stateKey string) error {
	// Validate email format.
	if _, err := mail.ParseAddress(text); err != nil {
		s.sendMessage(chatID, "Format email tidak valid. Silakan ketik ulang email Anda dengan benar.")
		return nil
	}

	email := text

	// Check if user exists.
	user, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil || user == nil {
		s.sendMessage(chatID, "Email tidak terdaftar di WalletX. Silakan periksa kembali email Anda.")
		return nil
	}

	// Generate verification token.
	token := uuid.New().String()
	verifyKey := keyPrefixTgVerify + token

	data := VerifyData{Email: email, ChatID: chatID}
	dataBytes, _ := json.Marshal(data)

	if err := s.redis.Set(ctx, verifyKey, dataBytes, ttlVerify).Err(); err != nil {
		logrus.WithError(err).Error("Failed to set verify token to redis")
		return err
	}

	// Delete the awaiting state.
	s.redis.Del(ctx, stateKey)

	// Send verification email.
	verifyURL := fmt.Sprintf("%s/api/v1/telegram/verify?token=%s", s.appConfig.URL, token)
	if err := s.emailSvc.SendVerificationEmail(email, verifyURL); err != nil {
		logrus.WithError(err).Error("Failed to send verification email")
		s.sendMessage(chatID, "Gagal mengirim email verifikasi. Silakan coba lagi nanti.")
		return err
	}

	s.sendMessage(chatID, "Cek inbox email Anda! Klik link yang dikirimkan untuk konfirmasi.")
	return nil
}

// ─── VerifyToken handles the one-time email verification link ─────────────────

func (s *telegramService) VerifyToken(token string) error {
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

	// Find user.
	user, err := s.userRepo.FindByEmail(ctx, data.Email)
	if err != nil || user == nil {
		return fmt.Errorf("user not found")
	}

	// Update telegram_chat_id.
	chatIDStr := strconv.FormatInt(data.ChatID, 10)
	user.TelegramChatID = &chatIDStr
	if err := s.userRepo.Update(ctx, user); err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	// Delete one-time token.
	s.redis.Del(ctx, verifyKey)

	// Notify user on Telegram.
	s.sendMessage(data.ChatID, "🎉 Selamat! Akun Telegram Anda berhasil terhubung dengan WalletX.\n\nSekarang kamu bisa langsung catat transaksi, contoh: \"Makan warteg 20k\"")

	return nil
}
