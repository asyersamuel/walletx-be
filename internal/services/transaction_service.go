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

// TransactionService defines the contract for transaction-related business logic
type TransactionService interface {
	// ProcessTransactionEmail is called by the IMAP worker to handle incoming emails
	ProcessTransactionEmail(emailSender string, messageID string, rawBody string, date time.Time) error
	
	GetUserTransactions(userID uuid.UUID, limit, offset int) ([]models.Transaction, error)
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

	// Create the Transaction object
	transaction := &models.Transaction{
		UserID:          user.ID,
		Amount:          parsedData.Amount,     
		Merchant:        parsedData.Merchant,  
		TransactionDate: date,
		MessageID:       messageID, 
	}

	// Save to Database
	err = s.transactionRepo.Create(transaction)
	if err != nil {
		// If the error is a duplicate key (meaning the email was already processed), can ignore it
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

// GetUserTransactions user's list transaction
func (s *transactionService) GetUserTransactions(userID uuid.UUID, limit, offset int) ([]models.Transaction, error) {
	// Pagination
	if limit <= 0 || limit > 100 {
		limit = 20 
	}
	if offset < 0 {
		offset = 0
	}

	// Call Repository
	transactions, err := s.transactionRepo.ListByUserID(userID, limit, offset)
	if err != nil {
		logrus.WithError(err).WithField("user_id", userID).Error("❌ [Service] Failed to get user transactions")
		return nil, err
	}

	return transactions, nil
}