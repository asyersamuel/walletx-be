package router

import (
	config "walletx-be/configs"
	"walletx-be/internal/handlers"
	"walletx-be/internal/middleware"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func SetupRouter(
	txHandler *handlers.TransactionHandler, 
	jwtSecret string, 
	cfg *config.Config,
) *gin.Engine {
	r := gin.Default()

	// CORS Configuration
	r.Use(cors.New(cors.Config{
		// Izinkan semua origin (termasuk ngrok, localhost, dll)
		AllowAllOrigins: true,

		// Method yang diizinkan
		AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},

		// Header yang diizinkan (tambahkan jika ada custom header dari frontend)
		AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Requested-With"},

		// Ekspos header tertentu ke frontend
		ExposeHeaders: []string{"Content-Length"},

		// Izinkan kredensial (cookies/auth headers)
		AllowCredentials: true,

		// Cache preflight request selama 12 jam
		MaxAge: 12 * time.Hour,
	}))

	// Serve static files only for local storage (Supabase serves files via CDN)
	if cfg.Media.StorageType == "local" {
		r.Static(cfg.Media.BaseURL, cfg.Media.UploadDir)
	}


	api := r.Group("/api/v1")
	{

		// Route Transaction
		transactions := api.Group("/transactions")
		transactions.Use(middleware.AuthMiddleware(jwtSecret)) 
		{
			// Endpoint: GET /api/v1/transactions?limit=20&offset=0
			transactions.GET("", txHandler.GetUserTransactions)
		}
	}

	return r
}