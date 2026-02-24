package main

import (
	"os"

	"walletx-be/configs"
	"walletx-be/internal/database"
	"walletx-be/internal/repository" 
	"walletx-be/internal/services"   
	"walletx-be/internal/workers"
	"walletx-be/internal/handlers"
	"walletx-be/internal/router"


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
	
	// Load application configuration
	cfg := configs.Load()

	// Initialize database connection
	logrus.Info("🚀 Initializing database connection...")
	db, err := database.Init(cfg.Database)
	
	if err != nil {
		logrus.WithError(err).Fatal("❌ Failed to connect to database")
	}

	logrus.Info("✅ Database connected and models migrated successfully")
	
	// Initialize Repositories (Injecting Database)
	userRepo := repository.NewUserRepository(db)
	transactionRepo := repository.NewTransactionRepository(db)
	
	// Initialize Service
	parserService := services.NewParserService()
	txService := services.NewTransactionService(userRepo,transactionRepo, parserService)

	// Initialize Handler
	txHandler := handlers.NewTransactionHandler(txService)

	// Goroutine IMAP Worker
	go func() {
		logrus.WithField("bot_email", cfg.IMAP.Email).Info("Starting IMAP Worker in background...")

		worker := workers.NewIMAPWorker(cfg.IMAP.Email, cfg.IMAP.Password, txService)
		err := worker.ProcessUnseenEmails()
		if err != nil {
			logrus.WithError(err).Error("❌ IMAP Worker stopped due to an error")
		} else {
			logrus.Info("✅ IMAP Worker executed successfully")
		}
	}()

	// Setup and Run Router Gin
	logrus.Info("Starting REST API Server...")
	r := router.SetupRouter(txHandler, cfg.JWT.Secret, cfg)
	
	port := cfg.Server.Port
	if port == "" {
		port = "8080"
	}
	
	logrus.WithFields(logrus.Fields{
		"port": port,
		"env":  os.Getenv("GIN_MODE"),
		"url":  "http://localhost" + port,
	}).Info("🚀 WalletX REST API Server is starting to listen")

	if err := r.Run(":" + port); err != nil {
		logrus.WithError(err).Fatal("❌ Failed to start server")
	}

}