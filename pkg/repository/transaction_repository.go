package repository

import (
	"context"
	"errors"
	"time"

	"walletx-be/internal/domain"
	"walletx-be/internal/domain/query"
	"walletx-be/internal/domain/report"
	"walletx-be/internal/ports"
	"walletx-be/pkg/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type TransactionFilters = query.TransactionFilters
type SortOption = query.SortOption

type TransactionRepository interface {
	Create(ctx context.Context, transaction *models.Transaction) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.Transaction, error)
	GetByMessageID(ctx context.Context, messageID string) (*models.Transaction, error)
	Update(ctx context.Context, transaction *models.Transaction) error
	Delete(ctx context.Context, id uuid.UUID) error
	ListByUserID(ctx context.Context, userID uuid.UUID, limit, offset int, filters TransactionFilters, sort SortOption) ([]models.Transaction, error)
	CountByUserID(ctx context.Context, userID uuid.UUID, filters TransactionFilters) (int64, error)
	SearchByMerchant(ctx context.Context, userID uuid.UUID, query string, limit, offset int) ([]models.Transaction, error)
	CountSearchByMerchant(ctx context.Context, userID uuid.UUID, query string) (int64, error)
	GetExpensesByCategory(ctx context.Context, userID uuid.UUID, month, year int) ([]report.CategoryExpense, error)
	GetForExport(ctx context.Context, userID uuid.UUID, month, year int) ([]models.Transaction, error)
	GetTotalSpentByCategoryThisMonth(ctx context.Context, userID uuid.UUID, categoryID uuid.UUID, month, year int) (float64, error)
}

type transactionRepository struct {
	db     *gorm.DB
	logger ports.Logger
}

func NewTransactionRepository(db *gorm.DB, logger ports.Logger) TransactionRepository {
	return &transactionRepository{
		db:     db,
		logger: logger,
	}
}

func (r *transactionRepository) Create(ctx context.Context, transaction *models.Transaction) error {
	err := r.db.WithContext(ctx).Create(transaction).Error
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return domain.ErrDuplicate
	}
	return err
}

func (r *transactionRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Transaction, error) {
	var transaction models.Transaction
	if err := r.db.WithContext(ctx).First(&transaction, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &transaction, nil
}

func (r *transactionRepository) GetByMessageID(ctx context.Context, messageID string) (*models.Transaction, error) {
	if messageID == "" {
		return nil, nil
	}
	var transaction models.Transaction
	if err := r.db.WithContext(ctx).Where("message_id = ?", messageID).First(&transaction).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &transaction, nil
}

func (r *transactionRepository) ListByUserID(ctx context.Context, userID uuid.UUID, limit, offset int, filters TransactionFilters, sort SortOption) ([]models.Transaction, error) {
	var items []models.Transaction
	
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

func (r *transactionRepository) CountByUserID(ctx context.Context, userID uuid.UUID, filters TransactionFilters) (int64, error) {
	var count int64
	q := r.db.WithContext(ctx).Model(&models.Transaction{}).Where("user_id = ?", userID)

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

func (r *transactionRepository) SearchByMerchant(ctx context.Context, userID uuid.UUID, query string, limit, offset int) ([]models.Transaction, error) {
	var items []models.Transaction
	if err := r.db.WithContext(ctx).Where("user_id = ? AND merchant ILIKE ?", userID, "%"+query+"%").
		Order("transaction_date DESC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (r *transactionRepository) CountSearchByMerchant(ctx context.Context, userID uuid.UUID, query string) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&models.Transaction{}).Where("user_id = ? AND merchant ILIKE ?", userID, "%"+query+"%").Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *transactionRepository) GetExpensesByCategory(ctx context.Context, userID uuid.UUID, month, year int) ([]report.CategoryExpense, error) {
	var results []report.CategoryExpense
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

func (r *transactionRepository) GetForExport(ctx context.Context, userID uuid.UUID, month, year int) ([]models.Transaction, error) {
	var items []models.Transaction
	startDate := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local)
	endDate := startDate.AddDate(0, 1, 0)

	if err := r.db.WithContext(ctx).Where("user_id = ? AND transaction_date >= ? AND transaction_date < ?", userID, startDate, endDate).
		Preload("Category").Order("transaction_date ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (r *transactionRepository) Update(ctx context.Context, transaction *models.Transaction) error {
	return r.db.WithContext(ctx).Save(transaction).Error
}

func (r *transactionRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&models.Transaction{}, "id = ?", id).Error
}

func (r *transactionRepository) GetTotalSpentByCategoryThisMonth(ctx context.Context, userID uuid.UUID, categoryID uuid.UUID, month, year int) (float64, error) {
	var total float64
	startDate := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local)
	endDate := startDate.AddDate(0, 1, 0)

	err := r.db.WithContext(ctx).
		Model(&models.Transaction{}).
		Where("user_id = ? AND category_id = ? AND transaction_date >= ? AND transaction_date < ?", userID, categoryID, startDate, endDate).
		Select("COALESCE(SUM(amount), 0)").
		Scan(&total).Error

	if err != nil {
		return 0, err
	}
	return total, nil
}
