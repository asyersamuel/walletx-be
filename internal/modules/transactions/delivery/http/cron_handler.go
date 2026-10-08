package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	domain "walletx-be/internal/modules/transactions/contracts"
	"walletx-be/internal/shared/response"
)

type CronHandler struct {
	processor domain.PendingReceiptProcessor
}

func NewCronHandler(processor domain.PendingReceiptProcessor) *CronHandler {
	return &CronHandler{processor: processor}
}

func (h *CronHandler) ProcessPendingReceipts(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if h.processor == nil {
		response.ErrorWithStatus(c, http.StatusServiceUnavailable, "Receipt processor unavailable")
		return
	}
	summary, err := h.processor.ProcessPendingReceipts(c.Request.Context())
	if err != nil {
		response.Error(c, "Unable to process pending receipts")
		return
	}
	response.Success(c, summary, "Pending receipt batch completed")
}

func RegisterCronRoutes(group *gin.RouterGroup, handler *CronHandler) {
	group.GET("/process-pending-receipts", handler.ProcessPendingReceipts)
}
