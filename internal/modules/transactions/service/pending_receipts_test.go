package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"walletx-be/internal/modules/ai"
	domain "walletx-be/internal/modules/transactions/contracts"
	apperrors "walletx-be/internal/shared/errors"
)

type pendingReaderFunc func(context.Context, int) ([]domain.PendingTransaction, error)

func (f pendingReaderFunc) GetPendingTransactions(ctx context.Context, limit int) ([]domain.PendingTransaction, error) {
	return f(ctx, limit)
}

type imageReaderFunc func(context.Context, uuid.UUID, string) ([]byte, error)

func (f imageReaderFunc) ReadReceipt(ctx context.Context, owner uuid.UUID, ref string) ([]byte, error) {
	return f(ctx, owner, ref)
}

func pendingFixture(t *testing.T, rows []domain.PendingTransaction) (*pendingReceiptService, *receiptLog) {
	t.Helper()
	location, err := time.LoadLocation("Asia/Jakarta")
	require.NoError(t, err)
	category := uuid.New()
	log := &receiptLog{}
	pending := pendingReaderFunc(func(ctx context.Context, limit int) ([]domain.PendingTransaction, error) {
		require.Equal(t, 10, limit)
		_, bounded := ctx.Deadline()
		require.True(t, bounded)
		return rows, nil
	})
	reader := imageReaderFunc(func(context.Context, uuid.UUID, string) ([]byte, error) { return []byte("\x89PNG\r\n\x1a\n"), nil })
	extractor := receiptExtractorFunc(func(_ context.Context, input ai.ExtractionInput) (ai.ExtractedTransaction, error) {
		require.Equal(t, "image/png", input.MIMEType)
		require.Equal(t, "Asia/Jakarta", input.TimeZone)
		require.Equal(t, location, input.ReferenceTime.Location())
		return ai.ExtractedTransaction{Merchant: " Coffee ", Amount: 10, Date: "2026-10-08", CategoryID: category}, nil
	})
	categories := receiptCategoriesFunc(func(context.Context, uuid.UUID) ([]ai.CategoryOption, error) {
		return []ai.CategoryOption{{ID: category, Name: "Food"}}, nil
	})
	writer := receiptWriter{complete: func(_ context.Context, owner, id uuid.UUID, input domain.CompletePendingTransactionInput) (*domain.Transaction, error) {
		require.Equal(t, owner, input.UserID)
		require.Equal(t, id, input.TransactionID)
		require.Equal(t, "Coffee", input.MerchantName)
		require.Equal(t, "Food", input.CategoryName)
		return &domain.Transaction{ID: id, UserID: owner, Status: domain.TransactionStatusSuccess}, nil
	}}
	svc, err := NewPendingReceiptService(pending, reader, writer, extractor, categories, log, PendingReceiptOptions{BatchSize: 10, Timeout: time.Second, DownloadTimeout: time.Second, ExtractionTimeout: time.Second, CompletionReserve: 10 * time.Millisecond}, location, time.Now)
	require.NoError(t, err)
	return svc.(*pendingReceiptService), log
}

func pendingRows(n int) []domain.PendingTransaction {
	rows := make([]domain.PendingTransaction, n)
	for i := range rows {
		rows[i] = domain.PendingTransaction{ID: uuid.New(), UserID: uuid.New(), ReceiptImageURL: "owned/image.png"}
	}
	return rows
}

func TestPendingReceiptsSuccessAndEmpty(t *testing.T) {
	for _, count := range []int{0, 2} {
		rows := pendingRows(count)
		svc, log := pendingFixture(t, rows)
		summary, err := svc.ProcessPendingReceipts(context.Background())
		require.NoError(t, err)
		require.Equal(t, domain.PendingReceiptSummary{Fetched: count, Succeeded: count}, summary)
		require.Zero(t, log.errors)
	}
}

func TestPendingReceiptRetriesOnNextInvocation(t *testing.T) {
	rows := pendingRows(1)
	svc, _ := pendingFixture(t, rows)
	extractor, writer := svc.extraction.extractor, svc.completion
	attempts, completions := 0, 0
	svc.extraction.extractor = receiptExtractorFunc(func(ctx context.Context, input ai.ExtractionInput) (ai.ExtractedTransaction, error) {
		attempts++
		if attempts == 1 {
			return ai.ExtractedTransaction{}, errors.New("temporary provider failure")
		}
		return extractor.ExtractTransaction(ctx, input)
	})
	svc.completion = receiptWriter{complete: func(ctx context.Context, owner, id uuid.UUID, input domain.CompletePendingTransactionInput) (*domain.Transaction, error) {
		completions++
		require.Equal(t, rows[0].ID, id)
		return writer.CompletePendingTransaction(ctx, owner, id, input)
	}}
	first, err := svc.ProcessPendingReceipts(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, first.Failed)
	require.Equal(t, 1, attempts)
	require.Zero(t, completions)
	second, err := svc.ProcessPendingReceipts(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, second.Succeeded)
	require.Equal(t, 2, attempts)
	require.Equal(t, 1, completions)
}

func TestPendingReceiptsLogRawProviderDetailOnce(t *testing.T) {
	svc, log := pendingFixture(t, pendingRows(1))
	svc.extraction.extractor = receiptExtractorFunc(func(context.Context, ai.ExtractionInput) (ai.ExtractedTransaction, error) {
		return ai.ExtractedTransaction{}, fmt.Errorf("AI wrapper: %w", receiptDiagnosticError{})
	})
	summary, err := svc.ProcessPendingReceipts(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, summary.Failed)
	require.Equal(t, 1, log.errors)
	require.Equal(t, receiptDiagnosticError{}.SafeLogFields()["provider_raw_error_detail"], log.fields["provider_raw_error_detail"])
}

func TestPendingReceiptConstructorRejectsInvalidLimits(t *testing.T) {
	svc, log := pendingFixture(t, nil)
	for _, mutate := range []func(*PendingReceiptOptions){
		func(o *PendingReceiptOptions) { o.BatchSize = 0 },
		func(o *PendingReceiptOptions) { o.BatchSize = 21 },
		func(o *PendingReceiptOptions) { o.DownloadTimeout = 0 },
		func(o *PendingReceiptOptions) { o.ExtractionTimeout = 0 },
		func(o *PendingReceiptOptions) { o.CompletionReserve = 0 },
		func(o *PendingReceiptOptions) { o.Timeout = o.CompletionReserve },
	} {
		options := svc.options
		mutate(&options)
		_, err := NewPendingReceiptService(svc.pending, svc.images, svc.completion, svc.extraction.extractor, svc.extraction.categories, log, options, svc.extraction.location, time.Now)
		require.ErrorIs(t, err, apperrors.ErrInvalidInput)
	}
	_, err := NewPendingReceiptService(nil, svc.images, svc.completion, svc.extraction.extractor, svc.extraction.categories, log, svc.options, svc.extraction.location, time.Now)
	require.ErrorIs(t, err, apperrors.ErrInvalidInput)
}

func TestPendingReceiptsFailuresContinueWithoutRetry(t *testing.T) {
	for _, stage := range []string{"download", "image", "extract", "date", "category", "complete", "nil completion"} {
		t.Run(stage, func(t *testing.T) {
			rows := pendingRows(2)
			svc, log := pendingFixture(t, rows)
			cause := errors.New("secret token and private receipt")
			reads, extracts, writes := 0, 0, 0
			images, extractor, categories, writer := svc.images, svc.extraction.extractor, svc.extraction.categories, svc.completion
			svc.images = imageReaderFunc(func(ctx context.Context, owner uuid.UUID, ref string) ([]byte, error) {
				reads++
				if owner == rows[0].UserID && stage == "download" {
					return nil, cause
				}
				if owner == rows[0].UserID && stage == "image" {
					return []byte("HTML"), nil
				}
				return images.ReadReceipt(ctx, owner, ref)
			})
			svc.extraction.extractor = receiptExtractorFunc(func(ctx context.Context, input ai.ExtractionInput) (ai.ExtractedTransaction, error) {
				extracts++
				if input.UserID == rows[0].UserID && stage == "extract" {
					return ai.ExtractedTransaction{}, cause
				}
				out, err := extractor.ExtractTransaction(ctx, input)
				if input.UserID == rows[0].UserID && stage == "date" {
					out.Date = "2026-02-30"
				}
				return out, err
			})
			svc.extraction.categories = receiptCategoriesFunc(func(ctx context.Context, owner uuid.UUID) ([]ai.CategoryOption, error) {
				if owner == rows[0].UserID && stage == "category" {
					return nil, cause
				}
				return categories.ListByUser(ctx, owner)
			})
			svc.completion = receiptWriter{complete: func(ctx context.Context, owner, id uuid.UUID, input domain.CompletePendingTransactionInput) (*domain.Transaction, error) {
				writes++
				if id == rows[0].ID && stage == "complete" {
					return nil, cause
				}
				if id == rows[0].ID && stage == "nil completion" {
					return nil, nil
				}
				return writer.CompletePendingTransaction(ctx, owner, id, input)
			}}
			summary, err := svc.ProcessPendingReceipts(context.Background())
			require.NoError(t, err)
			require.Equal(t, domain.PendingReceiptSummary{Fetched: 2, Succeeded: 1, Failed: 1}, summary)
			require.Equal(t, 2, reads)
			require.LessOrEqual(t, extracts, 2)
			require.LessOrEqual(t, writes, 2)
			require.Equal(t, 1, log.errors)
			require.NotContains(t, log.errorValue.Error(), "secret")
			// Only owned completion writes are available; no status-failure,
			// upload, insertion, or deletion operation is invoked.
		})
	}
}

func TestPendingReceiptsSelectionFailureAndCancellation(t *testing.T) {
	svc, log := pendingFixture(t, nil)
	svc.pending = pendingReaderFunc(func(context.Context, int) ([]domain.PendingTransaction, error) {
		return nil, errors.New("database URL secret")
	})
	_, err := svc.ProcessPendingReceipts(context.Background())
	require.ErrorIs(t, err, apperrors.ErrInternal)
	require.Equal(t, 1, log.errors)
	require.NotContains(t, log.errorValue.Error(), "secret")
	_, err = svc.ProcessPendingReceipts(nil)
	require.ErrorIs(t, err, apperrors.ErrInvalidInput)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	summary, err := svc.ProcessPendingReceipts(ctx)
	require.NoError(t, err)
	require.True(t, summary.StoppedEarly)
}

func TestPendingReceiptsStopBetweenItems(t *testing.T) {
	svc, _ := pendingFixture(t, pendingRows(3))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	writer := svc.completion
	svc.completion = receiptWriter{complete: func(ctx context.Context, owner, id uuid.UUID, input domain.CompletePendingTransactionInput) (*domain.Transaction, error) {
		out, err := writer.CompletePendingTransaction(ctx, owner, id, input)
		cancel()
		return out, err
	}}
	summary, err := svc.ProcessPendingReceipts(ctx)
	require.NoError(t, err)
	require.Equal(t, domain.PendingReceiptSummary{Fetched: 3, Succeeded: 1, Deferred: 2, StoppedEarly: true}, summary)
}

func TestPendingReceiptsExtractionTimeoutAndCompletionReserve(t *testing.T) {
	for _, batchExpires := range []bool{false, true} {
		t.Run("batch="+strconv.FormatBool(batchExpires), func(t *testing.T) {
			svc, _ := pendingFixture(t, pendingRows(2))
			svc.options.Timeout = 100 * time.Millisecond
			svc.options.CompletionReserve = 20 * time.Millisecond
			svc.extraction.extractionTimeout = time.Millisecond
			if batchExpires {
				svc.extraction.extractionTimeout = time.Second
			}
			calls := 0
			original := svc.extraction.extractor
			svc.extraction.extractor = receiptExtractorFunc(func(ctx context.Context, input ai.ExtractionInput) (ai.ExtractedTransaction, error) {
				calls++
				if calls == 1 {
					<-ctx.Done()
					// A provider returning late success must not be completed.
					return original.ExtractTransaction(ctx, input)
				}
				return original.ExtractTransaction(ctx, input)
			})
			summary, err := svc.ProcessPendingReceipts(context.Background())
			require.NoError(t, err)
			require.Equal(t, 1, summary.Failed)
			if batchExpires {
				require.Equal(t, 1, calls)
				require.Equal(t, 1, summary.Deferred)
				require.True(t, summary.StoppedEarly)
			} else {
				require.Equal(t, 1, summary.Succeeded)
			}
		})
	}
	// Completion gets the longer parent budget rather than extraction's deadline.
	svc, _ := pendingFixture(t, pendingRows(1))
	var extractionDeadline time.Time
	base := svc.extraction.extractor
	svc.extraction.extractor = receiptExtractorFunc(func(ctx context.Context, input ai.ExtractionInput) (ai.ExtractedTransaction, error) {
		extractionDeadline, _ = ctx.Deadline()
		return base.ExtractTransaction(ctx, input)
	})
	writer := svc.completion
	svc.completion = receiptWriter{complete: func(ctx context.Context, owner, id uuid.UUID, input domain.CompletePendingTransactionInput) (*domain.Transaction, error) {
		deadline, _ := ctx.Deadline()
		require.True(t, deadline.After(extractionDeadline))
		return writer.CompletePendingTransaction(ctx, owner, id, input)
	}}
	_, err := svc.ProcessPendingReceipts(context.Background())
	require.NoError(t, err)
}
