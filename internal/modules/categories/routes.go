package categories

import "github.com/gin-gonic/gin"

func RegisterProtectedRoutes(r *gin.RouterGroup, h *Handler) {
	group := r.Group("/categories")
	{
		group.POST("", h.Create)
		group.GET("", h.List)
		group.PUT("/:id", h.Update)
		group.DELETE("/:id", h.Delete)
	}
}
