package cron

import (
	"context"

	"walletx-be/internal/shared/response"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// RecurringProcessor is the capability that runs the recurring transaction job.
type RecurringProcessor interface {
	ProcessDue(ctx context.Context) error
}

type Handler struct {
	healthRepo         HealthRepository
	imapSyncer         IMAPSyncer
	recurringProcessor RecurringProcessor
}

func NewHandler(healthRepo HealthRepository, imapSyncer IMAPSyncer, recurringProcessor RecurringProcessor) *Handler {
	return &Handler{
		healthRepo:         healthRepo,
		imapSyncer:         imapSyncer,
		recurringProcessor: recurringProcessor,
	}
}

func (h *Handler) KeepAlive(c *gin.Context) {
	if err := h.healthRepo.Ping(c.Request.Context()); err != nil {
		logrus.WithError(err).Error("[Cron] Keep-alive query failed")
		response.Error(c, "Database keep-alive failed")
		return
	}

	logrus.Info("[Cron] Keep-alive query executed successfully")
	response.Success(c, nil, "Keep-alive successful")
}

func (h *Handler) IMAPSync(c *gin.Context) {
	logrus.Info("[Cron] Starting IMAP Email Sync...")

	if err := h.imapSyncer.ProcessUnseenEmails(c.Request.Context()); err != nil {
		logrus.WithError(err).Error("[Cron] IMAP Sync failed")
		response.Error(c, "IMAP sync failed")
		return
	}

	logrus.Info("[Cron] IMAP Sync completed")
	response.Success(c, nil, "IMAP sync executed successfully")
}

func (h *Handler) RecurringSync(c *gin.Context) {
	logrus.Info("[Cron] Starting Recurring Processor...")

	if err := h.recurringProcessor.ProcessDue(c.Request.Context()); err != nil {
		logrus.WithError(err).Error("[Cron] Recurring processing failed")
		response.Error(c, "Recurring processing failed")
		return
	}

	logrus.Info("[Cron] Recurring processing completed")
	response.Success(c, nil, "Recurring processing executed successfully")
}
