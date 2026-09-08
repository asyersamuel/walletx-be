package budget

import (
	"errors"
	"net/http"
	"time"

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

	limit, err := h.service.Create(c.Request.Context(), userID, input)
	if err != nil {
		if errors.Is(err, apperrors.ErrDuplicate) {
			response.FailWithStatus(c, http.StatusConflict, "Budget already exists for this category")
			return
		}
		response.FailWithStatus(c, http.StatusBadRequest, err.Error())
		return
	}

	response.SuccessWithStatus(c, http.StatusCreated, limit, "Budget limit created successfully")
}

func (h *Handler) List(c *gin.Context) {
	userID, ok := request.ParseUserID(c)
	if !ok {
		return
	}

	limits, err := h.service.List(c.Request.Context(), userID)
	if err != nil {
		response.Error(c, "Failed to retrieve budget limits")
		return
	}

	response.Success(c, limits, "Budget limits retrieved successfully")
}

func (h *Handler) GetProgress(c *gin.Context) {
	userID, ok := request.ParseUserID(c)
	if !ok {
		return
	}

	dateStr := c.Query("date")
	targetDate := time.Now()
	if dateStr != "" {
		parsed, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			response.FailWithStatus(c, http.StatusBadRequest, "Invalid date format. Use YYYY-MM-DD")
			return
		}
		targetDate = parsed
	}

	progress, err := h.service.GetProgress(c.Request.Context(), userID, targetDate)
	if err != nil {
		response.Error(c, "Failed to retrieve budget progress")
		return
	}

	response.Success(c, progress, "Budget progress retrieved successfully")
}

func (h *Handler) GetByID(c *gin.Context) {
	userID, ok := request.ParseUserID(c)
	if !ok {
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.FailWithStatus(c, http.StatusBadRequest, "Invalid budget ID")
		return
	}

	limit, err := h.service.GetByID(c.Request.Context(), id, userID)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			response.NotFound(c, "Budget limit not found")
			return
		}
		response.Error(c, "Failed to retrieve budget limit")
		return
	}

	response.Success(c, limit, "Budget limit retrieved successfully")
}

func (h *Handler) Update(c *gin.Context) {
	userID, ok := request.ParseUserID(c)
	if !ok {
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.FailWithStatus(c, http.StatusBadRequest, "Invalid budget ID")
		return
	}

	var input UpdateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.FailWithDetails(c, "Invalid request body", err.Error())
		return
	}

	limit, err := h.service.Update(c.Request.Context(), id, userID, input)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			response.NotFound(c, "Budget limit not found")
			return
		}
		response.FailWithStatus(c, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(c, limit, "Budget limit updated successfully")
}

func (h *Handler) Delete(c *gin.Context) {
	userID, ok := request.ParseUserID(c)
	if !ok {
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.FailWithStatus(c, http.StatusBadRequest, "Invalid budget ID")
		return
	}

	if err := h.service.Delete(c.Request.Context(), id, userID); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			response.NotFound(c, "Budget limit not found")
			return
		}
		response.Error(c, "Failed to delete budget limit")
		return
	}

	c.Status(http.StatusNoContent)
}
