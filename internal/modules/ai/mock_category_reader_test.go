package ai

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
)

type MockCategoryReader struct{ mock.Mock }

var _ CategoryReader = (*MockCategoryReader)(nil)

func newMockCategoryReader(t *testing.T) *MockCategoryReader {
	t.Helper()
	m := &MockCategoryReader{}
	m.Test(t)
	t.Cleanup(func() { m.AssertExpectations(t) })
	return m
}

func (m *MockCategoryReader) ListByUser(ctx context.Context, userID uuid.UUID) ([]CategoryOption, error) {
	args := m.Called(ctx, userID)
	value, _ := args.Get(0).([]CategoryOption)
	return value, args.Error(1)
}
