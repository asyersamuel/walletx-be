package handlers

import (
	"walletx-be/pkg/repository"
	"walletx-be/pkg/utils"

	"github.com/gin-gonic/gin"
)

// UserHandler handles all user-profile related HTTP requests.
type UserHandler struct {
	userRepo repository.UserRepository
}

// NewUserHandler creates a new UserHandler with the provided UserRepository.
func NewUserHandler(userRepo repository.UserRepository) *UserHandler {
	return &UserHandler{userRepo: userRepo}
}

// GetMe returns the profile of the currently authenticated user.
// The user_id is extracted from the JWT claims injected by AuthMiddleware.
//
// GET /api/v1/users/me
func (h *UserHandler) GetMe(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}

	user, err := h.userRepo.GetByID(c.Request.Context(), userID)
	if err != nil {
		utils.ErrorResponse(c, "Failed to retrieve user profile")
		return
	}

	utils.SuccessResponse(c, user, "User profile retrieved successfully")
}
