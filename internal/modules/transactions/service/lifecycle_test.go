package service

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	domain "walletx-be/internal/modules/transactions/contracts"
	apperrors "walletx-be/internal/shared/errors"
)

func TestTransactionService_PendingUsesAuthenticatedOwnerWithoutEmbedding(t *testing.T) {
	user := uuid.New()
	reference := "receipt://" + user.String() + "/" + uuid.NewString() + ".png"
	repo := new(mockTransactionRepository)
	embedding := new(mockEmbeddingService)
	expected := domain.CreatePendingTransactionInput{UserID: user, ReceiptImageURL: reference}
	pending := &domain.PendingTransaction{ID: uuid.New(), UserID: user, ReceiptImageURL: reference}
	repo.On("CreatePendingTransaction", mock.Anything, expected).Return(pending, nil).Once()
	got, err := NewTransactionService(repo, embedding).CreatePendingTransaction(context.Background(), user, domain.CreatePendingTransactionInput{UserID: uuid.New(), ReceiptImageURL: reference})
	require.NoError(t, err)
	require.Same(t, pending, got)
	repo.AssertExpectations(t)
	require.Empty(t, embedding.Calls)
	for _, input := range []domain.CreatePendingTransactionInput{{}, {ReceiptImageURL: " "}} {
		_, err := NewTransactionService(new(mockTransactionRepository)).CreatePendingTransaction(context.Background(), user, input)
		require.ErrorIs(t, err, apperrors.ErrInvalidInput)
	}
}

func TestTransactionService_CompletionEmbedsCommittedState(t *testing.T) {
	user, id, category := uuid.New(), uuid.New(), uuid.New()
	saved := &domain.Transaction{ID: id, UserID: user, CategoryID: &category, MerchantName: "Cafe", Amount: 123.45, UpdatedAt: time.Now()}
	repo := new(mockTransactionRepository)
	embedding := new(mockEmbeddingService)
	vector := validEmbedding()
	done := make(chan struct{}, 1)
	repo.On("CompletePendingTransaction", mock.Anything, mock.MatchedBy(func(i domain.CompletePendingTransactionInput) bool {
		return i.UserID == user && i.TransactionID == id && i.MerchantName == "Cafe"
	})).Return(saved, nil).Once()
	repo.On("SaveEmbedding", mock.Anything, id, user, vector, saved.UpdatedAt).Run(func(mock.Arguments) { done <- struct{}{} }).Return(nil).Once()
	embedding.On("Generate", mock.Anything, "merchant_name: Cafe\namount: 123.45\ncategory: Food").Return(vector, nil).Once()
	got, err := NewTransactionService(repo, embedding).CompletePendingTransaction(context.Background(), user, id, domain.CompletePendingTransactionInput{
		UserID: uuid.New(), TransactionID: uuid.New(), CategoryID: category, CategoryName: "Food", MerchantName: " Cafe ", Amount: 123.45, TransactionDate: time.Now(),
	})
	require.NoError(t, err)
	require.Same(t, saved, got)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("completed receipt was not embedded")
	}
	repo.AssertExpectations(t)
	embedding.AssertExpectations(t)
}

func TestTransactionService_InvalidAndFailedCompletionsDoNotEmbed(t *testing.T) {
	user, id, category := uuid.New(), uuid.New(), uuid.New()
	input := domain.CompletePendingTransactionInput{CategoryID: category, MerchantName: "Cafe", Amount: 10, TransactionDate: time.Now()}
	for _, amount := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		repo := new(mockTransactionRepository)
		embedding := new(mockEmbeddingService)
		bad := input
		bad.Amount = amount
		_, err := NewTransactionService(repo, embedding).CompletePendingTransaction(context.Background(), user, id, bad)
		require.ErrorIs(t, err, apperrors.ErrInvalidInput)
		require.Empty(t, repo.Calls)
		require.Empty(t, embedding.Calls)
	}
	for _, failure := range []error{apperrors.ErrNotFound, apperrors.ErrInternal} {
		repo := new(mockTransactionRepository)
		embedding := new(mockEmbeddingService)
		repo.On("CompletePendingTransaction", mock.Anything, mock.Anything).Return(nil, failure).Once()
		_, err := NewTransactionService(repo, embedding).CompletePendingTransaction(context.Background(), user, id, input)
		require.ErrorIs(t, err, failure)
		repo.AssertExpectations(t)
		require.Empty(t, embedding.Calls)
	}
}
