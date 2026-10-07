package service

import (
	"context"
	"strings"

	"github.com/google/uuid"
	domain "walletx-be/internal/modules/transactions/contracts"
	apperrors "walletx-be/internal/shared/errors"
)

func (s *transactionService) CreatePendingTransaction(ctx context.Context, userID uuid.UUID, input domain.CreatePendingTransactionInput) (*domain.PendingTransaction, error) {
	if ctx == nil || userID == uuid.Nil || strings.TrimSpace(input.ReceiptImageURL) == "" {
		return nil, invalidInputError("pending receipt requires a context, owner and image reference")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	input.UserID = userID
	pending, err := s.repo.CreatePendingTransaction(ctx, input)
	if err != nil {
		return nil, mapRepositoryError(err)
	}
	if pending == nil || pending.ID == uuid.Nil || pending.UserID != userID || pending.ReceiptImageURL != input.ReceiptImageURL {
		return nil, apperrors.ErrInternal
	}
	return pending, nil
}

func (s *transactionService) CompletePendingTransaction(ctx context.Context, userID, transactionID uuid.UUID, input domain.CompletePendingTransactionInput) (*domain.Transaction, error) {
	if ctx == nil || transactionID == uuid.Nil {
		return nil, invalidInputError("completion requires a context and transaction ID")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	normalized, err := normalizeCreateInput(userID, domain.CreateTransactionInput{
		CategoryID: input.CategoryID, CategoryName: input.CategoryName, MerchantName: input.MerchantName,
		Amount: input.Amount, TransactionDate: input.TransactionDate, Notes: input.Notes,
	})
	if err != nil {
		return nil, err
	}
	input.UserID, input.TransactionID, input.MerchantName = userID, transactionID, normalized.MerchantName
	completed, err := s.repo.CompletePendingTransaction(ctx, input)
	if err != nil {
		return nil, mapRepositoryError(err)
	}
	if completed == nil || completed.ID != transactionID || completed.UserID != userID {
		return nil, apperrors.ErrInternal
	}
	s.enqueueEmbedding(completed, input.CategoryName)
	return completed, nil
}
