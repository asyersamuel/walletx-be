package database

import (
	"fmt"
	"time"

	"walletx-be/configs"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Init establishes a connection to the database and performs auto-migration
func Init(cfg configs.DatabaseConfig) (*gorm.DB, error) {
	var dsn string

	// Prioritize full connection URL (e.g., Supabase URI)
	if cfg.URL != "" {
		dsn = cfg.URL
	} else {
		// Fallback to manual connection string construction
		dsn = fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=Asia/Jakarta",
			cfg.Host, cfg.User, cfg.Password, cfg.DBName, cfg.Port, cfg.SSLMode)
	}

	var db *gorm.DB
	var err error

	// Retry connection with exponential backoff
	maxRetries := 30
	for i := 0; i < maxRetries; i++ {
		// db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
		// 	// Logger: logger.Default.LogMode(logger.Info),
		// 	Logger: logger.Default.LogMode(logger.Warn),
		// })

		db, err = gorm.Open(postgres.New(postgres.Config{
			DSN:                  dsn,
			PreferSimpleProtocol: true, // MATIKAN prepared statement untuk Supabase Pooler
		}), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Warn),
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

	// Verify the connection
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database instance: %w", err)
	}

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Auto-migrate models - Hanya saat pertama dijalankan/kalau belum Create Table di Supabase SQL Editor
	// if err := db.AutoMigrate(
	// 	&models.User{},
	// 	&models.Transaction{},
	// ); err != nil {
	// 	return nil, fmt.Errorf("failed to migrate database: %w", err)
	// }

	return db, nil
}