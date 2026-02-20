package database

import (
	config "walletx-be/configs"
	"walletx-be/internal/models"
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Init(cfg config.DatabaseConfig) (*gorm.DB, error) {
	var dsn string

	// Penggunaan URL penuh (Supabase)
	if cfg.URL != "" {
		dsn = cfg.URL
	} else {
		// Fallback ke rakit manual jika URL kosong
		dsn = fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=Asia/Jakarta",
			cfg.Host, cfg.User, cfg.Password, cfg.DBName, cfg.Port, cfg.SSLMode)
	}

	var db *gorm.DB
	var err error

	// Retry connection with exponential backoff
	maxRetries := 30
	for i := 0; i < maxRetries; i++ {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Info),
		})
		if err == nil {
			break
		}

		if i == maxRetries-1 {
			return nil, fmt.Errorf("failed to connect to database after %d retries: %w", maxRetries, err)
		}

		waitTime := time.Duration(i+1) * time.Second
		fmt.Printf("Database connection failed (attempt %d/%d), retrying in %v...\n", i+1, maxRetries, waitTime)
		time.Sleep(waitTime)
	}

	// Test the connection
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database instance: %w", err)
	}

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Auto migrate models
	if err := db.AutoMigrate(
		&models.User{},
		&models.Swipe{},
		&models.Match{},
		&models.Conversation{},
		&models.Message{},
		&models.Status{},
		&models.StatusLike{},
	); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return db, nil
}