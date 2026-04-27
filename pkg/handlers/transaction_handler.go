package handlers

import (
	"net/http"
	"strconv"
	"encoding/csv"
	"fmt"

	"walletx-be/pkg/services"
	"walletx-be/pkg/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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
func (h *TransactionHandler) GetUserTransactions(c *gin.Context) {
    userID, ok := parseUserID(c) 
    if !ok {
        return
    }

    limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50")) 
    offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
    
    dateFilter := c.Query("date")
    
    // Tangkap query parameter last_updated_at dari Frontend
    lastUpdated := c.Query("last_updated_at")

    // Oper variabel lastUpdated ini sebagai parameter ke-5 ke dalam Service
    transactions, err := h.txService.GetUserTransactions(c.Request.Context(), userID, limit, offset, dateFilter, lastUpdated)
    if err != nil {
        utils.ErrorResponse(c, "Failed to retrieve transactions")
        return
    }

    utils.SuccessResponse(c, transactions, "Transactions retrieved successfully")
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
		utils.FailResponseWithStatus(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, nil, "Transaction deleted successfully")
}

func (h *TransactionHandler) SearchTransactions(c *gin.Context) {
    userID, ok := parseUserID(c)
    if !ok { return }

    query := c.Query("q")
    limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
    offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

    results, err := h.txService.SearchTransactions(c.Request.Context(), userID, query, limit, offset)
    if err != nil {
        utils.ErrorResponse(c, "Failed to search transactions")
        return
    }
    utils.SuccessResponse(c, results, "Search results")
}

func (h *TransactionHandler) GetReports(c *gin.Context) {
    userID, ok := parseUserID(c)
    if !ok { return }

    month, _ := strconv.Atoi(c.Query("month"))
    year, _ := strconv.Atoi(c.Query("year"))

    reports, err := h.txService.GetReportsByCategory(c.Request.Context(), userID, month, year)
    if err != nil {
        utils.ErrorResponse(c, "Failed to get reports")
        return
    }
    utils.SuccessResponse(c, reports, "Reports retrieved")
}

func (h *TransactionHandler) ExportCSV(c *gin.Context) {
    userID, ok := parseUserID(c)
    if !ok { return }

    month, _ := strconv.Atoi(c.Query("month"))
    year, _ := strconv.Atoi(c.Query("year"))

    txs, err := h.txService.GetAllTransactionsForExport(c.Request.Context(), userID, month, year)
    if err != nil {
        utils.ErrorResponse(c, "Failed to export data")
        return
    }

    // Set header agar browser/mobile tahu ini file download
    c.Header("Content-Disposition", "attachment; filename=transactions.csv")
    c.Header("Content-Type", "text/csv")
    c.Header("Transfer-Encoding", "chunked")

    writer := csv.NewWriter(c.Writer)
    // Tulis Header Kolom
    writer.Write([]string{"Tanggal", "Merchant", "Nominal", "Kategori", "Catatan"})
    
    for _, tx := range txs {
        catName := "Lainnya"
        if tx.Category != nil { catName = tx.Category.Name }
        
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