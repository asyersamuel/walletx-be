package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"walletx-be/internal/modules/ai"
	domain "walletx-be/internal/modules/transactions/contracts"
	"walletx-be/internal/platform/logger"
	apperrors "walletx-be/internal/shared/errors"
)

// ReceiptExtractor is satisfied by ai.AIService without depending on HTTP delivery.
type ReceiptExtractor interface {
	ExtractTransaction(context.Context, ai.ExtractionInput) (ai.ExtractedTransaction, error)
}

type receiptService struct {
	extractor         ReceiptExtractor
	categories        ai.CategoryReader
	transactions      domain.ReceiptTransactionWriter
	storage           domain.ReceiptStorage
	logger            logger.Logger
	extractionTimeout time.Duration
	location          *time.Location
	now               func() time.Time
}

var _ domain.ReceiptService = (*receiptService)(nil)

func NewReceiptService(extractor ReceiptExtractor, categories ai.CategoryReader, transactions domain.ReceiptTransactionWriter, storage domain.ReceiptStorage, appLogger logger.Logger, extractionTimeout time.Duration, location *time.Location, now func() time.Time) (domain.ReceiptService, error) {
	if extractor == nil || categories == nil || transactions == nil || storage == nil || appLogger == nil || extractionTimeout <= 0 || location == nil || location == time.Local || now == nil {
		return nil, fmt.Errorf("receipt dependencies, positive AI timeout, explicit timezone and clock are required: %w", apperrors.ErrInvalidInput)
	}
	return &receiptService{extractor: extractor, categories: categories, transactions: transactions, storage: storage, logger: appLogger, extractionTimeout: extractionTimeout, location: location, now: now}, nil
}

func (s *receiptService) ProcessReceipt(ctx context.Context, userID uuid.UUID, contentType, filename string, image []byte) (domain.ReceiptResult, error) {
	if ctx == nil {
		return domain.ReceiptResult{}, invalidInputError("receipt context is required")
	}
	if err := ctx.Err(); err != nil {
		return domain.ReceiptResult{}, err
	}
	if userID == uuid.Nil || len(image) == 0 || len(image) > domain.MaxReceiptBytes || !validReceiptMIME(contentType) {
		return domain.ReceiptResult{}, invalidInputError("receipt user, supported image and valid size are required")
	}
	reference, err := s.storage.SaveReceipt(ctx, userID, contentType, filename, image)
	if err != nil {
		s.logDependencyFailure(err, "save_receipt", "storage")
		if ctx.Err() != nil {
			return domain.ReceiptResult{}, ctx.Err()
		}
		return domain.ReceiptResult{}, apperrors.ErrInternal
	}
	if strings.TrimSpace(reference) == "" {
		s.logDependencyFailure(apperrors.ErrInternal, "save_receipt", "storage")
		return domain.ReceiptResult{}, apperrors.ErrInternal
	}
	// No database operation has started yet, so cancellation here is safe to compensate.
	if err := ctx.Err(); err != nil {
		s.cleanupUpload(ctx, userID, reference)
		return domain.ReceiptResult{}, err
	}
	pending, err := s.transactions.CreatePendingTransaction(ctx, userID, domain.CreatePendingTransactionInput{UserID: userID, ReceiptImageURL: reference})
	if err != nil {
		if errors.Is(err, apperrors.ErrInvalidInput) || errors.Is(err, apperrors.ErrNotFound) {
			s.cleanupUpload(ctx, userID, reference)
		} else {
			// Connection loss/cancellation can occur after INSERT committed. Never delete
			// a potentially referenced image; reconcile retained uploads against the DB.
			s.logger.WithFields(map[string]interface{}{"operation": "create_pending_transaction", "dependency": "storage"}).Warn("receipt upload retained for reconciliation")
		}
		return domain.ReceiptResult{}, mapRepositoryError(err)
	}
	if pending == nil || pending.ID == uuid.Nil || pending.UserID != userID || pending.ReceiptImageURL != reference {
		return domain.ReceiptResult{}, apperrors.ErrInternal
	}
	result := domain.ReceiptResult{Status: domain.TransactionStatusPending, TransactionID: pending.ID, ReceiptImageURL: reference}
	if ctx.Err() != nil {
		return result, nil
	}
	// The pending INSERT has committed. Only extraction/category resolution use
	// this timeout; completing the row uses the still-live parent request context.
	extractionCtx, cancel := context.WithTimeout(ctx, s.extractionTimeout)
	defer cancel()
	extracted, err := s.extractor.ExtractTransaction(extractionCtx, ai.ExtractionInput{
		UserID: userID, Image: image, MIMEType: contentType, ReferenceTime: s.now().In(s.location), TimeZone: s.location.String(),
	})
	if err == nil {
		err = extractionCtx.Err()
	}
	if err != nil {
		s.logDependencyFailure(err, "extract_receipt", "ai")
		return result, nil
	}
	date, err := time.ParseInLocation(time.DateOnly, extracted.Date, s.location)
	if err != nil || extracted.CategoryID == uuid.Nil || strings.TrimSpace(extracted.Merchant) == "" || extracted.Amount <= 0 || math.IsNaN(extracted.Amount) || math.IsInf(extracted.Amount, 0) {
		s.logDependencyFailure(apperrors.ErrInvalidInput, "validate_receipt_extraction", "ai")
		return result, nil
	}
	categories, err := s.categories.ListByUser(extractionCtx, userID)
	if err == nil {
		err = extractionCtx.Err()
	}
	if err != nil {
		s.logDependencyFailure(err, "resolve_receipt_category", "database")
		return result, nil
	}
	categoryName := ""
	for _, category := range categories {
		if category.ID == extracted.CategoryID {
			categoryName = category.Name
			break
		}
	}
	if strings.TrimSpace(categoryName) == "" {
		s.logDependencyFailure(apperrors.ErrNotFound, "validate_receipt_category", "database")
		return result, nil
	}
	if ctx.Err() != nil {
		return result, nil
	}
	completed, err := s.transactions.CompletePendingTransaction(ctx, userID, pending.ID, domain.CompletePendingTransactionInput{
		UserID: userID, TransactionID: pending.ID, CategoryID: extracted.CategoryID, CategoryName: categoryName,
		MerchantName: strings.TrimSpace(extracted.Merchant), Amount: extracted.Amount, TransactionDate: date, Notes: extracted.Notes,
	})
	if err != nil {
		return domain.ReceiptResult{}, mapRepositoryError(err)
	}
	if completed == nil || completed.ID != pending.ID || completed.UserID != userID {
		return domain.ReceiptResult{}, apperrors.ErrInternal
	}
	result.Status, result.Transaction = domain.TransactionStatusSuccess, completed
	return result, nil
}

const receiptCleanupTimeout = 5 * time.Second

func (s *receiptService) cleanupUpload(ctx context.Context, userID uuid.UUID, reference string) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), receiptCleanupTimeout)
	defer cancel()
	if err := s.storage.DeleteReceipt(cleanupCtx, userID, reference); err != nil {
		s.logDependencyFailure(err, "cleanup_receipt_upload", "storage")
	}
}

func (s *receiptService) logDependencyFailure(err error, operation, dependency string) {
	// Preserve safe provider diagnostics through wrapped errors. Arbitrary SDK
	// messages/causes may contain credentials or receipt contents; never log them.
	safe := apperrors.ErrInternal
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		safe = context.DeadlineExceeded
	case errors.Is(err, context.Canceled):
		safe = context.Canceled
	case errors.Is(err, apperrors.ErrInvalidInput):
		safe = apperrors.ErrInvalidInput
	case errors.Is(err, apperrors.ErrNotFound):
		safe = apperrors.ErrNotFound
	}
	fields := map[string]interface{}{"operation": operation, "dependency": dependency}
	var diagnostic interface{ SafeLogFields() map[string]interface{} }
	if errors.As(err, &diagnostic) {
		for key, value := range diagnostic.SafeLogFields() {
			if key != "operation" && key != "dependency" {
				fields[key] = value
			}
		}
	}
	var logSafe interface{ SafeLogError() error }
	if errors.As(err, &logSafe) {
		safe = logSafe.SafeLogError()
	}
	s.logger.WithError(safe).WithFields(fields).Error("receipt processing could not complete")
}

func validReceiptMIME(contentType string) bool {
	switch contentType {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return true
	default:
		return false
	}
}
