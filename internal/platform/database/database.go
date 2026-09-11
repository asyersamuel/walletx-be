package database

import (
	"context"
	"fmt"
	"time"

	platformlogger "walletx-be/internal/platform/logger"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Init opens the runtime connection pool. 
func Init(databaseURL string, appLogger platformlogger.Logger) (*pgxpool.Pool, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is not set")
	}

	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse DATABASE_URL: %w", err)
	}

	poolConfig.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

	const maxRetries = 10
	for attempt := 1; attempt <= maxRetries; attempt++ {
		pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			pingErr := pool.Ping(ctx)
			cancel()
			if pingErr == nil {
				return pool, nil
			}
			pool.Close()
			err = pingErr
		}

		if attempt == maxRetries {
			return nil, fmt.Errorf("failed to connect to database after %d attempts: %w", maxRetries, err)
		}

		wait := time.Duration(attempt) * time.Second
		appLogger.WithFields(map[string]interface{}{
			"attempt":      attempt,
			"max_attempts": maxRetries,
			"retry_in":     wait.String(),
		}).Warn("Database connection failed; retrying")
		time.Sleep(wait)
	}

	return nil, fmt.Errorf("database connection failed")
}
