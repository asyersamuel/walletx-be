package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/mail"
	"strconv"
	"time"

	"walletx-be/configs"
	"walletx-be/pkg/repository"
	"walletx-be/pkg/utils"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type TelegramService interface {
	HandleWebhook(update tgbotapi.Update) error
	VerifyToken(token string) error
}

type telegramService struct {
	bot       *tgbotapi.BotAPI
	redis     *redis.Client
	userRepo  repository.UserRepository
	emailSvc  *utils.EmailService
	appConfig configs.AppConfig
}

type VerifyData struct {
	Email  string `json:"email"`
	ChatID int64  `json:"chat_id"`
}

func NewTelegramService(cfg *configs.Config, rdb *redis.Client, userRepo repository.UserRepository, emailSvc *utils.EmailService) TelegramService {
	var bot *tgbotapi.BotAPI
	var err error
	if cfg.Telegram.BotToken != "" {
		bot, err = tgbotapi.NewBotAPI(cfg.Telegram.BotToken)
		if err != nil {
			logrus.WithError(err).Warn("Failed to initialize Telegram bot API")
		}
	}

	return &telegramService{
		bot:       bot,
		redis:     rdb,
		userRepo:  userRepo,
		emailSvc:  emailSvc,
		appConfig: cfg.App,
	}
}

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

func (s *telegramService) HandleWebhook(update tgbotapi.Update) error {
	if s.bot == nil || s.redis == nil {
		return fmt.Errorf("service not properly initialized (bot or redis is nil)")
	}

	// We only care about message updates
	if update.Message == nil {
		return nil
	}

	chatID := update.Message.Chat.ID
	text := update.Message.Text
	ctx := context.Background()

	stateKey := fmt.Sprintf("tg_state:%d", chatID)

	if text == "/start" {
		// Set state to awaiting email
		err := s.redis.Set(ctx, stateKey, "awaiting_email", 5*time.Minute).Err()
		if err != nil {
			logrus.WithError(err).Error("Failed to set redis state")
			return err
		}

		s.sendMessage(chatID, "Halo! Silakan ketik email yang terdaftar di akun WalletX Anda.")
		return nil
	}

	// Check state
	state, err := s.redis.Get(ctx, stateKey).Result()
	if err == redis.Nil {
		// State not found
		s.sendMessage(chatID, "Silakan kirim /start untuk memulai menghubungkan akun.")
		return nil
	} else if err != nil {
		logrus.WithError(err).Error("Failed to get redis state")
		return err
	}

	if state == "awaiting_email" {
		// Validate email
		_, err := mail.ParseAddress(text)
		if err != nil {
			s.sendMessage(chatID, "Format email tidak valid. Silakan ketik ulang email Anda dengan benar.")
			return nil
		}

		email := text

		// Check if user exists
		user, err := s.userRepo.FindByEmail(email)
		if err != nil || user == nil {
			s.sendMessage(chatID, "Email tidak terdaftar di WalletX. Silakan periksa kembali email Anda.")
			return nil
		}

		// Generate verification token
		token := uuid.New().String()
		verifyKey := fmt.Sprintf("tg_verify:%s", token)

		data := VerifyData{
			Email:  email,
			ChatID: chatID,
		}
		dataBytes, _ := json.Marshal(data)

		// Save to redis for 15 mins
		err = s.redis.Set(ctx, verifyKey, dataBytes, 15*time.Minute).Err()
		if err != nil {
			logrus.WithError(err).Error("Failed to set verify token to redis")
			return err
		}

		// Delete state
		s.redis.Del(ctx, stateKey)

		// Send email
		verifyURL := fmt.Sprintf("%s/api/v1/telegram/verify?token=%s", s.appConfig.URL, token)
		err = s.emailSvc.SendVerificationEmail(email, verifyURL)
		if err != nil {
			logrus.WithError(err).Error("Failed to send verification email")
			s.sendMessage(chatID, "Gagal mengirim email verifikasi. Silakan coba lagi nanti.")
			return err
		}

		s.sendMessage(chatID, "Cek inbox email Anda! Klik link yang dikirimkan untuk konfirmasi.")
	}

	return nil
}

func (s *telegramService) VerifyToken(token string) error {
	if s.redis == nil {
		return fmt.Errorf("redis client is nil")
	}

	ctx := context.Background()
	verifyKey := fmt.Sprintf("tg_verify:%s", token)

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

	// Find user
	user, err := s.userRepo.FindByEmail(data.Email)
	if err != nil || user == nil {
		return fmt.Errorf("user not found")
	}

	// Update DB
	chatIDStr := strconv.FormatInt(data.ChatID, 10)
	user.TelegramChatID = &chatIDStr

	if err := s.userRepo.Update(user); err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	// Delete token
	s.redis.Del(ctx, verifyKey)

	// Send success message to Telegram
	s.sendMessage(data.ChatID, "Selamat! Akun Telegram Anda berhasil terhubung dengan WalletX.")

	return nil
}
