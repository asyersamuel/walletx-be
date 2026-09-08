package transaction

import (
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"walletx-be/internal/shared/errors"
	"walletx-be/internal/shared/pagination"
	"walletx-be/internal/shared/request"
	"walletx-be/internal/shared/response"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) GetUserTransactions(c *gin.Context) {
	userID, ok := request.ParseUserID(c)
	if !ok {
		return
	}

	acceptHeader := c.GetHeader("Accept")
	isCSVExport := acceptHeader == "text/csv"

	if isCSVExport {
		h.exportCSV(c, userID)
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	filters := TransactionFilters{
		DateFrom:       c.Query("date_from"),
		DateTo:         c.Query("date_to"),
		LastUpdated:    c.Query("last_updated_at"),
		ExcludeDeleted: true,
	}

	if dateFilter := c.Query("date"); dateFilter != "" {
		filters.DateFrom = dateFilter
		filters.DateTo = dateFilter
	}

	if amountMinStr := c.Query("amount_min"); amountMinStr != "" {
		if amountMin, err := strconv.ParseFloat(amountMinStr, 64); err == nil {
			filters.AmountMin = &amountMin
		}
	}
	if amountMaxStr := c.Query("amount_max"); amountMaxStr != "" {
		if amountMax, err := strconv.ParseFloat(amountMaxStr, 64); err == nil {
			filters.AmountMax = &amountMax
		}
	}

	sort := SortOption{
		Field:     "transaction_date",
		Direction: "DESC",
	}
	if sortParam := c.Query("sort"); sortParam != "" {
		parts := strings.Split(sortParam, ":")
		if len(parts) == 2 {
			sort.Field = parts[0]
			sort.Direction = strings.ToUpper(parts[1])
		}
	}

	searchQuery := c.Query("q")

	var transactions interface{}
	var totalCount int

	if searchQuery != "" {
		results, err := h.service.SearchTransactions(c.Request.Context(), userID, searchQuery, limit, offset)
		if err != nil {
			response.Error(c, "Failed to search transactions")
			return
		}
		count, _ := h.service.SearchTransactionsCount(c.Request.Context(), userID, searchQuery)
		transactions = results
		totalCount = count
	} else {
		var err error
		transactions, totalCount, err = h.service.GetUserTransactionsWithCount(c.Request.Context(), userID, limit, offset, filters, sort)
		if err != nil {
			response.Error(c, "Failed to retrieve transactions")
			return
		}
	}

	meta := pagination.CalculateMeta(totalCount, limit, offset)
	response.SuccessWithPagination(c, transactions, "Transactions retrieved successfully", meta)
}

func (h *Handler) CreateTransaction(c *gin.Context) {
	userID, ok := request.ParseUserID(c)
	if !ok {
		return
	}

	var input CreateTransactionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.FailWithDetails(c, "Invalid request body", err.Error())
		return
	}

	transaction, err := h.service.CreateManualTransaction(c.Request.Context(), userID, input)
	if err != nil {
		if errors.Is(err, apperrors.ErrDuplicate) {
			response.FailWithStatus(c, http.StatusConflict, "Transaction already exists")
			return
		}
		response.FailWithStatus(c, http.StatusBadRequest, err.Error())
		return
	}

	response.SuccessWithStatus(c, http.StatusCreated, transaction, "Transaction created successfully")
}

func (h *Handler) UpdateTransaction(c *gin.Context) {
	userID, ok := request.ParseUserID(c)
	if !ok {
		return
	}

	txIDStr := c.Param("id")
	txID, err := uuid.Parse(txIDStr)
	if err != nil {
		response.FailWithStatus(c, http.StatusBadRequest, "Invalid transaction ID format")
		return
	}

	var input UpdateTransactionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.FailWithDetails(c, "Invalid request body", err.Error())
		return
	}

	transaction, err := h.service.UpdateTransaction(c.Request.Context(), userID, txID, input)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			response.NotFound(c, "Transaction not found")
			return
		}
		if errors.Is(err, apperrors.ErrUnauthorized) {
			response.Forbidden(c, "You are not authorized to access this transaction")
			return
		}
		response.FailWithStatus(c, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(c, transaction, "Transaction updated successfully")
}

func (h *Handler) DeleteTransaction(c *gin.Context) {
	userID, ok := request.ParseUserID(c)
	if !ok {
		return
	}

	txIDStr := c.Param("id")
	txID, err := uuid.Parse(txIDStr)
	if err != nil {
		response.FailWithStatus(c, http.StatusBadRequest, "Invalid transaction ID format")
		return
	}

	err = h.service.DeleteTransaction(c.Request.Context(), userID, txID)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			response.NotFound(c, "Transaction not found")
			return
		}
		if errors.Is(err, apperrors.ErrUnauthorized) {
			response.Forbidden(c, "You are not authorized to access this transaction")
			return
		}
		response.FailWithStatus(c, http.StatusBadRequest, err.Error())
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *Handler) GetReports(c *gin.Context) {
	userID, ok := request.ParseUserID(c)
	if !ok {
		return
	}

	month, _ := strconv.Atoi(c.Query("month"))
	year, _ := strconv.Atoi(c.Query("year"))
	groupBy := c.DefaultQuery("group_by", "category")

	if groupBy != "category" {
		response.FailWithStatus(c, http.StatusBadRequest, "Unsupported group_by value. Use 'category'")
		return
	}

	reports, err := h.service.GetReportsByCategory(c.Request.Context(), userID, month, year)
	if err != nil {
		response.Error(c, "Failed to get reports")
		return
	}
	response.Success(c, reports, "Reports retrieved")
}

func (h *Handler) exportCSV(c *gin.Context, userID uuid.UUID) {
	monthStr := c.Query("month")
	yearStr := c.Query("year")

	if monthStr == "" || yearStr == "" {
		response.FailWithStatus(c, http.StatusBadRequest, "month and year query parameters are required for CSV export")
		return
	}

	month, err := strconv.Atoi(monthStr)
	if err != nil || month < 1 || month > 12 {
		response.FailWithStatus(c, http.StatusBadRequest, "invalid month parameter")
		return
	}

	year, err := strconv.Atoi(yearStr)
	if err != nil || year < 1900 {
		response.FailWithStatus(c, http.StatusBadRequest, "invalid year parameter")
		return
	}

	txs, err := h.service.GetAllTransactionsForExport(c.Request.Context(), userID, month, year)
	if err != nil {
		response.Error(c, "Failed to export data")
		return
	}

	c.Header("Content-Disposition", "attachment; filename=transactions.csv")
	c.Header("Content-Type", "text/csv")
	c.Header("Transfer-Encoding", "chunked")

	writer := csv.NewWriter(c.Writer)
	writer.Write([]string{"Tanggal", "Merchant", "Nominal", "Kategori", "Catatan"})

	for _, tx := range txs {
		catName := "Lainnya"
		if tx.Category != nil {
			catName = tx.Category.Name
		}

		writer.Write([]string{
			tx.TransactionDate.Format("2006-01-02 15:04"),
			tx.Merchant,
			fmt.Sprintf("%.2f", tx.Amount),
			catName,
			tx.Note,
		})
	}
	writer.Flush()
}
