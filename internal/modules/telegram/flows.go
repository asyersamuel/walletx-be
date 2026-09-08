package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"walletx-be/internal/modules/budget"
	"walletx-be/internal/modules/category"
	"walletx-be/internal/modules/transaction"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (s *service) sendMessage(chatID int64, text string) {
	if s.bot == nil {
		logrus.Warn("Telegram Bot is not initialized, cannot send message")
		return
	}
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := s.bot.Send(msg); err != nil {
		logrus.WithError(err).Error("Failed to send Telegram message")
	}
}

func (s *service) sendMessageWithKeyboard(chatID int64, text string, keyboard tgbotapi.InlineKeyboardMarkup) {
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
func shortKey() string {
	id := uuid.New().String()
	return id[:8]
}

// ─── CategoryState Redis helpers ──────────────────────────────────────────────

func (s *service) saveCategoryState(ctx context.Context, chatID int64, state CategoryState) error {
	key := keyPrefixTgStateCat + strconv.FormatInt(chatID, 10)
	b, _ := json.Marshal(state)
	return s.redis.Set(ctx, key, b, ttlStateCat).Err()
}

func (s *service) getCategoryState(ctx context.Context, chatID int64) (*CategoryState, error) {
	key := keyPrefixTgStateCat + strconv.FormatInt(chatID, 10)
	val, err := s.redis.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
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

func (s *service) deleteCategoryState(ctx context.Context, chatID int64) {
	key := keyPrefixTgStateCat + strconv.FormatInt(chatID, 10)
	s.redis.Del(ctx, key)
}

// buildCategoryKeyboard constructs an InlineKeyboardMarkup from a category list.
func buildCategoryKeyboard(categories []category.Category, redisKey string) tgbotapi.InlineKeyboardMarkup {
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

func (s *service) handleQuickTransaction(ctx context.Context, chatID int64, parsed *ParsedTransaction) error {
	chatIDStr := strconv.FormatInt(chatID, 10)
	user, err := s.userStore.FindByTelegramChatID(ctx, chatIDStr)
	if err != nil {
		s.sendMessage(chatID, "⚠️ Akun kamu belum terhubung dengan WalletX. Kirim /start untuk menghubungkan.")
		return nil
	}

	categories, err := s.categoryStore.List(ctx, user.ID)
	if err != nil {
		logrus.WithError(err).Error("Failed to fetch user categories")
		s.sendMessage(chatID, "❌ Gagal mengambil data kategori. Coba lagi nanti.")
		return err
	}

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

// ─── handleTambahCategoryCommand — Step 1: save name, ask for limit ────────────

func (s *service) handleTambahCategoryCommand(ctx context.Context, chatID int64, text string) error {
	chatIDStr := strconv.FormatInt(chatID, 10)
	_, err := s.userStore.FindByTelegramChatID(ctx, chatIDStr)
	if err != nil {
		s.sendMessage(chatID, "⚠️ Akun kamu belum terhubung dengan WalletX. Kirim /start untuk menghubungkan.")
		return nil
	}

	parts := strings.SplitN(text, " ", 2)
	if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
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

func (s *service) handleCategoryLimitReply(ctx context.Context, chatID int64, text string, catState *CategoryState) error {
	chatIDStr := strconv.FormatInt(chatID, 10)
	user, err := s.userStore.FindByTelegramChatID(ctx, chatIDStr)
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

	normalised := strings.TrimSpace(strings.ToLower(text))

	var limitAmount *float64

	if normalised == "tidak" {
		// No limit — limitAmount stays nil.
	} else if amount, ok := ParseAmount(text); ok {
		limitAmount = &amount
	} else {
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
	newCategory := &category.Category{
		UserID: user.ID,
		Name:   catState.CategoryName,
	}
	if err := s.categoryStore.Create(ctx, newCategory); err != nil {
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
		limit := &budget.CategoryLimit{
			UserID:      user.ID,
			CategoryID:  newCategory.ID,
			LimitAmount: *limitAmount,
			Period:      "monthly",
			IsActive:    true,
		}
		if err := s.budgetStore.Create(ctx, limit); err != nil {
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
				tx := &transaction.Transaction{
					UserID:          user.ID,
					CategoryID:      &newCategory.ID,
					Amount:          pending.Amount,
					Merchant:        pending.Merchant,
					TransactionDate: time.Now(),
				}
				if err := s.transactionStore.Create(ctx, tx); err == nil {
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

func (s *service) handleLimitCommand(ctx context.Context, chatID int64) error {
	chatIDStr := strconv.FormatInt(chatID, 10)
	user, err := s.userStore.FindByTelegramChatID(ctx, chatIDStr)
	if err != nil {
		s.sendMessage(chatID, "⚠️ Akun kamu belum terhubung dengan WalletX. Kirim /start untuk menghubungkan.")
		return nil
	}

	reports, err := s.budgetStore.GetLimitReportByUserID(ctx, user.ID)
	if err != nil {
		logrus.WithError(err).Error("Failed to fetch limit report")
		s.sendMessage(chatID, "❌ Gagal mengambil data limit. Coba lagi nanti.")
		return err
	}

	if len(reports) == 0 {
		s.sendMessage(chatID,
			"Kamu belum mengatur limit kategori apa pun nih. "+
				"Yuk atur dulu di aplikasi WalletX biar keuanganmu lebih rapi! 💸",
		)
		return nil
	}

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

func (s *service) buildLimitReport(reports []budget.LimitReport) string {
	var sb strings.Builder
	sb.WriteString("📊 <b>Laporan Limit Bulan Ini:</b>\n\n")

	for _, r := range reports {
		remaining := r.LimitAmount - r.TotalSpent

		label := r.CategoryName
		if r.CategoryIcon != "" {
			label = r.CategoryIcon + " " + r.CategoryName
		}

		sb.WriteString(fmt.Sprintf("%s: %s / %s\n",
			label,
			formatRupiah(r.TotalSpent),
			formatRupiah(r.LimitAmount),
		))

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

// ─── handleAccountLinking handles the email-based account-linking sub-flow ─────

func (s *service) handleAccountLinking(ctx context.Context, chatID int64, text, stateKey string) error {
	if _, err := mail.ParseAddress(text); err != nil {
		s.sendMessage(chatID, "Format email tidak valid. Silakan ketik ulang email Anda dengan benar.")
		return nil
	}

	email := text

	user, err := s.userStore.FindByEmail(ctx, email)
	if err != nil || user == nil {
		s.sendMessage(chatID, "Email tidak terdaftar di WalletX. Silakan periksa kembali email Anda.")
		return nil
	}

	token := uuid.New().String()
	verifyKey := keyPrefixTgVerify + token

	data := VerifyData{Email: email, ChatID: chatID}
	dataBytes, _ := json.Marshal(data)

	if err := s.redis.Set(ctx, verifyKey, dataBytes, ttlVerify).Err(); err != nil {
		logrus.WithError(err).Error("Failed to set verify token to redis")
		return err
	}

	s.redis.Del(ctx, stateKey)

	verifyURL := fmt.Sprintf("%s/api/v1/telegram/verify?token=%s", s.appConfig.URL, token)
	if err := s.emailSvc.SendVerificationEmail(email, verifyURL); err != nil {
		logrus.WithError(err).Error("Failed to send verification email")
		s.sendMessage(chatID, "Gagal mengirim email verifikasi. Silakan coba lagi nanti.")
		return err
	}

	s.sendMessage(chatID, "Cek inbox email Anda! Klik link yang dikirimkan untuk konfirmasi.")
	return nil
}
