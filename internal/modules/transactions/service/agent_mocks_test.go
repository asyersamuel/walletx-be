package service

import (
	"context"
	domain "walletx-be/internal/modules/transactions/contracts"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
)

type mockTransactionService struct {
	mock.Mock
}

func (m *mockTransactionService) CreateTransaction(ctx context.Context, userID uuid.UUID, input domain.CreateTransactionInput) (*domain.Transaction, error) {
	args := m.Called(ctx, userID, input)
	transaction, _ := args.Get(0).(*domain.Transaction)
	return transaction, args.Error(1)
}
func (m *mockTransactionService) GetTransaction(ctx context.Context, userID, transactionID uuid.UUID) (*domain.Transaction, error) {
	args := m.Called(ctx, userID, transactionID)
	transaction, _ := args.Get(0).(*domain.Transaction)
	return transaction, args.Error(1)
}
func (m *mockTransactionService) ListTransactions(ctx context.Context, userID uuid.UUID, input domain.ListTransactionsInput) ([]domain.Transaction, error) {
	args := m.Called(ctx, userID, input)
	transactions, _ := args.Get(0).([]domain.Transaction)
	return transactions, args.Error(1)
}
func (m *mockTransactionService) UpdateTransaction(ctx context.Context, userID, transactionID uuid.UUID, input domain.UpdateTransactionInput) (*domain.Transaction, error) {
	args := m.Called(ctx, userID, transactionID, input)
	transaction, _ := args.Get(0).(*domain.Transaction)
	return transaction, args.Error(1)
}
func (m *mockTransactionService) DeleteTransaction(ctx context.Context, userID, transactionID uuid.UUID) error {
	return m.Called(ctx, userID, transactionID).Error(0)
}

func (m *mockTransactionService) CreatePendingTransaction(ctx context.Context, userID uuid.UUID, input domain.CreatePendingTransactionInput) (*domain.PendingTransaction, error) {
	args := m.Called(ctx, userID, input)
	result, _ := args.Get(0).(*domain.PendingTransaction)
	return result, args.Error(1)
}
func (m *mockTransactionService) CompletePendingTransaction(ctx context.Context, userID, transactionID uuid.UUID, input domain.CompletePendingTransactionInput) (*domain.Transaction, error) {
	args := m.Called(ctx, userID, transactionID, input)
	result, _ := args.Get(0).(*domain.Transaction)
	return result, args.Error(1)
}
