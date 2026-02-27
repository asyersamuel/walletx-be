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

// GetUserTransactions handle request GET /transaction
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

    // Ambil parameter pagination
    limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
    offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

    // Kirim userUUID (tipe uuid.UUID) ke service
    transactions, err := h.txService.GetUserTransactions(userUUID, limit, offset)
    if err != nil {
        utils.ErrorResponse(c, "Failed to retrieve transactions")
        return
    }

    utils.SuccessResponse(c, transactions, "Transactions retrieved successfully")
}