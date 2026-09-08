package cron

import (
	"context"

	sqlc "walletx-be/internal/platform/database/sqlc"
)

// HealthRepository is a lightweight DB health-check capability.
type HealthRepository interface {
	Ping(ctx context.Context) error
}

type healthRepository struct {
	queries *sqlc.Queries
}

func NewHealthRepository(queries *sqlc.Queries) HealthRepository {
	return &healthRepository{queries: queries}
}

func (r *healthRepository) Ping(ctx context.Context) error {
	_, err := r.queries.Ping(ctx)
	return err
}
