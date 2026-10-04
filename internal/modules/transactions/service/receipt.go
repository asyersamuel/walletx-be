package service

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"walletx-be/internal/modules/ai"
	domain "walletx-be/internal/modules/transactions/contracts"
	apperrors "walletx-be/internal/shared/errors"
)

// ReceiptExtractor is satisfied by ai.AIService without depending on HTTP delivery.
type ReceiptExtractor interface {
	ExtractTransaction(context.Context, ai.ExtractionInput) (ai.ExtractedTransaction, error)
}

type receiptService struct {
	extractor    ReceiptExtractor
	categories   ai.CategoryReader
	transactions domain.TransactionService
	location     *time.Location
	now          func() time.Time
}

var _ domain.ReceiptService = (*receiptService)(nil)

// NewReceiptService requires an explicit timezone and clock; time.Now can be
// supplied by the composition root and a fixed clock by tests.
func NewReceiptService(extractor ReceiptExtractor, categories ai.CategoryReader, transactions domain.TransactionService, location *time.Location, now func() time.Time) (domain.ReceiptService, error) {
	if extractor == nil || categories == nil || transactions == nil || location == nil || now == nil || location == time.Local {
		return nil, fmt.Errorf("receipt dependencies, explicit timezone and clock are required: %w", apperrors.ErrInvalidInput)
	}
	return &receiptService{extractor: extractor, categories: categories, transactions: transactions, location: location, now: now}, nil
}

func (s *receiptService) ProcessReceipt(ctx context.Context, userID uuid.UUID, contentType, filename string, image []byte) (domain.AgentResponse, error) {
	if ctx == nil {
		return domain.AgentResponse{}, fmt.Errorf("receipt context is required: %w", apperrors.ErrInvalidInput)
	}
	if err := ctx.Err(); err != nil {
		return domain.AgentResponse{}, err
	}
	if userID == uuid.Nil || len(image) == 0 || strings.TrimSpace(contentType) == "" {
		return domain.AgentResponse{}, fmt.Errorf("receipt user, image and MIME type are required: %w", apperrors.ErrInvalidInput)
	}
	extracted, err := s.extractor.ExtractTransaction(ctx, ai.ExtractionInput{
		UserID: userID, Image: image, MIMEType: contentType,
		ReferenceTime: s.now().In(s.location), TimeZone: s.location.String(),
	})
	if err != nil {
		return domain.AgentResponse{}, fmt.Errorf("extract receipt transaction: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return domain.AgentResponse{}, err
	}
	date, err := time.ParseInLocation(time.DateOnly, extracted.Date, s.location)
	if err != nil || extracted.CategoryID == uuid.Nil || strings.TrimSpace(extracted.Merchant) == "" || extracted.Amount <= 0 || math.IsNaN(extracted.Amount) || math.IsInf(extracted.Amount, 0) {
		return domain.AgentResponse{}, fmt.Errorf("invalid extracted receipt transaction: %w", apperrors.ErrInvalidInput)
	}
	categories, err := s.categories.ListByUser(ctx, userID)
	if err != nil {
		return domain.AgentResponse{}, fmt.Errorf("load receipt categories: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return domain.AgentResponse{}, err
	}
	categoryName := ""
	for _, category := range categories {
		if category.ID == extracted.CategoryID {
			categoryName = category.Name
			break
		}
	}
	if strings.TrimSpace(categoryName) == "" {
		return domain.AgentResponse{}, fmt.Errorf("receipt category is not available for user: %w", apperrors.ErrNotFound)
	}
	saved, err := s.transactions.CreateTransaction(ctx, userID, domain.CreateTransactionInput{
		UserID: userID, CategoryID: extracted.CategoryID, CategoryName: categoryName,
		MerchantName: strings.TrimSpace(extracted.Merchant), Amount: extracted.Amount,
		TransactionDate: date, Notes: extracted.Notes,
	})
	if err != nil {
		return domain.AgentResponse{}, fmt.Errorf("create receipt transaction: %w", err)
	}
	if saved == nil {
		return domain.AgentResponse{}, fmt.Errorf("receipt creation returned no transaction: %w", apperrors.ErrInternal)
	}
	// filename is transport metadata, not a persisted image URL.
	return domain.AgentResponse{Text: "Transaction recorded.", Transaction: saved}, nil
}
