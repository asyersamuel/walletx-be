package handlers

import (
	"context"
	"walletx-be/pkg/repository"
	"walletx-be/pkg/services"
	"walletx-be/pkg/utils"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

type IMAPService interface {
	ProcessUnseenEmails(ctx context.Context) error
}

type CronHandler struct {
	healthRepo       repository.HealthRepository
	imapService      IMAPService
	recurringService services.RecurringService
}

func NewCronHandler(healthRepo repository.HealthRepository, imapService IMAPService, recurringService services.RecurringService) *CronHandler {
	return &CronHandler{
		healthRepo:       healthRepo,
		imapService:      imapService,
		recurringService: recurringService,
	}
}

func (h *CronHandler) KeepAlive(c *gin.Context) {
	if err := h.healthRepo.Ping(); err != nil {
		logrus.WithError(err).Error("[Cron] Keep-alive query failed")
		utils.ErrorResponse(c, "Database keep-alive failed")
		return
	}

	logrus.Info("[Cron] Keep-alive query executed successfully")
	utils.SuccessResponse(c, nil, "Keep-alive successful")
}

func (h *CronHandler) IMAPSync(c *gin.Context) {
	logrus.Info("[Cron] Starting IMAP Email Sync...")

	if err := h.imapService.ProcessUnseenEmails(c.Request.Context()); err != nil {
		logrus.WithError(err).Error("[Cron] IMAP Sync failed")
		utils.ErrorResponse(c, "IMAP sync failed")
		return
	}

	logrus.Info("[Cron] IMAP Sync completed")
	utils.SuccessResponse(c, nil, "IMAP sync executed successfully")
}

func (h *CronHandler) RecurringSync(c *gin.Context) {
	logrus.Info("[Cron] Starting Recurring Processor...")

	if err := h.recurringService.ProcessDueRecurrings(c.Request.Context()); err != nil {
		logrus.WithError(err).Error("[Cron] Recurring processing failed")
		utils.ErrorResponse(c, "Recurring processing failed")
		return
	}

	logrus.Info("[Cron] Recurring processing completed")
	utils.SuccessResponse(c, nil, "Recurring processing executed successfully")
}
