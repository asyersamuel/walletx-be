package middleware

import (
	"net/http"
	"strings"

	"walletx-be/pkg/utils"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func CronAuthMiddleware(cronSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if cronSecret == "" {
			logrus.Error("[CronAuth] CRON_SECRET is not configured")
			utils.FailResponseWithStatus(c, http.StatusInternalServerError, "Cron secret not configured")
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
			utils.FailResponseWithStatus(c, http.StatusUnauthorized, "Unauthorized")
			c.Abort()
			return
		}

		c.Next()
	}
}
