package services

import (
	"context"
	"errors"

	"walletx-be/internal/domain"
	"walletx-be/internal/domain/dto"
	"walletx-be/internal/domain/query"
	"walletx-be/internal/domain/report"
	"walletx-be/internal/ports"
	"walletx-be/pkg/models"

	"github.com/google/uuid"
)

type TransactionFilters = query.TransactionFilters
type SortOption = query.SortOption
type CategoryExpenseDTO = report.CategoryExpense
type CreateTransactionInput = dto.CreateTransactionInput
type UpdateTransactionInput = dto.UpdateTransactionInput

type TransactionService interface {
	CreateManualTransaction(ctx context.Context, userID uuid.UUID, input CreateTransactionInput) (*models.Transaction, error)
	UpdateTransaction(ctx context.Context, userID, txID uuid.UUID, input UpdateTransactionInput) (*models.Transaction, error)
	DeleteTransaction(ctx context.Context, userID, txID uuid.UUID) error
	GetUserTransactions(ctx context.Context, userID uuid.UUID, limit, offset int, filters TransactionFilters, sort SortOption) ([]models.Transaction, error)
	GetUserTransactionsWithCount(ctx context.Context, userID uuid.UUID, limit, offset int, filters TransactionFilters, sort SortOption) ([]models.Transaction, int, error)
	SearchTransactions(ctx context.Context, userID uuid.UUID, query string, limit, offset int) ([]models.Transaction, error)
	SearchTransactionsCount(ctx context.Context, userID uuid.UUID, query string) (int, error)
	GetReportsByCategory(ctx context.Context, userID uuid.UUID, month, year int) ([]CategoryExpenseDTO, error)
	GetAllTransactionsForExport(ctx context.Context, userID uuid.UUID, month, year int) ([]models.Transaction, error)
}

type transactionService struct {
	userRepo        UserRepository
	transactionRepo TransactionRepository
	categoryRepo    CategoryRepository
	cacheManager    ports.DashboardCacheManager
	logger          ports.Logger
}

type UserRepository interface {
	FindByEmail(ctx context.Context, email string) (*models.User, error)
}

type TransactionRepository interface {
	Create(ctx context.Context, transaction *models.Transaction) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.Transaction, error)
	GetByMessageID(ctx context.Context, messageID string) (*models.Transaction, error)
	Update(ctx context.Context, transaction *models.Transaction) error
	Delete(ctx context.Context, id uuid.UUID) error
	ListByUserID(ctx context.Context, userID uuid.UUID, limit, offset int, filters TransactionFilters, sort SortOption) ([]models.Transaction, error)
	CountByUserID(ctx context.Context, userID uuid.UUID, filters TransactionFilters) (int64, error)
	SearchByMerchant(ctx context.Context, userID uuid.UUID, query string, limit, offset int) ([]models.Transaction, error)
	CountSearchByMerchant(ctx context.Context, userID uuid.UUID, query string) (int64, error)
	GetExpensesByCategory(ctx context.Context, userID uuid.UUID, month, year int) ([]report.CategoryExpense, error)
	GetForExport(ctx context.Context, userID uuid.UUID, month, year int) ([]models.Transaction, error)
}

type CategoryRepository interface {
	GetByName(ctx context.Context, name string, userID uuid.UUID) (*models.Category, error)
}

func NewTransactionService(
	userRepo UserRepository,
	transactionRepo TransactionRepository,
	categoryRepo CategoryRepository,
	cacheManager ports.DashboardCacheManager,
	logger ports.Logger,
) TransactionService {
	return &transactionService{
		userRepo:        userRepo,
		transactionRepo: transactionRepo,
		categoryRepo:    categoryRepo,
		cacheManager:    cacheManager,
		logger:          logger,
	}
}

func (s *transactionService) GetUserTransactions(ctx context.Context, userID uuid.UUID, limit, offset int, filters TransactionFilters, sort SortOption) ([]models.Transaction, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	if filters.LastUpdated == "" {
		filters.ExcludeDeleted = true
	}

	transactions, err := s.transactionRepo.ListByUserID(ctx, userID, limit, offset, filters, sort)
	if err != nil {
		s.logger.WithError(err).WithField("user_id", userID).Error("Failed to get user transactions")
		return nil, err
	}

	return transactions, nil
}

func (s *transactionService) GetUserTransactionsWithCount(ctx context.Context, userID uuid.UUID, limit, offset int, filters TransactionFilters, sort SortOption) ([]models.Transaction, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	if filters.LastUpdated == "" {
		filters.ExcludeDeleted = true
	}

	transactions, err := s.transactionRepo.ListByUserID(ctx, userID, limit, offset, filters, sort)
	if err != nil {
		s.logger.WithError(err).WithField("user_id", userID).Error("Failed to get user transactions")
		return nil, 0, err
	}

	totalCount, err := s.transactionRepo.CountByUserID(ctx, userID, filters)
	if err != nil {
		s.logger.WithError(err).WithField("user_id", userID).Error("Failed to count transactions")
		return nil, 0, err
	}

	return transactions, int(totalCount), nil
}

func (s *transactionService) CreateManualTransaction(ctx context.Context, userID uuid.UUID, input CreateTransactionInput) (*models.Transaction, error) {
	if input.Amount <= 0 {
		return nil, errors.New("amount must be greater than 0")
	}
	if input.Merchant == "" {
		return nil, errors.New("merchant cannot be empty")
	}

	transaction := &models.Transaction{
		UserID:          userID,
		CategoryID:      input.CategoryID,
		Amount:          input.Amount,
		Merchant:        input.Merchant,
		Note:            input.Note,
		TransactionDate: input.TransactionDate,
		MessageID:       nil,
		IsRecurring:     false,
	}

	if err := s.transactionRepo.Create(ctx, transaction); err != nil {
		s.logger.WithError(err).Error("Failed to save manual transaction")
		return nil, err
	}

	if err := s.cacheManager.InvalidateUserCache(ctx, userID.String()); err != nil {
		s.logger.WithError(err).Warn("Failed to invalidate dashboard cache")
	}

	s.logger.WithField("user_id", userID).Info("Manual transaction saved successfully")
	return transaction, nil
}

func (s *transactionService) SearchTransactions(ctx context.Context, userID uuid.UUID, query string, limit, offset int) ([]models.Transaction, error) {
	return s.transactionRepo.SearchByMerchant(ctx, userID, query, limit, offset)
}

func (s *transactionService) SearchTransactionsCount(ctx context.Context, userID uuid.UUID, query string) (int, error) {
	count, err := s.transactionRepo.CountSearchByMerchant(ctx, userID, query)
	return int(count), err
}

func (s *transactionService) GetReportsByCategory(ctx context.Context, userID uuid.UUID, month, year int) ([]CategoryExpenseDTO, error) {
	rawStats, err := s.transactionRepo.GetExpensesByCategory(ctx, userID, month, year)
	if err != nil {
		return nil, err
	}

	var grandTotal float64
	for _, stat := range rawStats {
		grandTotal += stat.TotalAmount
	}

	reports := make([]CategoryExpenseDTO, 0, len(rawStats))
	for _, stat := range rawStats {
		pct := 0.0
		if grandTotal > 0 {
			pct = (stat.TotalAmount / grandTotal) * 100
		}

		reports = append(reports, CategoryExpenseDTO{
			CategoryName: stat.CategoryName,
			TotalAmount:  stat.TotalAmount,
			Percentage:   pct,
		})
	}
	return reports, nil
}

func (s *transactionService) GetAllTransactionsForExport(ctx context.Context, userID uuid.UUID, month, year int) ([]models.Transaction, error) {
	return s.transactionRepo.GetForExport(ctx, userID, month, year)
}

func (s *transactionService) UpdateTransaction(ctx context.Context, userID, txID uuid.UUID, input UpdateTransactionInput) (*models.Transaction, error) {
	tx, err := s.transactionRepo.GetByID(ctx, txID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}

	if tx.UserID != userID {
		return nil, domain.ErrUnauthorized
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

	if err := s.transactionRepo.Update(ctx, tx); err != nil {
		s.logger.WithError(err).Error("Failed to update transaction")
		return nil, err
	}

	if err := s.cacheManager.InvalidateUserCache(ctx, userID.String()); err != nil {
		s.logger.WithError(err).Warn("Failed to invalidate dashboard cache")
	}

	return tx, nil
}

func (s *transactionService) DeleteTransaction(ctx context.Context, userID, txID uuid.UUID) error {
	tx, err := s.transactionRepo.GetByID(ctx, txID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrNotFound
		}
		return err
	}

	if tx.UserID != userID {
		return domain.ErrUnauthorized
	}

	if err := s.transactionRepo.Delete(ctx, txID); err != nil {
		s.logger.WithError(err).Error("Failed to delete transaction")
		return err
	}

	if err := s.cacheManager.InvalidateUserCache(ctx, userID.String()); err != nil {
		s.logger.WithError(err).Warn("Failed to invalidate dashboard cache")
	}

	return nil
}
