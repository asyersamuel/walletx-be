package cron

import (
	"walletx-be/internal/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers the cron-triggered endpoints protected by the cron secret.
func RegisterRoutes(r *gin.RouterGroup, h *Handler, cronSecret string) {
	group := r.Group("/cron")
	group.Use(middleware.CronAuthMiddleware(cronSecret))
	{
		group.GET("/keep-alive", h.KeepAlive)
		group.GET("/imap", h.IMAPSync)
		group.GET("/recurring", h.RecurringSync)
	}
}
