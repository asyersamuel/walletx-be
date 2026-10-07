package service

import (
	"walletx-be/internal/modules/transactions/jobs"

	"context"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"
	domain "walletx-be/internal/modules/transactions/contracts"
	"walletx-be/internal/platform/logger"

	apperrors "walletx-be/internal/shared/errors"

	"github.com/google/uuid"
)

// defaultEmbeddingJobConfig is the production retry policy used when the
// service is constructed without an explicit override.
var defaultEmbeddingJobConfig = jobs.EmbeddingJobConfig{
	MaxAttempts: 3,
	Backoff:     250 * time.Millisecond,
}

type transactionService struct {
	repo      domain.TransactionRepository
	embedding domain.EmbeddingProvider
	logger    logger.Logger
	storage   domain.ReceiptStorage
	jobCfg    jobs.EmbeddingJobConfig
}

// NewTransactionService constructs a service that depends only on the
// repository abstraction, never on its concrete database implementation.
// An optional EmbeddingProvider enables async vector generation; without one
// the service runs without embedding support.
func NewTransactionService(repo domain.TransactionRepository, embedding ...domain.EmbeddingService) domain.TransactionService {
	var provider domain.EmbeddingProvider
	if len(embedding) > 0 {
		provider = embedding[0]
	}
	return &transactionService{
		repo:      repo,
		embedding: provider,
		logger:    nopServiceLogger{},
		jobCfg:    defaultEmbeddingJobConfig,
	}
}

// NewTransactionServiceWithLogger constructs a service with a concrete logger
// injected. Use this variant in production wiring where structured log output
// is required for the async embedding pipeline.
func NewTransactionServiceWithLogger(
	repo domain.TransactionRepository,
	embedding domain.EmbeddingProvider,
	appLogger logger.Logger,
	receiptStorage domain.ReceiptStorage,
) domain.TransactionService {
	return &transactionService{
		repo:      repo,
		embedding: embedding,
		logger:    appLogger,
		storage:   receiptStorage,
		jobCfg:    defaultEmbeddingJobConfig,
	}
}

// nopServiceLogger satisfies logger.Logger with zero-cost no-ops. It is the
// default used when NewTransactionService is called without a logger (e.g. in
// unit tests that do not care about log output).
type nopServiceLogger struct{}

func (nopServiceLogger) Info(string)                                       {}
func (nopServiceLogger) Warn(string)                                       {}
func (nopServiceLogger) Error(string)                                      {}
func (nopServiceLogger) Debug(string)                                      {}
func (l nopServiceLogger) WithField(string, interface{}) logger.Logger     { return l }
func (l nopServiceLogger) WithError(error) logger.Logger                   { return l }
func (l nopServiceLogger) WithFields(map[string]interface{}) logger.Logger { return l }

// CreateTransaction validates business-owned fields, applies normalization, and
// delegates persistence using the authenticated user ID. The request cannot
// override ownership through input.UserID.
func (s *transactionService) CreateTransaction(ctx context.Context, userID uuid.UUID, input domain.CreateTransactionInput) (*domain.Transaction, error) {
	input, err := normalizeCreateInput(userID, input)
	if err != nil {
		return nil, err
	}
	created, err := s.repo.Create(ctx, input)
	if err != nil {
		return nil, mapRepositoryError(err)
	}

	s.enqueueEmbedding(created, input.CategoryName)
	return created, nil
}

func (s *transactionService) GetTransaction(ctx context.Context, userID, transactionID uuid.UUID) (*domain.Transaction, error) {
	if userID == uuid.Nil || transactionID == uuid.Nil {
		return nil, invalidInputError("user ID and transaction ID must not be empty")
	}
	transaction, err := s.repo.GetTransactionByID(ctx, userID, transactionID)
	if err != nil {
		return nil, mapRepositoryError(err)
	}
	return transaction, nil
}

func (s *transactionService) ListTransactions(ctx context.Context, userID uuid.UUID, input domain.ListTransactionsInput) ([]domain.Transaction, error) {
	if userID == uuid.Nil {
		return nil, invalidInputError("user ID must not be empty")
	}
	if input.Limit < 0 || input.Offset < 0 {
		return nil, invalidInputError("limit and offset must not be negative")
	}
	if input.Limit == 0 {
		input.Limit = domain.DefaultListLimit
	}
	if input.StartDate != nil && input.EndDate != nil && input.StartDate.After(*input.EndDate) {
		return nil, invalidInputError("start date must not be after end date")
	}
	input.UserID = userID
	transactions, err := s.repo.ListTransactions(ctx, input)
	if err != nil {
		return nil, mapRepositoryError(err)
	}
	return transactions, nil
}

func (s *transactionService) UpdateTransaction(ctx context.Context, userID, transactionID uuid.UUID, input domain.UpdateTransactionInput) (*domain.Transaction, error) {
	if userID == uuid.Nil || transactionID == uuid.Nil {
		return nil, invalidInputError("user ID and transaction ID must not be empty")
	}
	if input.Amount != nil && (*input.Amount <= 0 || math.IsNaN(*input.Amount) || math.IsInf(*input.Amount, 0)) {
		return nil, errAmountMustBePositive
	}
	if input.MerchantName != nil {
		trimmed := strings.TrimSpace(*input.MerchantName)
		if trimmed == "" {
			return nil, invalidInputError("merchant name must not be empty")
		}
		input.MerchantName = &trimmed
	}

	// Detect semantic changes before delegating to the repository so we know
	// whether to enqueue an embedding job after a successful update.
	semanticChange := hasSemanticChange(input)

	input.UserID = userID
	input.TransactionID = transactionID
	updated, err := s.repo.UpdateTransaction(ctx, input)
	if err != nil {
		return nil, mapRepositoryError(err)
	}

	if s.embedding != nil && semanticChange {
		// Build the embedding text from the committed state. The updated
		// transaction does not carry the category name, so we construct the
		// best available text from the fields we do have. A future iteration
		// can enrich this with a category lookup if needed.
		job := jobs.NewEmbeddingJob(s.repo, s.embedding, s.logger, s.jobCfg)
		inp := jobs.EmbeddingJobInput{
			TransactionID: updated.ID,
			UserID:        updated.UserID,
			UpdatedAt:     updated.UpdatedAt,
			Text:          embeddingTextFromTransaction(*updated),
		}
		go func() { _ = job.Run(context.Background(), inp) }()
	}

	return updated, nil
}

func (s *transactionService) DeleteTransaction(ctx context.Context, userID, transactionID uuid.UUID) error {
	if ctx == nil || userID == uuid.Nil || transactionID == uuid.Nil {
		return invalidInputError("user ID and transaction ID must not be empty")
	}
	transaction, err := s.repo.GetTransactionForDeletion(ctx, userID, transactionID)
	if err != nil {
		return mapRepositoryError(err)
	}
	if transaction == nil || transaction.ID != transactionID || transaction.UserID != userID {
		return apperrors.ErrInternal
	}
	if err := s.repo.DeleteTransaction(ctx, userID, transactionID); err != nil {
		return mapRepositoryError(err)
	}
	// Clean only after confirmed DB deletion. A failed/uncertain delete must not
	// remove an image that may still belong to a persisted transaction.
	if transaction.ReceiptImageUrl != nil && strings.TrimSpace(*transaction.ReceiptImageUrl) != "" {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), receiptCleanupTimeout)
		defer cancel()
		var cleanupErr error
		if s.storage == nil {
			cleanupErr = apperrors.ErrInternal
		} else {
			cleanupErr = s.storage.DeleteReceipt(cleanupCtx, userID, *transaction.ReceiptImageUrl)
		}
		if cleanupErr != nil {
			safe := apperrors.ErrInternal
			if errors.Is(cleanupErr, context.DeadlineExceeded) {
				safe = context.DeadlineExceeded
			}
			s.logger.WithError(safe).WithFields(map[string]interface{}{"operation": "delete_transaction_receipt", "dependency": "storage"}).Error("transaction deleted but receipt cleanup failed")
		}
	}
	return nil
}

// hasSemanticChange reports whether any field that affects the meaning of a
// transaction for embedding/search purposes is being changed. Changes to
// receipt_image_url and transaction_date are intentionally excluded because
// they carry no semantic content.
func hasSemanticChange(input domain.UpdateTransactionInput) bool {
	return input.Amount != nil ||
		input.MerchantName != nil ||
		input.CategoryID != nil ||
		input.Notes != nil ||
		input.ClearNotes
}

// embeddingText builds the canonical embedding input string for a transaction
// using the category name resolved at creation time.
func embeddingText(transaction domain.Transaction, categoryName string) string {
	var b strings.Builder
	b.WriteString("merchant_name: ")
	b.WriteString(transaction.MerchantName)
	b.WriteString("\namount: ")
	b.WriteString(strconv.FormatFloat(transaction.Amount, 'f', -1, 64))
	b.WriteString("\ncategory: ")
	b.WriteString(categoryName)
	if transaction.Notes != nil {
		b.WriteString("\nnotes: ")
		b.WriteString(*transaction.Notes)
	}
	return b.String()
}

// embeddingTextFromTransaction builds the embedding input string from the
// committed transaction state after an update. It omits the category name
// because the domain Transaction does not carry it (the category is joined from
// a separate table). A future iteration can enrich this via a category lookup.
func embeddingTextFromTransaction(t domain.Transaction) string {
	var b strings.Builder
	b.WriteString("merchant_name: ")
	b.WriteString(t.MerchantName)
	b.WriteString("\namount: ")
	b.WriteString(strconv.FormatFloat(t.Amount, 'f', -1, 64))
	if t.Notes != nil {
		b.WriteString("\nnotes: ")
		b.WriteString(*t.Notes)
	}
	return b.String()
}

// generateAndSaveEmbedding is the legacy fire-and-forget helper retained for
// compatibility with existing tests that call it directly through the create
// path. New code goes through embeddingJob.run.
func (s *transactionService) generateAndSaveEmbedding(transaction domain.Transaction, categoryName string) {
	text := embeddingText(transaction, categoryName)
	embedding, err := s.embedding.Generate(context.Background(), text)
	if err != nil {
		return
	}
	_ = s.repo.SaveEmbedding(context.Background(), transaction.ID, transaction.UserID, embedding, transaction.UpdatedAt)
}

// errAmountMustBePositive keeps the public message exact while still
// classifying the error as invalid input through errors.Is.
var errAmountMustBePositive = validationError{message: "Amount must be > 0"}

type validationError struct {
	message string
}

func (e validationError) Error() string { return e.message }
func (e validationError) Unwrap() error { return apperrors.ErrInvalidInput }

func invalidInputError(message string) error {
	return validationError{message: message}
}

func mapRepositoryError(err error) error {
	if errors.Is(err, apperrors.ErrNotFound) ||
		errors.Is(err, apperrors.ErrInvalidInput) ||
		errors.Is(err, apperrors.ErrUnauthorized) ||
		errors.Is(err, apperrors.ErrConflict) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return apperrors.ErrInternal
}

func normalizeCreateInput(userID uuid.UUID, input domain.CreateTransactionInput) (domain.CreateTransactionInput, error) {
	if userID == uuid.Nil {
		return domain.CreateTransactionInput{}, invalidInputError("user ID must not be empty")
	}
	if input.CategoryID == uuid.Nil {
		return domain.CreateTransactionInput{}, invalidInputError("category ID must not be empty")
	}
	if input.Amount <= 0 || math.IsNaN(input.Amount) || math.IsInf(input.Amount, 0) {
		return domain.CreateTransactionInput{}, errAmountMustBePositive
	}

	merchantName := strings.TrimSpace(input.MerchantName)
	if merchantName == "" {
		return domain.CreateTransactionInput{}, invalidInputError("merchant name must not be empty")
	}
	if input.TransactionDate.IsZero() {
		return domain.CreateTransactionInput{}, invalidInputError("transaction date must not be empty")
	}

	input.UserID = userID
	input.MerchantName = merchantName
	return input, nil
}

func (s *transactionService) enqueueEmbedding(created *domain.Transaction, categoryName string) {
	if s.embedding != nil {
		// Embedding generation is deliberately detached from the request context:
		// the transaction has already committed and must still be embedded if the
		// caller disconnects or the request context is canceled.
		job := jobs.NewEmbeddingJob(s.repo, s.embedding, s.logger, s.jobCfg)
		inp := jobs.EmbeddingJobInput{
			TransactionID: created.ID,
			UserID:        created.UserID,
			UpdatedAt:     created.UpdatedAt,
			Text:          embeddingText(*created, categoryName),
		}
		go func() { _ = job.Run(context.Background(), inp) }()
	}
}
