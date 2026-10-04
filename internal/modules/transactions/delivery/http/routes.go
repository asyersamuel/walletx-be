package http

import "github.com/gin-gonic/gin"

// RegisterProtectedRoutes registers transaction endpoints under a JWT-protected
// router group. Authentication middleware is applied by the caller's group.
func RegisterProtectedRoutes(r *gin.RouterGroup, handler *Handler) {
	group := r.Group("/transactions")
	group.POST("", handler.Create)
	group.POST("/chat", handler.Chat)
	group.POST("/receipt", handler.Receipt)
	group.GET("", handler.List)
	group.GET("/:id", handler.Get)
	group.PATCH("/:id", handler.Update)
	group.DELETE("/:id", handler.Delete)
}
