package middleware

import (
	"walletx-be/pkg/utils"

	"github.com/gin-gonic/gin"
)

func AuthMiddleware(validator TokenValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString := extractToken(c)
		if tokenString == "" {
			utils.UnauthorizedResponse(c, "Token required")
			c.Abort()
			return
		}

		claims, err := validator.ValidateToken(c.Request.Context(), tokenString)
		if err != nil {
			if err == ErrTokenRevoked {
				utils.UnauthorizedResponse(c, "Token has been revoked")
			} else {
				utils.UnauthorizedResponse(c, "Invalid or expired token")
			}
			c.Abort()
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("email", claims.Email)
		if claims.Role != "" {
			c.Set("role", claims.Role)
		}

		c.Next()
	}
}

func extractToken(c *gin.Context) string {
	authHeader := c.GetHeader("Authorization")
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		return authHeader[7:]
	}

	if token := c.Query("token"); token != "" {
		return token
	}

	return ""
}
