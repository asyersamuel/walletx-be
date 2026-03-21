package handlers

import (
	"strconv"
	"walletx-be/pkg/services"
	"walletx-be/pkg/utils"

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

// GetDailyCalendar GET /api/v1/dashboard/calendar
// Returns aggregated daily spending for a specific month and year.
func (h *DashboardHandler) GetDailyCalendar(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}

	monthStr := c.Query("month")
	yearStr := c.Query("year")

	if monthStr == "" || yearStr == "" {
		utils.ErrorResponse(c, "month and year query parameters are required")
		return
	}

	month, err := strconv.Atoi(monthStr)
	if err != nil || month < 1 || month > 12 {
		utils.ErrorResponse(c, "invalid month parameter")
		return
	}

	year, err := strconv.Atoi(yearStr)
	if err != nil || year < 1900 {
		utils.ErrorResponse(c, "invalid year parameter")
		return
	}

	dailyTotals, err := h.dashboardService.GetDailyTotal(userID, month, year)
	if err != nil {
		utils.ErrorResponse(c, "Failed to retrieve daily calendar data")
		return
	}

	utils.SuccessResponse(c, dailyTotals, "Daily calendar retrieved successfully")
}
