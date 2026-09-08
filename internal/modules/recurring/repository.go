package recurring

import (
	"context"

	database "walletx-be/internal/platform/database"
	sqlc "walletx-be/internal/platform/database/sqlc"
	"walletx-be/internal/platform/logger"
	apperrors "walletx-be/internal/shared/errors"

	"github.com/google/uuid"
)

// Repository is the persisted data access for recurring configs.
type Repository interface {
	Create(ctx context.Context, config *Config) error
	List(ctx context.Context, userID uuid.UUID) ([]Config, error)
	GetByID(ctx context.Context, id, userID uuid.UUID) (*Config, error)
	Update(ctx context.Context, config *Config) error
	Delete(ctx context.Context, id, userID uuid.UUID) error
	GetDueConfigs(ctx context.Context) ([]Config, error)
}

type repository struct {
	queries *sqlc.Queries
	logger  logger.Logger
}

func NewRepository(queries *sqlc.Queries, logger logger.Logger) Repository {
	return &repository{queries: queries, logger: logger}
}

func (r *repository) Create(ctx context.Context, config *Config) error {
	row, err := r.queries.CreateRecurringConfig(ctx, sqlc.CreateRecurringConfigParams{
		UserID:      database.UUIDParam(config.UserID),
		CategoryID:  database.NullableUUIDParam(config.CategoryID),
		Amount:      config.Amount,
		Frequency:   config.Frequency,
		StartDate:   database.TimeParam(config.StartDate),
		NextDueDate: database.TimeParam(config.NextDueDate),
	})
	if err != nil {
		if database.IsUniqueViolation(err) {
			return apperrors.ErrDuplicate
		}
		return err
	}
	*config = configFromRow(row)
	return nil
}

func (r *repository) List(ctx context.Context, userID uuid.UUID) ([]Config, error) {
	rows, err := r.queries.ListRecurringConfigs(ctx, database.UUIDParam(userID))
	if err != nil {
		return nil, err
	}

	items := make([]Config, 0, len(rows))
	for _, row := range rows {
		items = append(items, configFromRow(row))
	}
	return items, nil
}

func (r *repository) GetByID(ctx context.Context, id, userID uuid.UUID) (*Config, error) {
	row, err := r.queries.GetRecurringConfigByID(ctx, sqlc.GetRecurringConfigByIDParams{
		ID:     database.UUIDParam(id),
		UserID: database.UUIDParam(userID),
	})
	if err != nil {
		if database.IsNoRows(err) {
			return nil, apperrors.ErrNotFound
		}
		return nil, err
	}
	config := configFromRow(row)
	return &config, nil
}

func (r *repository) Update(ctx context.Context, config *Config) error {
	row, err := r.queries.UpdateRecurringConfig(ctx, sqlc.UpdateRecurringConfigParams{
		ID:          database.UUIDParam(config.ID),
		UserID:      database.UUIDParam(config.UserID),
		CategoryID:  database.NullableUUIDParam(config.CategoryID),
		Amount:      config.Amount,
		Frequency:   config.Frequency,
		StartDate:   database.TimeParam(config.StartDate),
		NextDueDate: database.TimeParam(config.NextDueDate),
	})
	if err != nil {
		if database.IsNoRows(err) {
			return apperrors.ErrNotFound
		}
		return err
	}
	*config = configFromRow(row)
	return nil
}

func (r *repository) Delete(ctx context.Context, id, userID uuid.UUID) error {
	rows, err := r.queries.DeleteRecurringConfig(ctx, sqlc.DeleteRecurringConfigParams{
		ID:     database.UUIDParam(id),
		UserID: database.UUIDParam(userID),
	})
	if err != nil {
		return err
	}
	if rows == 0 {
		return apperrors.ErrNotFound
	}
	return nil
}

func (r *repository) GetDueConfigs(ctx context.Context) ([]Config, error) {
	rows, err := r.queries.ListDueRecurringConfigs(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]Config, 0, len(rows))
	for _, row := range rows {
		items = append(items, configFromRow(row))
	}
	return items, nil
}

func configFromRow(row sqlc.RecurringConfig) Config {
	return Config{
		ID:          database.UUIDValue(row.ID),
		UserID:      database.UUIDValue(row.UserID),
		CategoryID:  database.UUIDPtr(row.CategoryID),
		Amount:      row.Amount,
		Frequency:   row.Frequency,
		StartDate:   database.TimeValue(row.StartDate),
		NextDueDate: database.TimeValue(row.NextDueDate),
		CreatedAt:   database.TimeValue(row.CreatedAt),
		UpdatedAt:   database.TimeValue(row.UpdatedAt),
	}
}
