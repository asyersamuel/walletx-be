package handlers

import (
	"net/http"
	"strconv"

	"walletx-be/internal/services"
	"walletx-be/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
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
	userIDVal, exists := c.Get("user_id")
	if !exists {
		utils.UnauthorizedResponse(c, "User not authenticated")
		return
	}

	userIDStr := userIDVal.(string)
	userUUID, err := uuid.Parse(userIDStr)
	if err != nil {
		logrus.WithError(err).Warn("Invalid UUID format from token")
		utils.FailResponseWithStatus(c, http.StatusBadRequest, "Invalid user ID format")
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	transactions, err := h.txService.GetUserTransactions(userUUID, limit, offset)
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

	transaction, err := h.txService.CreateManualTransaction(userID, input)
	if err != nil {
		utils.FailResponseWithStatus(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponseWithStatus(c, http.StatusCreated, transaction, "Transaction created successfully")
}