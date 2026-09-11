package middleware

import (
	"fmt"
	"net/http"

	"walletx-be/internal/platform/logger"

	"github.com/gin-gonic/gin"
)

// Recovery recovers from panics and returns a sanitised 500 response.
func Recovery(appLogger logger.Logger) gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		appLogger.WithFields(map[string]interface{}{
			"panic": fmt.Sprint(recovered),
			"path":  c.Request.URL.Path,
			"ip":    c.ClientIP(),
		}).Error("Panic recovered")

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Internal server error",
		})
	})
}
