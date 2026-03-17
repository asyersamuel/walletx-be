package handlers

import (
	"walletx-be/internal/services"
	"walletx-be/internal/utils"

	"github.com/gin-gonic/gin"
)

type DashboardHandler struct {
	dashboardService services.DashboardService
}

func NewDashboardHandler(dashboardService services.DashboardService) *DashboardHandler {
	return &DashboardHandler{dashboardService: dashboardService}
}

// GetBudgetSummary GET /api/v1/dashboard/budget-summary
// Returns each active budget limit merged with actual spending from the VIEW,
// exposing LimitAmount, SpentAmount, and RemainingBudget per category.
func (h *DashboardHandler) GetBudgetSummary(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}

	summary, err := h.dashboardService.GetBudgetSummary(userID)
	if err != nil {
		utils.ErrorResponse(c, "Failed to retrieve budget summary")
		return
	}

	utils.SuccessResponse(c, summary, "Budget summary retrieved successfully")
}
