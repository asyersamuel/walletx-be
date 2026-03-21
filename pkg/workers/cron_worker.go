package workers

import (
	"time"

	"github.com/robfig/cron/v3"
	"github.com/sirupsen/logrus"
)

// SetupCronJobs initializes and starts the background task scheduler
func SetupCronJobs(imapWorker *IMAPWorker, recurringWorker *RecurringWorker) *cron.Cron {
	// Use Western Indonesia Time (WIB)
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		logrus.WithError(err).Warn("⚠️ [Cron] Failed to load Asia/Jakarta timezone, falling back to local time")
		loc = time.Local
	}

	// Initialize cron with location support
	c := cron.New(cron.WithLocation(loc))

	// --- Job 1: IMAP Email Sync (runs at 00:00 WIB every day) ---
	imapCronExpr := "0 0 * * *"
	imapEntryID, err := c.AddFunc(imapCronExpr, func() {
		logrus.WithFields(logrus.Fields{
			"execution_time": time.Now().In(loc).Format("2006-01-02 15:04:05"),
			"task":           "IMAP_Sync",
		}).Info("⏰ [Cron] Starting scheduled task: Extracting transactions from email...")

		if err := imapWorker.ProcessUnseenEmails(); err != nil {
			logrus.WithError(err).Error("❌ [Cron] IMAP Worker execution failed")
		} else {
			logrus.Info("✅ [Cron] Scheduled IMAP Worker task completed successfully")
		}
	})

	if err != nil {
		logrus.WithError(err).Fatal("❌ [Cron] Failed to register IMAP cron job")
	}

	imapEntry := c.Entry(imapEntryID)
	logrus.WithFields(logrus.Fields{
		"schedule": imapCronExpr,
		"timezone": loc.String(),
		"next_run": imapEntry.Next.Format("2006-01-02 15:04:05"),
		"status":   "ACTIVE",
	}).Info("📅 [Cron] IMAP Scheduler registered")

	// --- Job 2: Recurring Transaction Processor (runs at 01:00 WIB every day) ---
	recurringCronExpr := "0 1 * * *"
	recurringEntryID, err := c.AddFunc(recurringCronExpr, func() {
		logrus.WithFields(logrus.Fields{
			"execution_time": time.Now().In(loc).Format("2006-01-02 15:04:05"),
			"task":           "Recurring_Processor",
		}).Info("⏰ [Cron] Starting scheduled task: Processing due recurring transactions...")

		if err := recurringWorker.ProcessDueRecurring(); err != nil {
			logrus.WithError(err).Error("❌ [Cron] Recurring Worker execution failed")
		} else {
			logrus.Info("✅ [Cron] Scheduled Recurring Worker task completed successfully")
		}
	})

	if err != nil {
		logrus.WithError(err).Fatal("❌ [Cron] Failed to register Recurring cron job")
	}

	recurringEntry := c.Entry(recurringEntryID)
	logrus.WithFields(logrus.Fields{
		"schedule": recurringCronExpr,
		"timezone": loc.String(),
		"next_run": recurringEntry.Next.Format("2006-01-02 15:04:05"),
		"status":   "ACTIVE",
	}).Info("📅 [Cron] Recurring Scheduler registered")

	return c
}