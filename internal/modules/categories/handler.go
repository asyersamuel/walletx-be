package categories

import (
	"encoding/json"
	"errors"
	"net/http"

	"walletx-be/internal/platform/logger"
	apperrors "walletx-be/internal/shared/errors"
	"walletx-be/internal/shared/request"
	"walletx-be/internal/shared/response"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct {
	service CategoryService
	logger  logger.Logger
}

func NewHandler(service CategoryService, appLogger logger.Logger) *Handler {
	return &Handler{service: service, logger: appLogger}
}

func (h *Handler) Create(c *gin.Context) {
	userID, ok := request.ParseUserID(c)
	if !ok {
		return
	}

	input, ok := h.bindCreateRequest(c)
	if !ok {
		return
	}

	category, err := h.service.Create(c.Request.Context(), userID, input)
	if err != nil {
		h.handleError(c, "create_category", "Failed to create category", err)
		return
	}

	response.SuccessWithStatus(c, http.StatusCreated, CreateCategoryResponse{
		Category: toCategoryResponse(category),
	}, "Category created successfully")
}

// List handles GET /api/v1/categories with an optional ?type= query parameter.
//
// Behaviour:
//   - No query param → returns all categories (expense + income).
//   - ?type=expense  → returns expense categories only.
//   - ?type=income   → returns income categories only.
//   - ?type=<other>  → 400 Bad Request.
func (h *Handler) List(c *gin.Context) {
	userID, ok := request.ParseUserID(c)
	if !ok {
		return
	}

	var filterType *string
	if raw, exists := c.GetQuery("type"); exists {
		// Validate the query value eagerly before hitting the service.
		if raw != CategoryTypeExpense && raw != CategoryTypeIncome {
			response.FailWithStatus(c, http.StatusBadRequest,
				"invalid type filter: must be 'expense' or 'income'")
			return
		}
		filterType = &raw
	}

	categories, err := h.service.List(c.Request.Context(), userID, filterType)
	if err != nil {
		h.handleError(c, "list_categories", "Failed to retrieve categories", err)
		return
	}

	response.Success(c, ListCategoriesResponse{
		Categories: toCategoryResponseList(categories),
	}, "Categories retrieved successfully")
}

func (h *Handler) Update(c *gin.Context) {
	userID, ok := request.ParseUserID(c)
	if !ok {
		return
	}

	categoryID, ok := h.parseCategoryID(c)
	if !ok {
		return
	}

	input, ok := h.bindUpdateRequest(c)
	if !ok {
		return
	}

	category, err := h.service.Update(c.Request.Context(), userID, categoryID, input)
	if err != nil {
		h.handleError(c, "update_category", "Failed to update category", err)
		return
	}

	response.Success(c, CreateCategoryResponse{
		Category: toCategoryResponse(category),
	}, "Category updated successfully")
}

func (h *Handler) Delete(c *gin.Context) {
	userID, ok := request.ParseUserID(c)
	if !ok {
		return
	}

	categoryID, ok := h.parseCategoryID(c)
	if !ok {
		return
	}

	if err := h.service.Delete(c.Request.Context(), userID, categoryID); err != nil {
		h.handleError(c, "delete_category", "Failed to delete category", err)
		return
	}

	c.Status(http.StatusNoContent)
}

// bindCreateRequest decodes the JSON body with strict unknown-field rejection.
func (h *Handler) bindCreateRequest(c *gin.Context) (CreateCategoryRequest, bool) {
	var input CreateCategoryRequest
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		response.Fail(c, "Invalid request body")
		return input, false
	}
	return input, true
}

// bindUpdateRequest decodes the JSON body with strict unknown-field rejection.
func (h *Handler) bindUpdateRequest(c *gin.Context) (UpdateCategoryRequest, bool) {
	var input UpdateCategoryRequest
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		response.Fail(c, "Invalid request body")
		return input, false
	}
	return input, true
}

func (h *Handler) parseCategoryID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Fail(c, "Invalid category ID format")
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) handleError(c *gin.Context, operation, message string, err error) {
	switch {
	case errors.Is(err, apperrors.ErrInvalidInput):
		response.FailWithStatus(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, apperrors.ErrNotFound):
		response.NotFound(c, "Category not found")
	case errors.Is(err, apperrors.ErrConflict):
		response.FailWithStatus(c, http.StatusConflict,
			"A category with this name and type already exists")
	default:
		h.logger.WithError(err).WithField("operation", operation).Error("Category operation failed")
		response.Error(c, message)
	}
}
