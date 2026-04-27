package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"walletx-be/pkg/models"
	"walletx-be/pkg/repository"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// CreateTransactionInput is the DTO for manually creating a transaction via the API
type CreateTransactionInput struct {
	Amount          float64    `json:"amount"           binding:"required,gt=0"`
	Merchant        string     `json:"merchant"         binding:"required"`
	Note            string     `json:"note"`
	CategoryID      *uuid.UUID `json:"category_id"`
	TransactionDate time.Time  `json:"transaction_date" binding:"required"`
}

type UpdateTransactionInput struct {
	Amount          float64    `json:"amount"          binding:"required,gt=0"`
	Merchant        string     `json:"merchant"        binding:"required"`
	Note            string     `json:"note"`
	CategoryID      *uuid.UUID `json:"category_id"`
	TransactionDate time.Time  `json:"transaction_date" binding:"required"`
}

// TransactionService defines the contract for transaction-related business logic
type TransactionService interface {
	// ProcessTransactionEmail is called by the IMAP worker to handle incoming emails
	ProcessTransactionEmail(ctx context.Context, emailSender string, messageID string, rawBody string, date time.Time) error

	// CreateManualTransaction allows the mobile app to create a transaction directly
	CreateManualTransaction(ctx context.Context, userID uuid.UUID, input CreateTransactionInput) (*models.Transaction, error)

	UpdateTransaction(ctx context.Context, userID, txID uuid.UUID, input UpdateTransactionInput) (*models.Transaction, error)
	DeleteTransaction(ctx context.Context, userID, txID uuid.UUID) error

	GetUserTransactions(ctx context.Context, userID uuid.UUID, limit, offset int, dateFilter, lastUpdated string) ([]models.Transaction, error)
	SearchTransactions(ctx context.Context, userID uuid.UUID, query string, limit, offset int) ([]models.Transaction, error)
	GetReportsByCategory(ctx context.Context, userID uuid.UUID, month, year int) ([]CategoryExpenseDTO, error)
	GetAllTransactionsForExport(ctx context.Context, userID uuid.UUID, month, year int) ([]models.Transaction, error) 
}

type transactionService struct {
	userRepo        repository.UserRepository
	transactionRepo repository.TransactionRepository
	categoryRepo    repository.CategoryRepository
	parserService   ParserService
	cacheRepo       repository.CacheRepository
}

type CategoryExpenseDTO struct {
    CategoryName string  `json:"category_name"`
    TotalAmount  float64 `json:"total_amount"`
    Percentage   float64 `json:"percentage"`
}

// NewTransactionService is the constructor
func NewTransactionService(
	userRepo repository.UserRepository, 
	transactionRepo repository.TransactionRepository, 
	categoryRepo repository.CategoryRepository, 
	parserService ParserService,
	cacheRepo repository.CacheRepository,
) TransactionService {
	return &transactionService{
		userRepo:        userRepo,
		transactionRepo: transactionRepo,
		categoryRepo:    categoryRepo,
		parserService:   parserService,
		cacheRepo:       cacheRepo,
	}
}

// invalidateDashboardCache removes cached dashboard data for a user
func (s *transactionService) invalidateDashboardCache(ctx context.Context, userID uuid.UUID) {
	// Delete budget summary cache
	cacheKey := fmt.Sprintf("cache:dashboard:budget_summary:%s", userID.String())
	_ = s.cacheRepo.DeleteCache(ctx, cacheKey)

	// Delete daily calendar cache (all months - we delete common patterns)
	// In production, consider using Redis SCAN for pattern-based deletion
	for _, month := range []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12} {
		for _, year := range []int{2024, 2025, 2026, 2027} {
			dailyKey := fmt.Sprintf("cache:dashboard:daily_total:%s:%d:%d", userID.String(), month, year)
			_ = s.cacheRepo.DeleteCache(ctx, dailyKey)
		}
	}
}

// ProcessTransactionEmail handles the core logic of matching emails to users and saving transactions
func (s *transactionService) ProcessTransactionEmail(ctx context.Context, emailSender string, messageID string, rawBody string, date time.Time) error {
	logEntry := logrus.WithFields(logrus.Fields{
		"sender":     emailSender,
		"message_id": messageID,
	})

	// Check if the sender's email belongs to a registered user
	user, err := s.userRepo.FindByEmail(emailSender)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			logEntry.Warn("⚠️ [Service] User not found for this email. Ignoring.")
			return nil
		}
		logEntry.WithError(err).Error("❌ [Service] Database error while finding user")
		return err
	}

	logEntry.WithField("user_id", user.ID).Info("✅ [Service] User matched. Parsing email body...")

	// DEBUG RAW BODY --> melihat bentuk dan struktur body untuk menyesuaikan parser
	logrus.Info("\n========== RAW EMAIL BODY ==========\n", rawBody, "\n====================================\n")

	parsedData, err := s.parserService.ParseTransactionEmail(rawBody)
	if err != nil {
		logEntry.WithError(err).Error("❌ [Service] Failed to parse email body. Transaction skipped.")
		return err
	}

	// MessageID pointer for the nullable DB column
	msgID := messageID

	// Tentukan kategori otomatis untuk email: "Lainnya" jika ada
	var categoryID *uuid.UUID
	category, err := s.categoryRepo.GetByName("Lainnya", user.ID)
	if err == nil && category != nil {
		categoryID = &category.ID
		logEntry.WithField("category", "Lainnya").Info("🏷️ [Service] Auto-categorized as 'Lainnya'")
	} else {
		logEntry.Info("❓ [Service] No 'Lainnya' category found, leaving uncategorised")
	}

	// Create the Transaction object — IMAP source defaults
	transaction := &models.Transaction{
		UserID:          user.ID,
		CategoryID:      categoryID, 
		Amount:          parsedData.Amount,
		Merchant:        parsedData.Merchant,
		Note:            "",
		TransactionDate: date,
		MessageID:       &msgID,
		IsRecurring:     false,
	}

	// Save to Database
	err = s.transactionRepo.Create(transaction)
	if err != nil {
		// If the error is a duplicate key (meaning the email was already processed), ignore it
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			logEntry.Warn("⚠️ [Service] Transaction already exists (Duplicate Message-ID). Ignoring.")
			return nil
		}
		logEntry.WithError(err).Error("❌ [Service] Failed to save transaction")
		return err
	}

	// Invalidate dashboard cache after successful transaction creation
	s.invalidateDashboardCache(ctx, user.ID)

	logEntry.Info("🎉 [Service] Transaction successfully saved to database!")
	return nil
}

// GetUserTransactions returns the user's paginated transaction list
func (s *transactionService) GetUserTransactions(ctx context.Context, userID uuid.UUID, limit, offset int, dateFilter, lastUpdated string) ([]models.Transaction, error) {
    if limit <= 0 || limit > 100 {
        limit = 20
    }
    if offset < 0 {
        offset = 0
    }

    transactions, err := s.transactionRepo.ListByUserID(userID, limit, offset, dateFilter, lastUpdated)
    if err != nil {
        logrus.WithError(err).WithField("user_id", userID).Error("❌ [Service] Failed to get user transactions")
        return nil, err
    }

    return transactions, nil
}

// CreateManualTransaction creates a transaction from the mobile app without an email source
func (s *transactionService) CreateManualTransaction(ctx context.Context, userID uuid.UUID, input CreateTransactionInput) (*models.Transaction, error) {
	if input.Amount <= 0 {
		return nil, errors.New("amount must be greater than 0")
	}
	if input.Merchant == "" {
		return nil, errors.New("merchant cannot be empty")
	}

	transaction := &models.Transaction{
		UserID:          userID,
		CategoryID:      input.CategoryID, // may be nil
		Amount:          input.Amount,
		Merchant:        input.Merchant,
		Note:            input.Note,
		TransactionDate: input.TransactionDate,
		MessageID:       nil,  // no email source
		IsRecurring:     false,
	}

	if err := s.transactionRepo.Create(transaction); err != nil {
		logrus.WithError(err).Error("❌ [Service] Failed to save manual transaction")
		return nil, err
	}

	// Invalidate dashboard cache after successful transaction creation
	s.invalidateDashboardCache(ctx, userID)

	logrus.WithField("user_id", userID).Info("🎉 [Service] Manual transaction saved successfully")
	return transaction, nil
}

func (s *transactionService) SearchTransactions(ctx context.Context, userID uuid.UUID, query string, limit, offset int) ([]models.Transaction, error) {
    return s.transactionRepo.SearchByMerchant(userID, query, limit, offset)
}

func (s *transactionService) GetReportsByCategory(ctx context.Context, userID uuid.UUID, month, year int) ([]CategoryExpenseDTO, error) {
    rawStats, err := s.transactionRepo.GetExpensesByCategory(userID, month, year)
    if err != nil { return nil, err }

    // Hitung grand total untuk cari persentase
    var grandTotal float64
    for _, stat := range rawStats {
        val, _ := stat["total_amount"].(float64)
        grandTotal += val
    }

    var reports []CategoryExpenseDTO
    for _, stat := range rawStats {
        catName, _ := stat["category_name"].(string)
        total, _ := stat["total_amount"].(float64)
        
        pct := 0.0
        if grandTotal > 0 {
            pct = (total / grandTotal) * 100
        }

        reports = append(reports, CategoryExpenseDTO{
            CategoryName: catName,
            TotalAmount:  total,
            Percentage:   pct,
        })
    }
    return reports, nil
}

func (s *transactionService) GetAllTransactionsForExport(ctx context.Context, userID uuid.UUID, month, year int) ([]models.Transaction, error) {
    return s.transactionRepo.GetForExport(userID, month, year)
}

// UpdateTransaction memodifikasi transaksi yang sudah ada
func (s *transactionService) UpdateTransaction(ctx context.Context, userID, txID uuid.UUID, input UpdateTransactionInput) (*models.Transaction, error) {
	// Transaksi berdasarkan ID
	tx, err := s.transactionRepo.GetByID(txID)
	if err != nil {
		return nil, errors.New("transaction not found")
	}

	// Verifikasi kepemilikan 
	if tx.UserID != userID {
		return nil, errors.New("unauthorized to update this transaction")
	}

	// Update field
	tx.Amount = input.Amount
	tx.Merchant = input.Merchant
	tx.Note = input.Note
	tx.CategoryID = input.CategoryID
	tx.TransactionDate = input.TransactionDate

	// Simpan ke database
	if err := s.transactionRepo.Update(tx); err != nil {
		logrus.WithError(err).Error("❌ [Service] Failed to update transaction")
		return nil, err
	}

	// Invalidate dashboard cache after successful update
	s.invalidateDashboardCache(ctx, userID)

	return tx, nil
}

// DeleteTransaction menghapus transaksi
func (s *transactionService) DeleteTransaction(ctx context.Context, userID, txID uuid.UUID) error {
	tx, err := s.transactionRepo.GetByID(txID)
	if err != nil {
		return errors.New("transaction not found")
	}

	if tx.UserID != userID {
		return errors.New("unauthorized to delete this transaction")
	}

	if err := s.transactionRepo.Delete(txID); err != nil {
		logrus.WithError(err).Error("❌ [Service] Failed to delete transaction")
		return err
	}

	// Invalidate dashboard cache after successful delete
	s.invalidateDashboardCache(ctx, userID)

	return nil
}