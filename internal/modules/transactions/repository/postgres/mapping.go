package postgres

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	domain "walletx-be/internal/modules/transactions/contracts"
	sqlc "walletx-be/internal/platform/database/sqlc"
	apperrors "walletx-be/internal/shared/errors"
)

// Mapping failures describe persistence integrity, not user input. Do not include
// the database value in the message: receipt fields may contain personal data.
func malformedRow(field string) error {
	return fmt.Errorf("invalid transaction database field %s: %w", field, apperrors.ErrInternal)
}

func requiredUUID(value pgtype.UUID, field string) (uuid.UUID, error) {
	if !value.Valid || value.Bytes == [16]byte{} {
		return uuid.Nil, malformedRow(field)
	}
	return uuid.UUID(value.Bytes), nil
}

func optionalUUID(value pgtype.UUID, field string) (*uuid.UUID, error) {
	if !value.Valid {
		return nil, nil
	}
	result, err := requiredUUID(value, field)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func requiredTime(value pgtype.Timestamptz, field string) (time.Time, error) {
	if !value.Valid || value.InfinityModifier != pgtype.Finite || value.Time.IsZero() {
		return time.Time{}, malformedRow(field)
	}
	return value.Time, nil
}

func requiredText(value *string, field string) (string, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return "", malformedRow(field)
	}
	return *value, nil
}

func amountValue(value pgtype.Numeric) (float64, error) {
	if !value.Valid || value.Int == nil || value.NaN || value.InfinityModifier != pgtype.Finite {
		return 0, malformedRow("amount")
	}
	// pgx's conversion applies both the arbitrary-precision coefficient and
	// decimal exponent. Reading Int alone would silently change receipt amounts.
	converted, err := value.Float64Value()
	if err != nil || !converted.Valid || converted.Float64 <= 0 || math.IsNaN(converted.Float64) || math.IsInf(converted.Float64, 0) {
		return 0, malformedRow("amount")
	}
	return converted.Float64, nil
}

// amountParam preserves the shortest decimal representation of the existing
// float64 domain amount. A bare float64 cannot encode nullable pgtype.Numeric.
func amountParam(value float64) (pgtype.Numeric, error) {
	if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return pgtype.Numeric{}, fmt.Errorf("amount must be finite and positive: %w", apperrors.ErrInvalidInput)
	}
	var result pgtype.Numeric
	if err := result.Scan(strconv.FormatFloat(value, 'f', -1, 64)); err != nil {
		return pgtype.Numeric{}, fmt.Errorf("encode transaction amount: %w", apperrors.ErrInvalidInput)
	}
	return result, nil
}

// sqlc emits separate row types with the same business projection. This shared
// mapper validates metadata and permits absent extraction fields only on pending rows.
func transactionFrom(
	id, userID, categoryID pgtype.UUID,
	merchantName *string,
	amount pgtype.Numeric,
	transactionDate pgtype.Timestamptz,
	receiptImageURL, notes *string,
	createdAt, updatedAt pgtype.Timestamptz,
	status string,
) (domain.Transaction, error) {
	state := domain.TransactionStatus(status)
	if state != domain.TransactionStatusSuccess && state != domain.TransactionStatusPending {
		return domain.Transaction{}, malformedRow("status")
	}
	result, err := transactionDetailsFrom(id, userID, categoryID, merchantName, amount, transactionDate, state)
	if err != nil {
		return domain.Transaction{}, err
	}
	if result.CreatedAt, err = requiredTime(createdAt, "created_at"); err != nil {
		return domain.Transaction{}, err
	}
	if result.UpdatedAt, err = requiredTime(updatedAt, "updated_at"); err != nil {
		return domain.Transaction{}, err
	}
	result.ReceiptImageUrl, result.Notes = receiptImageURL, notes
	return result, nil
}

func transactionDetailsFrom(
	id, userID, categoryID pgtype.UUID,
	merchantName *string, amount pgtype.Numeric, date pgtype.Timestamptz,
	status domain.TransactionStatus,
) (domain.Transaction, error) {
	result := domain.Transaction{Status: status}
	var err error
	if result.ID, err = requiredUUID(id, "id"); err != nil {
		return domain.Transaction{}, err
	}
	if result.UserID, err = requiredUUID(userID, "user_id"); err != nil {
		return domain.Transaction{}, err
	}
	if result.CategoryID, err = optionalUUID(categoryID, "category_id"); err != nil {
		return domain.Transaction{}, err
	}
	if merchantName != nil || status != domain.TransactionStatusPending {
		if result.MerchantName, err = requiredText(merchantName, "merchant_name"); err != nil {
			return domain.Transaction{}, err
		}
	}
	if amount.Valid || status != domain.TransactionStatusPending {
		if result.Amount, err = amountValue(amount); err != nil {
			return domain.Transaction{}, err
		}
	}
	if date.Valid || status != domain.TransactionStatusPending {
		if result.TransactionDate, err = requiredTime(date, "transaction_date"); err != nil {
			return domain.Transaction{}, err
		}
	}
	return result, nil
}

func pendingTransactionFrom(
	id, userID pgtype.UUID, receiptImageURL *string,
	createdAt, updatedAt pgtype.Timestamptz, status string,
) (domain.PendingTransaction, error) {
	if domain.TransactionStatus(status) != domain.TransactionStatusPending {
		return domain.PendingTransaction{}, malformedRow("status")
	}
	var result domain.PendingTransaction
	var err error
	if result.ID, err = requiredUUID(id, "id"); err != nil {
		return domain.PendingTransaction{}, err
	}
	if result.UserID, err = requiredUUID(userID, "user_id"); err != nil {
		return domain.PendingTransaction{}, err
	}
	if result.ReceiptImageURL, err = requiredText(receiptImageURL, "receipt_image_url"); err != nil {
		return domain.PendingTransaction{}, err
	}
	if result.CreatedAt, err = requiredTime(createdAt, "created_at"); err != nil {
		return domain.PendingTransaction{}, err
	}
	if result.UpdatedAt, err = requiredTime(updatedAt, "updated_at"); err != nil {
		return domain.PendingTransaction{}, err
	}
	return result, nil
}

func similarTransactionFrom(row sqlc.SearchSimilarTransactionsRow) (domain.SimilarTransaction, error) {
	if domain.TransactionStatus(row.Status) != domain.TransactionStatusSuccess {
		return domain.SimilarTransaction{}, malformedRow("status")
	}
	details, err := transactionDetailsFrom(row.ID, row.UserID, row.CategoryID, row.MerchantName, row.Amount, row.TransactionDate, domain.TransactionStatusSuccess)
	if err != nil {
		return domain.SimilarTransaction{}, err
	}
	if math.IsNaN(row.Similarity) || math.IsInf(row.Similarity, 0) {
		return domain.SimilarTransaction{}, malformedRow("similarity")
	}
	return domain.SimilarTransaction{
		ID: details.ID, UserID: details.UserID, CategoryID: details.CategoryID,
		MerchantName: details.MerchantName, Amount: details.Amount,
		TransactionDate: details.TransactionDate, Notes: row.Notes,
		CategoryName: row.CategoryName, CategoryType: row.CategoryType, Similarity: row.Similarity,
	}, nil
}
