package services

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"walletx-be/configs"
	"walletx-be/internal/domain/report"
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
	keyPrefixTgState    = "tg_state:"    // tg_state:<chatID>
	keyPrefixTgVerify   = "tg_verify:"   // tg_verify:<token>
	keyPrefixTgPendingTx = "tg_ptx:"    // tg_ptx:<shortKey>
	keyPrefixTgStateCat = "tg_stat_cat:" // tg_stat_cat:<chatID>
	ttlVerify           = 15 * time.Minute
	ttlPendingTx        = 5 * time.Minute
	ttlState            = 5 * time.Minute
	ttlStateCat         = 5 * time.Minute
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

// CategoryState is stored in Redis between /tambah_category Step 1 and Step 2.
type CategoryState struct {
	Step         string `json:"step"`                     // "AWAITING_NAME" or "AWAITING_LIMIT"
	CategoryName string `json:"category_name,omitempty"`
	PendingTxKey string `json:"pending_tx_key,omitempty"` // For linking the transaction after creation
}

// ─── Implementation ───────────────────────────────────────────────────────────

type telegramService struct {
	bot             *tgbotapi.BotAPI
	redis           *redis.Client
	userRepo        repository.UserRepository
	categoryRepo    repository.CategoryRepository
	transactionRepo repository.TransactionRepository
	budgetRepo      repository.BudgetRepository
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
	budgetRepo repository.BudgetRepository,
	emailSvc *utils.EmailService,
) TelegramService {
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

	return &telegramService{
		bot:             bot,
		redis:           rdb,
		userRepo:        userRepo,
		categoryRepo:    categoryRepo,
		transactionRepo: transactionRepo,
		budgetRepo:      budgetRepo,
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

// ─── CategoryState Redis helpers ──────────────────────────────────────────────

func (s *telegramService) saveCategoryState(ctx context.Context, chatID int64, state CategoryState) error {
	key := keyPrefixTgStateCat + strconv.FormatInt(chatID, 10)
	b, _ := json.Marshal(state)
	return s.redis.Set(ctx, key, b, ttlStateCat).Err()
}

func (s *telegramService) getCategoryState(ctx context.Context, chatID int64) (*CategoryState, error) {
	key := keyPrefixTgStateCat + strconv.FormatInt(chatID, 10)
	val, err := s.redis.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil // no active state
	}
	if err != nil {
		return nil, err
	}
	var state CategoryState
	if err := json.Unmarshal([]byte(val), &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func (s *telegramService) deleteCategoryState(ctx context.Context, chatID int64) {
	key := keyPrefixTgStateCat + strconv.FormatInt(chatID, 10)
	s.redis.Del(ctx, key)
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

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🔘 Lainnya / Tanpa Kategori", fmt.Sprintf("tx_cat:%s:none", redisKey)),
	))
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("➕ Tambah Kategori Baru", fmt.Sprintf("tx_new:%s", redisKey)),
	))

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

	// Cache transaction to Redis (whether they have categories or not).
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
		msg := tgbotapi.NewMessage(chatID, "❌ Gagal menyimpan data sementara. Coba lagi.")
		msg.ParseMode = "HTML"
		if s.bot != nil {
			s.bot.Send(msg)
		}
		return err
	}

	keyboard := buildCategoryKeyboard(categories, key)
	var replyText string

	if len(categories) == 0 {
		replyText = fmt.Sprintf(
			"✅ Sip! %s buat <b>%s</b> tercatat sementara.\n\n"+
				"Kamu belum punya kategori nih. Buat dulu yuk dengan klik tombol di bawah atau ketik /tambah_category.",
			formatRupiah(parsed.Amount),
			parsed.Merchant,
		)
	} else {
		replyText = fmt.Sprintf(
			"✅ Sip! %s buat <b>%s</b>.\nMasuk kategori mana?",
			formatRupiah(parsed.Amount),
			parsed.Merchant,
		)
	}

	msg := tgbotapi.NewMessage(chatID, replyText)
	msg.ParseMode = "HTML"
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
	if parsed := utils.ParseTransactionMessage(text); parsed != nil {
		return s.handleQuickTransaction(ctx, chatID, parsed)
	}

	// ── Fallback ──────────────────────────────────────────────────────────────
	s.sendMessage(chatID, "Ketik transaksi kamu, contoh: \"Makan warteg 20k\", atau gunakan /tambah_category untuk membuat kategori baru.")
	return nil
}

// ─── handleTambahCategoryCommand — Step 1: save name, ask for limit ────────────

func (s *telegramService) handleTambahCategoryCommand(ctx context.Context, chatID int64, text string) error {
	// Resolve user first.
	chatIDStr := strconv.FormatInt(chatID, 10)
	_, err := s.userRepo.FindByTelegramChatID(ctx, chatIDStr)
	if err != nil {
		s.sendMessage(chatID, "⚠️ Akun kamu belum terhubung dengan WalletX. Kirim /start untuk menghubungkan.")
		return nil
	}

	// Extract category name: everything after "/tambah_category ".
	parts := strings.SplitN(text, " ", 2)
	if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
		// Send as HTML so the code tags render.
		msg := tgbotapi.NewMessage(chatID,
			"Format: <code>/tambah_category [nama kategori]</code>\n"+
				"Contoh: <code>/tambah_category Jajan</code>")
		msg.ParseMode = "HTML"
		if s.bot != nil {
			s.bot.Send(msg)
		}
		return nil
	}

	categoryName := strings.TrimSpace(parts[1])

	// Persist state in Redis.
	if err := s.saveCategoryState(ctx, chatID, CategoryState{Step: "AWAITING_LIMIT", CategoryName: categoryName}); err != nil {
		logrus.WithError(err).Error("Failed to save category state")
		msg := tgbotapi.NewMessage(chatID, "❌ Gagal menyimpan state. Coba lagi nanti.")
		msg.ParseMode = "HTML"
		if s.bot != nil {
			s.bot.Send(msg)
		}
		return err
	}

	reply := fmt.Sprintf(
		"Sip! Kategori '<b>%s</b>' akan dibuat. "+
			"Berapa limit bulanan untuk kategori ini?\n\n"+
			"(Balas dengan angka seperti <code>500k</code> / <code>500.000</code>, "+
			"atau balas <b>Tidak</b> jika tanpa limit)",
		categoryName,
	)
	msg := tgbotapi.NewMessage(chatID, reply)
	msg.ParseMode = "HTML"
	if s.bot != nil {
		if _, err := s.bot.Send(msg); err != nil {
			logrus.WithError(err).Error("Failed to send tambah_category step-1 reply")
			return err
		}
	}
	return nil
}

// ─── handleCategoryLimitReply — Step 2: parse limit, create records ────────────

func (s *telegramService) handleCategoryLimitReply(ctx context.Context, chatID int64, text string, catState *CategoryState) error {
	chatIDStr := strconv.FormatInt(chatID, 10)
	user, err := s.userRepo.FindByTelegramChatID(ctx, chatIDStr)
	if err != nil {
		s.deleteCategoryState(ctx, chatID)
		msg := tgbotapi.NewMessage(chatID, "⚠️ Akun kamu belum terhubung dengan WalletX. Kirim /start untuk menghubungkan.")
		msg.ParseMode = "HTML"
		if s.bot != nil {
			s.bot.Send(msg)
		}
		return nil
	}

	if catState.Step == "AWAITING_NAME" {
		categoryName := strings.TrimSpace(text)
		if categoryName == "" {
			msg := tgbotapi.NewMessage(chatID, "Nama kategori tidak boleh kosong. Silakan ketik nama kategorinya.")
			msg.ParseMode = "HTML"
			if s.bot != nil {
				s.bot.Send(msg)
			}
			return nil
		}
		catState.Step = "AWAITING_LIMIT"
		catState.CategoryName = categoryName
		if err := s.saveCategoryState(ctx, chatID, *catState); err != nil {
			s.deleteCategoryState(ctx, chatID)
			return err
		}

		reply := fmt.Sprintf(
			"Sip! Kategori '<b>%s</b>' akan dibuat. "+
				"Berapa limit bulanan untuk kategori ini?\n\n"+
				"(Balas dengan angka seperti <code>500k</code> / <code>500.000</code>, "+
				"atau balas <b>Tidak</b> jika tanpa limit)",
			categoryName,
		)
		msg := tgbotapi.NewMessage(chatID, reply)
		msg.ParseMode = "HTML"
		if s.bot != nil {
			s.bot.Send(msg)
		}
		return nil
	}

	// Always clear state on exit (success or cancel).
	defer s.deleteCategoryState(ctx, chatID)

	// Determine intent: "tidak" → no limit, number → set limit, else → invalid.
	normalised := strings.TrimSpace(strings.ToLower(text))

	var limitAmount *float64

	if normalised == "tidak" {
		// No limit — limitAmount stays nil.
	} else if amount, ok := utils.ParseAmount(text); ok {
		limitAmount = &amount
	} else {
		// Invalid input: inform the user and restore state so they can retry.
		s.saveCategoryState(ctx, chatID, *catState) // re-save (defer deleted it)
		msg := tgbotapi.NewMessage(chatID,
			"Input tidak valid. Balas dengan angka (contoh: <code>500k</code>) atau ketik <b>Tidak</b> jika tanpa limit.")
		msg.ParseMode = "HTML"
		if s.bot != nil {
			s.bot.Send(msg)
		}
		return nil
	}

	// 1. Create Category.
	category := &models.Category{
		UserID: user.ID,
		Name:   catState.CategoryName,
	}
	if err := s.categoryRepo.Create(ctx, category); err != nil {
		logrus.WithError(err).Error("Failed to create category via telegram")
		msg := tgbotapi.NewMessage(chatID, "❌ Gagal membuat kategori. Mungkin nama sudah dipakai. Coba lagi.")
		msg.ParseMode = "HTML"
		if s.bot != nil {
			s.bot.Send(msg)
		}
		return err
	}

	// 2. Optionally create CategoryLimit.
	if limitAmount != nil {
		limit := &models.CategoryLimit{
			UserID:      user.ID,
			CategoryID:  category.ID,
			LimitAmount: *limitAmount,
			Period:      "monthly",
			IsActive:    true,
		}
		if err := s.budgetRepo.Create(ctx, limit); err != nil {
			logrus.WithError(err).Error("Failed to create category limit via telegram")
			msg := tgbotapi.NewMessage(chatID, "⚠️ Kategori berhasil dibuat tapi gagal menyimpan limit. Atur limit di aplikasi WalletX ya.")
			msg.ParseMode = "HTML"
			if s.bot != nil {
				s.bot.Send(msg)
			}
			return nil // category created; treat as partial success
		}
	}

	// 3. Link Pending Transaction if exists.
	var linkedTxText string
	if catState.PendingTxKey != "" {
		fullRedisKey := keyPrefixTgPendingTx + catState.PendingTxKey
		dataStr, err := s.redis.Get(ctx, fullRedisKey).Result()
		if err == nil {
			var pending PendingTxData
			if err := json.Unmarshal([]byte(dataStr), &pending); err == nil {
				tx := &models.Transaction{
					UserID:          user.ID,
					CategoryID:      &category.ID,
					Amount:          pending.Amount,
					Merchant:        pending.Merchant,
					TransactionDate: time.Now(),
				}
				if err := s.transactionRepo.Create(ctx, tx); err == nil {
					linkedTxText = fmt.Sprintf("\n✅ Transaksi <b>%s</b> sebesar <b>%s</b> berhasil dicatat ke kategori ini.", pending.Merchant, formatRupiah(pending.Amount))
				}
				s.redis.Del(ctx, fullRedisKey)
			}
		}
	}

	// 4. Send success reply.
	var successText string
	if limitAmount != nil {
		successText = fmt.Sprintf(
			"✅ Kategori '<b>%s</b>' berhasil dibuat dengan limit bulanan <b>%s</b>! 🎉%s",
			catState.CategoryName,
			formatRupiah(*limitAmount),
			linkedTxText,
		)
	} else {
		successText = fmt.Sprintf(
			"✅ Kategori '<b>%s</b>' berhasil dibuat tanpa limit bulanan. 🎉%s",
			catState.CategoryName,
			linkedTxText,
		)
	}

	msg := tgbotapi.NewMessage(chatID, successText)
	msg.ParseMode = "HTML"
	if s.bot != nil {
		if _, err := s.bot.Send(msg); err != nil {
			logrus.WithError(err).Error("Failed to send category creation success")
			return err
		}
	}
	return nil
}

// ─── handleLimitCommand returns the real-time budget report for the current month ─

// handleLimitCommand fetches active category limits and current-month spending
// directly from PostgreSQL (no cache) and replies with a formatted report.
func (s *telegramService) handleLimitCommand(ctx context.Context, chatID int64) error {
	// 1. Resolve user.
	chatIDStr := strconv.FormatInt(chatID, 10)
	user, err := s.userRepo.FindByTelegramChatID(ctx, chatIDStr)
	if err != nil {
		s.sendMessage(chatID, "⚠️ Akun kamu belum terhubung dengan WalletX. Kirim /start untuk menghubungkan.")
		return nil
	}

	// 2. Fetch limit report from DB (single JOIN query — no Redis).
	reports, err := s.budgetRepo.GetLimitReportByUserID(ctx, user.ID)
	if err != nil {
		logrus.WithError(err).Error("Failed to fetch limit report")
		s.sendMessage(chatID, "❌ Gagal mengambil data limit. Coba lagi nanti.")
		return err
	}

	// 3. Handle empty state.
	if len(reports) == 0 {
		s.sendMessage(chatID,
			"Kamu belum mengatur limit kategori apa pun nih. "+
				"Yuk atur dulu di aplikasi WalletX biar keuanganmu lebih rapi! 💸",
		)
		return nil
	}

	// 4. Build reply.
	reply := s.buildLimitReport(reports)

	msg := tgbotapi.NewMessage(chatID, reply)
	msg.ParseMode = "HTML"
	if s.bot != nil {
		if _, err := s.bot.Send(msg); err != nil {
			logrus.WithError(err).Error("Failed to send limit report message")
			return err
		}
	}
	return nil
}

// buildLimitReport formats a slice of LimitReport rows into a Telegram HTML string.
func (s *telegramService) buildLimitReport(reports []report.LimitReport) string {
	var sb strings.Builder
	sb.WriteString("📊 <b>Laporan Limit Bulan Ini:</b>\n\n")

	for _, r := range reports {
		remaining := r.LimitAmount - r.TotalSpent

		// Label: icon (if any) + bold category name.
		label := r.CategoryName
		if r.CategoryIcon != "" {
			label = r.CategoryIcon + " " + r.CategoryName
		}

		// Spent / Limit line.
		sb.WriteString(fmt.Sprintf("%s: %s / %s\n",
			label,
			formatRupiah(r.TotalSpent),
			formatRupiah(r.LimitAmount),
		))

		// Remaining / Over-budget line with colour indicator.
		if remaining >= 0 {
			sb.WriteString(fmt.Sprintf("(Sisa: %s) 🟢\n\n",
				formatRupiah(remaining),
			))
		} else {
			sb.WriteString(fmt.Sprintf("(⚠️ OVER BUDGET: %s) 🔴\n\n",
				formatRupiah(math.Abs(remaining)),
			))
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}

// ─── HandleCallbackQuery intercepts inline keyboard clicks ────────────────────

func (s *telegramService) HandleCallbackQuery(query *tgbotapi.CallbackQuery) error {
	ctx := context.Background()
	chatID := query.Message.Chat.ID
	messageID := query.Message.MessageID

	// Extract data
	data := query.Data
	parts := strings.Split(data, ":")
	if len(parts) < 2 {
		return nil
	}

	if parts[0] == "tx_new" {
		redisKeyStr := parts[1]
		// Save state
		if err := s.saveCategoryState(ctx, chatID, CategoryState{
			Step:         "AWAITING_NAME",
			PendingTxKey: redisKeyStr,
		}); err != nil {
			logrus.WithError(err).Error("Failed to save state for new category via inline button")
			s.bot.Request(tgbotapi.NewCallbackWithAlert(query.ID, "Gagal memproses. Coba lagi nanti."))
			return err
		}
		// Answer callback
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
		// Answer callback with alert
		callback := tgbotapi.NewCallbackWithAlert(query.ID, "Waktu habis, silakan ketik ulang transaksinya.")
		s.bot.Request(callback)
		return nil
	} else if err != nil {
		return err
	}

	// Answer callback to stop loading spinner
	s.bot.Request(tgbotapi.NewCallback(query.ID, ""))

	var pending PendingTxData
	if err := json.Unmarshal([]byte(dataStr), &pending); err != nil {
		return err
	}

	// Look up user
	chatIDStr := strconv.FormatInt(chatID, 10)
	user, err := s.userRepo.FindByTelegramChatID(ctx, chatIDStr)
	if err != nil {
		return err
	}

	// Insert transaction
	tx := &models.Transaction{
		UserID:          user.ID,
		CategoryID:      categoryID,
		Amount:          pending.Amount,
		Merchant:        pending.Merchant,
		TransactionDate: time.Now(),
	}

	if err := s.transactionRepo.Create(ctx, tx); err != nil {
		return err
	}

	// Clean up redis
	s.redis.Del(ctx, fullRedisKey)

	// Check limit
	replyText := fmt.Sprintf("✅ <b>%s</b> untuk <b>%s</b> berhasil dicatat.", formatRupiah(pending.Amount), pending.Merchant)

	if categoryID != nil {
		limit, err := s.budgetRepo.FindByCategory(ctx, user.ID, *categoryID)
		if err == nil && limit != nil { // limit exists
			now := time.Now()
			totalSpent, err := s.transactionRepo.GetTotalSpentByCategoryThisMonth(ctx, user.ID, *categoryID, int(now.Month()), now.Year())
			if err == nil {
				remaining := limit.LimitAmount - totalSpent
				if remaining >= 0 {
					replyText += fmt.Sprintf("\n\nSisa budget kategori ini: %s 🟢", formatRupiah(remaining))
				} else {
					replyText += fmt.Sprintf("\n\n⚠️ OVER BUDGET! Kamu melebihi limit sebesar %s 🔴", formatRupiah(math.Abs(remaining)))
				}
			}
		}
	} else {
		replyText += " (Tanpa Kategori)"
	}

	// Update original message
	editMsg := tgbotapi.NewEditMessageTextAndMarkup(chatID, messageID, replyText, tgbotapi.InlineKeyboardMarkup{InlineKeyboard: make([][]tgbotapi.InlineKeyboardButton, 0)})
	editMsg.ParseMode = "HTML"
	if _, err := s.bot.Send(editMsg); err != nil {
		logrus.WithError(err).Error("Failed to edit telegram message")
	}

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
