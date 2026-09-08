package transaction

import "github.com/gin-gonic/gin"

// RegisterRoutes registers the transaction CRUD + report endpoints.
func RegisterRoutes(r *gin.RouterGroup, h *Handler) {
	group := r.Group("/transactions")
	{
		group.GET("", h.GetUserTransactions)
		group.POST("", h.CreateTransaction)
		group.PUT("/:id", h.UpdateTransaction)
		group.DELETE("/:id", h.DeleteTransaction)
	}
}

// RegisterReportRoutes registers the expense report endpoints served by the
// transaction module (reads transaction data aggregates).
func RegisterReportRoutes(r *gin.RouterGroup, h *Handler) {
	report := r.Group("/reports")
	{
		report.GET("/expenses", h.GetReports)
	}
}
