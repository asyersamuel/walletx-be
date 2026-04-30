package router

import (
	"time"

	"walletx-be/pkg/utils"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func SetupCORS() gin.HandlerFunc {
	return cors.New(cors.Config{
		AllowAllOrigins:  true,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Requested-With"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	})
}

func SetupStaticFileServing(r *gin.Engine, storageType, baseURL, uploadDir string) {
	if storageType == "local" {
		r.Static(baseURL, uploadDir)
	}
}

func RegisterHealthCheck(r *gin.RouterGroup) {
	r.GET("/ping", func(c *gin.Context) {
		utils.SuccessResponse(c, nil, "WalletX API is running!")
	})
}
