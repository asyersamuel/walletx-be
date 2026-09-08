package transaction

import (
	"context"
	"errors"
	"time"

	"walletx-be/internal/platform/logger"
	"walletx-be/internal/shared/errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Repository is the persisted data access for Transaction aggregates.
type Repository interface {
	Create(ctx context.Context, transaction *Transaction) error
	GetByID(ctx context.Context, id uuid.UUID) (*Transaction, error)
	GetByMessageID(ctx context.Context, messageID string) (*Transaction, error)
	Update(ctx context.Context, transaction *Transaction) error
	Delete(ctx context.Context, id uuid.UUID) error
	ListByUserID(ctx context.Context, userID uuid.UUID, limit, offset int, filters TransactionFilters, sort SortOption) ([]Transaction, error)
	CountByUserID(ctx context.Context, userID uuid.UUID, filters TransactionFilters) (int64, error)
	SearchByMerchant(ctx context.Context, userID uuid.UUID, query string, limit, offset int) ([]Transaction, error)
	CountSearchByMerchant(ctx context.Context, userID uuid.UUID, query string) (int64, error)
	GetExpensesByCategory(ctx context.Context, userID uuid.UUID, month, year int) ([]CategoryExpense, error)
	GetForExport(ctx context.Context, userID uuid.UUID, month, year int) ([]Transaction, error)
	GetTotalSpentByCategoryThisMonth(ctx context.Context, userID uuid.UUID, categoryID uuid.UUID, month, year int) (float64, error)
}

type repository struct {
	db     *gorm.DB
	logger logger.Logger
}

func NewRepository(db *gorm.DB, logger logger.Logger) Repository {
	return &repository{
		db:     db,
		logger: logger,
	}
}

func (r *repository) Create(ctx context.Context, transaction *Transaction) error {
	err := r.db.WithContext(ctx).Create(transaction).Error
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return apperrors.ErrDuplicate
	}
	return err
}

func (r *repository) GetByID(ctx context.Context, id uuid.UUID) (*Transaction, error) {
	var transaction Transaction
	if err := r.db.WithContext(ctx).First(&transaction, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrNotFound
		}
		return nil, err
	}
	return &transaction, nil
}

func (r *repository) GetByMessageID(ctx context.Context, messageID string) (*Transaction, error) {
	if messageID == "" {
		return nil, nil
	}
	var transaction Transaction
	if err := r.db.WithContext(ctx).Where("message_id = ?", messageID).First(&transaction).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &transaction, nil
}

func (r *repository) ListByUserID(ctx context.Context, userID uuid.UUID, limit, offset int, filters TransactionFilters, sort SortOption) ([]Transaction, error) {
	var items []Transaction

	q := r.db.WithContext(ctx).Where("user_id = ?", userID)

	if filters.DateFrom != "" {
		q = q.Where("DATE(transaction_date) >= ?", filters.DateFrom)
	}
	if filters.DateTo != "" {
		q = q.Where("DATE(transaction_date) <= ?", filters.DateTo)
	}
	if filters.AmountMin != nil {
		q = q.Where("amount >= ?", *filters.AmountMin)
	}
	if filters.AmountMax != nil {
		q = q.Where("amount <= ?", *filters.AmountMax)
	}
	if filters.LastUpdated != "" {
		q = q.Where("updated_at > ?", filters.LastUpdated)
	}
	if filters.ExcludeDeleted {
		q = q.Where("deleted_at IS NULL")
	}

	sortField := sort.Field
	if sortField == "" {
		sortField = "transaction_date"
	}
	sortDir := sort.Direction
	if sortDir == "" {
		sortDir = "DESC"
	}

	if err := q.Order(sortField + " " + sortDir).Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (r *repository) CountByUserID(ctx context.Context, userID uuid.UUID, filters TransactionFilters) (int64, error) {
	var count int64
	q := r.db.WithContext(ctx).Model(&Transaction{}).Where("user_id = ?", userID)

	if filters.DateFrom != "" {
		q = q.Where("DATE(transaction_date) >= ?", filters.DateFrom)
	}
	if filters.DateTo != "" {
		q = q.Where("DATE(transaction_date) <= ?", filters.DateTo)
	}
	if filters.AmountMin != nil {
		q = q.Where("amount >= ?", *filters.AmountMin)
	}
	if filters.AmountMax != nil {
		q = q.Where("amount <= ?", *filters.AmountMax)
	}
	if filters.LastUpdated != "" {
		q = q.Where("updated_at > ?", filters.LastUpdated)
	}
	if filters.ExcludeDeleted {
		q = q.Where("deleted_at IS NULL")
	}

	if err := q.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *repository) SearchByMerchant(ctx context.Context, userID uuid.UUID, query string, limit, offset int) ([]Transaction, error) {
	var items []Transaction
	if err := r.db.WithContext(ctx).Where("user_id = ? AND merchant ILIKE ?", userID, "%"+query+"%").
		Order("transaction_date DESC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (r *repository) CountSearchByMerchant(ctx context.Context, userID uuid.UUID, query string) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&Transaction{}).Where("user_id = ? AND merchant ILIKE ?", userID, "%"+query+"%").Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *repository) GetExpensesByCategory(ctx context.Context, userID uuid.UUID, month, year int) ([]CategoryExpense, error) {
	var results []CategoryExpense
	startDate := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local)
	endDate := startDate.AddDate(0, 1, 0)

	query := `
		SELECT
			c.name AS category_name,
			SUM(t.amount) AS total_amount
		FROM transactions t
		LEFT JOIN categories c ON t.category_id = c.id
		WHERE t.user_id = ?
		  AND t.transaction_date >= ?
		  AND t.transaction_date < ?
		GROUP BY c.name
		ORDER BY total_amount DESC
	`

	if err := r.db.WithContext(ctx).Raw(query, userID, startDate, endDate).Scan(&results).Error; err != nil {
		return nil, err
	}
	return results, nil
}

func (r *repository) GetForExport(ctx context.Context, userID uuid.UUID, month, year int) ([]Transaction, error) {
	var items []Transaction
	startDate := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local)
	endDate := startDate.AddDate(0, 1, 0)

	if err := r.db.WithContext(ctx).Where("user_id = ? AND transaction_date >= ? AND transaction_date < ?", userID, startDate, endDate).
		Preload("Category").Order("transaction_date ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (r *repository) Update(ctx context.Context, transaction *Transaction) error {
	return r.db.WithContext(ctx).Save(transaction).Error
}

func (r *repository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&Transaction{}, "id = ?", id).Error
}

func (r *repository) GetTotalSpentByCategoryThisMonth(ctx context.Context, userID uuid.UUID, categoryID uuid.UUID, month, year int) (float64, error) {
	var total float64
	startDate := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local)
	endDate := startDate.AddDate(0, 1, 0)

	err := r.db.WithContext(ctx).
		Model(&Transaction{}).
		Where("user_id = ? AND category_id = ? AND transaction_date >= ? AND transaction_date < ?", userID, categoryID, startDate, endDate).
		Select("COALESCE(SUM(amount), 0)").
		Scan(&total).Error

	if err != nil {
		return 0, err
	}
	return total, nil
}
