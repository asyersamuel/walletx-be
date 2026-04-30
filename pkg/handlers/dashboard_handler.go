package handlers

import (
	"net/http"
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

func (h *DashboardHandler) GetBudgetSummary(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}

	summary, err := h.dashboardService.GetBudgetSummary(c.Request.Context(), userID)
	if err != nil {
		utils.ErrorResponse(c, "Failed to retrieve budget summary")
		return
	}

	utils.SuccessResponse(c, summary, "Budget summary retrieved successfully")
}

func (h *DashboardHandler) GetDailyCalendar(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}

	monthStr := c.Query("month")
	yearStr := c.Query("year")

	if monthStr == "" || yearStr == "" {
		utils.FailResponseWithStatus(c, http.StatusBadRequest, "month and year query parameters are required")
		return
	}

	month, err := strconv.Atoi(monthStr)
	if err != nil || month < 1 || month > 12 {
		utils.FailResponseWithStatus(c, http.StatusBadRequest, "invalid month parameter")
		return
	}

	year, err := strconv.Atoi(yearStr)
	if err != nil || year < 1900 {
		utils.FailResponseWithStatus(c, http.StatusBadRequest, "invalid year parameter")
		return
	}

	dailyTotals, err := h.dashboardService.GetDailyTotal(c.Request.Context(), userID, month, year)
	if err != nil {
		utils.ErrorResponse(c, "Failed to retrieve daily calendar data")
		return
	}

	utils.SuccessResponse(c, dailyTotals, "Daily calendar retrieved successfully")
}
