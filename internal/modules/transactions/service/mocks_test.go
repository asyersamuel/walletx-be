package service

import (
	"context"
	"sync"
	"time"
	domain "walletx-be/internal/modules/transactions/contracts"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
)

// mockEmbeddingProvider is the testify/mock implementation of
// EmbeddingProvider used by service and job tests.
type mockEmbeddingProvider struct{ mock.Mock }

func (m *mockEmbeddingProvider) Generate(ctx context.Context, text string) ([]float32, error) {
	args := m.Called(ctx, text)
	vector, _ := args.Get(0).([]float32)
	return vector, args.Error(1)
}

var _ domain.EmbeddingProvider = (*mockEmbeddingProvider)(nil)

// mockEmbeddingService keeps existing tests readable while sharing the same
// generated-style implementation and contract.
type mockEmbeddingService = mockEmbeddingProvider

// mockTransactionRepository is the testify/mock implementation of
// TransactionRepository used by unit tests.
//
// mu is a companion mutex used only by NumberOfCalls. Testify holds its own
// internal lock during Called/MethodCalled, so all call recording is safe.
// NumberOfCalls reads Mock.Calls outside testify's lock; mu ensures the read
// is race-free when goroutines call SaveEmbedding concurrently during async
// embedding tests.
type mockTransactionRepository struct {
	mock.Mock
	mu sync.Mutex
}

func (m *mockTransactionRepository) Create(ctx context.Context, input domain.CreateTransactionInput) (*domain.Transaction, error) {
	args := m.Called(ctx, input)
	transaction, _ := args.Get(0).(*domain.Transaction)
	return transaction, args.Error(1)
}

func (m *mockTransactionRepository) GetTransactionByID(ctx context.Context, userID, transactionID uuid.UUID) (*domain.Transaction, error) {
	args := m.Called(ctx, userID, transactionID)
	transaction, _ := args.Get(0).(*domain.Transaction)
	return transaction, args.Error(1)
}

func (m *mockTransactionRepository) ListTransactions(ctx context.Context, input domain.ListTransactionsInput) ([]domain.Transaction, error) {
	args := m.Called(ctx, input)
	transactions, _ := args.Get(0).([]domain.Transaction)
	return transactions, args.Error(1)
}

func (m *mockTransactionRepository) UpdateTransaction(ctx context.Context, input domain.UpdateTransactionInput) (*domain.Transaction, error) {
	args := m.Called(ctx, input)
	transaction, _ := args.Get(0).(*domain.Transaction)
	return transaction, args.Error(1)
}

func (m *mockTransactionRepository) DeleteTransaction(ctx context.Context, userID, transactionID uuid.UUID) error {
	return m.Called(ctx, userID, transactionID).Error(0)
}

func (m *mockTransactionRepository) SaveEmbedding(ctx context.Context, transactionID, userID uuid.UUID, vector []float32, updatedAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Called(ctx, transactionID, userID, vector, updatedAt).Error(0)
}

func (m *mockTransactionRepository) SearchSimilarTransactions(ctx context.Context, input domain.SearchSimilarTransactionsInput) ([]domain.SimilarTransaction, error) {
	args := m.Called(ctx, input)
	transactions, _ := args.Get(0).([]domain.SimilarTransaction)
	return transactions, args.Error(1)
}

// NumberOfCalls returns how many times the named method has been called.
// It is safe to call from goroutines: SaveEmbedding holds mu while recording
// the call, so this reader also takes mu to guarantee a consistent read.
func (m *mockTransactionRepository) NumberOfCalls(methodName string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, c := range m.Mock.Calls {
		if c.Method == methodName {
			count++
		}
	}
	return count
}

var _ domain.TransactionRepository = (*mockTransactionRepository)(nil)
