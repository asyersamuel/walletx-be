package middleware

import (
	"context"
	"strings"

	"walletx-be/pkg/repository"
	"walletx-be/pkg/utils"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func AuthMiddleware(jwtSecret string, blacklistRepo repository.TokenBlacklistRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		var tokenString string

		// 1. Cek dulu dari Header
		authHeader := c.GetHeader("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenString = strings.TrimPrefix(authHeader, "Bearer ")
		}

		// 2. Kalau kosong, cek dari query param (?token=xxx)
		if tokenString == "" {
			tokenString = c.Query("token")
		}

		if tokenString == "" {
			utils.UnauthorizedResponse(c, "Token required")
			c.Abort()
			return
		}

		// 3. Parse dan validasi token
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			return []byte(jwtSecret), nil
		})
		if err != nil || !token.Valid {
			utils.UnauthorizedResponse(c, "Invalid or expired token")
			c.Abort()
			return
		}

		// 4. Extract claims
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			utils.UnauthorizedResponse(c, "Invalid token claims")
			c.Abort()
			return
		}

		// 5. Check if token is blacklisted
		if jti, exists := claims["jti"].(string); exists && jti != "" {
			blacklisted, err := blacklistRepo.IsTokenBlacklisted(context.Background(), jti)
			if err != nil {
				utils.ErrorResponse(c, "Failed to validate token")
				c.Abort()
				return
			}
			if blacklisted {
				utils.UnauthorizedResponse(c, "Token has been revoked")
				c.Abort()
				return
			}
		}

		if userID, exists := claims["user_id"]; exists {
			c.Set("user_id", userID)
		}
		if email, exists := claims["email"]; exists {
			c.Set("email", email)
		}
		if role, exists := claims["role"]; exists {
			c.Set("role", role)
		}

		c.Next()
	}
}
