package handlers

import (
	"errors"
	"net/http"

	"walletx-be/core/domain"
	"walletx-be/pkg/services"
	"walletx-be/pkg/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type RecurringHandler struct {
	recurringService services.RecurringService
}

func NewRecurringHandler(recurringService services.RecurringService) *RecurringHandler {
	return &RecurringHandler{recurringService: recurringService}
}

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

	config, err := h.recurringService.CreateRecurring(c.Request.Context(), userID, input)
	if err != nil {
		utils.FailResponseWithStatus(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponseWithStatus(c, http.StatusCreated, config, "Recurring config created successfully")
}

func (h *RecurringHandler) ListRecurrings(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}

	configs, err := h.recurringService.ListRecurrings(c.Request.Context(), userID)
	if err != nil {
		utils.ErrorResponse(c, "Failed to retrieve recurring configs")
		return
	}

	utils.SuccessResponse(c, configs, "Recurring configs retrieved successfully")
}

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

	config, err := h.recurringService.GetRecurringByID(c.Request.Context(), id, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			utils.NotFoundResponse(c, "Recurring config not found")
			return
		}
		utils.ErrorResponse(c, "Failed to retrieve recurring config")
		return
	}

	utils.SuccessResponse(c, config, "Recurring config retrieved successfully")
}

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

	config, err := h.recurringService.UpdateRecurring(c.Request.Context(), id, userID, input)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			utils.NotFoundResponse(c, "Recurring config not found")
			return
		}
		utils.FailResponseWithStatus(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, config, "Recurring config updated successfully")
}

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

	if err := h.recurringService.DeleteRecurring(c.Request.Context(), id, userID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			utils.NotFoundResponse(c, "Recurring config not found")
			return
		}
		utils.ErrorResponse(c, "Failed to delete recurring config")
		return
	}

	c.Status(http.StatusNoContent)
}
