package middleware

import (
	"net/http"
	"strings"

	"walletx-be/internal/shared/response"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// CronAuthMiddleware validates the shared cron secret on cron-triggered endpoints.
func CronAuthMiddleware(cronSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if cronSecret == "" {
			logrus.Error("[CronAuth] CRON_SECRET is not configured")
			response.FailWithStatus(c, http.StatusInternalServerError, "Cron secret not configured")
			c.Abort()
			return
		}

		var providedSecret string

		authHeader := c.GetHeader("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			providedSecret = strings.TrimPrefix(authHeader, "Bearer ")
		}

		if providedSecret == "" {
			providedSecret = c.GetHeader("x-cron-secret")
		}

		if providedSecret == "" || providedSecret != cronSecret {
			logrus.Warn("[CronAuth] Unauthorized access attempt to cron endpoint")
			response.FailWithStatus(c, http.StatusUnauthorized, "Unauthorized")
			c.Abort()
			return
		}

		c.Next()
	}
}
