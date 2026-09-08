package auth

import "github.com/gin-gonic/gin"

// RegisterRoutes registers the public auth routes (OAuth sign-in + dev helpers).
func RegisterRoutes(r *gin.RouterGroup, h *Handler, devMode bool) {
	group := r.Group("/auth")
	{
		group.POST("/google", h.HandleGoogleAuth)

		if devMode {
			group.GET("/google/test-login", h.GoogleLoginTest)
			group.GET("/google/callback", h.GoogleCallbackTest)
		}
	}
}

// RegisterProtectedRoutes registers routes requiring a valid JWT (logout).
func RegisterProtectedRoutes(r *gin.RouterGroup, h *Handler) {
	group := r.Group("/auth")
	{
		group.POST("/logout", h.Logout)
	}
}
