package service

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"walletx-be/internal/modules/ai"
	domain "walletx-be/internal/modules/transactions/contracts"
	apperrors "walletx-be/internal/shared/errors"
)

type receiptExtractorFunc func(context.Context, ai.ExtractionInput) (ai.ExtractedTransaction, error)

func (f receiptExtractorFunc) ExtractTransaction(ctx context.Context, input ai.ExtractionInput) (ai.ExtractedTransaction, error) {
	return f(ctx, input)
}

type receiptCategoriesFunc func(context.Context, uuid.UUID) ([]ai.CategoryOption, error)

func (f receiptCategoriesFunc) ListByUser(ctx context.Context, id uuid.UUID) ([]ai.CategoryOption, error) {
	return f(ctx, id)
}

type receiptCreator struct {
	domain.TransactionService
	create func(context.Context, uuid.UUID, domain.CreateTransactionInput) (*domain.Transaction, error)
}

func (c receiptCreator) CreateTransaction(ctx context.Context, id uuid.UUID, input domain.CreateTransactionInput) (*domain.Transaction, error) {
	return c.create(ctx, id, input)
}

func TestReceiptService_ProcessReceipt(t *testing.T) {
	location, err := time.LoadLocation("Asia/Jakarta")
	require.NoError(t, err)
	now := time.Date(2026, 10, 4, 5, 30, 0, 0, time.UTC)
	userID, categoryID := uuid.New(), uuid.New()
	notes := "Coffee"
	extractionErr, categoryErr, creationErr := errors.New("extraction failed"), errors.New("categories failed"), errors.New("creation failed")

	for _, tc := range []struct {
		name            string
		extractionErr   error
		categoryErr     error
		creationErr     error
		mutate          func(*ai.ExtractedTransaction)
		missingCategory bool
		cancelAt        string
		nilTransaction  bool
		wantErr         error
		wantExtract     int
		wantCategories  int
		wantCreate      int
	}{
		{name: "happy path", wantExtract: 1, wantCategories: 1, wantCreate: 1},
		{name: "extraction failure", extractionErr: extractionErr, wantErr: extractionErr, wantExtract: 1},
		{name: "category failure", categoryErr: categoryErr, wantErr: categoryErr, wantExtract: 1, wantCategories: 1},
		{name: "creation failure", creationErr: creationErr, wantErr: creationErr, wantExtract: 1, wantCategories: 1, wantCreate: 1},
		{name: "cancelled before extraction", cancelAt: "before", wantErr: context.Canceled},
		{name: "cancelled during extraction", cancelAt: "extraction", wantErr: context.Canceled, wantExtract: 1},
		{name: "cancelled during category lookup", cancelAt: "categories", wantErr: context.Canceled, wantExtract: 1, wantCategories: 1},
		{name: "foreign category", missingCategory: true, wantErr: apperrors.ErrNotFound, wantExtract: 1, wantCategories: 1},
		{name: "invalid date", mutate: func(x *ai.ExtractedTransaction) { x.Date = "2026-02-30" }, wantErr: apperrors.ErrInvalidInput, wantExtract: 1},
		{name: "zero amount", mutate: func(x *ai.ExtractedTransaction) { x.Amount = 0 }, wantErr: apperrors.ErrInvalidInput, wantExtract: 1},
		{name: "nonfinite amount", mutate: func(x *ai.ExtractedTransaction) { x.Amount = math.Inf(1) }, wantErr: apperrors.ErrInvalidInput, wantExtract: 1},
		{name: "empty merchant", mutate: func(x *ai.ExtractedTransaction) { x.Merchant = "  " }, wantErr: apperrors.ErrInvalidInput, wantExtract: 1},
		{name: "nil saved transaction", nilTransaction: true, wantErr: apperrors.ErrInternal, wantExtract: 1, wantCategories: 1, wantCreate: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancelAt == "before" {
				cancel()
			}
			extractCalls, categoryCalls, createCalls := 0, 0, 0
			extracted := ai.ExtractedTransaction{Amount: 25000, Merchant: "Coffee shop", CategoryID: categoryID, Notes: &notes, Date: "2026-10-04"}
			if tc.mutate != nil {
				tc.mutate(&extracted)
			}
			image := []byte{1, 2, 3}
			saved := &domain.Transaction{ID: uuid.New(), UserID: userID}
			extractor := receiptExtractorFunc(func(gotCtx context.Context, input ai.ExtractionInput) (ai.ExtractedTransaction, error) {
				extractCalls++
				require.Equal(t, ctx, gotCtx)
				require.Equal(t, ai.ExtractionInput{UserID: userID, Image: image, MIMEType: "image/png", ReferenceTime: now.In(location), TimeZone: "Asia/Jakarta"}, input)
				if tc.cancelAt == "extraction" {
					cancel()
				}
				return extracted, tc.extractionErr
			})
			categories := receiptCategoriesFunc(func(gotCtx context.Context, id uuid.UUID) ([]ai.CategoryOption, error) {
				categoryCalls++
				require.Equal(t, ctx, gotCtx)
				require.Equal(t, userID, id)
				if tc.cancelAt == "categories" {
					cancel()
				}
				if tc.missingCategory {
					return []ai.CategoryOption{{ID: uuid.New(), Name: "Other"}}, nil
				}
				return []ai.CategoryOption{{ID: categoryID, Name: "Food", Type: "expense"}}, tc.categoryErr
			})
			creator := receiptCreator{create: func(gotCtx context.Context, id uuid.UUID, input domain.CreateTransactionInput) (*domain.Transaction, error) {
				createCalls++
				require.Equal(t, ctx, gotCtx)
				require.Equal(t, userID, id)
				require.Equal(t, domain.CreateTransactionInput{UserID: userID, CategoryID: categoryID, CategoryName: "Food", MerchantName: "Coffee shop", Amount: 25000, TransactionDate: time.Date(2026, 10, 4, 0, 0, 0, 0, location), Notes: &notes}, input)
				require.Equal(t, "2026-10-03T17:00:00Z", input.TransactionDate.UTC().Format(time.RFC3339))
				if tc.nilTransaction {
					return nil, nil
				}
				return saved, tc.creationErr
			}}
			svc, err := NewReceiptService(extractor, categories, creator, location, func() time.Time { return now })
			require.NoError(t, err)
			got, err := svc.ProcessReceipt(ctx, userID, "image/png", "receipt.png", image)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				require.Equal(t, domain.AgentResponse{}, got)
			} else {
				require.NoError(t, err)
				require.Same(t, saved, got.Transaction)
				require.NotEmpty(t, got.Text)
			}
			require.Equal(t, tc.wantExtract, extractCalls)
			require.Equal(t, tc.wantCategories, categoryCalls)
			require.Equal(t, tc.wantCreate, createCalls)
		})
	}
}
