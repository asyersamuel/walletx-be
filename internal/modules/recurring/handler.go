package recurring

import (
	"errors"
	"net/http"

	"walletx-be/internal/shared/errors"
	"walletx-be/internal/shared/request"
	"walletx-be/internal/shared/response"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Create(c *gin.Context) {
	userID, ok := request.ParseUserID(c)
	if !ok {
		return
	}

	var input CreateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.FailWithDetails(c, "Invalid request body", err.Error())
		return
	}

	config, err := h.service.Create(c.Request.Context(), userID, input)
	if err != nil {
		response.FailWithStatus(c, http.StatusBadRequest, err.Error())
		return
	}

	response.SuccessWithStatus(c, http.StatusCreated, config, "Recurring config created successfully")
}

func (h *Handler) List(c *gin.Context) {
	userID, ok := request.ParseUserID(c)
	if !ok {
		return
	}

	configs, err := h.service.List(c.Request.Context(), userID)
	if err != nil {
		response.Error(c, "Failed to retrieve recurring configs")
		return
	}

	response.Success(c, configs, "Recurring configs retrieved successfully")
}

func (h *Handler) GetByID(c *gin.Context) {
	userID, ok := request.ParseUserID(c)
	if !ok {
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.FailWithStatus(c, http.StatusBadRequest, "Invalid recurring config ID")
		return
	}

	config, err := h.service.GetByID(c.Request.Context(), id, userID)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			response.NotFound(c, "Recurring config not found")
			return
		}
		response.Error(c, "Failed to retrieve recurring config")
		return
	}

	response.Success(c, config, "Recurring config retrieved successfully")
}

func (h *Handler) Update(c *gin.Context) {
	userID, ok := request.ParseUserID(c)
	if !ok {
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.FailWithStatus(c, http.StatusBadRequest, "Invalid recurring config ID")
		return
	}

	var input UpdateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.FailWithDetails(c, "Invalid request body", err.Error())
		return
	}

	config, err := h.service.Update(c.Request.Context(), id, userID, input)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			response.NotFound(c, "Recurring config not found")
			return
		}
		response.FailWithStatus(c, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(c, config, "Recurring config updated successfully")
}

func (h *Handler) Delete(c *gin.Context) {
	userID, ok := request.ParseUserID(c)
	if !ok {
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.FailWithStatus(c, http.StatusBadRequest, "Invalid recurring config ID")
		return
	}

	if err := h.service.Delete(c.Request.Context(), id, userID); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			response.NotFound(c, "Recurring config not found")
			return
		}
		response.Error(c, "Failed to delete recurring config")
		return
	}

	c.Status(http.StatusNoContent)
}
