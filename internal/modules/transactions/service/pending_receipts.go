package service

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"walletx-be/internal/modules/ai"
	domain "walletx-be/internal/modules/transactions/contracts"
	"walletx-be/internal/platform/logger"
	apperrors "walletx-be/internal/shared/errors"
)

// PendingReceiptOptions bounds one invocation. CompletionReserve is kept out of
// download/extraction deadlines so a successful extraction can still be saved.
type PendingReceiptOptions struct {
	BatchSize         int
	Timeout           time.Duration
	DownloadTimeout   time.Duration
	ExtractionTimeout time.Duration
	CompletionReserve time.Duration
}

type pendingReceiptService struct {
	pending    domain.PendingReceiptReader
	images     domain.ReceiptReader
	completion domain.ReceiptTransactionWriter
	extraction *receiptService
	options    PendingReceiptOptions
}

func NewPendingReceiptService(pending domain.PendingReceiptReader, images domain.ReceiptReader, completion domain.ReceiptTransactionWriter, extractor ReceiptExtractor, categories ai.CategoryReader, appLogger logger.Logger, options PendingReceiptOptions, location *time.Location, now func() time.Time) (domain.PendingReceiptProcessor, error) {
	if pending == nil || images == nil || completion == nil || extractor == nil || categories == nil || appLogger == nil || location == nil || location == time.Local || now == nil || options.BatchSize < 1 || options.BatchSize > domain.MaxPendingReceiptBatchSize || options.DownloadTimeout <= 0 || options.ExtractionTimeout <= 0 || options.CompletionReserve <= 0 || options.Timeout <= options.CompletionReserve {
		return nil, fmt.Errorf("pending receipt dependencies and valid execution limits are required: %w", apperrors.ErrInvalidInput)
	}
	return &pendingReceiptService{
		pending: pending, images: images, completion: completion, options: options,
		extraction: &receiptService{extractor: extractor, categories: categories, logger: appLogger, extractionTimeout: options.ExtractionTimeout, location: location, now: now},
	}, nil
}

func (s *pendingReceiptService) ProcessPendingReceipts(ctx context.Context) (domain.PendingReceiptSummary, error) {
	var summary domain.PendingReceiptSummary
	if ctx == nil {
		return summary, apperrors.ErrInvalidInput
	}
	batchCtx, cancel := context.WithTimeout(ctx, s.options.Timeout)
	defer cancel()
	deadline, _ := batchCtx.Deadline()
	workCtx, stopWork := context.WithDeadline(batchCtx, deadline.Add(-s.options.CompletionReserve))
	defer stopWork()
	if workCtx.Err() != nil {
		summary.StoppedEarly = true
		return summary, nil
	}
	rows, err := s.pending.GetPendingTransactions(workCtx, s.options.BatchSize)
	if err != nil {
		if workCtx.Err() != nil {
			summary.StoppedEarly = true
			return summary, nil
		}
		s.extraction.logDependencyFailure(err, "fetch_pending_receipts", "database")
		return summary, apperrors.ErrInternal
	}
	summary.Fetched = len(rows)
	for _, row := range rows {
		if workCtx.Err() != nil {
			summary.StoppedEarly = true
			break
		}
		if err := s.processOne(workCtx, batchCtx, row); err != nil {
			summary.Failed++
		} else {
			summary.Succeeded++
		}
	}
	if workCtx.Err() != nil {
		summary.StoppedEarly = true
	}
	summary.Deferred = summary.Fetched - summary.Succeeded - summary.Failed
	return summary, nil
}

func (s *pendingReceiptService) processOne(workCtx, batchCtx context.Context, row domain.PendingTransaction) error {
	if row.ID == uuid.Nil || row.UserID == uuid.Nil || row.ReceiptImageURL == "" {
		s.extraction.logDependencyFailure(apperrors.ErrInvalidInput, "validate_pending_receipt", "database")
		return apperrors.ErrInvalidInput
	}
	downloadCtx, cancel := context.WithTimeout(workCtx, s.options.DownloadTimeout)
	image, err := s.images.ReadReceipt(downloadCtx, row.UserID, row.ReceiptImageURL)
	if err == nil {
		err = downloadCtx.Err()
	}
	cancel()
	if err != nil {
		s.extraction.logDependencyFailure(err, "read_pending_receipt", "storage")
		return err
	}
	mime := http.DetectContentType(image)
	if len(image) == 0 || len(image) > domain.MaxReceiptBytes || !validReceiptMIME(mime) {
		s.extraction.logDependencyFailure(apperrors.ErrInvalidInput, "validate_pending_image", "storage")
		return apperrors.ErrInvalidInput
	}
	input, err := s.extraction.prepareCompletion(workCtx, row.UserID, mime, image)
	if err != nil {
		// Shared extraction logging includes provider_raw_error_detail; do not
		// log a second event or replace it with the summarized Error string.
		return err
	}
	if err := batchCtx.Err(); err != nil {
		return err
	}
	input.TransactionID = row.ID
	completed, err := s.completion.CompletePendingTransaction(batchCtx, row.UserID, row.ID, input)
	if err != nil {
		s.extraction.logDependencyFailure(err, "complete_pending_receipt", "database")
		return err
	}
	if completed == nil || completed.ID != row.ID || completed.UserID != row.UserID || completed.Status != domain.TransactionStatusSuccess {
		s.extraction.logDependencyFailure(apperrors.ErrInternal, "validate_pending_completion", "database")
		return apperrors.ErrInternal
	}
	return nil
}
