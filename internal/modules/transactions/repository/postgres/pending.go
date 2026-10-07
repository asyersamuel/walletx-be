package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	domain "walletx-be/internal/modules/transactions/contracts"
	"walletx-be/internal/platform/database"
	sqlc "walletx-be/internal/platform/database/sqlc"
	apperrors "walletx-be/internal/shared/errors"
)

func (r *transactionRepository) CreatePendingTransaction(ctx context.Context, input domain.CreatePendingTransactionInput) (*domain.PendingTransaction, error) {
	if input.UserID == uuid.Nil || strings.TrimSpace(input.ReceiptImageURL) == "" {
		return nil, fmt.Errorf("pending transaction owner and receipt reference are required: %w", apperrors.ErrInvalidInput)
	}
	row, err := r.queries.CreatePendingTransaction(ctx, sqlc.CreatePendingTransactionParams{
		UserID: database.UUIDParam(input.UserID), ReceiptImageUrl: &input.ReceiptImageURL,
	})
	if err != nil {
		return nil, r.pendingWriteError(err, "create_pending_transaction")
	}
	pending, err := pendingTransactionFrom(row.ID, row.UserID, row.ReceiptImageUrl, row.CreatedAt, row.UpdatedAt, row.Status)
	if err != nil {
		return nil, err
	}
	return &pending, nil
}

func (r *transactionRepository) CompletePendingTransaction(ctx context.Context, input domain.CompletePendingTransactionInput) (*domain.Transaction, error) {
	if input.UserID == uuid.Nil || input.TransactionID == uuid.Nil || input.CategoryID == uuid.Nil ||
		strings.TrimSpace(input.MerchantName) == "" || input.TransactionDate.IsZero() {
		return nil, fmt.Errorf("completion requires owner, transaction, category, merchant and date: %w", apperrors.ErrInvalidInput)
	}
	amount, err := amountParam(input.Amount)
	if err != nil {
		return nil, err
	}
	row, err := r.queries.CompletePendingTransaction(ctx, sqlc.CompletePendingTransactionParams{
		ID: database.UUIDParam(input.TransactionID), UserID: database.UUIDParam(input.UserID),
		CategoryID: database.UUIDParam(input.CategoryID), MerchantName: &input.MerchantName,
		Amount: amount, TransactionDate: database.TimeParam(input.TransactionDate), Notes: input.Notes,
	})
	if err != nil {
		return nil, r.pendingWriteError(err, "complete_pending_transaction")
	}
	transaction, err := transactionFrom(row.ID, row.UserID, row.CategoryID, row.MerchantName,
		row.Amount, row.TransactionDate, row.ReceiptImageUrl, row.Notes, row.CreatedAt, row.UpdatedAt, row.Status)
	if err != nil {
		return nil, err
	}
	return &transaction, nil
}

func (r *transactionRepository) pendingWriteError(err error, operation string) error {
	if database.IsNoRows(err) {
		return apperrors.ErrNotFound
	}
	if database.IsCheckViolation(err) || database.IsNotNullViolation(err) || database.IsForeignKeyViolation(err) {
		return apperrors.ErrInvalidInput
	}
	r.logger.WithError(err).WithFields(map[string]interface{}{"operation": operation}).Error("transaction persistence failed")
	return err
}
