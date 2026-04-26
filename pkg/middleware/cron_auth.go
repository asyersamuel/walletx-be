package middleware

import (
	"net/http"
	"os"
	"strings"

	"walletx-be/pkg/utils"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// CronAuthMiddleware protects cron endpoints by validating the secret token.
// It checks the "Authorization: Bearer <token>" header OR the "x-cron-secret" header
// against the CRON_SECRET environment variable.
func CronAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		expectedSecret := os.Getenv("CRON_SECRET")
		if expectedSecret == "" {
			logrus.Error("[CronAuth] CRON_SECRET environment variable is not set")
			utils.FailResponseWithStatus(c, http.StatusInternalServerError, "Cron secret not configured")
			c.Abort()
			return
		}

		var providedSecret string

		// 1. Check Authorization: Bearer <secret>
		authHeader := c.GetHeader("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			providedSecret = strings.TrimPrefix(authHeader, "Bearer ")
		}

		// 2. Fallback to x-cron-secret header
		if providedSecret == "" {
			providedSecret = c.GetHeader("x-cron-secret")
		}

		if providedSecret == "" || providedSecret != expectedSecret {
			logrus.Warn("[CronAuth] Unauthorized access attempt to cron endpoint")
			utils.FailResponseWithStatus(c, http.StatusUnauthorized, "Unauthorized")
			c.Abort()
			return
		}

		c.Next()
	}
}
