package dashboard

import "github.com/gin-gonic/gin"

// RegisterReportRoutes registers the dashboard report endpoints under /reports.
func RegisterReportRoutes(r *gin.RouterGroup, h *Handler) {
	report := r.Group("/reports")
	{
		report.GET("/budget-summary", h.GetBudgetSummary)
		report.GET("/daily-calendar", h.GetDailyCalendar)
	}
}
