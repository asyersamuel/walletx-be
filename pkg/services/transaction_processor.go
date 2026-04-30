package services

import (
	"context"
	"time"

	"walletx-be/internal/domain/parser"
	"walletx-be/internal/ports"
	"walletx-be/pkg/models"

	"github.com/google/uuid"
)

type TransactionProcessor interface {
	ProcessTransactionEmail(ctx context.Context, senderEmail, messageID, rawBody string, date time.Time) error
}

type transactionProcessor struct {
	userRepo         UserRepository
	transactionRepo  TransactionRepository
	categoryRepo     CategoryRepository
	emailParser      parser.EmailParser
	cacheManager     ports.DashboardCacheManager
	logger           ports.Logger
	defaultCategory  string
}

func NewTransactionProcessor(
	userRepo UserRepository,
	transactionRepo TransactionRepository,
	categoryRepo CategoryRepository,
	emailParser parser.EmailParser,
	cacheManager ports.DashboardCacheManager,
	logger ports.Logger,
) TransactionProcessor {
	return &transactionProcessor{
		userRepo:         userRepo,
		transactionRepo:  transactionRepo,
		categoryRepo:     categoryRepo,
		emailParser:      emailParser,
		cacheManager:     cacheManager,
		logger:           logger,
		defaultCategory:  "Lainnya",
	}
}

func (p *transactionProcessor) ProcessTransactionEmail(ctx context.Context, senderEmail, messageID, rawBody string, date time.Time) error {
	logger := p.logger.WithField("sender", senderEmail).WithField("message_id", messageID)

	user, err := p.userRepo.FindByEmail(ctx, senderEmail)
	if err != nil {
		logger.WithError(err).Warn("User not found, ignoring email")
		return err
	}

	existing, _ := p.transactionRepo.GetByMessageID(ctx, messageID)
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

	transaction := &models.Transaction{
		UserID:          user.ID,
		CategoryID:      categoryID,
		Amount:          parsedData.Amount,
		Merchant:        parsedData.Merchant,
		TransactionDate: txDate,
		MessageID:       &messageID,
		IsRecurring:     false,
	}

	if err := p.transactionRepo.Create(ctx, transaction); err != nil {
		logger.WithError(err).Error("Failed to save transaction")
		return err
	}

	if err := p.cacheManager.InvalidateUserCache(ctx, user.ID.String()); err != nil {
		logger.WithError(err).Warn("Failed to invalidate cache")
	}

	logger.Info("Transaction saved successfully")
	return nil
}

func (p *transactionProcessor) resolveCategory(ctx context.Context, userID uuid.UUID) (*uuid.UUID, error) {
	category, err := p.categoryRepo.GetByName(ctx, p.defaultCategory, userID)
	if err != nil {
		return nil, err
	}
	return &category.ID, nil
}
