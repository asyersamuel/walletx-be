package middleware

import (
	"time"

	platformlogger "walletx-be/internal/platform/logger"

	"github.com/gin-gonic/gin"
)

// RequestLogger records one structured event after every HTTP request.
func RequestLogger(appLogger platformlogger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		startedAt := time.Now()
		c.Next()

		fields := map[string]interface{}{
			"method":     c.Request.Method,
			"path":       c.Request.URL.Path,
			"status":     c.Writer.Status(),
			"latency_ms": float64(time.Since(startedAt).Microseconds()) / 1000,
			"client_ip":  c.ClientIP(),
		}

		entry := appLogger.WithFields(fields)
		if len(c.Errors) > 0 {
			entry = entry.WithField("errors", c.Errors.String())
		}

		switch {
		case c.Writer.Status() >= 500:
			entry.Error("HTTP request completed")
		case c.Writer.Status() >= 400:
			entry.Warn("HTTP request completed")
		default:
			entry.Info("HTTP request completed")
		}
	}
}
