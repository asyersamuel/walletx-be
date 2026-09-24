package request

import (
	"net/http"

	"walletx-be/internal/shared/response"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ParseUserID extracts the authenticated user id from the Gin context populated by the JWT middleware.
// A missing key or a non-string value is treated as unauthenticated; the
// comma-ok assertion guarantees this function never panics.
func ParseUserID(c *gin.Context) (uuid.UUID, bool) {
	val, _ := c.Get("user_id")
	userIDString, ok := val.(string)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return uuid.Nil, false
	}
	userUUID, err := uuid.Parse(userIDString)
	if err != nil {
		response.FailWithStatus(c, http.StatusBadRequest, "Invalid user ID format")
		return uuid.Nil, false
	}
	return userUUID, true
}
