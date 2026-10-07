package postgres

import (
	"context"
	"math"
	"strconv"
	"strings"
	"time"
	domain "walletx-be/internal/modules/transactions/contracts"

	database "walletx-be/internal/platform/database"

	sqlc "walletx-be/internal/platform/database/sqlc"
	"walletx-be/internal/platform/logger"

	apperrors "walletx-be/internal/shared/errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type transactionRepository struct {
	queries *sqlc.Queries
	logger  logger.Logger
}

// NewTransactionRepository builds the repository on top of the generated sqlc
// queries. SQL access goes through sqlc/pgx; checked mapping keeps nullable
// persistence representations out of the domain.
func NewTransactionRepository(queries *sqlc.Queries, appLogger logger.Logger) domain.TransactionRepository {
	return &transactionRepository{queries: queries, logger: appLogger}
}

func (r *transactionRepository) Create(ctx context.Context, input domain.CreateTransactionInput) (*domain.Transaction, error) {
	amount, err := amountParam(input.Amount)
	if err != nil {
		return nil, err
	}
	row, err := r.queries.CreateTransaction(ctx, sqlc.CreateTransactionParams{
		UserID:          database.UUIDParam(input.UserID),
		CategoryID:      database.UUIDParam(input.CategoryID),
		MerchantName:    &input.MerchantName,
		Amount:          amount,
		TransactionDate: database.TimeParam(input.TransactionDate),
		ReceiptImageUrl: input.ReceiptImageUrl,
		Notes:           input.Notes,
	})
	if err != nil {
		// The guarded_category CTE selects from categories where the category
		// exists AND belongs to the executing user. When that guard yields no
		// rows, the INSERT ... SELECT inserts nothing and pgx returns
		// ErrNoRows. Surfacing ErrNotFound here satisfies two rules at once:
		//   1. Referential integrity: a missing category is rejected.
		//   2. Cross-user isolation: an existing category owned by a DIFFERENT
		//      user is rejected identically, without leaking whether it exists
		//      or who owns it.
		if database.IsNoRows(err) {
			return nil, apperrors.ErrNotFound
		}
		// CHECK (amount > 0) failure (zero or negative amount) and any NOT NULL
		// violation are invalid-input errors, not opaque internal failures.
		if database.IsCheckViolation(err) || database.IsNotNullViolation(err) {
			return nil, apperrors.ErrInvalidInput
		}
		// A foreign-key violation that slips past the guard (e.g. a missing
		// user) is likewise invalid input.
		if database.IsForeignKeyViolation(err) {
			return nil, apperrors.ErrInvalidInput
		}
		r.logger.WithError(err).Error("failed to create transaction")
		return nil, err
	}
	txn, err := transactionFrom(row.ID, row.UserID, row.CategoryID, row.MerchantName,
		row.Amount, row.TransactionDate, row.ReceiptImageUrl, row.Notes,
		row.CreatedAt, row.UpdatedAt, row.Status)
	if err != nil {
		return nil, err
	}
	return &txn, nil
}

func (r *transactionRepository) GetTransactionByID(ctx context.Context, userID, transactionID uuid.UUID) (*domain.Transaction, error) {
	row, err := r.queries.GetTransactionByID(ctx, sqlc.GetTransactionByIDParams{
		ID:     database.UUIDParam(transactionID),
		UserID: database.UUIDParam(userID),
	})
	if err != nil {
		// Pending, missing and foreign transactions all map to ErrNotFound.
		if database.IsNoRows(err) {
			return nil, apperrors.ErrNotFound
		}
		r.logger.WithError(err).Error("failed to get transaction")
		return nil, err
	}
	txn, err := transactionFrom(row.ID, row.UserID, row.CategoryID, row.MerchantName,
		row.Amount, row.TransactionDate, row.ReceiptImageUrl, row.Notes,
		row.CreatedAt, row.UpdatedAt, row.Status)
	if err != nil {
		return nil, err
	}
	return &txn, nil
}

func (r *transactionRepository) GetTransactionForDeletion(ctx context.Context, userID, transactionID uuid.UUID) (*domain.Transaction, error) {
	row, err := r.queries.GetTransactionForDeletion(ctx, sqlc.GetTransactionForDeletionParams{
		ID:     database.UUIDParam(transactionID),
		UserID: database.UUIDParam(userID),
	})
	if err != nil {
		// Zero rows means the transaction either does not exist or belongs to
		// another user (the SQL carries both id AND user_id predicates). Both
		// map to ErrNotFound so a cross-user probe cannot distinguish the two.
		if database.IsNoRows(err) {
			return nil, apperrors.ErrNotFound
		}
		r.logger.WithError(err).Error("failed to get transaction")
		return nil, err
	}
	txn, err := transactionFrom(row.ID, row.UserID, row.CategoryID, row.MerchantName,
		row.Amount, row.TransactionDate, row.ReceiptImageUrl, row.Notes,
		row.CreatedAt, row.UpdatedAt, row.Status)
	if err != nil {
		return nil, err
	}
	return &txn, nil
}

func (r *transactionRepository) ListTransactions(ctx context.Context, input domain.ListTransactionsInput) ([]domain.Transaction, error) {
	// Defensive normalization so an invalid value can never reach PostgreSQL
	// (LIMIT/OFFSET reject negative numbers with an error). The service layer
	// also validates user-facing input, but SQL safety is not delegated.
	limit := input.Limit
	if limit <= 0 {
		limit = domain.DefaultListLimit
	}
	offset := input.Offset
	if offset < 0 {
		offset = 0
	}

	rows, err := r.queries.ListTransactions(ctx, sqlc.ListTransactionsParams{
		UserID:    database.UUIDParam(input.UserID),
		StartDate: database.NullableTimeParam(input.StartDate),
		EndDate:   database.NullableTimeParam(input.EndDate),
		RowLimit:  int32(limit),
		RowOffset: int32(offset),
	})
	if err != nil {
		r.logger.WithError(err).Error("failed to list transactions")
		return nil, err
	}
	// make() yields a non-nil (possibly empty) slice, so callers can safely
	// range over / JSON-marshal the result without a nil check.
	result := make([]domain.Transaction, len(rows))
	for i, row := range rows {
		txn, err := transactionFrom(row.ID, row.UserID, row.CategoryID, row.MerchantName,
			row.Amount, row.TransactionDate, row.ReceiptImageUrl, row.Notes,
			row.CreatedAt, row.UpdatedAt, row.Status)
		if err != nil {
			return nil, err
		}
		result[i] = txn
	}
	return result, nil
}

func (r *transactionRepository) UpdateTransaction(ctx context.Context, input domain.UpdateTransactionInput) (*domain.Transaction, error) {
	var amount pgtype.Numeric
	if input.Amount != nil {
		var err error
		amount, err = amountParam(*input.Amount)
		if err != nil {
			return nil, err
		}
	}
	row, err := r.queries.UpdateTransaction(ctx, sqlc.UpdateTransactionParams{
		ID:     database.UUIDParam(input.TransactionID),
		UserID: database.UUIDParam(input.UserID),

		// Each set-flag is derived from the matching pointer being non-nil, so
		// the SQL CASE writes only the fields the caller supplied. The value
		// passed alongside a false flag may be NULL (the CASE ignores it).
		SetMerchantName:    input.MerchantName != nil,
		MerchantName:       input.MerchantName,
		SetAmount:          input.Amount != nil,
		Amount:             amount,
		SetTransactionDate: input.TransactionDate != nil,
		TransactionDate:    database.NullableTimeParam(input.TransactionDate),
		SetCategoryID:      input.CategoryID != nil,
		CategoryID:         database.NullableUUIDParam(input.CategoryID),
		SetReceiptImageUrl: input.ReceiptImageUrl != nil || input.ClearReceiptURL,
		ReceiptImageUrl:    input.ReceiptImageUrl,
		SetNotes:           input.Notes != nil || input.ClearNotes,
		Notes:              input.Notes,
	})
	if err != nil {
		// Zero rows matched. That happens when: the transaction is missing; it
		// belongs to another user (WHERE user_id guard); or, when changing the
		// category, the target category is missing or another user's (EXISTS
		// guard). All collapse to ErrNotFound so nothing about existence or
		// ownership leaks, and the row is left untouched.
		if database.IsNoRows(err) {
			return nil, apperrors.ErrNotFound
		}
		// CHECK (amount > 0) on a supplied amount, or any NOT NULL violation, is
		// invalid input rather than an opaque internal failure.
		if database.IsCheckViolation(err) || database.IsNotNullViolation(err) {
			return nil, apperrors.ErrInvalidInput
		}
		if database.IsForeignKeyViolation(err) {
			return nil, apperrors.ErrInvalidInput
		}
		r.logger.WithError(err).Error("failed to update transaction")
		return nil, err
	}
	txn, err := transactionFrom(row.ID, row.UserID, row.CategoryID, row.MerchantName,
		row.Amount, row.TransactionDate, row.ReceiptImageUrl, row.Notes,
		row.CreatedAt, row.UpdatedAt, row.Status)
	if err != nil {
		return nil, err
	}
	return &txn, nil
}

func (r *transactionRepository) DeleteTransaction(ctx context.Context, userID, transactionID uuid.UUID) error {
	rows, err := r.queries.DeleteTransaction(ctx, sqlc.DeleteTransactionParams{
		ID:     database.UUIDParam(transactionID),
		UserID: database.UUIDParam(userID),
	})
	if err != nil {
		r.logger.WithError(err).Error("failed to delete transaction")
		return err
	}
	// Zero rows affected means the transaction is missing or belongs to another
	// user; both are reported identically as ErrNotFound.
	if rows == 0 {
		return apperrors.ErrNotFound
	}
	return nil
}

func (r *transactionRepository) SaveEmbedding(ctx context.Context, transactionID, userID uuid.UUID, embedding []float32, updatedAt time.Time) error {
	literal, err := embeddingLiteral(embedding)
	if err != nil {
		return err
	}

	rows, err := r.queries.SaveEmbedding(ctx, sqlc.SaveEmbeddingParams{
		Embedding: literal,
		ID:        database.UUIDParam(transactionID),
		UserID:    database.UUIDParam(userID),
		UpdatedAt: database.TimeParam(updatedAt),
	})
	if err != nil {
		r.logger.WithError(err).Error("failed to save transaction embedding")
		return err
	}
	if rows == 0 {
		return apperrors.ErrNotFound
	}
	return nil
}

func (r *transactionRepository) SearchSimilarTransactions(ctx context.Context, input domain.SearchSimilarTransactionsInput) ([]domain.SimilarTransaction, error) {
	literal, err := embeddingLiteral(input.QueryEmbedding)
	if err != nil {
		return nil, err
	}

	limit := input.Limit
	if limit <= 0 {
		limit = domain.DefaultListLimit
	}
	rows, err := r.queries.SearchSimilarTransactions(ctx, sqlc.SearchSimilarTransactionsParams{
		QueryEmbedding: literal,
		UserID:         database.UUIDParam(input.UserID),
		MatchThreshold: input.MatchThreshold,
		StartDate:      database.NullableTimeParam(input.StartDate),
		EndDate:        database.NullableTimeParam(input.EndDate),
		MatchCount:     int32(limit),
	})
	if err != nil {
		r.logger.WithError(err).Error("failed to search similar transactions")
		return nil, err
	}

	result := make([]domain.SimilarTransaction, len(rows))
	for i, row := range rows {
		item, err := similarTransactionFrom(row)
		if err != nil {
			return nil, err
		}
		result[i] = item
	}
	return result, nil
}

func embeddingLiteral(embedding []float32) (string, error) {
	if len(embedding) != domain.EmbeddingDimension {
		return "", apperrors.ErrInvalidInput
	}

	var builder strings.Builder
	builder.WriteByte('[')
	for i, value := range embedding {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return "", apperrors.ErrInvalidInput
		}
		if i > 0 {
			builder.WriteByte(',')
		}
		builder.WriteString(strconv.FormatFloat(float64(value), 'g', -1, 32))
	}
	builder.WriteByte(']')
	return builder.String(), nil
}

// Ensure the concrete type satisfies the interface at compile time.
var _ domain.TransactionRepository = (*transactionRepository)(nil)
