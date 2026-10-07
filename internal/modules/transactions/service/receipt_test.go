package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"walletx-be/internal/modules/ai"
	domain "walletx-be/internal/modules/transactions/contracts"
	"walletx-be/internal/platform/logger"
	apperrors "walletx-be/internal/shared/errors"
)

type receiptExtractorFunc func(context.Context, ai.ExtractionInput) (ai.ExtractedTransaction, error)

func (f receiptExtractorFunc) ExtractTransaction(ctx context.Context, input ai.ExtractionInput) (ai.ExtractedTransaction, error) {
	return f(ctx, input)
}

type receiptDiagnosticError struct{}

func (receiptDiagnosticError) Error() string { return "secret Gemini body" }
func (receiptDiagnosticError) SafeLogError() error {
	return errors.New("Gemini HTTP 404 NOT_FOUND: model_access_restricted")
}
func (receiptDiagnosticError) SafeLogFields() map[string]interface{} {
	return map[string]interface{}{"provider_http_code": 404, "provider_status": "NOT_FOUND", "provider_reason": "model_access_restricted", "provider_model": "gemini-test", "provider_exact_message": "Consumer invalid", "provider_error_reason": "CONSUMER_INVALID"}
}

func TestReceiptDependencyLogPreservesWrappedDiagnostics(t *testing.T) {
	log := &receiptLog{}
	svc := &receiptService{logger: log}
	svc.logDependencyFailure(fmt.Errorf("extract transaction: %w", receiptDiagnosticError{}), "extract_receipt", "ai")
	require.Equal(t, 1, log.errors)
	require.Equal(t, "extract_receipt", log.fields["operation"])
	require.Equal(t, "ai", log.fields["dependency"])
	require.Equal(t, 404, log.fields["provider_http_code"])
	require.Equal(t, "NOT_FOUND", log.fields["provider_status"])
	require.Equal(t, "gemini-test", log.fields["provider_model"])
	require.Equal(t, "Consumer invalid", log.fields["provider_exact_message"])
	require.Equal(t, "CONSUMER_INVALID", log.fields["provider_error_reason"])
	require.Contains(t, log.errorValue.Error(), "model_access_restricted")
	require.NotContains(t, log.errorValue.Error(), "secret")
}

type receiptCategoriesFunc func(context.Context, uuid.UUID) ([]ai.CategoryOption, error)

func (f receiptCategoriesFunc) ListByUser(ctx context.Context, id uuid.UUID) ([]ai.CategoryOption, error) {
	return f(ctx, id)
}

type receiptWriter struct {
	pending  func(context.Context, uuid.UUID, domain.CreatePendingTransactionInput) (*domain.PendingTransaction, error)
	complete func(context.Context, uuid.UUID, uuid.UUID, domain.CompletePendingTransactionInput) (*domain.Transaction, error)
}

func (w receiptWriter) CreatePendingTransaction(ctx context.Context, user uuid.UUID, input domain.CreatePendingTransactionInput) (*domain.PendingTransaction, error) {
	return w.pending(ctx, user, input)
}
func (w receiptWriter) CompletePendingTransaction(ctx context.Context, user, id uuid.UUID, input domain.CompletePendingTransactionInput) (*domain.Transaction, error) {
	return w.complete(ctx, user, id, input)
}

type receiptStorageStub struct {
	save   func(context.Context, uuid.UUID, string, string, []byte) (string, error)
	delete func(context.Context, uuid.UUID, string) error
}

func (s receiptStorageStub) SaveReceipt(ctx context.Context, user uuid.UUID, mime, name string, image []byte) (string, error) {
	return s.save(ctx, user, mime, name, image)
}
func (s receiptStorageStub) DeleteReceipt(ctx context.Context, user uuid.UUID, ref string) error {
	return s.delete(ctx, user, ref)
}

type receiptLog struct {
	errors, warnings int
	errorValue       error
	fields           map[string]interface{}
}

func (*receiptLog) Info(string)                         {}
func (*receiptLog) Debug(string)                        {}
func (l *receiptLog) Warn(string)                       { l.warnings++ }
func (l *receiptLog) Error(string)                      { l.errors++ }
func (l *receiptLog) WithError(err error) logger.Logger { l.errorValue = err; return l }
func (l *receiptLog) WithField(key string, value interface{}) logger.Logger {
	return l.WithFields(map[string]interface{}{key: value})
}
func (l *receiptLog) WithFields(fields map[string]interface{}) logger.Logger {
	l.fields = fields
	return l
}

func TestReceiptService_ResilientFlow(t *testing.T) {
	location, err := time.LoadLocation("Asia/Jakarta")
	require.NoError(t, err)
	now := time.Date(2026, 10, 4, 5, 30, 0, 0, time.UTC)
	user, category, id := uuid.New(), uuid.New(), uuid.New()
	notes := "Coffee"
	reference := "receipt://" + user.String() + "/" + uuid.NewString() + ".png"
	providerError := errors.New("provider secret and receipt text must stay private")
	for _, tc := range []struct {
		name                                                                       string
		storageErr, insertErr, extractionErr, categoryErr, completeErr, cleanupErr error
		timeout, lateSuccess, nilPending, nilCompleted, foreignCategory            bool
		cancelAt                                                                   string
		mutate                                                                     func(*ai.ExtractedTransaction)
		wantStatus                                                                 domain.TransactionStatus
		wantErr                                                                    error
		wantSequence                                                               []string
	}{
		{name: "success", wantStatus: domain.TransactionStatusSuccess, wantSequence: []string{"save", "pending", "extract", "categories", "complete"}},
		{name: "storage failure", storageErr: providerError, wantErr: apperrors.ErrInternal, wantSequence: []string{"save"}},
		{name: "confirmed insertion rejection cleans upload", insertErr: apperrors.ErrInvalidInput, wantErr: apperrors.ErrInvalidInput, wantSequence: []string{"save", "pending", "delete"}},
		{name: "cleanup failure preserves insertion error", insertErr: apperrors.ErrInvalidInput, cleanupErr: providerError, wantErr: apperrors.ErrInvalidInput, wantSequence: []string{"save", "pending", "delete"}},
		{name: "ambiguous insertion retains image", insertErr: providerError, wantErr: apperrors.ErrInternal, wantSequence: []string{"save", "pending"}},
		{name: "nil pending retains image", nilPending: true, wantErr: apperrors.ErrInternal, wantSequence: []string{"save", "pending"}},
		{name: "provider failure keeps pending", extractionErr: providerError, wantStatus: domain.TransactionStatusPending, wantSequence: []string{"save", "pending", "extract"}},
		{name: "timeout keeps pending", timeout: true, wantStatus: domain.TransactionStatusPending, wantSequence: []string{"save", "pending", "extract"}},
		{name: "late success after deadline keeps pending", timeout: true, lateSuccess: true, wantStatus: domain.TransactionStatusPending, wantSequence: []string{"save", "pending", "extract"}},
		{name: "invalid date keeps pending", mutate: func(e *ai.ExtractedTransaction) { e.Date = "2026-02-30" }, wantStatus: domain.TransactionStatusPending, wantSequence: []string{"save", "pending", "extract"}},
		{name: "nonfinite amount keeps pending", mutate: func(e *ai.ExtractedTransaction) { e.Amount = math.NaN() }, wantStatus: domain.TransactionStatusPending, wantSequence: []string{"save", "pending", "extract"}},
		{name: "category dependency failure keeps pending", categoryErr: providerError, wantStatus: domain.TransactionStatusPending, wantSequence: []string{"save", "pending", "extract", "categories"}},
		{name: "foreign category keeps pending", foreignCategory: true, wantStatus: domain.TransactionStatusPending, wantSequence: []string{"save", "pending", "extract", "categories"}},
		{name: "completion failure retains receipt", completeErr: apperrors.ErrInternal, wantErr: apperrors.ErrInternal, wantSequence: []string{"save", "pending", "extract", "categories", "complete"}},
		{name: "nil completion is an error", nilCompleted: true, wantErr: apperrors.ErrInternal, wantSequence: []string{"save", "pending", "extract", "categories", "complete"}},
		{name: "cancel before any work", cancelAt: "before", wantErr: context.Canceled},
		{name: "cancel after upload cleans uninserted image", cancelAt: "save", wantErr: context.Canceled, wantSequence: []string{"save", "delete"}},
		{name: "cancel after pending commit", cancelAt: "pending", wantStatus: domain.TransactionStatusPending, wantSequence: []string{"save", "pending"}},
		{name: "cancel during extraction", cancelAt: "extract", wantStatus: domain.TransactionStatusPending, wantSequence: []string{"save", "pending", "extract"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancelAt == "before" {
				cancel()
			}
			var sequence []string
			log := &receiptLog{}
			storage := receiptStorageStub{
				save: func(got context.Context, owner uuid.UUID, mime, name string, bytes []byte) (string, error) {
					sequence = append(sequence, "save")
					require.Equal(t, user, owner)
					require.Equal(t, "image/png", mime)
					require.Equal(t, "receipt.png", name)
					require.Equal(t, []byte{1, 2, 3}, bytes)
					if tc.cancelAt == "save" {
						cancel()
					}
					return reference, tc.storageErr
				},
				delete: func(got context.Context, owner uuid.UUID, ref string) error {
					sequence = append(sequence, "delete")
					require.NoError(t, got.Err())
					_, bounded := got.Deadline()
					require.True(t, bounded)
					require.Equal(t, user, owner)
					require.Equal(t, reference, ref)
					return tc.cleanupErr
				},
			}
			writer := receiptWriter{
				pending: func(got context.Context, owner uuid.UUID, input domain.CreatePendingTransactionInput) (*domain.PendingTransaction, error) {
					sequence = append(sequence, "pending")
					require.Equal(t, user, owner)
					require.Equal(t, domain.CreatePendingTransactionInput{UserID: user, ReceiptImageURL: reference}, input)
					if tc.cancelAt == "pending" {
						cancel()
					}
					if tc.nilPending {
						return nil, nil
					}
					return &domain.PendingTransaction{ID: id, UserID: user, ReceiptImageURL: reference}, tc.insertErr
				},
				complete: func(got context.Context, owner, transaction uuid.UUID, input domain.CompletePendingTransactionInput) (*domain.Transaction, error) {
					sequence = append(sequence, "complete")
					require.NoError(t, got.Err())
					require.Equal(t, id, transaction)
					require.Equal(t, user, owner)
					require.Equal(t, domain.CompletePendingTransactionInput{UserID: user, TransactionID: id, CategoryID: category, CategoryName: "Food", MerchantName: "Coffee shop", Amount: 25000, TransactionDate: time.Date(2026, 10, 4, 0, 0, 0, 0, location), Notes: &notes}, input)
					if tc.nilCompleted {
						return nil, nil
					}
					return &domain.Transaction{ID: id, UserID: user, ReceiptImageUrl: &reference}, tc.completeErr
				},
			}
			extractor := receiptExtractorFunc(func(got context.Context, input ai.ExtractionInput) (ai.ExtractedTransaction, error) {
				sequence = append(sequence, "extract")
				_, bounded := got.Deadline()
				require.True(t, bounded)
				require.Equal(t, ai.ExtractionInput{UserID: user, Image: []byte{1, 2, 3}, MIMEType: "image/png", ReferenceTime: now.In(location), TimeZone: "Asia/Jakarta"}, input)
				if tc.cancelAt == "extract" {
					cancel()
				}
				if tc.timeout {
					<-got.Done()
					if !tc.lateSuccess {
						return ai.ExtractedTransaction{}, got.Err()
					}
				}
				e := ai.ExtractedTransaction{Amount: 25000, Merchant: " Coffee shop ", CategoryID: category, Notes: &notes, Date: "2026-10-04"}
				if tc.mutate != nil {
					tc.mutate(&e)
				}
				return e, tc.extractionErr
			})
			categories := receiptCategoriesFunc(func(got context.Context, owner uuid.UUID) ([]ai.CategoryOption, error) {
				sequence = append(sequence, "categories")
				require.Equal(t, user, owner)
				if tc.foreignCategory {
					return nil, nil
				}
				return []ai.CategoryOption{{ID: category, Name: "Food"}}, tc.categoryErr
			})
			timeout := time.Second
			if tc.timeout {
				timeout = 10 * time.Millisecond
			}
			svc, err := NewReceiptService(extractor, categories, writer, storage, log, timeout, location, func() time.Time { return now })
			require.NoError(t, err)
			result, err := svc.ProcessReceipt(ctx, user, "image/png", "receipt.png", []byte{1, 2, 3})
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				require.Equal(t, domain.ReceiptResult{}, result)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.wantStatus, result.Status)
				require.Equal(t, id, result.TransactionID)
				require.Equal(t, reference, result.ReceiptImageURL)
				if tc.wantStatus == domain.TransactionStatusPending {
					require.Nil(t, result.Transaction)
				} else {
					require.Equal(t, id, result.Transaction.ID)
				}
			}
			require.Equal(t, tc.wantSequence, sequence)
			if log.errorValue != nil {
				require.NotContains(t, log.errorValue.Error(), "secret")
				require.NotContains(t, log.errorValue.Error(), reference)
			}
		})
	}
}

func TestReceiptService_InvalidInputBeforeStorage(t *testing.T) {
	location := time.FixedZone("Asia/Jakarta", 7*60*60)
	store := receiptStorageStub{save: func(context.Context, uuid.UUID, string, string, []byte) (string, error) {
		t.Fatal("storage must not be called")
		return "", nil
	}}
	svc, err := NewReceiptService(receiptExtractorFunc(nil), receiptCategoriesFunc(nil), receiptWriter{}, store, nopServiceLogger{}, time.Second, location, time.Now)
	require.NoError(t, err)
	for _, tc := range []struct {
		ctx   context.Context
		user  uuid.UUID
		mime  string
		image []byte
	}{
		{nil, uuid.New(), "image/png", []byte{1}}, {context.Background(), uuid.Nil, "image/png", []byte{1}},
		{context.Background(), uuid.New(), "text/plain", []byte{1}}, {context.Background(), uuid.New(), "image/png", nil},
		{context.Background(), uuid.New(), "image/png", make([]byte, domain.MaxReceiptBytes+1)},
	} {
		_, err := svc.ProcessReceipt(tc.ctx, tc.user, tc.mime, "receipt.png", tc.image)
		require.ErrorIs(t, err, apperrors.ErrInvalidInput)
	}
}

func TestReceiptService_ConstructorRejectsInvalidDependencies(t *testing.T) {
	location := time.FixedZone("Asia/Jakarta", 7*60*60)
	extract := receiptExtractorFunc(func(context.Context, ai.ExtractionInput) (ai.ExtractedTransaction, error) {
		return ai.ExtractedTransaction{}, nil
	})
	categories := receiptCategoriesFunc(func(context.Context, uuid.UUID) ([]ai.CategoryOption, error) { return nil, nil })
	writer := receiptWriter{}
	store := receiptStorageStub{}
	for _, tc := range []struct {
		name       string
		extractor  ReceiptExtractor
		categories ai.CategoryReader
		writer     domain.ReceiptTransactionWriter
		store      domain.ReceiptStorage
		logger     logger.Logger
		timeout    time.Duration
		location   *time.Location
		now        func() time.Time
	}{
		{name: "missing extractor", categories: categories, writer: writer, store: store, logger: nopServiceLogger{}, timeout: time.Second, location: location, now: time.Now},
		{name: "missing category reader", extractor: extract, writer: writer, store: store, logger: nopServiceLogger{}, timeout: time.Second, location: location, now: time.Now},
		{name: "missing transaction writer", extractor: extract, categories: categories, store: store, logger: nopServiceLogger{}, timeout: time.Second, location: location, now: time.Now},
		{name: "missing storage", extractor: extract, categories: categories, writer: writer, logger: nopServiceLogger{}, timeout: time.Second, location: location, now: time.Now},
		{name: "missing logger", extractor: extract, categories: categories, writer: writer, store: store, timeout: time.Second, location: location, now: time.Now},
		{name: "zero timeout", extractor: extract, categories: categories, writer: writer, store: store, logger: nopServiceLogger{}, location: location, now: time.Now},
		{name: "negative timeout", extractor: extract, categories: categories, writer: writer, store: store, logger: nopServiceLogger{}, timeout: -time.Second, location: location, now: time.Now},
		{name: "implicit timezone", extractor: extract, categories: categories, writer: writer, store: store, logger: nopServiceLogger{}, timeout: time.Second, location: time.Local, now: time.Now},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, err := NewReceiptService(tc.extractor, tc.categories, tc.writer, tc.store, tc.logger, tc.timeout, tc.location, tc.now)
			require.ErrorIs(t, err, apperrors.ErrInvalidInput)
			require.Nil(t, svc)
		})
	}
}
