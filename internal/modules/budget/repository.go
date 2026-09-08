package budget

import (
	"context"
	"time"

	"walletx-be/internal/modules/category"
	database "walletx-be/internal/platform/database"
	sqlc "walletx-be/internal/platform/database/sqlc"
	"walletx-be/internal/platform/logger"
	apperrors "walletx-be/internal/shared/errors"

	"github.com/google/uuid"
)

// Repository is the persisted data access for budget limits.
type Repository interface {
	Create(ctx context.Context, limit *CategoryLimit) error
	List(ctx context.Context, userID uuid.UUID) ([]CategoryLimit, error)
	GetByID(ctx context.Context, id, userID uuid.UUID) (*CategoryLimit, error)
	Update(ctx context.Context, limit *CategoryLimit) error
	Delete(ctx context.Context, id, userID uuid.UUID) error
	GetActiveByUserID(ctx context.Context, userID uuid.UUID) ([]CategoryLimit, error)
	FindByCategory(ctx context.Context, userID, categoryID uuid.UUID) (*CategoryLimit, error)
	GetLimitReportByUserID(ctx context.Context, userID uuid.UUID) ([]LimitReport, error)
}

// SummaryRepository provides read-only aggregates over the daily_expense_summary
// view. It is shared by the budget progress calculation and the dashboard.
type SummaryRepository interface {
	GetSummary(ctx context.Context, userID uuid.UUID, period string) ([]SummaryDTO, error)
	GetDailyTotal(ctx context.Context, userID uuid.UUID, month int, year int) ([]DailyTotalDTO, error)
	GetSpendingByCategoryAndDateRange(ctx context.Context, userID uuid.UUID, startDate time.Time, endDate time.Time) ([]SummaryDTO, error)
}

type repository struct {
	queries *sqlc.Queries
	logger  logger.Logger
}

func NewRepository(queries *sqlc.Queries, logger logger.Logger) Repository {
	return &repository{queries: queries, logger: logger}
}

func (r *repository) Create(ctx context.Context, limit *CategoryLimit) error {
	row, err := r.queries.CreateCategoryLimit(ctx, sqlc.CreateCategoryLimitParams{
		UserID:      database.UUIDParam(limit.UserID),
		CategoryID:  database.UUIDParam(limit.CategoryID),
		Period:      limit.Period,
		LimitAmount: limit.LimitAmount,
		IsActive:    limit.IsActive,
	})
	if err != nil {
		if database.IsUniqueViolation(err) {
			return apperrors.ErrDuplicate
		}
		return err
	}
	*limit = categoryLimitFromRow(row)
	return nil
}

func (r *repository) List(ctx context.Context, userID uuid.UUID) ([]CategoryLimit, error) {
	rows, err := r.queries.ListCategoryLimits(ctx, database.UUIDParam(userID))
	if err != nil {
		return nil, err
	}

	items := make([]CategoryLimit, 0, len(rows))
	for _, row := range rows {
		items = append(items, categoryLimitFromRow(row))
	}
	return items, nil
}

func (r *repository) GetByID(ctx context.Context, id, userID uuid.UUID) (*CategoryLimit, error) {
	row, err := r.queries.GetCategoryLimitByID(ctx, sqlc.GetCategoryLimitByIDParams{
		ID:     database.UUIDParam(id),
		UserID: database.UUIDParam(userID),
	})
	if err != nil {
		if database.IsNoRows(err) {
			return nil, apperrors.ErrNotFound
		}
		return nil, err
	}
	limit := categoryLimitFromRow(row)
	return &limit, nil
}

func (r *repository) Update(ctx context.Context, limit *CategoryLimit) error {
	row, err := r.queries.UpdateCategoryLimit(ctx, sqlc.UpdateCategoryLimitParams{
		ID:          database.UUIDParam(limit.ID),
		UserID:      database.UUIDParam(limit.UserID),
		Period:      limit.Period,
		LimitAmount: limit.LimitAmount,
		IsActive:    limit.IsActive,
	})
	if err != nil {
		if database.IsNoRows(err) {
			return apperrors.ErrNotFound
		}
		return err
	}
	*limit = categoryLimitFromRow(row)
	return nil
}

func (r *repository) Delete(ctx context.Context, id, userID uuid.UUID) error {
	rows, err := r.queries.DeleteCategoryLimit(ctx, sqlc.DeleteCategoryLimitParams{
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

func (r *repository) GetActiveByUserID(ctx context.Context, userID uuid.UUID) ([]CategoryLimit, error) {
	rows, err := r.queries.ListActiveCategoryLimitsWithCategory(ctx, database.UUIDParam(userID))
	if err != nil {
		return nil, err
	}

	items := make([]CategoryLimit, 0, len(rows))
	for _, row := range rows {
		items = append(items, CategoryLimit{
			ID:          database.UUIDValue(row.ID),
			UserID:      database.UUIDValue(row.UserID),
			CategoryID:  database.UUIDValue(row.CategoryID),
			Period:      row.Period,
			LimitAmount: row.LimitAmount,
			IsActive:    row.IsActive,
			CreatedAt:   database.TimeValue(row.CreatedAt),
			UpdatedAt:   database.TimeValue(row.UpdatedAt),
			Category: category.Category{
				ID:     database.UUIDValue(row.CategoryID),
				UserID: database.UUIDValue(row.UserID),
				Name:   row.CategoryName,
				Icon:   row.CategoryIcon,
			},
		})
	}
	return items, nil
}

func (r *repository) FindByCategory(ctx context.Context, userID, categoryID uuid.UUID) (*CategoryLimit, error) {
	row, err := r.queries.GetCategoryLimitByCategory(ctx, sqlc.GetCategoryLimitByCategoryParams{
		UserID:     database.UUIDParam(userID),
		CategoryID: database.UUIDParam(categoryID),
	})
	if err != nil {
		if database.IsNoRows(err) {
			return nil, apperrors.ErrNotFound
		}
		return nil, err
	}
	limit := categoryLimitFromRow(row)
	return &limit, nil
}

func (r *repository) GetLimitReportByUserID(ctx context.Context, userID uuid.UUID) ([]LimitReport, error) {
	rows, err := r.queries.GetLimitReportByUserID(ctx, database.UUIDParam(userID))
	if err != nil {
		return nil, err
	}

	items := make([]LimitReport, 0, len(rows))
	for _, row := range rows {
		items = append(items, LimitReport{
			CategoryName: row.CategoryName,
			CategoryIcon: row.CategoryIcon,
			LimitAmount:  row.LimitAmount,
			TotalSpent:   row.TotalSpent,
		})
	}
	return items, nil
}

func categoryLimitFromRow(row sqlc.CategoryLimit) CategoryLimit {
	return CategoryLimit{
		ID:          database.UUIDValue(row.ID),
		UserID:      database.UUIDValue(row.UserID),
		CategoryID:  database.UUIDValue(row.CategoryID),
		Period:      row.Period,
		LimitAmount: row.LimitAmount,
		IsActive:    row.IsActive,
		CreatedAt:   database.TimeValue(row.CreatedAt),
		UpdatedAt:   database.TimeValue(row.UpdatedAt),
	}
}

// ─── Summary repository ───────────────────────────────────────────────────────

type summaryRepository struct {
	queries *sqlc.Queries
	logger  logger.Logger
}

func NewSummaryRepository(queries *sqlc.Queries, logger logger.Logger) SummaryRepository {
	return &summaryRepository{queries: queries, logger: logger}
}

func (r *summaryRepository) GetSummary(ctx context.Context, userID uuid.UUID, period string) ([]SummaryDTO, error) {
	now := time.Now()
	var startDate time.Time
	switch period {
	case "weekly":
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		startDate = now.AddDate(0, 0, -(weekday - 1))
		startDate = time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, startDate.Location())
	default:
		startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	}

	rows, err := r.queries.GetSummaryByUserAndStartDate(ctx, sqlc.GetSummaryByUserAndStartDateParams{
		UserID:    database.UUIDParam(userID),
		StartDate: database.DateParam(startDate),
	})
	if err != nil {
		return nil, err
	}

	items := make([]SummaryDTO, 0, len(rows))
	for _, row := range rows {
		items = append(items, SummaryDTO{
			CategoryID:       database.UUIDPtr(row.CategoryID),
			TotalAmount:      row.TotalAmount,
			TransactionCount: row.TransactionCount,
		})
	}
	return items, nil
}

func (r *summaryRepository) GetSpendingByCategoryAndDateRange(ctx context.Context, userID uuid.UUID, startDate time.Time, endDate time.Time) ([]SummaryDTO, error) {
	rows, err := r.queries.GetSpendingByCategoryAndRange(ctx, sqlc.GetSpendingByCategoryAndRangeParams{
		UserID:    database.UUIDParam(userID),
		StartDate: database.DateParam(startDate),
		EndDate:   database.DateParam(endDate),
	})
	if err != nil {
		return nil, err
	}

	items := make([]SummaryDTO, 0, len(rows))
	for _, row := range rows {
		items = append(items, SummaryDTO{
			CategoryID:       database.UUIDPtr(row.CategoryID),
			TotalAmount:      row.TotalAmount,
			TransactionCount: row.TransactionCount,
		})
	}
	return items, nil
}

func (r *summaryRepository) GetDailyTotal(ctx context.Context, userID uuid.UUID, month int, year int) ([]DailyTotalDTO, error) {
	startDate := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local)
	endDate := startDate.AddDate(0, 1, 0)

	rows, err := r.queries.GetDailyTotalByUserAndRange(ctx, sqlc.GetDailyTotalByUserAndRangeParams{
		UserID:    database.UUIDParam(userID),
		StartDate: database.DateParam(startDate),
		EndDate:   database.DateParam(endDate),
	})
	if err != nil {
		return nil, err
	}

	items := make([]DailyTotalDTO, 0, len(rows))
	for _, row := range rows {
		items = append(items, DailyTotalDTO{
			Day:         database.DateValue(row.Day),
			TotalAmount: row.TotalAmount,
		})
	}
	return items, nil
}
