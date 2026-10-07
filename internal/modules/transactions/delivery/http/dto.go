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
	ID              uuid.UUID                `json:"id"`
	UserID          uuid.UUID                `json:"user_id"`
	Status          domain.TransactionStatus `json:"status"`
	CategoryID      *uuid.UUID               `json:"category_id"`
	MerchantName    *string                  `json:"merchant_name"`
	Amount          *float64                 `json:"amount"`
	TransactionDate *string                  `json:"transaction_date"`
	ReceiptImageURL *string                  `json:"receipt_image_url"`
	Notes           *string                  `json:"notes"`
	CreatedAt       string                   `json:"created_at"`
	UpdatedAt       string                   `json:"updated_at"`
}

type receiptResponse struct {
	Status          domain.TransactionStatus `json:"status"`
	TransactionID   uuid.UUID                `json:"transaction_id"`
	ReceiptImageURL string                   `json:"receipt_image_url"`
	Transaction     *transactionResponse     `json:"transaction"`
}

func receiptResponseFromDomain(result domain.ReceiptResult) receiptResponse {
	return receiptResponse{
		Status: result.Status, TransactionID: result.TransactionID,
		ReceiptImageURL: result.ReceiptImageURL, Transaction: transactionResponsePtr(result.Transaction),
	}
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
	merchant := &transaction.MerchantName
	amount := &transaction.Amount
	date := transaction.TransactionDate.Format(time.RFC3339)
	datePtr := &date
	if transaction.Status == domain.TransactionStatusPending {
		if transaction.MerchantName == "" {
			merchant = nil
		}
		if transaction.Amount == 0 {
			amount = nil
		}
		if transaction.TransactionDate.IsZero() {
			datePtr = nil
		}
	}
	return transactionResponse{
		ID:              transaction.ID,
		UserID:          transaction.UserID,
		Status:          transaction.Status,
		CategoryID:      transaction.CategoryID,
		MerchantName:    merchant,
		Amount:          amount,
		TransactionDate: datePtr,
		ReceiptImageURL: transaction.ReceiptImageUrl,
		Notes:           transaction.Notes,
		CreatedAt:       transaction.CreatedAt.Format(time.RFC3339),
		UpdatedAt:       transaction.UpdatedAt.Format(time.RFC3339),
	}
}
