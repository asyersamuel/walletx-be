package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// Menangkap & menghandle panic
func Recovery(logger *logrus.Logger) gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		if err, ok := recovered.(string); ok {
			logger.WithFields(logrus.Fields{
				"error": err,
				"path":  c.Request.URL.Path,
				"ip":    c.ClientIP(),
			}).Error("Panic recovered")
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Internal server error",
		})
	})
} 