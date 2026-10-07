package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	domain "walletx-be/internal/modules/transactions/contracts"
	apperrors "walletx-be/internal/shared/errors"
)

func TestDeleteTransactionCleansReceiptAfterDatabaseDeletion(t *testing.T) {
	for _, status := range []domain.TransactionStatus{domain.TransactionStatusSuccess, domain.TransactionStatusPending} {
		t.Run(string(status), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			owner, id := uuid.New(), uuid.New()
			ref := owner.String() + "/" + uuid.NewString() + "_123.png"
			repo := new(mockTransactionRepository)
			deleted := false
			repo.On("GetTransactionForDeletion", ctx, owner, id).Return(&domain.Transaction{ID: id, UserID: owner, Status: status, ReceiptImageUrl: &ref}, nil).Once()
			repo.On("DeleteTransaction", ctx, owner, id).Run(func(mock.Arguments) { deleted = true; cancel() }).Return(nil).Once()
			calls := 0
			store := receiptStorageStub{delete: func(cleanupCtx context.Context, user uuid.UUID, path string) error {
				calls++
				require.True(t, deleted)
				require.NoError(t, cleanupCtx.Err(), "cleanup survives cancellation after DB commit")
				deadline, ok := cleanupCtx.Deadline()
				require.True(t, ok)
				require.LessOrEqual(t, time.Until(deadline), 5*time.Second)
				require.Equal(t, owner, user)
				require.Equal(t, ref, path)
				return nil
			}}
			log := &receiptLog{}
			require.NoError(t, NewTransactionServiceWithLogger(repo, nil, log, store).DeleteTransaction(ctx, owner, id))
			require.Equal(t, 1, calls)
			require.Zero(t, log.errors)
			repo.AssertExpectations(t)
		})
	}
}

func TestDeleteTransactionCleanupFailureDoesNotFailCommittedDeletion(t *testing.T) {
	ctx := context.Background()
	owner, id := uuid.New(), uuid.New()
	ref := owner.String() + "/" + uuid.NewString() + "_123.png"
	repo := new(mockTransactionRepository)
	repo.On("GetTransactionForDeletion", ctx, owner, id).Return(&domain.Transaction{ID: id, UserID: owner, ReceiptImageUrl: &ref}, nil).Once()
	repo.On("DeleteTransaction", ctx, owner, id).Return(nil).Once()
	log := &receiptLog{}
	store := receiptStorageStub{delete: func(context.Context, uuid.UUID, string) error { return errors.New("secret provider response") }}
	require.NoError(t, NewTransactionServiceWithLogger(repo, nil, log, store).DeleteTransaction(ctx, owner, id))
	require.Equal(t, 1, log.errors)
	require.NotContains(t, log.errorValue.Error(), "secret")
	require.Equal(t, "delete_transaction_receipt", log.fields["operation"])
	require.Equal(t, "storage", log.fields["dependency"])
	repo.AssertExpectations(t)
}

func TestDeleteTransactionDoesNotCleanReceiptOnDatabaseFailure(t *testing.T) {
	ctx := context.Background()
	owner, id := uuid.New(), uuid.New()
	ref := "owned-receipt"
	repo := new(mockTransactionRepository)
	repo.On("GetTransactionForDeletion", ctx, owner, id).Return(&domain.Transaction{ID: id, UserID: owner, ReceiptImageUrl: &ref}, nil).Once()
	repo.On("DeleteTransaction", ctx, owner, id).Return(apperrors.ErrInternal).Once()
	store := receiptStorageStub{delete: func(context.Context, uuid.UUID, string) error { t.Fatal("cleanup ran after DB failure"); return nil }}
	require.ErrorIs(t, NewTransactionServiceWithLogger(repo, nil, &receiptLog{}, store).DeleteTransaction(ctx, owner, id), apperrors.ErrInternal)
	repo.AssertExpectations(t)
}

func TestDeleteTransactionWithoutReceiptSkipsStorage(t *testing.T) {
	for _, ref := range []*string{nil, strPtr(" ")} {
		ctx := context.Background()
		owner, id := uuid.New(), uuid.New()
		repo := new(mockTransactionRepository)
		repo.On("GetTransactionForDeletion", ctx, owner, id).Return(&domain.Transaction{ID: id, UserID: owner, ReceiptImageUrl: ref}, nil).Once()
		repo.On("DeleteTransaction", ctx, owner, id).Return(nil).Once()
		store := receiptStorageStub{delete: func(context.Context, uuid.UUID, string) error { t.Fatal("cleanup ran without receipt"); return nil }}
		require.NoError(t, NewTransactionServiceWithLogger(repo, nil, &receiptLog{}, store).DeleteTransaction(ctx, owner, id))
		repo.AssertExpectations(t)
	}
}

func TestDeleteTransactionFetchFailureOrInvalidOwnershipStopsDeletion(t *testing.T) {
	for _, scenario := range []string{"not found", "database error", "nil transaction", "wrong owner", "wrong id"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			owner, id := uuid.New(), uuid.New()
			transaction := &domain.Transaction{ID: id, UserID: owner}
			var fetchErr error
			want := apperrors.ErrInternal
			switch scenario {
			case "not found":
				fetchErr = apperrors.ErrNotFound
				want = apperrors.ErrNotFound
			case "database error":
				fetchErr = apperrors.ErrInternal
			case "nil transaction":
				transaction = nil
			case "wrong owner":
				transaction.UserID = uuid.New()
			case "wrong id":
				transaction.ID = uuid.New()
			}
			repo := new(mockTransactionRepository)
			repo.On("GetTransactionForDeletion", ctx, owner, id).Return(transaction, fetchErr).Once()
			store := receiptStorageStub{delete: func(context.Context, uuid.UUID, string) error {
				t.Fatal("storage called after invalid fetch")
				return nil
			}}
			require.ErrorIs(t, NewTransactionServiceWithLogger(repo, nil, &receiptLog{}, store).DeleteTransaction(ctx, owner, id), want)
			repo.AssertNotCalled(t, "DeleteTransaction", mock.Anything, mock.Anything, mock.Anything)
			repo.AssertExpectations(t)
		})
	}
}
