package services

import (
	"errors"
	"time"

	"walletx-be/internal/models"
	"walletx-be/internal/repository"

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

// TransactionService defines the contract for transaction-related business logic
type TransactionService interface {
	// ProcessTransactionEmail is called by the IMAP worker to handle incoming emails
	ProcessTransactionEmail(emailSender string, messageID string, rawBody string, date time.Time) error

	GetUserTransactions(userID uuid.UUID, limit, offset int) ([]models.Transaction, error)

	// CreateManualTransaction allows the mobile app to create a transaction directly
	CreateManualTransaction(userID uuid.UUID, input CreateTransactionInput) (*models.Transaction, error)
}

type transactionService struct {
	userRepo        repository.UserRepository
	transactionRepo repository.TransactionRepository
	parserService   ParserService
}

// NewTransactionService is the constructor
func NewTransactionService(userRepo repository.UserRepository, transactionRepo repository.TransactionRepository, parserService ParserService) TransactionService {
	return &transactionService{
		userRepo:        userRepo,
		transactionRepo: transactionRepo,
		parserService:   parserService,
	}
}

// ProcessTransactionEmail handles the core logic of matching emails to users and saving transactions
func (s *transactionService) ProcessTransactionEmail(emailSender string, messageID string, rawBody string, date time.Time) error {
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

	// Create the Transaction object — IMAP source defaults
	transaction := &models.Transaction{
		UserID:          user.ID,
		CategoryID:      nil,        // uncategorised by default
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

	logEntry.Info("🎉 [Service] Transaction successfully saved to database!")
	return nil
}

// GetUserTransactions returns the user's paginated transaction list
func (s *transactionService) GetUserTransactions(userID uuid.UUID, limit, offset int) ([]models.Transaction, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	transactions, err := s.transactionRepo.ListByUserID(userID, limit, offset)
	if err != nil {
		logrus.WithError(err).WithField("user_id", userID).Error("❌ [Service] Failed to get user transactions")
		return nil, err
	}

	return transactions, nil
}

// CreateManualTransaction creates a transaction from the mobile app without an email source
func (s *transactionService) CreateManualTransaction(userID uuid.UUID, input CreateTransactionInput) (*models.Transaction, error) {
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

	logrus.WithField("user_id", userID).Info("🎉 [Service] Manual transaction saved successfully")
	return transaction, nil
}