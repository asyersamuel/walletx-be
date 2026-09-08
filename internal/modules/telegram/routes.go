package telegram

import "github.com/gin-gonic/gin"

// RegisterRoutes registers the Telegram webhook and email-verification
// endpoints. Both are intentionally public (no JWT required):
//   - POST /telegram/webhook  → receives updates pushed by Telegram servers
//   - GET  /telegram/verify   → handles the one-time email verification link
func RegisterRoutes(r *gin.RouterGroup, h *Handler) {
	group := r.Group("/telegram")
	{
		group.POST("/webhook", h.HandleWebhook)
		group.GET("/verify", h.VerifyToken)
	}
}
