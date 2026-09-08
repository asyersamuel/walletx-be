package dashboard

import (
	"net/http"
	"strconv"

	"walletx-be/internal/shared/request"
	"walletx-be/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) GetBudgetSummary(c *gin.Context) {
	userID, ok := request.ParseUserID(c)
	if !ok {
		return
	}

	summary, err := h.service.GetBudgetSummary(c.Request.Context(), userID)
	if err != nil {
		response.Error(c, "Failed to retrieve budget summary")
		return
	}

	response.Success(c, summary, "Budget summary retrieved successfully")
}

func (h *Handler) GetDailyCalendar(c *gin.Context) {
	userID, ok := request.ParseUserID(c)
	if !ok {
		return
	}

	monthStr := c.Query("month")
	yearStr := c.Query("year")

	if monthStr == "" || yearStr == "" {
		response.FailWithStatus(c, http.StatusBadRequest, "month and year query parameters are required")
		return
	}

	month, err := strconv.Atoi(monthStr)
	if err != nil || month < 1 || month > 12 {
		response.FailWithStatus(c, http.StatusBadRequest, "invalid month parameter")
		return
	}

	year, err := strconv.Atoi(yearStr)
	if err != nil || year < 1900 {
		response.FailWithStatus(c, http.StatusBadRequest, "invalid year parameter")
		return
	}

	dailyTotals, err := h.service.GetDailyTotal(c.Request.Context(), userID, month, year)
	if err != nil {
		response.Error(c, "Failed to retrieve daily calendar data")
		return
	}

	response.Success(c, dailyTotals, "Daily calendar retrieved successfully")
}
