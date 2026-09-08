package category

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

	category, err := h.service.Create(c.Request.Context(), userID, input)
	if err != nil {
		if errors.Is(err, apperrors.ErrDuplicate) {
			response.FailWithStatus(c, http.StatusConflict, "Category already exists")
			return
		}
		response.Error(c, err.Error())
		return
	}

	response.SuccessWithStatus(c, http.StatusCreated, category, "Category created successfully")
}

func (h *Handler) List(c *gin.Context) {
	userID, ok := request.ParseUserID(c)
	if !ok {
		return
	}

	categories, err := h.service.List(c.Request.Context(), userID)
	if err != nil {
		response.Error(c, "Failed to retrieve categories")
		return
	}

	response.Success(c, categories, "Categories retrieved successfully")
}

func (h *Handler) GetByID(c *gin.Context) {
	userID, ok := request.ParseUserID(c)
	if !ok {
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.FailWithStatus(c, http.StatusBadRequest, "Invalid category ID")
		return
	}

	category, err := h.service.GetByID(c.Request.Context(), id, userID)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			response.NotFound(c, "Category not found")
			return
		}
		response.Error(c, "Failed to retrieve category")
		return
	}

	response.Success(c, category, "Category retrieved successfully")
}

func (h *Handler) Update(c *gin.Context) {
	userID, ok := request.ParseUserID(c)
	if !ok {
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.FailWithStatus(c, http.StatusBadRequest, "Invalid category ID")
		return
	}

	var input UpdateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.FailWithDetails(c, "Invalid request body", err.Error())
		return
	}

	category, err := h.service.Update(c.Request.Context(), id, userID, input)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			response.NotFound(c, "Category not found")
			return
		}
		response.Error(c, err.Error())
		return
	}

	response.Success(c, category, "Category updated successfully")
}

func (h *Handler) Delete(c *gin.Context) {
	userID, ok := request.ParseUserID(c)
	if !ok {
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.FailWithStatus(c, http.StatusBadRequest, "Invalid category ID")
		return
	}

	if err := h.service.Delete(c.Request.Context(), id, userID); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			response.NotFound(c, "Category not found")
			return
		}
		response.Error(c, "Failed to delete category")
		return
	}

	c.Status(http.StatusNoContent)
}
