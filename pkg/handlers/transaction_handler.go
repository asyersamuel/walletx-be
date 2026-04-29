package handlers

import (
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"walletx-be/pkg/services"
	"walletx-be/pkg/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type TransactionHandler struct {
	txService services.TransactionService
}

func NewTransactionHandler(txService services.TransactionService) *TransactionHandler {
	return &TransactionHandler{
		txService: txService,
	}
}

// GetUserTransactions handles GET /api/v1/transactions
// RESTful API endpoint with full filtering, sorting, and pagination support
//
// Query Parameters:
// - Pagination: limit (default: 50, max: 100), offset (default: 0)
// - Date Filter: date (exact), date_from (YYYY-MM-DD), date_to (YYYY-MM-DD)
// - Amount Filter: amount_min, amount_max
// - Search: q (searches merchant name)
// - Sorting: sort=field:direction (e.g., transaction_date:desc, amount:asc)
// - Delta Sync: last_updated_at (ISO 8601 timestamp)
// - CSV Export: Accept header = text/csv
func (h *TransactionHandler) GetUserTransactions(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}

	acceptHeader := c.GetHeader("Accept")
	isCSVExport := acceptHeader == "text/csv"

	if isCSVExport {
		h.exportCSV(c, userID)
		return
	}

	// Parse pagination
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	// Parse filters
	filters := services.TransactionFilters{
		DateFrom:     c.Query("date_from"),
		DateTo:       c.Query("date_to"),
		LastUpdated:  c.Query("last_updated_at"),
		ExcludeDeleted: true,
	}

	// Support legacy single date filter
	if dateFilter := c.Query("date"); dateFilter != "" {
		filters.DateFrom = dateFilter
		filters.DateTo = dateFilter
	}

	// Parse amount range filters
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

	// Parse sorting
	sort := services.SortOption{
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

	// Handle search query
	searchQuery := c.Query("q")

	var transactions interface{}
	var totalCount int
	var err error

	if searchQuery != "" {
		// Search mode: search by merchant name
		results, err := h.txService.SearchTransactions(c.Request.Context(), userID, searchQuery, limit, offset)
		if err != nil {
			utils.ErrorResponse(c, "Failed to search transactions")
			return
		}
		count, _ := h.txService.SearchTransactionsCount(c.Request.Context(), userID, searchQuery)
		transactions = results
		totalCount = count
	} else {
		// List mode: with filters and sorting
		transactions, totalCount, err = h.txService.GetUserTransactionsWithCount(c.Request.Context(), userID, limit, offset, filters, sort)
		if err != nil {
			utils.ErrorResponse(c, "Failed to retrieve transactions")
			return
		}
	}

	pagination := utils.CalculatePaginationMeta(totalCount, limit, offset)
	utils.SuccessResponseWithPagination(c, transactions, "Transactions retrieved successfully", pagination)
}

// CreateTransaction handles POST /api/v1/transactions — manual transaction creation
func (h *TransactionHandler) CreateTransaction(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}

	var input services.CreateTransactionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.FailResponseWithDetails(c, "Invalid request body", err.Error())
		return
	}

	transaction, err := h.txService.CreateManualTransaction(c.Request.Context(), userID, input)
	if err != nil {
		utils.FailResponseWithStatus(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponseWithStatus(c, http.StatusCreated, transaction, "Transaction created successfully")
}

// UpdateTransaction handles PUT /api/v1/transactions/:id
// Supports partial updates - only provided fields will be updated
func (h *TransactionHandler) UpdateTransaction(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}

	txIDStr := c.Param("id")
	txID, err := uuid.Parse(txIDStr)
	if err != nil {
		utils.FailResponseWithStatus(c, http.StatusBadRequest, "Invalid transaction ID format")
		return
	}

	var input services.UpdateTransactionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.FailResponseWithDetails(c, "Invalid request body", err.Error())
		return
	}

	transaction, err := h.txService.UpdateTransaction(c.Request.Context(), userID, txID, input)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.NotFoundResponse(c, "Transaction not found")
			return
		}
		if err.Error() == "unauthorized to update this transaction" {
			utils.ForbiddenResponse(c, "You are not authorized to access this transaction")
			return
		}
		utils.FailResponseWithStatus(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, transaction, "Transaction updated successfully")
}

// DeleteTransaction handles DELETE /api/v1/transactions/:id
func (h *TransactionHandler) DeleteTransaction(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}

	txIDStr := c.Param("id")
	txID, err := uuid.Parse(txIDStr)
	if err != nil {
		utils.FailResponseWithStatus(c, http.StatusBadRequest, "Invalid transaction ID format")
		return
	}

	err = h.txService.DeleteTransaction(c.Request.Context(), userID, txID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.NotFoundResponse(c, "Transaction not found")
			return
		}
		if err.Error() == "unauthorized to delete this transaction" {
			utils.ForbiddenResponse(c, "You are not authorized to access this transaction")
			return
		}
		utils.FailResponseWithStatus(c, http.StatusBadRequest, err.Error())
		return
	}

	c.Status(http.StatusNoContent)
}

// GetReports handles GET /api/v1/reports/expenses
// RESTful reporting endpoint with grouping support
// Query Parameters: month, year, group_by (default: category)
func (h *TransactionHandler) GetReports(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}

	month, _ := strconv.Atoi(c.Query("month"))
	year, _ := strconv.Atoi(c.Query("year"))
	groupBy := c.DefaultQuery("group_by", "category")

	if groupBy != "category" {
		utils.FailResponseWithStatus(c, http.StatusBadRequest, "Unsupported group_by value. Use 'category'")
		return
	}

	reports, err := h.txService.GetReportsByCategory(c.Request.Context(), userID, month, year)
	if err != nil {
		utils.ErrorResponse(c, "Failed to get reports")
		return
	}
	utils.SuccessResponse(c, reports, "Reports retrieved")
}

// exportCSV handles CSV export via content negotiation
func (h *TransactionHandler) exportCSV(c *gin.Context, userID uuid.UUID) {
	monthStr := c.Query("month")
	yearStr := c.Query("year")

	if monthStr == "" || yearStr == "" {
		utils.FailResponseWithStatus(c, http.StatusBadRequest, "month and year query parameters are required for CSV export")
		return
	}

	month, err := strconv.Atoi(monthStr)
	if err != nil || month < 1 || month > 12 {
		utils.FailResponseWithStatus(c, http.StatusBadRequest, "invalid month parameter")
		return
	}

	year, err := strconv.Atoi(yearStr)
	if err != nil || year < 1900 {
		utils.FailResponseWithStatus(c, http.StatusBadRequest, "invalid year parameter")
		return
	}

	txs, err := h.txService.GetAllTransactionsForExport(c.Request.Context(), userID, month, year)
	if err != nil {
		utils.ErrorResponse(c, "Failed to export data")
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
