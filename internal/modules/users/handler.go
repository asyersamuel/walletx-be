package users

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"walletx-be/internal/platform/logger"
	apperrors "walletx-be/internal/shared/errors"
	"walletx-be/internal/shared/request"
	"walletx-be/internal/shared/response"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

type Handler struct {
	service Service
	logger  logger.Logger
}

func NewHandler(service Service, logger logger.Logger) *Handler {
	return &Handler{service: service, logger: logger}
}

// UpdateProfile handles PUT /users/me.
func (h *Handler) UpdateProfile(c *gin.Context) {
	userID, ok := request.ParseUserID(c)
	if !ok {
		return
	}

	input, ok := h.bindUpdateRequest(c)
	if !ok {
		return
	}

	user, err := h.service.UpdateProfile(c.Request.Context(), userID, input)
	if err != nil {
		h.handleError(c, "update_profile", "Failed to update user profile", err)
		return
	}

	response.Success(c, gin.H{"user": user}, "User profile updated successfully")
}

// DeleteAccount handles DELETE /users/me.
func (h *Handler) DeleteAccount(c *gin.Context) {
	userID, ok := request.ParseUserID(c)
	if !ok {
		return
	}

	if err := h.service.DeleteAccount(c.Request.Context(), userID, bearerToken(c)); err != nil {
		h.handleError(c, "delete_account", "Failed to delete user account", err)
		return
	}

	c.Status(http.StatusNoContent)
}

// bindUpdateRequest decodes the payload strictly: unknown fields are rejected
// so immutable attributes cannot be smuggled into the request.
func (h *Handler) bindUpdateRequest(c *gin.Context) (UpdateUserRequest, bool) {
	var input UpdateUserRequest

	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		response.FailWithDetails(c, "Invalid request body", err.Error())
		return input, false
	}

	if err := binding.Validator.ValidateStruct(input); err != nil {
		response.FailWithDetails(c, "Validation failed", err.Error())
		return input, false
	}

	return input, true
}

func (h *Handler) handleError(c *gin.Context, operation, message string, err error) {
	switch {
	case errors.Is(err, apperrors.ErrInvalidInput):
		response.FailWithStatus(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, apperrors.ErrNotFound):
		response.NotFound(c, "User not found")
	case errors.Is(err, apperrors.ErrConflict):
		response.FailWithStatus(c, http.StatusConflict, "User already exists")
	default:
		h.logger.WithError(err).WithField("operation", operation).Error("User operation failed")
		response.Error(c, message)
	}
}

func bearerToken(c *gin.Context) string {
	authHeader := c.GetHeader("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(authHeader, "Bearer ")
}
