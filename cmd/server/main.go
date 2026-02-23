package main

import (
	"os"

	"walletx-be/configs"
	"walletx-be/internal/database"

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
	
	// Optional: verify connection using raw SQL ping
	sqlDB, _ := db.DB()
	if err := sqlDB.Ping(); err == nil {
		logrus.Info("📡 Supabase ping successful")
	}

	// TODO: Initialize services and IMAP worker here...
}