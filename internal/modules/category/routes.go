package category

import "github.com/gin-gonic/gin"

// RegisterRoutes registers the category CRUD endpoints.
func RegisterRoutes(r *gin.RouterGroup, h *Handler) {
	group := r.Group("/categories")
	{
		group.GET("", h.List)
		group.POST("", h.Create)
		group.GET("/:id", h.GetByID)
		group.PUT("/:id", h.Update)
		group.DELETE("/:id", h.Delete)
	}
}
