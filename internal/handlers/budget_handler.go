package handlers

import (
	"errors"
	"net/http"

	"walletx-be/internal/services"
	"walletx-be/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type BudgetHandler struct {
	budgetService services.BudgetService
}

func NewBudgetHandler(budgetService services.BudgetService) *BudgetHandler {
	return &BudgetHandler{budgetService: budgetService}
}

// CreateBudget POST /api/v1/budgets
func (h *BudgetHandler) CreateBudget(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}

	var input services.CreateBudgetInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.FailResponseWithDetails(c, "Invalid request body", err.Error())
		return
	}

	limit, err := h.budgetService.CreateBudget(userID, input)
	if err != nil {
		utils.FailResponseWithStatus(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponseWithStatus(c, http.StatusCreated, limit, "Budget limit created successfully")
}

// ListBudgets GET /api/v1/budgets
func (h *BudgetHandler) ListBudgets(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}

	limits, err := h.budgetService.ListBudgets(userID)
	if err != nil {
		utils.ErrorResponse(c, "Failed to retrieve budget limits")
		return
	}

	utils.SuccessResponse(c, limits, "Budget limits retrieved successfully")
}

// GetBudgetByID GET /api/v1/budgets/:id
func (h *BudgetHandler) GetBudgetByID(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.FailResponseWithStatus(c, http.StatusBadRequest, "Invalid budget ID")
		return
	}

	limit, err := h.budgetService.GetBudgetByID(id, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.NotFoundResponse(c, "Budget limit not found")
			return
		}
		utils.ErrorResponse(c, "Failed to retrieve budget limit")
		return
	}

	utils.SuccessResponse(c, limit, "Budget limit retrieved successfully")
}

// UpdateBudget PUT /api/v1/budgets/:id
func (h *BudgetHandler) UpdateBudget(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.FailResponseWithStatus(c, http.StatusBadRequest, "Invalid budget ID")
		return
	}

	var input services.UpdateBudgetInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.FailResponseWithDetails(c, "Invalid request body", err.Error())
		return
	}

	limit, err := h.budgetService.UpdateBudget(id, userID, input)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.NotFoundResponse(c, "Budget limit not found")
			return
		}
		utils.FailResponseWithStatus(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, limit, "Budget limit updated successfully")
}

// DeleteBudget DELETE /api/v1/budgets/:id
func (h *BudgetHandler) DeleteBudget(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.FailResponseWithStatus(c, http.StatusBadRequest, "Invalid budget ID")
		return
	}

	if err := h.budgetService.DeleteBudget(id, userID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.NotFoundResponse(c, "Budget limit not found")
			return
		}
		utils.ErrorResponse(c, "Failed to delete budget limit")
		return
	}

	utils.SuccessResponse(c, nil, "Budget limit deleted successfully")
}
