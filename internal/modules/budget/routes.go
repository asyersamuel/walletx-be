package budget

import "github.com/gin-gonic/gin"

// RegisterRoutes registers the budget CRUD + progress endpoints.
func RegisterRoutes(r *gin.RouterGroup, h *Handler) {
	group := r.Group("/budgets")
	{
		group.GET("", h.List)
		group.GET("/progress", h.GetProgress)
		group.POST("", h.Create)
		group.GET("/:id", h.GetByID)
		group.PUT("/:id", h.Update)
		group.DELETE("/:id", h.Delete)
	}
}
