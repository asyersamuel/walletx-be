package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	domain "walletx-be/internal/modules/transactions/contracts"

	apperrors "walletx-be/internal/shared/errors"
	"walletx-be/internal/shared/request"
	"walletx-be/internal/shared/response"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func NewHandler(service domain.TransactionService, dependencies ...interface{}) *Handler {
	handler := &Handler{service: service}
	for _, dependency := range dependencies {
		switch value := dependency.(type) {
		case domain.AgentService:
			handler.agent = value
		case domain.ReceiptService:
			handler.receipt = value
		}
	}
	return handler
}

const maxReceiptBytes int64 = domain.MaxReceiptBytes

func (h *Handler) Create(c *gin.Context) {
	userID, ok := request.ParseUserID(c)
	if !ok {
		return
	}

	var body createTransactionRequest
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		response.Fail(c, "Invalid request body")
		return
	}

	transactionDate, err := parseTransactionDate(body.TransactionDate)
	if err != nil {
		response.Fail(c, "Invalid transaction date")
		return
	}

	created, err := h.service.CreateTransaction(c.Request.Context(), userID, domain.CreateTransactionInput{
		UserID:          userID,
		CategoryID:      body.CategoryID,
		CategoryName:    body.CategoryName,
		MerchantName:    body.MerchantName,
		Amount:          body.Amount,
		TransactionDate: transactionDate,
		ReceiptImageUrl: body.ReceiptImageURL,
		Notes:           body.Notes,
	})
	if err != nil {
		h.handleError(c, err)
		return
	}

	response.SuccessWithStatus(c, http.StatusCreated, transactionResponseFromDomain(created), "Transaction created successfully")
}

func (h *Handler) Chat(c *gin.Context) {
	userID, ok := request.ParseUserID(c)
	if !ok {
		return
	}
	if h.agent == nil {
		response.ErrorWithStatus(c, http.StatusServiceUnavailable, "AI service unavailable")
		return
	}
	var body chatRequest
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil || body.Message == "" {
		response.Fail(c, "Invalid chat request")
		return
	}
	if body.UserID != nil && *body.UserID != userID {
		response.Fail(c, "user_id cannot override authenticated user")
		return
	}
	result, err := h.agent.Handle(c.Request.Context(), userID, body.Message)
	if err != nil {
		h.handleError(c, err)
		return
	}
	status := http.StatusOK
	if result.Transaction != nil {
		status = http.StatusCreated
	}
	response.SuccessWithStatus(c, status, agentResponseDTO(result), result.Text)
}

func (h *Handler) Receipt(c *gin.Context) {
	userID, ok := request.ParseUserID(c)
	if !ok {
		return
	}
	if h.receipt == nil {
		response.ErrorWithStatus(c, http.StatusServiceUnavailable, "Receipt service unavailable")
		return
	}
	if c.Request.ContentLength > maxReceiptBytes && c.Request.ContentLength >= 0 {
		response.Fail(c, "Receipt image is too large")
		return
	}
	header, err := c.FormFile("receipt")
	if err != nil {
		response.Fail(c, "Receipt image is required")
		return
	}
	if header.Size <= 0 || header.Size > maxReceiptBytes {
		response.Fail(c, "Invalid or oversized receipt image")
		return
	}
	file, err := header.Open()
	if err != nil {
		response.Fail(c, "Invalid receipt image")
		return
	}
	defer file.Close()
	image, err := io.ReadAll(io.LimitReader(file, maxReceiptBytes+1))
	if err != nil || len(image) == 0 || int64(len(image)) > maxReceiptBytes {
		response.Fail(c, "Invalid or oversized receipt image")
		return
	}
	contentType := http.DetectContentType(image)
	if !supportedReceiptContentType(contentType) {
		response.ErrorWithStatus(c, http.StatusUnsupportedMediaType, "Unsupported receipt image type")
		return
	}
	result, err := h.receipt.ProcessReceipt(c.Request.Context(), userID, contentType, header.Filename, image)
	if err != nil {
		h.handleError(c, err)
		return
	}
	if result.TransactionID == uuid.Nil || strings.TrimSpace(result.ReceiptImageURL) == "" {
		h.handleError(c, apperrors.ErrInternal)
		return
	}
	switch result.Status {
	case domain.TransactionStatusSuccess:
		if result.Transaction == nil || result.Transaction.ID != result.TransactionID || result.Transaction.UserID != userID {
			h.handleError(c, apperrors.ErrInternal)
			return
		}
		response.SuccessWithStatus(c, http.StatusCreated, receiptResponseFromDomain(result), "Transaction recorded.")
	case domain.TransactionStatusPending:
		if result.Transaction != nil {
			h.handleError(c, apperrors.ErrInternal)
			return
		}
		response.SuccessWithStatus(c, http.StatusAccepted, receiptResponseFromDomain(result), "Receipt saved; processing pending.")
	default:
		h.handleError(c, apperrors.ErrInternal)
	}
}

func supportedReceiptContentType(contentType string) bool {
	switch contentType {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
		return true
	default:
		return false
	}
}

func (h *Handler) Get(c *gin.Context) {
	userID, ok := request.ParseUserID(c)
	if !ok {
		return
	}
	transactionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Fail(c, "Invalid transaction ID format")
		return
	}
	transaction, err := h.service.GetTransaction(c.Request.Context(), userID, transactionID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.Success(c, transactionResponseFromDomain(transaction), "Transaction retrieved successfully")
}

func (h *Handler) List(c *gin.Context) {
	userID, ok := request.ParseUserID(c)
	if !ok {
		return
	}
	limit, err := parseQueryInt(c, "limit", domain.DefaultListLimit)
	if err != nil || limit < 0 {
		response.Fail(c, "Invalid limit")
		return
	}
	offset, err := parseQueryInt(c, "offset", 0)
	if err != nil || offset < 0 {
		response.Fail(c, "Invalid offset")
		return
	}
	startDate, err := parseOptionalQueryDate(c, "start_date")
	if err != nil {
		response.Fail(c, "Invalid start date")
		return
	}
	endDate, err := parseOptionalQueryDate(c, "end_date")
	if err != nil {
		response.Fail(c, "Invalid end date")
		return
	}
	if startDate != nil && endDate != nil && startDate.After(*endDate) {
		response.Fail(c, "Invalid date range")
		return
	}
	transactions, err := h.service.ListTransactions(c.Request.Context(), userID, domain.ListTransactionsInput{
		UserID: userID, Limit: limit, Offset: offset, StartDate: startDate, EndDate: endDate,
	})
	if err != nil {
		h.handleError(c, err)
		return
	}
	items := make([]transactionResponse, len(transactions))
	for i := range transactions {
		items[i] = transactionResponseFromDomain(&transactions[i])
	}
	response.Success(c, items, "Transactions retrieved successfully")
}

func (h *Handler) Update(c *gin.Context) {
	userID, ok := request.ParseUserID(c)
	if !ok {
		return
	}
	transactionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Fail(c, "Invalid transaction ID format")
		return
	}
	var body updateTransactionRequest
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		response.Fail(c, "Invalid request body")
		return
	}
	var transactionDate *time.Time
	if body.TransactionDate != nil {
		parsed, parseErr := time.Parse(time.RFC3339, *body.TransactionDate)
		if parseErr != nil {
			response.Fail(c, "Invalid transaction date")
			return
		}
		transactionDate = &parsed
	}
	updated, err := h.service.UpdateTransaction(c.Request.Context(), userID, transactionID, domain.UpdateTransactionInput{
		MerchantName:    body.MerchantName,
		Amount:          body.Amount,
		TransactionDate: transactionDate,
		CategoryID:      body.CategoryID,
		ReceiptImageUrl: body.ReceiptImageURL,
		Notes:           body.Notes,
		ClearReceiptURL: body.ClearReceiptURL,
		ClearNotes:      body.ClearNotes,
	})
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.Success(c, transactionResponseFromDomain(updated), "Transaction updated successfully")
}

func (h *Handler) Delete(c *gin.Context) {
	userID, ok := request.ParseUserID(c)
	if !ok {
		return
	}
	transactionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Fail(c, "Invalid transaction ID format")
		return
	}
	if err := h.service.DeleteTransaction(c.Request.Context(), userID, transactionID); err != nil {
		h.handleError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func parseQueryInt(c *gin.Context, key string, defaultValue int) (int, error) {
	value := c.Query(key)
	if value == "" {
		return defaultValue, nil
	}
	return strconv.Atoi(value)
}

func parseOptionalQueryDate(c *gin.Context, key string) (*time.Time, error) {
	value, exists := c.GetQuery(key)
	if !exists || value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func parseTransactionDate(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, errors.New("transaction date is required")
	}
	return time.Parse(time.RFC3339, value)
}

func (h *Handler) handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, apperrors.ErrInvalidInput):
		response.FailWithStatus(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, apperrors.ErrNotFound):
		response.NotFound(c, "Category or transaction not found")
	case errors.Is(err, apperrors.ErrUnauthorized):
		response.Unauthorized(c, "Unauthorized access")
	case strings.Contains(strings.ToLower(err.Error()), "unavailable"):
		response.ErrorWithStatus(c, http.StatusServiceUnavailable, "Service temporarily unavailable")
	default:
		response.Error(c, "Failed to create transaction")
	}
}
