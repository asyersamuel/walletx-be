package middleware

import (
	"crypto/sha256"
	"crypto/subtle"
	"strings"

	"walletx-be/internal/shared/response"

	"github.com/gin-gonic/gin"
)

// CronAuth authenticates using the environment-loaded secret injected by config.
// It accepts one exact Bearer header and never falls back to JWT or query tokens.
func CronAuth(secret string) gin.HandlerFunc {
	validSecret := secret != "" && strings.IndexFunc(secret, func(r rune) bool {
		return r <= 32 || r >= 127
	}) < 0
	expected := sha256.Sum256([]byte("Bearer " + secret))
	return func(c *gin.Context) {
		values := c.Request.Header.Values("Authorization")
		if !validSecret || len(values) != 1 {
			response.Unauthorized(c, "Unauthorized")
			c.Abort()
			return
		}
		// Fixed-size digests keep comparison time independent of secret length.
		actual := sha256.Sum256([]byte(values[0]))
		if subtle.ConstantTimeCompare(actual[:], expected[:]) != 1 {
			response.Unauthorized(c, "Unauthorized")
			c.Abort()
			return
		}
		c.Next()
	}
}
