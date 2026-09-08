package transaction

import (
	"context"
	"strconv"
	"strings"
	"time"

	"walletx-be/internal/modules/category"
	database "walletx-be/internal/platform/database"
	sqlc "walletx-be/internal/platform/database/sqlc"
	"walletx-be/internal/platform/logger"
	apperrors "walletx-be/internal/shared/errors"

	"github.com/google/uuid"
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
	queries *sqlc.Queries
	logger  logger.Logger
}

func NewRepository(queries *sqlc.Queries, logger logger.Logger) Repository {
	return &repository{queries: queries, logger: logger}
}

func (r *repository) Create(ctx context.Context, transaction *Transaction) error {
	row, err := r.queries.CreateTransaction(ctx, sqlc.CreateTransactionParams{
		UserID:          database.UUIDParam(transaction.UserID),
		CategoryID:      database.NullableUUIDParam(transaction.CategoryID),
		Amount:          transaction.Amount,
		Merchant:        transaction.Merchant,
		Note:            &transaction.Note,
		TransactionDate: database.TimeParam(transaction.TransactionDate),
		MessageID:       transaction.MessageID,
		IsRecurring:     transaction.IsRecurring,
	})
	if err != nil {
		if database.IsUniqueViolation(err) {
			return apperrors.ErrDuplicate
		}
		return err
	}
	*transaction = transactionFromRow(row)
	return nil
}

func (r *repository) GetByID(ctx context.Context, id uuid.UUID) (*Transaction, error) {
	row, err := r.queries.GetTransactionByID(ctx, database.UUIDParam(id))
	if err != nil {
		if database.IsNoRows(err) {
			return nil, apperrors.ErrNotFound
		}
		return nil, err
	}
	transaction := transactionFromRow(row)
	return &transaction, nil
}

func (r *repository) GetByMessageID(ctx context.Context, messageID string) (*Transaction, error) {
	if messageID == "" {
		return nil, nil
	}

	row, err := r.queries.GetTransactionByMessageID(ctx, &messageID)
	if err != nil {
		if database.IsNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	transaction := transactionFromRow(row)
	return &transaction, nil
}

func (r *repository) ListByUserID(ctx context.Context, userID uuid.UUID, limit, offset int, filters TransactionFilters, sort SortOption) ([]Transaction, error) {
	sortField, sortDirection := normalizeSort(sort)
	rows, err := r.queries.ListTransactions(ctx, sqlc.ListTransactionsParams{
		UserID:         database.UUIDParam(userID),
		DateFrom:       filters.DateFrom,
		DateTo:         filters.DateTo,
		AmountMin:      formatAmountFilter(filters.AmountMin),
		AmountMax:      formatAmountFilter(filters.AmountMax),
		LastUpdated:    filters.LastUpdated,
		ExcludeDeleted: filters.ExcludeDeleted,
		SortField:      sortField,
		SortDirection:  sortDirection,
		LimitCount:     int32(limit),
		OffsetCount:    int32(offset),
	})
	if err != nil {
		return nil, err
	}

	items := make([]Transaction, 0, len(rows))
	for _, row := range rows {
		items = append(items, transactionFromRow(row))
	}
	return items, nil
}

func (r *repository) CountByUserID(ctx context.Context, userID uuid.UUID, filters TransactionFilters) (int64, error) {
	return r.queries.CountTransactions(ctx, sqlc.CountTransactionsParams{
		UserID:         database.UUIDParam(userID),
		DateFrom:       filters.DateFrom,
		DateTo:         filters.DateTo,
		AmountMin:      formatAmountFilter(filters.AmountMin),
		AmountMax:      formatAmountFilter(filters.AmountMax),
		LastUpdated:    filters.LastUpdated,
		ExcludeDeleted: filters.ExcludeDeleted,
	})
}

func (r *repository) SearchByMerchant(ctx context.Context, userID uuid.UUID, query string, limit, offset int) ([]Transaction, error) {
	rows, err := r.queries.SearchTransactions(ctx, sqlc.SearchTransactionsParams{
		UserID:      database.UUIDParam(userID),
		Query:       &query,
		LimitCount:  int32(limit),
		OffsetCount: int32(offset),
	})
	if err != nil {
		return nil, err
	}

	items := make([]Transaction, 0, len(rows))
	for _, row := range rows {
		items = append(items, transactionFromRow(row))
	}
	return items, nil
}

func (r *repository) CountSearchByMerchant(ctx context.Context, userID uuid.UUID, query string) (int64, error) {
	return r.queries.CountSearchTransactions(ctx, sqlc.CountSearchTransactionsParams{
		UserID: database.UUIDParam(userID),
		Query:  &query,
	})
}

func (r *repository) GetExpensesByCategory(ctx context.Context, userID uuid.UUID, month, year int) ([]CategoryExpense, error) {
	startDate := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local)
	endDate := startDate.AddDate(0, 1, 0)

	rows, err := r.queries.GetExpensesByCategory(ctx, sqlc.GetExpensesByCategoryParams{
		UserID:    database.UUIDParam(userID),
		StartDate: database.TimeParam(startDate),
		EndDate:   database.TimeParam(endDate),
	})
	if err != nil {
		return nil, err
	}

	items := make([]CategoryExpense, 0, len(rows))
	for _, row := range rows {
		name := ""
		if row.CategoryName != nil {
			name = *row.CategoryName
		}
		items = append(items, CategoryExpense{CategoryName: name, TotalAmount: row.TotalAmount})
	}
	return items, nil
}

func (r *repository) GetForExport(ctx context.Context, userID uuid.UUID, month, year int) ([]Transaction, error) {
	startDate := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local)
	endDate := startDate.AddDate(0, 1, 0)

	rows, err := r.queries.GetTransactionsForExport(ctx, sqlc.GetTransactionsForExportParams{
		UserID:    database.UUIDParam(userID),
		StartDate: database.TimeParam(startDate),
		EndDate:   database.TimeParam(endDate),
	})
	if err != nil {
		return nil, err
	}

	items := make([]Transaction, 0, len(rows))
	for _, row := range rows {
		transaction := transactionFromExportRow(row)
		items = append(items, transaction)
	}
	return items, nil
}

func (r *repository) Update(ctx context.Context, transaction *Transaction) error {
	row, err := r.queries.UpdateTransaction(ctx, sqlc.UpdateTransactionParams{
		ID:              database.UUIDParam(transaction.ID),
		CategoryID:      database.NullableUUIDParam(transaction.CategoryID),
		Amount:          transaction.Amount,
		Merchant:        transaction.Merchant,
		Note:            transaction.Note,
		TransactionDate: database.TimeParam(transaction.TransactionDate),
		MessageID:       transaction.MessageID,
		IsRecurring:     transaction.IsRecurring,
	})
	if err != nil {
		if database.IsNoRows(err) {
			return apperrors.ErrNotFound
		}
		if database.IsUniqueViolation(err) {
			return apperrors.ErrDuplicate
		}
		return err
	}
	*transaction = transactionFromRow(row)
	return nil
}

func (r *repository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.queries.SoftDeleteTransaction(ctx, database.UUIDParam(id))
	return err
}

func (r *repository) GetTotalSpentByCategoryThisMonth(ctx context.Context, userID uuid.UUID, categoryID uuid.UUID, month, year int) (float64, error) {
	startDate := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local)
	endDate := startDate.AddDate(0, 1, 0)

	return r.queries.GetTotalSpentByCategory(ctx, sqlc.GetTotalSpentByCategoryParams{
		UserID:     database.UUIDParam(userID),
		CategoryID: database.UUIDParam(categoryID),
		StartDate:  database.TimeParam(startDate),
		EndDate:    database.TimeParam(endDate),
	})
}

func transactionFromRow(row sqlc.Transaction) Transaction {
	return Transaction{
		ID:              database.UUIDValue(row.ID),
		UserID:          database.UUIDValue(row.UserID),
		CategoryID:      database.UUIDPtr(row.CategoryID),
		Amount:          row.Amount,
		Merchant:        row.Merchant,
		Note:            row.Note,
		TransactionDate: database.TimeValue(row.TransactionDate),
		MessageID:       row.MessageID,
		IsRecurring:     row.IsRecurring,
		CreatedAt:       database.TimeValue(row.CreatedAt),
		UpdatedAt:       database.TimeValue(row.UpdatedAt),
		DeletedAt:       database.TimePtr(row.DeletedAt),
	}
}

func transactionFromExportRow(row sqlc.GetTransactionsForExportRow) Transaction {
	transaction := Transaction{
		ID:              database.UUIDValue(row.ID),
		UserID:          database.UUIDValue(row.UserID),
		CategoryID:      database.UUIDPtr(row.CategoryID),
		Amount:          row.Amount,
		Merchant:        row.Merchant,
		Note:            row.Note,
		TransactionDate: database.TimeValue(row.TransactionDate),
		MessageID:       row.MessageID,
		IsRecurring:     row.IsRecurring,
		CreatedAt:       database.TimeValue(row.CreatedAt),
		UpdatedAt:       database.TimeValue(row.UpdatedAt),
		DeletedAt:       database.TimePtr(row.DeletedAt),
	}
	if row.CategoryName != nil {
		categoryID := database.UUIDPtr(row.CategoryID)
		if categoryID != nil {
			transaction.Category = &category.Category{
				ID:     *categoryID,
				UserID: transaction.UserID,
				Name:   *row.CategoryName,
			}
			if row.CategoryIcon != nil {
				transaction.Category.Icon = *row.CategoryIcon
			}
		}
	}
	return transaction
}

func formatAmountFilter(value *float64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatFloat(*value, 'f', -1, 64)
}

func normalizeSort(sort SortOption) (string, string) {
	field := sort.Field
	switch field {
	case "transaction_date", "amount", "merchant":
	default:
		field = "transaction_date"
	}

	direction := strings.ToUpper(sort.Direction)
	if direction != "ASC" && direction != "DESC" {
		direction = "DESC"
	}
	return field, direction
}
