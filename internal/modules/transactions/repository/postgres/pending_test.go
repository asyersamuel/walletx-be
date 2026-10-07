package postgres

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	domain "walletx-be/internal/modules/transactions/contracts"
	apperrors "walletx-be/internal/shared/errors"
)

func seedPending(t *testing.T, repo domain.TransactionRepository, userID uuid.UUID) *domain.PendingTransaction {
	t.Helper()
	pending, err := repo.CreatePendingTransaction(context.Background(), domain.CreatePendingTransactionInput{
		UserID: userID, ReceiptImageURL: "https://storage.example/receipts/" + uuid.NewString(),
	})
	require.NoError(t, err)
	require.NotNil(t, pending)
	return pending
}

func completionInput(pending *domain.PendingTransaction, categoryID uuid.UUID) domain.CompletePendingTransactionInput {
	return domain.CompletePendingTransactionInput{
		UserID: pending.UserID, TransactionID: pending.ID, CategoryID: categoryID,
		MerchantName: "Coffee shop", Amount: 123.45,
		TransactionDate: time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC), Notes: strPtr("Coffee"),
	}
}

func TestPendingRepository_PendingHiddenFromGetAndList(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx := context.Background()
	repo := newTestRepository(t)
	userID, otherUser := seedUser(t), seedUser(t)
	categoryID := seedCategory(t, userID, "Food", "expense")
	pending := seedPending(t, repo, userID)
	var status string
	var nullFields bool
	require.NoError(t, testPool.QueryRow(ctx, `select status,
		amount is null and merchant_name is null and category_id is null and transaction_date is null
		from public.transactions where id = $1`, pending.ID).Scan(&status, &nullFields))
	require.Equal(t, "pending", status)
	require.True(t, nullFields)
	got, err := repo.GetTransactionByID(ctx, userID, pending.ID)
	require.ErrorIs(t, err, apperrors.ErrNotFound)
	require.Nil(t, got)
	// The internal deletion lookup must retain access to pending receipts.
	got, err = repo.GetTransactionForDeletion(ctx, userID, pending.ID)
	require.NoError(t, err)
	require.Equal(t, domain.TransactionStatusPending, got.Status)
	require.Equal(t, pending.ReceiptImageURL, *got.ReceiptImageUrl)
	got, err = repo.GetTransactionForDeletion(ctx, otherUser, pending.ID)
	require.ErrorIs(t, err, apperrors.ErrNotFound)
	require.Nil(t, got)
	got, err = repo.GetTransactionByID(ctx, otherUser, pending.ID)
	require.ErrorIs(t, err, apperrors.ErrNotFound)
	require.Nil(t, got)
	rows, err := repo.ListTransactions(ctx, domain.ListTransactionsInput{UserID: userID})
	require.NoError(t, err)
	require.NotNil(t, rows)
	require.Empty(t, rows)
	older := seedTransaction(t, repo, userID, categoryID, "Older", 10, time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC))
	newer := seedTransaction(t, repo, userID, categoryID, "Newer", 20, time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC))
	seedPending(t, repo, otherUser)
	for offset, want := range []uuid.UUID{newer.ID, older.ID} {
		rows, err = repo.ListTransactions(ctx, domain.ListTransactionsInput{UserID: userID, Limit: 1, Offset: offset})
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.Equal(t, want, rows[0].ID)
	}
	require.NoError(t, testPool.QueryRow(ctx, "select status from public.transactions where id = $1", newer.ID).Scan(&status))
	require.Equal(t, "success", status)
	_, err = repo.UpdateTransaction(ctx, domain.UpdateTransactionInput{UserID: userID, TransactionID: pending.ID, MerchantName: strPtr("Changed")})
	require.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestPendingRepository_CompletionIsAtomicAndPreservesIdentity(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx := context.Background()
	repo := newTestRepository(t)
	userID := seedUser(t)
	categoryID := seedCategory(t, userID, "Food", "expense")
	pending := seedPending(t, repo, userID)
	input := completionInput(pending, categoryID)
	completed, err := repo.CompletePendingTransaction(ctx, input)
	require.NoError(t, err)
	require.Equal(t, pending.ID, completed.ID)
	require.Equal(t, pending.UserID, completed.UserID)
	require.True(t, pending.CreatedAt.Equal(completed.CreatedAt))
	require.Equal(t, pending.ReceiptImageURL, *completed.ReceiptImageUrl)
	require.Equal(t, input.Amount, completed.Amount)
	require.Equal(t, input.MerchantName, completed.MerchantName)
	require.Equal(t, categoryID, *completed.CategoryID)
	require.True(t, input.TransactionDate.Equal(completed.TransactionDate))
	require.Equal(t, input.Notes, completed.Notes)
	var count int
	require.NoError(t, testPool.QueryRow(ctx, "select count(*) from public.transactions where user_id = $1", userID).Scan(&count))
	require.Equal(t, 1, count)
	fetched, err := repo.GetTransactionByID(ctx, userID, pending.ID)
	require.NoError(t, err)
	require.Equal(t, completed, fetched)
	input.Amount = 999
	_, err = repo.CompletePendingTransaction(ctx, input)
	require.ErrorIs(t, err, apperrors.ErrNotFound)
	fetched, err = repo.GetTransactionByID(ctx, userID, pending.ID)
	require.NoError(t, err)
	require.Equal(t, 123.45, fetched.Amount)
}

func TestPendingRepository_CompletionGuardsLeaveRowPending(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx := context.Background()
	repo := newTestRepository(t)
	userID, otherUser := seedUser(t), seedUser(t)
	categoryID := seedCategory(t, userID, "Food", "expense")
	foreignCategory := seedCategory(t, otherUser, "Other", "expense")
	for _, tc := range []struct {
		name   string
		mutate func(*domain.CompletePendingTransactionInput)
		want   error
	}{
		{"foreign owner", func(i *domain.CompletePendingTransactionInput) { i.UserID = otherUser }, apperrors.ErrNotFound},
		{"foreign category", func(i *domain.CompletePendingTransactionInput) { i.CategoryID = foreignCategory }, apperrors.ErrNotFound},
		{"missing category", func(i *domain.CompletePendingTransactionInput) { i.CategoryID = uuid.New() }, apperrors.ErrNotFound},
		{"zero amount", func(i *domain.CompletePendingTransactionInput) { i.Amount = 0 }, apperrors.ErrInvalidInput},
		{"blank merchant", func(i *domain.CompletePendingTransactionInput) { i.MerchantName = " " }, apperrors.ErrInvalidInput},
		{"missing date", func(i *domain.CompletePendingTransactionInput) { i.TransactionDate = time.Time{} }, apperrors.ErrInvalidInput},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pending := seedPending(t, repo, userID)
			input := completionInput(pending, categoryID)
			tc.mutate(&input)
			got, err := repo.CompletePendingTransaction(ctx, input)
			require.ErrorIs(t, err, tc.want)
			require.Nil(t, got)
			var unchanged bool
			require.NoError(t, testPool.QueryRow(ctx, `select status = 'pending' and amount is null
				and merchant_name is null and category_id is null and transaction_date is null
				from public.transactions where id = $1`, pending.ID).Scan(&unchanged))
			require.True(t, unchanged)
		})
	}
}

func TestPendingRepository_ConcurrentCompletionHasOneWinner(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	repo := newTestRepository(t)
	userID := seedUser(t)
	categoryID := seedCategory(t, userID, "Food", "expense")
	input := completionInput(seedPending(t, repo, userID), categoryID)
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for n := 0; n < 2; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := repo.CompletePendingTransaction(context.Background(), input)
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else {
			require.ErrorIs(t, err, apperrors.ErrNotFound)
			conflicts++
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, conflicts)
}

func TestPendingRepository_EmbeddingOperationsRequireSuccess(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx := context.Background()
	repo := newTestRepository(t)
	userID := seedUser(t)
	pending := seedPending(t, repo, userID)
	vector := make([]float32, domain.EmbeddingDimension)
	vector[0] = 1
	err := repo.SaveEmbedding(ctx, pending.ID, userID, vector, pending.UpdatedAt)
	require.ErrorIs(t, err, apperrors.ErrNotFound)
	// Seed a pending embedding independently so search filtering does not rely on SaveEmbedding.
	literal, err := embeddingLiteral(vector)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, "update public.transactions set embedding = $1::text::extensions.vector where id = $2", literal, pending.ID)
	require.NoError(t, err)
	rows, err := repo.SearchSimilarTransactions(ctx, domain.SearchSimilarTransactionsInput{UserID: userID, QueryEmbedding: vector, MatchThreshold: 0})
	require.NoError(t, err)
	require.Empty(t, rows)
	categoryID := seedCategory(t, userID, "Food", "expense")
	completed, err := repo.CompletePendingTransaction(ctx, completionInput(pending, categoryID))
	require.NoError(t, err)
	require.NoError(t, repo.SaveEmbedding(ctx, completed.ID, userID, vector, completed.UpdatedAt))
	rows, err = repo.SearchSimilarTransactions(ctx, domain.SearchSimilarTransactionsInput{UserID: userID, QueryEmbedding: vector, MatchThreshold: 0})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, completed.ID, rows[0].ID)
	require.Equal(t, 123.45, rows[0].Amount)
	require.Equal(t, "Coffee shop", rows[0].MerchantName)
	require.Equal(t, strPtr("Food"), rows[0].CategoryName)
}

func TestTransactionRepository_MalformedSuccessRowsReturnErrors(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx := context.Background()
	repo := newTestRepository(t)
	userID := seedUser(t)
	categoryID := seedCategory(t, userID, "Food", "expense")
	valid := seedTransaction(t, repo, userID, categoryID, "Valid", 10, time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC))
	var corruptID uuid.UUID
	// Nullable SQL fields do not make incomplete successful rows valid domain entities.
	require.NoError(t, testPool.QueryRow(ctx, `insert into public.transactions
		(user_id, status, amount, merchant_name, transaction_date)
		values ($1, 'success', NULL, 'Incomplete', '2026-10-05T00:00:00Z') returning id`, userID).Scan(&corruptID))
	got, err := repo.GetTransactionByID(ctx, userID, corruptID)
	require.ErrorIs(t, err, apperrors.ErrInternal)
	require.Nil(t, got)
	rows, err := repo.ListTransactions(ctx, domain.ListTransactionsInput{UserID: userID})
	require.ErrorIs(t, err, apperrors.ErrInternal)
	require.Nil(t, rows, "a malformed row must not expose a partial page")
	got, err = repo.GetTransactionByID(ctx, userID, valid.ID)
	require.NoError(t, err)
	require.Equal(t, valid.ID, got.ID)
}
