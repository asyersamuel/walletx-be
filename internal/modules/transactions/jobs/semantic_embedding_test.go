package jobs

import (
	"context"
	"testing"
	"time"
	domain "walletx-be/internal/modules/transactions/contracts"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestTransactionService_UpdateTransaction_TriggersEmbeddingForSemanticFields(t *testing.T) {
	semanticInputs := []struct {
		name  string
		input domain.UpdateTransactionInput
	}{
		{name: "amount", input: domain.UpdateTransactionInput{Amount: floatPtr(25)}},
		{name: "merchant", input: domain.UpdateTransactionInput{MerchantName: strPtr("new merchant")}},
		{name: "category", input: domain.UpdateTransactionInput{CategoryID: uuidPtr(uuid.New())}},
		{name: "notes", input: domain.UpdateTransactionInput{Notes: strPtr("new notes")}},
		{name: "cleared notes", input: domain.UpdateTransactionInput{ClearNotes: true}},
	}

	for _, test := range semanticInputs {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			userID := uuid.New()
			transactionID := uuid.New()
			updatedAt := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
			updated := &domain.Transaction{ID: transactionID, UserID: userID, UpdatedAt: updatedAt}
			repo := new(mockTransactionRepository)
			repo.On("UpdateTransaction", ctx, mock.Anything).Return(updated, nil).Once()
			embedding := new(mockEmbeddingProvider)
			generated := make(chan struct{}, 1)
			embedding.On("Generate", mock.Anything, mock.Anything).
				Run(func(mock.Arguments) { generated <- struct{}{} }).
				Return(validEmbedding(), nil).Once()
			repo.On("SaveEmbedding", mock.Anything, transactionID, userID, validEmbedding(), updatedAt).Return(nil).Once()

			_, err := NewTransactionService(repo, embedding).UpdateTransaction(ctx, userID, transactionID, test.input)

			require.NoError(t, err)
			select {
			case <-generated:
			case <-time.After(time.Second):
				t.Fatal("semantic update did not trigger embedding generation")
			}
			assert.Eventually(t, func() bool {
				return repo.NumberOfCalls("SaveEmbedding") == 1
			}, time.Second, time.Millisecond)
		})
	}
}

func TestTransactionService_UpdateTransaction_DoesNotTriggerEmbeddingForReceiptOnlyChange(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	transactionID := uuid.New()
	updated := &domain.Transaction{ID: transactionID, UserID: userID, UpdatedAt: time.Now().UTC()}
	repo := new(mockTransactionRepository)
	repo.On("UpdateTransaction", ctx, mock.Anything).Return(updated, nil).Once()
	embedding := new(mockEmbeddingProvider)

	_, err := NewTransactionService(repo, embedding).UpdateTransaction(ctx, userID, transactionID, domain.UpdateTransactionInput{
		ReceiptImageUrl: strPtr("https://example.test/receipt.png"),
	})

	require.NoError(t, err)
	time.Sleep(20 * time.Millisecond)
	embedding.AssertNotCalled(t, "Generate", mock.Anything, mock.Anything)
	repo.AssertNotCalled(t, "SaveEmbedding", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func floatPtr(value float64) *float64 { return &value }
