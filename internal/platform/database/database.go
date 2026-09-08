package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Init opens the runtime connection pool. Schema changes are deliberately not
// performed here; Supabase CLI migrations are the single source of truth for
// schema changes.
func Init(databaseURL string) (*pgxpool.Pool, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is not set")
	}

	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse DATABASE_URL: %w", err)
	}

	// Supabase transaction pooler does not support prepared statements in the
	// same way as a direct PostgreSQL connection. Simple protocol works for both
	// the local Supabase database and the hosted pooler connection.
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
		fmt.Printf("Database connection failed (attempt %d/%d), retrying in %v...\n", attempt, maxRetries, wait)
		time.Sleep(wait)
	}

	return nil, fmt.Errorf("database connection failed")
}
