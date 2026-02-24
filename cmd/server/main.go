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

	// Initialize IMAP Worker
	imapWorker := workers.NewIMAPWorker(cfg.IMAP.Email, cfg.IMAP.Password, txService)

	// Setup Cron
	cronScheduler := workers.SetupCronJobs(imapWorker)

	// Start the cron scheduler in a non-blocking way
	cronScheduler.Start()
	logrus.Info("🛡️ [System] Background scheduler started successfully")

	// Pastikan cron dimatikan saat aplikasi berhenti (Graceful Shutdown)
    defer func() {
        logrus.Info("🛑 [System] Stopping background scheduler...")
        cronScheduler.Stop()
    }()

	// Development - worker test
	logrus.Info("🛠️ [Dev Mode] Menjalankan IMAP Worker satu kali saat startup...")
	_ = imapWorker.ProcessUnseenEmails()

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
		"url":  "http://localhost:" + port,
	}).Info("🚀 WalletX REST API Server is starting to listen")

	if err := r.Run(":" + port); err != nil {
		logrus.WithError(err).Fatal("❌ Failed to start server")
	}

}