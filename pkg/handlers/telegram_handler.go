package handlers

import (
	"net/http"

	"walletx-be/pkg/services"

	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
)

type TelegramHandler struct {
	telegramService services.TelegramService
}

func NewTelegramHandler(telegramService services.TelegramService) *TelegramHandler {
	return &TelegramHandler{
		telegramService: telegramService,
	}
}

// HandleWebhook receives updates from Telegram
func (h *TelegramHandler) HandleWebhook(c *gin.Context) {
	var update tgbotapi.Update
	if err := c.ShouldBindJSON(&update); err != nil {
		logrus.WithError(err).Error("Failed to parse Telegram webhook payload")
		// Always return 200 to Telegram to prevent retries for bad payloads
		c.Status(http.StatusOK)
		return
	}

	err := h.telegramService.HandleWebhook(update)
	if err != nil {
		logrus.WithError(err).Error("Error handling Telegram webhook")
	}

	// Always return 200 OK
	c.Status(http.StatusOK)
}

// VerifyToken handles the verification link clicked by the user in their email
func (h *TelegramHandler) VerifyToken(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(`
			<html><body><h2>Error</h2><p>Token tidak ditemukan.</p></body></html>
		`))
		return
	}

	err := h.telegramService.VerifyToken(token)
	if err != nil {
		logrus.WithError(err).Error("Failed to verify telegram token")
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(`
			<html><body><h2>Verifikasi Gagal</h2><p>Tautan tidak valid atau sudah kedaluwarsa.</p></body></html>
		`))
		return
	}

	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(`
		<html>
			<body style="font-family: Arial, sans-serif; text-align: center; padding: 50px;">
				<h2 style="color: #4CAF50;">Verifikasi Berhasil!</h2>
				<p>Akun Telegram Anda telah berhasil terhubung dengan WalletX.</p>
				<p>Silakan kembali ke Telegram.</p>
			</body>
		</html>
	`))
}
