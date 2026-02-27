package router

import (
	"time"

	config "walletx-be/configs"
	"walletx-be/internal/handlers"
	"walletx-be/internal/middleware"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func SetupRouter(
	txHandler *handlers.TransactionHandler,
	authHandler *handlers.AuthHandler, // Menambahkan AuthHandler ke parameter
	jwtSecret string,
	cfg *config.Config,
) *gin.Engine {
	r := gin.Default()

	// CORS Configuration
	r.Use(cors.New(cors.Config{
		AllowAllOrigins:  true,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Requested-With"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// Serve static files only for local storage
	if cfg.Media.StorageType == "local" {
		r.Static(cfg.Media.BaseURL, cfg.Media.UploadDir)
	}

	// Setup API Routes
	api := r.Group("/api/v1")
	{
		// Cek status server
		api.GET("/ping", func(c *gin.Context) {
			c.JSON(200, gin.H{"message": "WalletX API is running!"})
		})


		// Route Auth
		auth := api.Group("/auth")
		{
			// Endpoint Utama untuk Aplikasi Mobile (Production)
			auth.POST("/google", authHandler.HandleGoogleAuth)

			// Endpoint Testing Khusus Development (Akses via Browser Komputer)
			if cfg.App.DevMode {
				auth.GET("/google/test-login", authHandler.GoogleLoginTest)
				auth.GET("/google/callback", authHandler.GoogleCallbackTest)
			}
		}

		// Route Transaction (Dilindungi Middleware)
		transactions := api.Group("/transactions")
		transactions.Use(middleware.AuthMiddleware(jwtSecret))
		{
			// Endpoint: GET /api/v1/transactions?limit=20&offset=0
			transactions.GET("", txHandler.GetUserTransactions)
		}
	}

	return r
}