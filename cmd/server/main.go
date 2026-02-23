package main

import (
	"os"

	"walletx-be/configs"
	"walletx-be/internal/database"
	"walletx-be/internal/repository" 
	"walletx-be/internal/services"   
	"walletx-be/internal/workers"

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
	
	// 2. Initialize Repositories (Injecting Database)
	userRepo := repository.NewUserRepository(db)
	transactionRepo := repository.NewTransactionRepository(db)
	
	// Inisialisasi Parser
	parserService := services.NewParserService()

	// 3. Initialize Services (Injecting Repositories)
	txService := services.NewTransactionService(userRepo, transactionRepo, parserService)

	// 4. Initialize and Run IMAP Worker (Injecting Service)
	logrus.WithFields(logrus.Fields{
		"bot_email": cfg.IMAP.Email,
	}).Info("Starting IMAP Worker...")

	worker := workers.NewIMAPWorker(cfg.IMAP.Email, cfg.IMAP.Password, txService)
	err = worker.ProcessUnseenEmails()
	
	if err != nil {
		logrus.WithError(err).Error("❌ IMAP Worker stopped due to an error")
	} else {
		logrus.Info("✅ IMAP Worker executed successfully")
	}
}