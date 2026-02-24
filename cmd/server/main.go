package main

import (
	"os"

	"walletx-be/configs"
	"walletx-be/internal/database"
	"walletx-be/internal/handlers"
	"walletx-be/internal/repository"
	"walletx-be/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
)

func init() {
	// Standardize global logging format
	logrus.SetFormatter(&logrus.JSONFormatter{})
	logrus.SetOutput(os.Stdout)
	logrus.SetLevel(logrus.InfoLevel)
}

func main() {
	if err := godotenv.Load(); err != nil {
		logrus.Warn("⚠️ File .env tidak ditemukan, menggunakan variabel sistem default")
	}

	// 1. Load application configuration & Database
	cfg := configs.Load()
	logrus.Info("🚀 Initializing database connection...")
	
	db, err := database.Init(cfg.Database)
	if err != nil {
		logrus.WithError(err).Fatal("❌ Failed to connect to database")
	}
	logrus.Info("✅ Database connected successfully")

	// 2. Initialize Repositories (Fokus ke User Repo saja)
	userRepo := repository.NewUserRepository(db)

	// 3. Initialize Services (Fokus ke Auth Service)
	authService := services.NewAuthService(userRepo)

	// 4. Initialize Handlers
	authHandler := handlers.NewAuthHandler(authService)

	// ==========================================
	// 5. SETUP GIN ROUTER & JALANKAN API SERVER
	// ==========================================
	router := gin.Default()

	// Setup API Routes
	api := router.Group("/api/v1")
	{
		// Cek status server
		api.GET("/ping", func(c *gin.Context) {
			c.JSON(200, gin.H{"message": "WalletX API is running!"})
		})

		// Rute untuk Testing Registrasi / Login OAuth2
		auth := api.Group("/auth")
		{
			auth.POST("/google", authHandler.HandleGoogleAuth) 
		}
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	logrus.Infof("🌐 Menjalankan REST API Server di port %s...", port)
	if err := router.Run(":" + port); err != nil {
		logrus.WithError(err).Fatal("❌ Gagal menjalankan server HTTP")
	}
}