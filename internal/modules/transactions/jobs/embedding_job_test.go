package jobs

import (
	"context"
	"errors"
	"testing"
	"time"
	domain "walletx-be/internal/modules/transactions/contracts"
	"walletx-be/internal/platform/logger"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type embeddingJobTestLogger struct{}

func (embeddingJobTestLogger) Info(string)  {}
func (embeddingJobTestLogger) Warn(string)  {}
func (embeddingJobTestLogger) Error(string) {}
func (embeddingJobTestLogger) Debug(string) {}
func (l embeddingJobTestLogger) WithField(string, interface{}) logger.Logger {
	return l
}
func (l embeddingJobTestLogger) WithError(error) logger.Logger { return l }
func (l embeddingJobTestLogger) WithFields(map[string]interface{}) logger.Logger {
	return l
}

func TestEmbeddingJob_RetriesTransientFailuresUpToThreeAttempts(t *testing.T) {
	provider := new(mockEmbeddingProvider)
	repo := new(mockTransactionRepository)
	vector := validEmbedding()
	provider.On("Generate", mock.Anything, "transaction text").Return(nil, errors.New("temporary Gemini failure")).Twice()
	provider.On("Generate", mock.Anything, "transaction text").Return(vector, nil).Once()
	repo.On("SaveEmbedding", mock.Anything, mock.Anything, mock.Anything, vector, mock.Anything).Return(nil).Once()

	job := NewEmbeddingJob(repo, provider, embeddingJobTestLogger{}, EmbeddingJobConfig{
		MaxAttempts: 3,
		Backoff:     time.Nanosecond,
	})

	err := job.Run(context.Background(), EmbeddingJobInput{
		TransactionID: uuid.New(),
		UserID:        uuid.New(),
		UpdatedAt:     time.Now().UTC(),
		Text:          "transaction text",
	})

	require.NoError(t, err)
	provider.AssertNumberOfCalls(t, "Generate", 3)
	repo.AssertNumberOfCalls(t, "SaveEmbedding", 1)
}

func TestEmbeddingJob_DoesNotRetryInvalidDimensions(t *testing.T) {
	provider := new(mockEmbeddingProvider)
	repo := new(mockTransactionRepository)
	provider.On("Generate", mock.Anything, "transaction text").Return([]float32{1}, nil).Once()

	job := NewEmbeddingJob(repo, provider, embeddingJobTestLogger{}, EmbeddingJobConfig{
		MaxAttempts: 3,
		Backoff:     time.Nanosecond,
	})

	err := job.Run(context.Background(), EmbeddingJobInput{
		TransactionID: uuid.New(),
		UserID:        uuid.New(),
		UpdatedAt:     time.Now().UTC(),
		Text:          "transaction text",
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, errInvalidEmbeddingDimensions)
	provider.AssertNumberOfCalls(t, "Generate", 1)
	repo.AssertNotCalled(t, "SaveEmbedding", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestEmbeddingJob_PassesUpdatedAtToRepositoryForStaleProtection(t *testing.T) {
	provider := new(mockEmbeddingProvider)
	repo := new(mockTransactionRepository)
	transactionID := uuid.New()
	userID := uuid.New()
	updatedAt := time.Date(2026, 10, 2, 12, 30, 0, 123000000, time.UTC)
	vector := validEmbedding()

	provider.On("Generate", mock.Anything, "transaction text").Return(vector, nil).Once()
	repo.On("SaveEmbedding", mock.Anything, transactionID, userID, vector, updatedAt).Return(nil).Once()

	job := NewEmbeddingJob(repo, provider, embeddingJobTestLogger{}, EmbeddingJobConfig{MaxAttempts: 1})
	err := job.Run(context.Background(), EmbeddingJobInput{
		TransactionID: transactionID,
		UserID:        userID,
		UpdatedAt:     updatedAt,
		Text:          "transaction text",
	})

	require.NoError(t, err)
	repo.AssertExpectations(t)
}

func validEmbedding() []float32 {
	return make([]float32, domain.EmbeddingDimension)
}
