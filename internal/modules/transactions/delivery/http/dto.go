package http

import (
	"time"
	domain "walletx-be/internal/modules/transactions/contracts"

	"github.com/google/uuid"
)

type createTransactionRequest struct {
	UserID          *uuid.UUID `json:"user_id"`
	CategoryID      uuid.UUID  `json:"category_id"`
	CategoryName    string     `json:"category_name"`
	MerchantName    string     `json:"merchant_name"`
	Amount          float64    `json:"amount"`
	TransactionDate string     `json:"transaction_date"`
	ReceiptImageURL *string    `json:"receipt_image_url"`
	Notes           *string    `json:"notes"`
}

type updateTransactionRequest struct {
	MerchantName    *string    `json:"merchant_name"`
	Amount          *float64   `json:"amount"`
	TransactionDate *string    `json:"transaction_date"`
	CategoryID      *uuid.UUID `json:"category_id"`
	ReceiptImageURL *string    `json:"receipt_image_url"`
	Notes           *string    `json:"notes"`
	ClearReceiptURL bool       `json:"clear_receipt_url"`
	ClearNotes      bool       `json:"clear_notes"`
}

type chatRequest struct {
	Message string     `json:"message"`
	UserID  *uuid.UUID `json:"user_id"`
}

type transactionResponse struct {
	ID              uuid.UUID  `json:"id"`
	UserID          uuid.UUID  `json:"user_id"`
	CategoryID      *uuid.UUID `json:"category_id"`
	MerchantName    string     `json:"merchant_name"`
	Amount          float64    `json:"amount"`
	TransactionDate string     `json:"transaction_date"`
	ReceiptImageURL *string    `json:"receipt_image_url"`
	Notes           *string    `json:"notes"`
	CreatedAt       string     `json:"created_at"`
	UpdatedAt       string     `json:"updated_at"`
}

func agentResponseDTO(result domain.AgentResponse) interface{} {
	return struct {
		Text         string                      `json:"text"`
		Transaction  *transactionResponse        `json:"transaction,omitempty"`
		SimilarItems []domain.SimilarTransaction `json:"similar_items,omitempty"`
	}{Text: result.Text, Transaction: transactionResponsePtr(result.Transaction), SimilarItems: result.SimilarItems}
}

func transactionResponsePtr(transaction *domain.Transaction) *transactionResponse {
	if transaction == nil {
		return nil
	}
	result := transactionResponseFromDomain(transaction)
	return &result
}

func transactionResponseFromDomain(transaction *domain.Transaction) transactionResponse {
	return transactionResponse{
		ID:              transaction.ID,
		UserID:          transaction.UserID,
		CategoryID:      transaction.CategoryID,
		MerchantName:    transaction.MerchantName,
		Amount:          transaction.Amount,
		TransactionDate: transaction.TransactionDate.Format(time.RFC3339),
		ReceiptImageURL: transaction.ReceiptImageUrl,
		Notes:           transaction.Notes,
		CreatedAt:       transaction.CreatedAt.Format(time.RFC3339),
		UpdatedAt:       transaction.UpdatedAt.Format(time.RFC3339),
	}
}
