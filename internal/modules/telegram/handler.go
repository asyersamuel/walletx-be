package telegram

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

// HandleWebhook receives updates from Telegram.
func (h *Handler) HandleWebhook(c *gin.Context) {
	var update tgbotapi.Update
	if err := c.ShouldBindJSON(&update); err != nil {
		logrus.WithError(err).Error("Failed to parse Telegram webhook payload")
		// Always return 200 to Telegram to prevent retries for bad payloads
		c.Status(http.StatusOK)
		return
	}

	err := h.service.HandleWebhook(update)
	if err != nil {
		logrus.WithError(err).Error("Error handling Telegram webhook")
	}

	// Always return 200 OK
	c.Status(http.StatusOK)
}

// VerifyToken handles the verification link clicked by the user in their email.
func (h *Handler) VerifyToken(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(`
			<html><body><h2>Error</h2><p>Token tidak ditemukan.</p></body></html>
		`))
		return
	}

	err := h.service.VerifyToken(token)
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
