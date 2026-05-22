package services_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"walletx-be/internal/domain/dto"
	"walletx-be/internal/ports"
	"walletx-be/pkg/models"
	"walletx-be/pkg/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockTransactionRepository struct {
	services.TransactionRepository
	mock.Mock
}

func (m *MockTransactionRepository) Create(ctx context.Context, transaction *models.Transaction) error {
	args := m.Called(ctx, transaction)
	return args.Error(0)
}

type MockCacheManager struct {
	ports.DashboardCacheManager
	mock.Mock
}

func (m *MockCacheManager) InvalidateUserCache(ctx context.Context, userID string) error {
	args := m.Called(ctx, userID)
	return args.Error(0)
}

type MockLogger struct {
	ports.Logger
	mock.Mock
}

func (m *MockLogger) Info(msg string) { m.Called(msg) }
func (m *MockLogger) Warn(msg string) { m.Called(msg) }
func (m *MockLogger) Error(msg string) { m.Called(msg) }
func (m *MockLogger) Debug(msg string) { m.Called(msg) }
func (m *MockLogger) WithField(key string, value interface{}) ports.Logger {
	m.Called(key, value)
	return m
}
func (m *MockLogger) WithError(err error) ports.Logger {
	m.Called(err)
	return m
}
func (m *MockLogger) WithFields(fields map[string]interface{}) ports.Logger {
	m.Called(fields)
	return m
}

func TestCreateManualTransaction(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	categoryID := uuid.New()
	now := time.Now()

	tests := []struct {
		name          string
		input         dto.CreateTransactionInput
		setupMocks    func(txRepo *MockTransactionRepository, cache *MockCacheManager, logger *MockLogger)
		expectedErr   string
		expectedNilTx bool
	}{
		{
			name: "Success - Transaction Created",
			input: dto.CreateTransactionInput{
				Amount:          50000,
				Merchant:        "Nasi Goreng",
				CategoryID:      &categoryID,
				TransactionDate: now,
			},
			setupMocks: func(txRepo *MockTransactionRepository, cache *MockCacheManager, logger *MockLogger) {
				txRepo.On("Create", ctx, mock.AnythingOfType("*models.Transaction")).Return(nil)
				cache.On("InvalidateUserCache", ctx, userID.String()).Return(nil)
				logger.On("WithField", "user_id", userID).Return(logger)
				logger.On("Info", "Manual transaction saved successfully")
			},
			expectedErr:   "",
			expectedNilTx: false,
		},
		{
			name: "Error - Amount Zero",
			input: dto.CreateTransactionInput{
				Amount:          0,
				Merchant:        "Nasi Goreng",
				TransactionDate: now,
			},
			setupMocks: func(txRepo *MockTransactionRepository, cache *MockCacheManager, logger *MockLogger) {
				// No mock calls expected
			},
			expectedErr:   "amount must be greater than 0",
			expectedNilTx: true,
		},
		{
			name: "Error - Empty Merchant",
			input: dto.CreateTransactionInput{
				Amount:          10000,
				Merchant:        "",
				TransactionDate: now,
			},
			setupMocks: func(txRepo *MockTransactionRepository, cache *MockCacheManager, logger *MockLogger) {
				// No mock calls expected
			},
			expectedErr:   "merchant cannot be empty",
			expectedNilTx: true,
		},
		{
			name: "Error - Repository Failed",
			input: dto.CreateTransactionInput{
				Amount:          50000,
				Merchant:        "Nasi Goreng",
				CategoryID:      &categoryID,
				TransactionDate: now,
			},
			setupMocks: func(txRepo *MockTransactionRepository, cache *MockCacheManager, logger *MockLogger) {
				dbErr := errors.New("database connection failed")
				txRepo.On("Create", ctx, mock.AnythingOfType("*models.Transaction")).Return(dbErr)
				logger.On("WithError", dbErr).Return(logger)
				logger.On("Error", "Failed to save manual transaction")
			},
			expectedErr:   "database connection failed",
			expectedNilTx: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockTxRepo := new(MockTransactionRepository)
			mockCache := new(MockCacheManager)
			mockLogger := new(MockLogger)

			tt.setupMocks(mockTxRepo, mockCache, mockLogger)

			service := services.NewTransactionService(
				nil, // userRepo
				mockTxRepo,
				nil, // categoryRepo
				nil, // txProcessor
				mockCache,
				mockLogger,
			)

			tx, err := service.CreateManualTransaction(ctx, userID, tt.input)

			if tt.expectedErr != "" {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErr, err.Error())
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, tx)
				assert.Equal(t, tt.input.Amount, tx.Amount)
				assert.Equal(t, tt.input.Merchant, tx.Merchant)
				assert.Equal(t, userID, tx.UserID)
			}

			if tt.expectedNilTx {
				assert.Nil(t, tx)
			}

			mockTxRepo.AssertExpectations(t)
			mockCache.AssertExpectations(t)
			mockLogger.AssertExpectations(t)
		})
	}
}
