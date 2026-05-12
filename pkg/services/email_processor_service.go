package services

import (
	"context"

	"walletx-be/internal/domain/dto"
	"walletx-be/internal/domain/parser"
	"walletx-be/internal/ports"
	"walletx-be/pkg/models"

	"github.com/google/uuid"
)

type EmailProcessorService interface {
	ProcessEmailPayload(ctx context.Context, payload dto.EmailProcessingPayload) error
}

type emailProcessorService struct {
	userRepo         UserRepository
	transactionRepo  TransactionRepository
	categoryRepo     CategoryRepository
	emailParser      parser.EmailParser
	cacheManager     ports.DashboardCacheManager
	logger           ports.Logger
	defaultCategory  string
}

func NewEmailProcessorService(
	userRepo UserRepository,
	transactionRepo TransactionRepository,
	categoryRepo CategoryRepository,
	emailParser parser.EmailParser,
	cacheManager ports.DashboardCacheManager,
	logger ports.Logger,
) EmailProcessorService {
	return &emailProcessorService{
		userRepo:        userRepo,
		transactionRepo: transactionRepo,
		categoryRepo:    categoryRepo,
		emailParser:     emailParser,
		cacheManager:    cacheManager,
		logger:          logger,
		defaultCategory: "Lainnya",
	}
}

func (s *emailProcessorService) ProcessEmailPayload(ctx context.Context, payload dto.EmailProcessingPayload) error {
	logger := s.logger.WithField("sender", payload.Sender).WithField("message_id", payload.MessageID)

	user, err := s.userRepo.FindByEmail(ctx, payload.Sender)
	if err != nil {
		logger.WithError(err).Warn("User not found, dropping message")
		return nil
	}

	existing, _ := s.transactionRepo.GetByMessageID(ctx, payload.MessageID)
	if existing != nil {
		logger.Info("Email already processed, dropping message")
		return nil
	}

	logger.WithField("user_id", user.ID).Info("User matched, parsing email")

	parsedData, err := s.emailParser.Parse(payload.Body)
	if err != nil {
		logger.WithError(err).Error("Failed to parse email")
		return err
	}

	s.logger.Info("Processing transaction: " + parsedData.Merchant)

	categoryID, err := s.resolveCategory(ctx, user.ID)
	if err != nil {
		logger.WithError(err).Warn("Failed to resolve category")
	}

	txDate := payload.Date
	if parsedData.TransactionDate != nil {
		txDate = *parsedData.TransactionDate
	}

	transaction := &models.Transaction{
		UserID:          user.ID,
		CategoryID:      categoryID,
		Amount:          parsedData.Amount,
		Merchant:        parsedData.Merchant,
		TransactionDate: txDate,
		MessageID:       &payload.MessageID,
		IsRecurring:     false,
	}

	if err := s.transactionRepo.Create(ctx, transaction); err != nil {
		logger.WithError(err).Error("Failed to save transaction")
		return err
	}

	if err := s.cacheManager.InvalidateUserCache(ctx, user.ID.String()); err != nil {
		logger.WithError(err).Warn("Failed to invalidate cache")
	}

	logger.Info("Transaction saved successfully")
	return nil
}

func (s *emailProcessorService) resolveCategory(ctx context.Context, userID uuid.UUID) (*uuid.UUID, error) {
	category, err := s.categoryRepo.GetByName(ctx, s.defaultCategory, userID)
	if err != nil {
		return nil, err
	}
	return &category.ID, nil
}
