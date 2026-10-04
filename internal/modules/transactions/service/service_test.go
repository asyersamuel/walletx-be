package service

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"
	domain "walletx-be/internal/modules/transactions/contracts"

	apperrors "walletx-be/internal/shared/errors"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestTransactionService_GetTransaction_PropagatesNotFoundWithoutLeak(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	transactionID := uuid.New()
	repo := new(mockTransactionRepository)
	repo.On("GetTransactionByID", ctx, userID, transactionID).Return(nil, apperrors.ErrNotFound).Once()

	got, err := NewTransactionService(repo).GetTransaction(ctx, userID, transactionID)

	assert.Nil(t, got)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
	repo.AssertExpectations(t)
}

func TestTransactionService_ListTransactions_NormalizesAndForwardsFilters(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 30, 23, 59, 59, 0, time.UTC)
	wantInput := domain.ListTransactionsInput{UserID: userID, Limit: domain.DefaultListLimit, Offset: 0, StartDate: &start, EndDate: &end}
	want := []domain.Transaction{{ID: uuid.New(), UserID: userID}}
	repo := new(mockTransactionRepository)
	repo.On("ListTransactions", ctx, wantInput).Return(want, nil).Once()

	got, err := NewTransactionService(repo).ListTransactions(ctx, userID, domain.ListTransactionsInput{
		Limit: 0, Offset: 0, StartDate: &start, EndDate: &end,
	})

	require.NoError(t, err)
	assert.Equal(t, want, got)
	repo.AssertExpectations(t)
}

func TestTransactionService_ListTransactions_RejectsNegativePaginationBeforeRepository(t *testing.T) {
	for _, input := range []domain.ListTransactionsInput{{Limit: -1}, {Offset: -1}} {
		repo := new(mockTransactionRepository)
		repo.On("ListTransactions", mock.Anything, mock.Anything).Maybe()

		got, err := NewTransactionService(repo).ListTransactions(context.Background(), uuid.New(), input)

		assert.Nil(t, got)
		assert.ErrorIs(t, err, apperrors.ErrInvalidInput)
		repo.AssertNotCalled(t, "ListTransactions", mock.Anything, mock.Anything)
	}
}

func TestTransactionService_ListTransactions_RejectsReversedDateRangeBeforeRepository(t *testing.T) {
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	repo := new(mockTransactionRepository)

	got, err := NewTransactionService(repo).ListTransactions(context.Background(), uuid.New(), domain.ListTransactionsInput{StartDate: &start, EndDate: &end})

	assert.Nil(t, got)
	assert.ErrorIs(t, err, apperrors.ErrInvalidInput)
	repo.AssertNotCalled(t, "ListTransactions", mock.Anything, mock.Anything)
}

func TestTransactionService_UpdateTransaction_ValidatesAndClearsNullableFields(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	transactionID := uuid.New()
	notes := "new notes"
	wantInput := domain.UpdateTransactionInput{
		UserID: userID, TransactionID: transactionID,
		MerchantName: strPtr("Renamed"), Amount: func() *float64 { v := 25.0; return &v }(),
		ClearReceiptURL: true, Notes: &notes,
	}
	want := &domain.Transaction{ID: transactionID, UserID: userID}
	repo := new(mockTransactionRepository)
	repo.On("UpdateTransaction", ctx, wantInput).Return(want, nil).Once()

	got, err := NewTransactionService(repo).UpdateTransaction(ctx, userID, transactionID, domain.UpdateTransactionInput{
		MerchantName: strPtr("  Renamed "), Amount: func() *float64 { v := 25.0; return &v }(),
		ClearReceiptURL: true, Notes: &notes,
	})

	require.NoError(t, err)
	assert.Equal(t, want, got)
	repo.AssertExpectations(t)
}

func TestTransactionService_UpdateTransaction_RejectsInvalidInputBeforeRepository(t *testing.T) {
	amount := 0.0
	for _, input := range []domain.UpdateTransactionInput{
		{Amount: &amount},
		{MerchantName: strPtr("   ")},
	} {
		repo := new(mockTransactionRepository)
		repo.On("UpdateTransaction", mock.Anything, mock.Anything).Maybe()

		got, err := NewTransactionService(repo).UpdateTransaction(context.Background(), uuid.New(), uuid.New(), input)

		assert.Nil(t, got)
		assert.ErrorIs(t, err, apperrors.ErrInvalidInput)
		repo.AssertNotCalled(t, "UpdateTransaction", mock.Anything, mock.Anything)
	}
}

func TestTransactionService_UpdateTransaction_PropagatesProtectedNotFoundAndDoesNotEmbed(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	transactionID := uuid.New()
	repo := new(mockTransactionRepository)
	repo.On("UpdateTransaction", ctx, mock.Anything).Return(nil, apperrors.ErrNotFound).Once()
	embedding := new(mockEmbeddingService)
	embedding.On("Generate", mock.Anything, mock.Anything).Maybe()

	got, err := NewTransactionService(repo, embedding).UpdateTransaction(ctx, userID, transactionID, domain.UpdateTransactionInput{MerchantName: strPtr("x")})

	assert.Nil(t, got)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
	embedding.AssertNotCalled(t, "Generate", mock.Anything, mock.Anything)
	repo.AssertNotCalled(t, "SaveEmbedding", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestTransactionService_DeleteTransaction_DelegatesAndPropagatesNotFound(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	transactionID := uuid.New()

	t.Run("success", func(t *testing.T) {
		repo := new(mockTransactionRepository)
		repo.On("DeleteTransaction", ctx, userID, transactionID).Return(nil).Once()
		embedding := new(mockEmbeddingService)

		err := NewTransactionService(repo, embedding).DeleteTransaction(ctx, userID, transactionID)

		require.NoError(t, err)
		embedding.AssertNotCalled(t, "Generate", mock.Anything, mock.Anything)
		repo.AssertNotCalled(t, "SaveEmbedding", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		repo.AssertExpectations(t)
	})

	t.Run("not found", func(t *testing.T) {
		repo := new(mockTransactionRepository)
		repo.On("DeleteTransaction", ctx, userID, transactionID).Return(apperrors.ErrNotFound).Once()

		err := NewTransactionService(repo).DeleteTransaction(ctx, userID, transactionID)

		assert.ErrorIs(t, err, apperrors.ErrNotFound)
		repo.AssertExpectations(t)
	})
}

func TestTransactionService_CreateTransaction_ValidInputsDelegateExactlyOnce(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	categoryID := uuid.New()
	requestUserID := uuid.New()
	transactionDate := time.Date(2026, 9, 20, 12, 30, 0, 0, time.FixedZone("UTC+7", 7*60*60))

	input := domain.CreateTransactionInput{
		UserID:          requestUserID,
		CategoryID:      categoryID,
		MerchantName:    "  AWANN COSTA  ",
		Amount:          50000,
		TransactionDate: transactionDate,
		ReceiptImageUrl: strPtr("https://example.test/receipt.png"),
		Notes:           strPtr("coffee purchase"),
	}
	wantInput := input
	wantInput.UserID = userID
	wantInput.MerchantName = "AWANN COSTA"
	want := &domain.Transaction{ID: uuid.New(), UserID: userID, CategoryID: &categoryID, MerchantName: "AWANN COSTA", Amount: 50000, TransactionDate: transactionDate}

	repo := new(mockTransactionRepository)
	repo.On("Create", ctx, wantInput).Return(want, nil).Once()

	service := NewTransactionService(repo)
	got, err := service.CreateTransaction(ctx, userID, input)

	require.NoError(t, err)
	assert.Equal(t, want, got)
	repo.AssertExpectations(t)
	repo.AssertNumberOfCalls(t, "Create", 1)
}

func TestTransactionService_CreateTransaction_ValidationTable(t *testing.T) {
	userID := uuid.New()
	categoryID := uuid.New()
	validDate := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		input      domain.CreateTransactionInput
		wantExact  string
		wantSubstr string
	}{
		{name: "zero amount", input: domain.CreateTransactionInput{CategoryID: categoryID, MerchantName: "Food", Amount: 0, TransactionDate: validDate}, wantExact: "Amount must be > 0"},
		{name: "negative amount", input: domain.CreateTransactionInput{CategoryID: categoryID, MerchantName: "Food", Amount: -1, TransactionDate: validDate}, wantExact: "Amount must be > 0"},
		{name: "NaN amount", input: domain.CreateTransactionInput{CategoryID: categoryID, MerchantName: "Food", Amount: math.NaN(), TransactionDate: validDate}, wantExact: "Amount must be > 0"},
		{name: "positive infinity amount", input: domain.CreateTransactionInput{CategoryID: categoryID, MerchantName: "Food", Amount: math.Inf(1), TransactionDate: validDate}, wantExact: "Amount must be > 0"},
		{name: "empty merchant", input: domain.CreateTransactionInput{CategoryID: categoryID, MerchantName: "", Amount: 1, TransactionDate: validDate}, wantSubstr: "merchant name must not be empty"},
		{name: "whitespace merchant", input: domain.CreateTransactionInput{CategoryID: categoryID, MerchantName: " \t\n ", Amount: 1, TransactionDate: validDate}, wantSubstr: "merchant name must not be empty"},
		{name: "missing category", input: domain.CreateTransactionInput{MerchantName: "Food", Amount: 1, TransactionDate: validDate}, wantSubstr: "category ID must not be empty"},
		{name: "missing transaction date", input: domain.CreateTransactionInput{CategoryID: categoryID, MerchantName: "Food", Amount: 1}, wantSubstr: "transaction date must not be empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := new(mockTransactionRepository)
			repo.On("Create", mock.Anything, mock.Anything).Maybe()
			service := NewTransactionService(repo)

			created, err := service.CreateTransaction(context.Background(), userID, tt.input)

			require.Error(t, err)
			assert.Nil(t, created)
			assert.ErrorIs(t, err, apperrors.ErrInvalidInput)
			if tt.wantExact != "" {
				assert.Equal(t, tt.wantExact, err.Error())
			}
			if tt.wantSubstr != "" {
				assert.Contains(t, err.Error(), tt.wantSubstr)
			}
			repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
		})
	}
}

func TestTransactionService_CreateTransaction_ValidBoundaryAndOptionalInputs(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	categoryID := uuid.New()
	date := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name   string
		amount float64
		notes  *string
	}{
		{name: "smallest positive amount", amount: math.SmallestNonzeroFloat64},
		{name: "nil optional fields", amount: 10, notes: nil},
		{name: "unicode notes", amount: 10.25, notes: strPtr("Kopi — meeting\nclient")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := domain.CreateTransactionInput{
				UserID:          uuid.New(),
				CategoryID:      categoryID,
				MerchantName:    "  Merchant  ",
				Amount:          tt.amount,
				TransactionDate: date,
				Notes:           tt.notes,
			}
			wantInput := input
			wantInput.UserID = userID
			wantInput.MerchantName = "Merchant"
			repo := new(mockTransactionRepository)
			repo.On("Create", ctx, wantInput).Return(&domain.Transaction{UserID: userID}, nil).Once()

			created, err := NewTransactionService(repo).CreateTransaction(ctx, userID, input)

			require.NoError(t, err)
			require.NotNil(t, created)
			repo.AssertExpectations(t)
		})
	}
}

func TestTransactionService_CreateTransaction_RepositoryErrorsAreClassified(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	categoryID := uuid.New()
	input := domain.CreateTransactionInput{CategoryID: categoryID, MerchantName: "Food", Amount: 10, TransactionDate: time.Now()}

	tests := []struct {
		name    string
		repoErr error
		wantErr error
	}{
		{name: "category not found", repoErr: apperrors.ErrNotFound, wantErr: apperrors.ErrNotFound},
		{name: "foreign key classified as invalid input", repoErr: apperrors.ErrInvalidInput, wantErr: apperrors.ErrInvalidInput},
		{name: "unknown database error is hidden", repoErr: errors.New("pq: relation transactions is unavailable"), wantErr: apperrors.ErrInternal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := new(mockTransactionRepository)
			repo.On("Create", ctx, mock.Anything).Return(nil, tt.repoErr).Once()

			created, err := NewTransactionService(repo).CreateTransaction(ctx, userID, input)

			assert.Nil(t, created)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
			assert.NotContains(t, err.Error(), "pq:")
			repo.AssertExpectations(t)
		})
	}
}

func TestTransactionService_CreateTransaction_RepositoryCalledOnceOnlyAfterAllValidation(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	categoryID := uuid.New()
	repo := new(mockTransactionRepository)
	want := &domain.Transaction{ID: uuid.New(), UserID: userID}
	repo.On("Create", ctx, mock.Anything).Return(want, nil).Once()

	created, err := NewTransactionService(repo).CreateTransaction(ctx, userID, domain.CreateTransactionInput{
		UserID:          uuid.New(),
		CategoryID:      categoryID,
		MerchantName:    " Valid ",
		Amount:          50,
		TransactionDate: time.Now(),
	})

	require.NoError(t, err)
	assert.Same(t, want, created)
	repo.AssertNumberOfCalls(t, "Create", 1)
	repo.AssertExpectations(t)
}

func TestTransactionService_CreateTransaction_ContextIsForwarded(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	userID := uuid.New()
	categoryID := uuid.New()
	repo := new(mockTransactionRepository)
	repo.On("Create", ctx, mock.Anything).Return(nil, context.Canceled).Once()

	created, err := NewTransactionService(repo).CreateTransaction(ctx, userID, domain.CreateTransactionInput{
		CategoryID:      categoryID,
		MerchantName:    "Food",
		Amount:          10,
		TransactionDate: time.Now(),
	})

	assert.Nil(t, created)
	assert.ErrorIs(t, err, context.Canceled)
	repo.AssertExpectations(t)
}

func TestTransactionService_CreateTransaction_NormalizesOnlyBusinessFields(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	categoryID := uuid.New()
	date := time.Date(2026, 9, 20, 12, 0, 0, 0, time.FixedZone("UTC+7", 7*60*60))
	input := domain.CreateTransactionInput{
		UserID:          uuid.New(),
		CategoryID:      categoryID,
		MerchantName:    "  Coffee Shop  ",
		Amount:          1234.56,
		TransactionDate: date,
		ReceiptImageUrl: strPtr("https://example.test/r?x=1&y=2"),
		Notes:           strPtr("a'b\nnotes"),
	}
	want := input
	want.UserID = userID
	want.MerchantName = strings.TrimSpace(input.MerchantName)
	repo := new(mockTransactionRepository)
	repo.On("Create", ctx, want).Return(&domain.Transaction{UserID: userID}, nil).Once()

	_, err := NewTransactionService(repo).CreateTransaction(ctx, userID, input)

	require.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestTransactionService_CreateTransaction_AsyncEmbeddingTrigger(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	categoryID := uuid.New()
	created := &domain.Transaction{
		ID:           uuid.New(),
		UserID:       userID,
		CategoryID:   &categoryID,
		MerchantName: "AWANN COSTA",
		Amount:       50000,
		Notes:        strPtr("coffee purchase"),
	}
	wantEmbedding := make([]float32, domain.EmbeddingDimension)
	generateStarted := make(chan string, 1)
	saveCompleted := make(chan struct{}, 1)

	repo := new(mockTransactionRepository)
	repo.On("Create", ctx, mock.Anything).Return(created, nil).Once()
	repo.On("SaveEmbedding", mock.Anything, created.ID, userID, wantEmbedding, mock.Anything).
		Run(func(args mock.Arguments) { saveCompleted <- struct{}{} }).
		Return(nil).Once()

	embedding := new(mockEmbeddingService)
	embedding.On("Generate", mock.Anything, "merchant_name: AWANN COSTA\namount: 50000\ncategory: Coffee\nnotes: coffee purchase").
		Run(func(args mock.Arguments) { generateStarted <- args.String(1) }).
		Return(wantEmbedding, nil).Once()

	service := NewTransactionService(repo, embedding)
	result, err := service.CreateTransaction(ctx, userID, domain.CreateTransactionInput{
		CategoryID:      categoryID,
		CategoryName:    "Coffee",
		MerchantName:    "  AWANN COSTA ",
		Amount:          50000,
		TransactionDate: time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC),
		Notes:           strPtr("coffee purchase"),
	})

	require.NoError(t, err)
	assert.Same(t, created, result)
	select {
	case text := <-generateStarted:
		assert.Equal(t, "merchant_name: AWANN COSTA\namount: 50000\ncategory: Coffee\nnotes: coffee purchase", text)
	case <-time.After(time.Second):
		t.Fatal("embedding generation was not triggered")
	}
	select {
	case <-saveCompleted:
	case <-time.After(time.Second):
		t.Fatal("embedding persistence was not triggered")
	}
	embedding.AssertExpectations(t)
	repo.AssertExpectations(t)
	repo.AssertNumberOfCalls(t, "SaveEmbedding", 1)
}

func TestTransactionService_CreateTransaction_AsyncEmbeddingOmitsAbsentNotes(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	categoryID := uuid.New()
	created := &domain.Transaction{ID: uuid.New(), UserID: userID, MerchantName: "Shop", Amount: 12.5}
	embedding := new(mockEmbeddingService)
	embedding.On("Generate", mock.Anything, "merchant_name: Shop\namount: 12.5\ncategory: Groceries").
		Return(make([]float32, domain.EmbeddingDimension), nil).Once()
	repo := new(mockTransactionRepository)
	repo.On("Create", ctx, mock.Anything).Return(created, nil).Once()
	saved := make(chan struct{}, 1)
	repo.On("SaveEmbedding", mock.Anything, created.ID, userID, mock.Anything, mock.Anything).
		Run(func(mock.Arguments) { saved <- struct{}{} }).Return(nil).Once()

	_, err := NewTransactionService(repo, embedding).CreateTransaction(ctx, userID, domain.CreateTransactionInput{
		CategoryID: categoryID, CategoryName: "Groceries", MerchantName: "Shop", Amount: 12.5,
		TransactionDate: time.Now(),
	})
	require.NoError(t, err)
	select {
	case <-saved:
	case <-time.After(time.Second):
		t.Fatal("embedding persistence was not triggered")
	}
	embedding.AssertExpectations(t)
}

func TestTransactionService_CreateTransaction_FailedCreateDoesNotTriggerEmbedding(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	categoryID := uuid.New()
	repo := new(mockTransactionRepository)
	repo.On("Create", ctx, mock.Anything).Return(nil, apperrors.ErrNotFound).Once()
	embedding := new(mockEmbeddingService)
	embedding.On("Generate", mock.Anything, mock.Anything).Maybe()

	created, err := NewTransactionService(repo, embedding).CreateTransaction(ctx, userID, domain.CreateTransactionInput{
		CategoryID: categoryID, CategoryName: "Food", MerchantName: "Shop", Amount: 10,
		TransactionDate: time.Now(),
	})

	assert.Nil(t, created)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
	embedding.AssertNotCalled(t, "Generate", mock.Anything, mock.Anything)
	repo.AssertNotCalled(t, "SaveEmbedding", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestTransactionService_CreateTransaction_EmbeddingProviderFailureDoesNotPersist(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	categoryID := uuid.New()
	created := &domain.Transaction{ID: uuid.New(), UserID: userID, MerchantName: "Shop", Amount: 10}
	repo := new(mockTransactionRepository)
	repo.On("Create", ctx, mock.Anything).Return(created, nil).Once()
	embedding := new(mockEmbeddingService)
	generated := make(chan struct{}, 1)
	embedding.On("Generate", mock.Anything, mock.Anything).
		Run(func(mock.Arguments) { generated <- struct{}{} }).
		Return(nil, errors.New("gemini unavailable")).Once()

	result, err := NewTransactionService(repo, embedding).CreateTransaction(ctx, userID, domain.CreateTransactionInput{
		CategoryID: categoryID, CategoryName: "Food", MerchantName: "Shop", Amount: 10,
		TransactionDate: time.Now(),
	})

	require.NoError(t, err)
	assert.Same(t, created, result)
	select {
	case <-generated:
	case <-time.After(time.Second):
		t.Fatal("embedding generation was not triggered")
	}
	embedding.AssertExpectations(t)
	repo.AssertNotCalled(t, "SaveEmbedding", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestTransactionService_CreateTransaction_EmbeddingUsesDetachedContext(t *testing.T) {
	requestCtx, cancel := context.WithCancel(context.Background())
	userID := uuid.New()
	categoryID := uuid.New()
	created := &domain.Transaction{ID: uuid.New(), UserID: userID, MerchantName: "Shop", Amount: 10}
	generationContext := make(chan context.Context, 1)
	saved := make(chan struct{}, 1)

	repo := new(mockTransactionRepository)
	repo.On("Create", requestCtx, mock.Anything).Return(created, nil).Once()
	repo.On("SaveEmbedding", mock.Anything, created.ID, userID, mock.Anything, mock.Anything).
		Run(func(mock.Arguments) { saved <- struct{}{} }).Return(nil).Once()
	embedding := new(mockEmbeddingService)
	embedding.On("Generate", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) { generationContext <- args.Get(0).(context.Context) }).
		Return(make([]float32, domain.EmbeddingDimension), nil).Once()

	_, err := NewTransactionService(repo, embedding).CreateTransaction(requestCtx, userID, domain.CreateTransactionInput{
		CategoryID: categoryID, CategoryName: "Food", MerchantName: "Shop", Amount: 10,
		TransactionDate: time.Now(),
	})
	require.NoError(t, err)
	cancel()

	select {
	case backgroundCtx := <-generationContext:
		assert.NotEqual(t, requestCtx, backgroundCtx)
		assert.NoError(t, backgroundCtx.Err())
	case <-time.After(time.Second):
		t.Fatal("embedding generation was not triggered")
	}
	select {
	case <-saved:
	case <-time.After(time.Second):
		t.Fatal("embedding persistence was not triggered")
	}
}
