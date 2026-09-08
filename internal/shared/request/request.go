package request

import (
	"net/http"

	"walletx-be/internal/shared/response"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ParseUserID extracts the authenticated user id from the Gin context populated
// by the JWT middleware. It writes the appropriate error response and returns
// false when the id is missing or malformed.
func ParseUserID(c *gin.Context) (uuid.UUID, bool) {
	val, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "User not authenticated")
		return uuid.Nil, false
	}
	userUUID, err := uuid.Parse(val.(string))
	if err != nil {
		response.FailWithStatus(c, http.StatusBadRequest, "Invalid user ID format")
		return uuid.Nil, false
	}
	return userUUID, true
}
