package transaction

import (
	"context"
	"errors"
	"time"

	"walletx-be/internal/modules/auth"
	"walletx-be/internal/modules/category"
	"walletx-be/internal/platform/cache"
	"walletx-be/internal/platform/logger"
	"walletx-be/internal/platform/parser"
	"walletx-be/internal/shared/errors"

	"github.com/google/uuid"
)

// UserStore is the narrow capability the transaction flow needs to resolve the
// owning user from an inbound email sender.
type UserStore interface {
	FindByEmail(ctx context.Context, email string) (*auth.User, error)
}

// CategoryLookup resolves a category by name for a given user.
type CategoryLookup interface {
	GetByName(ctx context.Context, name string, userID uuid.UUID) (*category.Category, error)
}

// Processor handles the "email → transaction" ingestion flow (used by the IMAP
// sync job in the cron module).
type Processor interface {
	ProcessTransactionEmail(ctx context.Context, senderEmail, messageID, rawBody string, date time.Time) error
}

// Service is the transaction business logic.
type Service interface {
	ProcessTransactionEmail(ctx context.Context, senderEmail, messageID, rawBody string, date time.Time) error
	CreateManualTransaction(ctx context.Context, userID uuid.UUID, input CreateTransactionInput) (*Transaction, error)
	UpdateTransaction(ctx context.Context, userID, txID uuid.UUID, input UpdateTransactionInput) (*Transaction, error)
	DeleteTransaction(ctx context.Context, userID, txID uuid.UUID) error
	GetUserTransactions(ctx context.Context, userID uuid.UUID, limit, offset int, filters TransactionFilters, sort SortOption) ([]Transaction, error)
	GetUserTransactionsWithCount(ctx context.Context, userID uuid.UUID, limit, offset int, filters TransactionFilters, sort SortOption) ([]Transaction, int, error)
	SearchTransactions(ctx context.Context, userID uuid.UUID, query string, limit, offset int) ([]Transaction, error)
	SearchTransactionsCount(ctx context.Context, userID uuid.UUID, query string) (int, error)
	GetReportsByCategory(ctx context.Context, userID uuid.UUID, month, year int) ([]CategoryExpense, error)
	GetAllTransactionsForExport(ctx context.Context, userID uuid.UUID, month, year int) ([]Transaction, error)
}

type service struct {
	userStore    UserStore
	repo         Repository
	category     CategoryLookup
	processor    Processor
	cacheManager cache.DashboardCacheManager
	logger       logger.Logger
}

type processor struct {
	userStore       UserStore
	repo            Repository
	category        CategoryLookup
	emailParser     parser.EmailParser
	cacheManager    cache.DashboardCacheManager
	logger          logger.Logger
	defaultCategory string
}

// NewProcessor wires the email-→-transaction ingestion worker.
func NewProcessor(
	userStore UserStore,
	repo Repository,
	category CategoryLookup,
	emailParser parser.EmailParser,
	cacheManager cache.DashboardCacheManager,
	logger logger.Logger,
) Processor {
	return &processor{
		userStore:       userStore,
		repo:            repo,
		category:        category,
		emailParser:     emailParser,
		cacheManager:    cacheManager,
		logger:          logger,
		defaultCategory: "Lainnya",
	}
}

// NewService wires the transaction business service.
func NewService(
	userStore UserStore,
	repo Repository,
	category CategoryLookup,
	processor Processor,
	cacheManager cache.DashboardCacheManager,
	logger logger.Logger,
) Service {
	return &service{
		userStore:    userStore,
		repo:         repo,
		category:     category,
		processor:    processor,
		cacheManager: cacheManager,
		logger:       logger,
	}
}

// ─── Processor (email ingestion) ─────────────────────────────────────────────

func (p *processor) ProcessTransactionEmail(ctx context.Context, senderEmail, messageID, rawBody string, date time.Time) error {
	logger := p.logger.WithField("sender", senderEmail).WithField("message_id", messageID)

	user, err := p.userStore.FindByEmail(ctx, senderEmail)
	if err != nil {
		logger.WithError(err).Warn("User not found, ignoring email")
		return err
	}

	existing, _ := p.repo.GetByMessageID(ctx, messageID)
	if existing != nil {
		logger.Info("Email already processed, skipping")
		return nil
	}

	logger.WithField("user_id", user.ID).Info("User matched, parsing email")

	parsedData, err := p.emailParser.Parse(rawBody)
	if err != nil {
		logger.WithError(err).Error("Failed to parse email")
		return err
	}

	p.logger.Info("Processing transaction: " + parsedData.Merchant)

	categoryID, err := p.resolveCategory(ctx, user.ID)
	if err != nil {
		logger.WithError(err).Warn("Failed to resolve category")
	}

	txDate := date
	if parsedData.TransactionDate != nil {
		txDate = *parsedData.TransactionDate
	}

	transaction := &Transaction{
		UserID:          user.ID,
		CategoryID:      categoryID,
		Amount:          parsedData.Amount,
		Merchant:        parsedData.Merchant,
		TransactionDate: txDate,
		MessageID:       &messageID,
		IsRecurring:     false,
	}

	if err := p.repo.Create(ctx, transaction); err != nil {
		logger.WithError(err).Error("Failed to save transaction")
		return err
	}

	if err := p.cacheManager.InvalidateUserCache(ctx, user.ID.String()); err != nil {
		logger.WithError(err).Warn("Failed to invalidate cache")
	}

	logger.Info("Transaction saved successfully")
	return nil
}

func (p *processor) resolveCategory(ctx context.Context, userID uuid.UUID) (*uuid.UUID, error) {
	cat, err := p.category.GetByName(ctx, p.defaultCategory, userID)
	if err != nil {
		return nil, err
	}
	return &cat.ID, nil
}

// ─── Service ─────────────────────────────────────────────────────────────────

func (s *service) ProcessTransactionEmail(ctx context.Context, senderEmail string, messageID string, rawBody string, date time.Time) error {
	return s.processor.ProcessTransactionEmail(ctx, senderEmail, messageID, rawBody, date)
}

func (s *service) GetUserTransactions(ctx context.Context, userID uuid.UUID, limit, offset int, filters TransactionFilters, sort SortOption) ([]Transaction, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	if filters.LastUpdated == "" {
		filters.ExcludeDeleted = true
	}

	transactions, err := s.repo.ListByUserID(ctx, userID, limit, offset, filters, sort)
	if err != nil {
		s.logger.WithError(err).WithField("user_id", userID).Error("Failed to get user transactions")
		return nil, err
	}

	return transactions, nil
}

func (s *service) GetUserTransactionsWithCount(ctx context.Context, userID uuid.UUID, limit, offset int, filters TransactionFilters, sort SortOption) ([]Transaction, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	if filters.LastUpdated == "" {
		filters.ExcludeDeleted = true
	}

	transactions, err := s.repo.ListByUserID(ctx, userID, limit, offset, filters, sort)
	if err != nil {
		s.logger.WithError(err).WithField("user_id", userID).Error("Failed to get user transactions")
		return nil, 0, err
	}

	totalCount, err := s.repo.CountByUserID(ctx, userID, filters)
	if err != nil {
		s.logger.WithError(err).WithField("user_id", userID).Error("Failed to count transactions")
		return nil, 0, err
	}

	return transactions, int(totalCount), nil
}

func (s *service) CreateManualTransaction(ctx context.Context, userID uuid.UUID, input CreateTransactionInput) (*Transaction, error) {
	if input.Amount <= 0 {
		return nil, errors.New("amount must be greater than 0")
	}
	if input.Merchant == "" {
		return nil, errors.New("merchant cannot be empty")
	}

	transaction := &Transaction{
		UserID:          userID,
		CategoryID:      input.CategoryID,
		Amount:          input.Amount,
		Merchant:        input.Merchant,
		Note:            input.Note,
		TransactionDate: input.TransactionDate,
		MessageID:       nil,
		IsRecurring:     false,
	}

	if err := s.repo.Create(ctx, transaction); err != nil {
		s.logger.WithError(err).Error("Failed to save manual transaction")
		return nil, err
	}

	if err := s.cacheManager.InvalidateUserCache(ctx, userID.String()); err != nil {
		s.logger.WithError(err).Warn("Failed to invalidate dashboard cache")
	}

	s.logger.WithField("user_id", userID).Info("Manual transaction saved successfully")
	return transaction, nil
}

func (s *service) SearchTransactions(ctx context.Context, userID uuid.UUID, query string, limit, offset int) ([]Transaction, error) {
	return s.repo.SearchByMerchant(ctx, userID, query, limit, offset)
}

func (s *service) SearchTransactionsCount(ctx context.Context, userID uuid.UUID, query string) (int, error) {
	count, err := s.repo.CountSearchByMerchant(ctx, userID, query)
	return int(count), err
}

func (s *service) GetReportsByCategory(ctx context.Context, userID uuid.UUID, month, year int) ([]CategoryExpense, error) {
	rawStats, err := s.repo.GetExpensesByCategory(ctx, userID, month, year)
	if err != nil {
		return nil, err
	}

	var grandTotal float64
	for _, stat := range rawStats {
		grandTotal += stat.TotalAmount
	}

	reports := make([]CategoryExpense, 0, len(rawStats))
	for _, stat := range rawStats {
		pct := 0.0
		if grandTotal > 0 {
			pct = (stat.TotalAmount / grandTotal) * 100
		}

		reports = append(reports, CategoryExpense{
			CategoryName: stat.CategoryName,
			TotalAmount:  stat.TotalAmount,
			Percentage:   pct,
		})
	}
	return reports, nil
}

func (s *service) GetAllTransactionsForExport(ctx context.Context, userID uuid.UUID, month, year int) ([]Transaction, error) {
	return s.repo.GetForExport(ctx, userID, month, year)
}

func (s *service) UpdateTransaction(ctx context.Context, userID, txID uuid.UUID, input UpdateTransactionInput) (*Transaction, error) {
	tx, err := s.repo.GetByID(ctx, txID)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return nil, apperrors.ErrNotFound
		}
		return nil, err
	}

	if tx.UserID != userID {
		return nil, apperrors.ErrUnauthorized
	}

	if input.Amount != nil {
		if *input.Amount <= 0 {
			return nil, errors.New("amount must be greater than 0")
		}
		tx.Amount = *input.Amount
	}

	if input.Merchant != nil {
		if *input.Merchant == "" {
			return nil, errors.New("merchant cannot be empty")
		}
		tx.Merchant = *input.Merchant
	}

	if input.Note != nil {
		tx.Note = *input.Note
	}

	if input.CategoryID != nil {
		tx.CategoryID = input.CategoryID
	}

	if input.TransactionDate != nil {
		tx.TransactionDate = *input.TransactionDate
	}

	if err := s.repo.Update(ctx, tx); err != nil {
		s.logger.WithError(err).Error("Failed to update transaction")
		return nil, err
	}

	if err := s.cacheManager.InvalidateUserCache(ctx, userID.String()); err != nil {
		s.logger.WithError(err).Warn("Failed to invalidate dashboard cache")
	}

	return tx, nil
}

func (s *service) DeleteTransaction(ctx context.Context, userID, txID uuid.UUID) error {
	tx, err := s.repo.GetByID(ctx, txID)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return apperrors.ErrNotFound
		}
		return err
	}

	if tx.UserID != userID {
		return apperrors.ErrUnauthorized
	}

	if err := s.repo.Delete(ctx, txID); err != nil {
		s.logger.WithError(err).Error("Failed to delete transaction")
		return err
	}

	if err := s.cacheManager.InvalidateUserCache(ctx, userID.String()); err != nil {
		s.logger.WithError(err).Warn("Failed to invalidate dashboard cache")
	}

	return nil
}
