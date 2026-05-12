package handlers

import (
	"net/http"

	"walletx-be/internal/domain/dto"
	"walletx-be/pkg/services"

	"github.com/gin-gonic/gin"
)

type WebhookHandler struct {
	emailProcessorService services.EmailProcessorService
}

func NewWebhookHandler(emailProcessorService services.EmailProcessorService) *WebhookHandler {
	return &WebhookHandler{
		emailProcessorService: emailProcessorService,
	}
}

func (h *WebhookHandler) ProcessEmail(c *gin.Context) {
	var payload dto.EmailProcessingPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	err := h.emailProcessorService.ProcessEmailPayload(c.Request.Context(), payload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process email"})
		return
	}

	c.Status(http.StatusOK)
}
