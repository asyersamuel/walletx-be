package users

import "github.com/gin-gonic/gin"

// RegisterProtectedRoutes registers user profile routes requiring a valid JWT.
func RegisterProtectedRoutes(r *gin.RouterGroup, h *Handler) {
	group := r.Group("/users")
	{
		group.PUT("/me", h.UpdateProfile)
		group.DELETE("/me", h.DeleteAccount)
	}
}
