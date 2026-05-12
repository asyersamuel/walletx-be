package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/sirupsen/logrus"
)

type QStashClaims struct {
	Sub string `json:"sub"`
	jwt.RegisteredClaims
}

func QStashAuthMiddleware(signingKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		signatureHeader := c.GetHeader("Upstash-Signature")
		if signatureHeader == "" {
			logrus.Warn("[QStashAuth] Missing Upstash-Signature header")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing signature"})
			return
		}

		token, err := jwt.ParseWithClaims(signatureHeader, &QStashClaims{}, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(signingKey), nil
		})

		if err != nil || !token.Valid {
			logrus.WithError(err).Warn("[QStashAuth] Invalid signature")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
			return
		}

		claims, ok := token.Claims.(*QStashClaims)
		if !ok {
			logrus.Warn("[QStashAuth] Invalid claims type")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid claims"})
			return
		}

		if claims.ExpiresAt != nil && time.Now().After(claims.ExpiresAt.Time) {
			logrus.Warn("[QStashAuth] Token expired")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "token expired"})
			return
		}

		expectedSub := c.Request.URL.Path
		if claims.Sub != "" && !strings.HasSuffix(expectedSub, claims.Sub) {
			logrus.WithField("sub", claims.Sub).Warn("[QStashAuth] Subject mismatch")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "subject mismatch"})
			return
		}

		c.Next()
	}
}
