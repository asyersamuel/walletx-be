package handlers

import (
	"walletx-be/pkg/services"
	"walletx-be/pkg/utils"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type CronHandler struct {
	db               *gorm.DB
	imapService      *services.IMAPService
	recurringService services.RecurringService
}

func NewCronHandler(db *gorm.DB, imapService *services.IMAPService, recurringService services.RecurringService) *CronHandler {
	return &CronHandler{
		db:               db,
		imapService:      imapService,
		recurringService: recurringService,
	}
}

// KeepAlive GET /api/v1/cron/keep-alive
// Performs a lightweight DB query to prevent Supabase free-tier pausing.
func (h *CronHandler) KeepAlive(c *gin.Context) {
	if err := h.db.Exec("SELECT 1").Error; err != nil {
		logrus.WithError(err).Error("[Cron] Keep-alive query failed")
		utils.ErrorResponse(c, "Database keep-alive failed")
		return
	}

	logrus.Info("[Cron] Keep-alive query executed successfully")
	utils.SuccessResponse(c, nil, "Keep-alive successful")
}

// IMAPSync POST /api/v1/cron/imap
// Triggers the IMAP email extraction worker.
func (h *CronHandler) IMAPSync(c *gin.Context) {
	logrus.Info("[Cron] Starting IMAP Email Sync...")

	if err := h.imapService.ProcessUnseenEmails(); err != nil {
		logrus.WithError(err).Error("[Cron] IMAP Sync failed")
		utils.ErrorResponse(c, "IMAP sync failed")
		return
	}

	logrus.Info("[Cron] IMAP Sync completed")
	utils.SuccessResponse(c, nil, "IMAP sync executed successfully")
}

// RecurringSync POST /api/v1/cron/recurring
// Triggers the recurring transaction processor.
func (h *CronHandler) RecurringSync(c *gin.Context) {
	logrus.Info("[Cron] Starting Recurring Processor...")

	if err := h.recurringService.ProcessDueRecurrings(); err != nil {
		logrus.WithError(err).Error("[Cron] Recurring processing failed")
		utils.ErrorResponse(c, "Recurring processing failed")
		return
	}

	logrus.Info("[Cron] Recurring processing completed")
	utils.SuccessResponse(c, nil, "Recurring processing executed successfully")
}
