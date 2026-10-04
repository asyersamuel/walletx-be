package ai

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
)

type MockAIChatProvider struct{ mock.Mock }

var _ AIChatProvider = (*MockAIChatProvider)(nil)

func newMockAIChatProvider(t *testing.T) *MockAIChatProvider {
	t.Helper()
	m := &MockAIChatProvider{}
	m.Test(t)
	t.Cleanup(func() { m.AssertExpectations(t) })
	return m
}

func (m *MockAIChatProvider) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	args := m.Called(ctx, text)
	value, _ := args.Get(0).([]float32)
	return value, args.Error(1)
}

func (m *MockAIChatProvider) StreamChat(ctx context.Context, request ProviderChatRequest) (ChatStream, error) {
	args := m.Called(ctx, request)
	value, _ := args.Get(0).(ChatStream)
	return value, args.Error(1)
}

func (m *MockAIChatProvider) ExtractTransaction(ctx context.Context, request ProviderExtractionRequest) (ExtractedTransaction, error) {
	args := m.Called(ctx, request)
	return args.Get(0).(ExtractedTransaction), args.Error(1)
}
