package handlers

import (
	"errors"
	"net/http"
	"os"

	"walletx-be/pkg/services"
	"walletx-be/pkg/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type RecurringHandler struct {
	recurringService services.RecurringService
}

func NewRecurringHandler(recurringService services.RecurringService) *RecurringHandler {
	return &RecurringHandler{recurringService: recurringService}
}

// CreateRecurring POST /api/v1/recurrings
func (h *RecurringHandler) CreateRecurring(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}

	var input services.CreateRecurringInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.FailResponseWithDetails(c, "Invalid request body", err.Error())
		return
	}

	config, err := h.recurringService.CreateRecurring(userID, input)
	if err != nil {
		utils.FailResponseWithStatus(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponseWithStatus(c, http.StatusCreated, config, "Recurring config created successfully")
}

// ListRecurrings GET /api/v1/recurrings
func (h *RecurringHandler) ListRecurrings(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}

	configs, err := h.recurringService.ListRecurrings(userID)
	if err != nil {
		utils.ErrorResponse(c, "Failed to retrieve recurring configs")
		return
	}

	utils.SuccessResponse(c, configs, "Recurring configs retrieved successfully")
}

// GetRecurringByID GET /api/v1/recurrings/:id
func (h *RecurringHandler) GetRecurringByID(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.FailResponseWithStatus(c, http.StatusBadRequest, "Invalid recurring config ID")
		return
	}

	config, err := h.recurringService.GetRecurringByID(id, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.NotFoundResponse(c, "Recurring config not found")
			return
		}
		utils.ErrorResponse(c, "Failed to retrieve recurring config")
		return
	}

	utils.SuccessResponse(c, config, "Recurring config retrieved successfully")
}

// UpdateRecurring PUT /api/v1/recurrings/:id
func (h *RecurringHandler) UpdateRecurring(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.FailResponseWithStatus(c, http.StatusBadRequest, "Invalid recurring config ID")
		return
	}

	var input services.UpdateRecurringInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.FailResponseWithDetails(c, "Invalid request body", err.Error())
		return
	}

	config, err := h.recurringService.UpdateRecurring(id, userID, input)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.NotFoundResponse(c, "Recurring config not found")
			return
		}
		utils.FailResponseWithStatus(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, config, "Recurring config updated successfully")
}

// DeleteRecurring DELETE /api/v1/recurrings/:id
func (h *RecurringHandler) DeleteRecurring(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.FailResponseWithStatus(c, http.StatusBadRequest, "Invalid recurring config ID")
		return
	}

	if err := h.recurringService.DeleteRecurring(id, userID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.NotFoundResponse(c, "Recurring config not found")
			return
		}
		utils.ErrorResponse(c, "Failed to delete recurring config")
		return
	}

	utils.SuccessResponse(c, nil, "Recurring config deleted successfully")
}

// ProcessRecurringCron POST /api/cron/recurring
func (h *RecurringHandler) ProcessRecurringCron(c *gin.Context) {
	// Security check for cron secret
	secret := c.GetHeader("X-Cron-Secret")
	
	// The variable "CRON_SECRET" should be set in Vercel environment variables or .env
	expectedSecret := os.Getenv("CRON_SECRET")

	if expectedSecret == "" || secret != expectedSecret {
		utils.FailResponseWithStatus(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	if err := h.recurringService.ProcessDueRecurrings(); err != nil {
		utils.ErrorResponse(c, "Failed to process due recurring transactions")
		return
	}

	utils.SuccessResponse(c, nil, "Processed due recurring transactions successfully")
}
