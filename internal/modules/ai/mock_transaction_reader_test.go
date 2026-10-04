package ai

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
)

type MockTransactionReader struct{ mock.Mock }

var _ TransactionReader = (*MockTransactionReader)(nil)

func newMockTransactionReader(t *testing.T) *MockTransactionReader {
	t.Helper()
	m := &MockTransactionReader{}
	m.Test(t)
	t.Cleanup(func() { m.AssertExpectations(t) })
	return m
}

func (m *MockTransactionReader) MatchTransactions(ctx context.Context, userID uuid.UUID, embedding []float32, matchThreshold float64, matchCount int) ([]RetrievedTransaction, error) {
	args := m.Called(ctx, userID, embedding, matchThreshold, matchCount)
	value, _ := args.Get(0).([]RetrievedTransaction)
	return value, args.Error(1)
}
