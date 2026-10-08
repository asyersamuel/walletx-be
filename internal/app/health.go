package app

import (
	"net/http"

	"walletx-be/internal/middleware"
	"walletx-be/internal/platform/logger"

	"github.com/gin-gonic/gin"
)

const HealthPath = "/api/v1/health"

// Health reports liveness without consulting application dependencies.
func Health(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// NewHealthRouter serves health requests before database and AI initialization.
func NewHealthRouter(appLogger logger.Logger) *gin.Engine {
	r := gin.New()
	r.Use(middleware.RequestLogger(appLogger), middleware.Recovery(appLogger))
	r.GET(HealthPath, Health)
	return r
}
