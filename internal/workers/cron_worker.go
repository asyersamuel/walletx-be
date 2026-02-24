package workers

import (
	"time"

	"github.com/robfig/cron/v3"
	"github.com/sirupsen/logrus"
)

// SetupCronJobs initializes and starts the background task scheduler
func SetupCronJobs(imapWorker *IMAPWorker) *cron.Cron {
	// Use Western Indonesia Time (WIB)
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		logrus.WithError(err).Warn("⚠️ [Cron] Failed to load Asia/Jakarta timezone, falling back to local time")
		loc = time.Local
	}

	// Initialize cron with location support
	c := cron.New(cron.WithLocation(loc))

	// Define the cron expression (00:00 every day)
	cronExpr := "0 0 * * *"

	// Add the job
	entryID, err := c.AddFunc(cronExpr, func() {
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
		logrus.WithError(err).Fatal("❌ [Cron] Failed to register cron job")
	}

	// Log the status and next execution time
	entry := c.Entry(entryID)
	logrus.WithFields(logrus.Fields{
		"schedule":      cronExpr,
		"timezone":      loc.String(),
		"next_run":      entry.Next.Format("2006-01-02 15:04:05"),
		"status":        "ACTIVE",
	}).Info("📅 [Cron] Scheduler is running and monitoring tasks")

	return c
}