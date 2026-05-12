package handlers

import (
	"errors"
	"net/http"

	"walletx-be/core/domain"
	"walletx-be/pkg/services"
	"walletx-be/pkg/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type CategoryHandler struct {
	categoryService services.CategoryService
}

func NewCategoryHandler(categoryService services.CategoryService) *CategoryHandler {
	return &CategoryHandler{categoryService: categoryService}
}

func parseUserID(c *gin.Context) (uuid.UUID, bool) {
	val, exists := c.Get("user_id")
	if !exists {
		utils.UnauthorizedResponse(c, "User not authenticated")
		return uuid.Nil, false
	}
	userUUID, err := uuid.Parse(val.(string))
	if err != nil {
		utils.FailResponseWithStatus(c, http.StatusBadRequest, "Invalid user ID format")
		return uuid.Nil, false
	}
	return userUUID, true
}

func (h *CategoryHandler) CreateCategory(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}

	var input struct {
		Name string `json:"name" binding:"required"`
		Icon string `json:"icon"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.FailResponseWithDetails(c, "Invalid request body", err.Error())
		return
	}

	category, err := h.categoryService.CreateCategory(c.Request.Context(), userID, input.Name, input.Icon)
	if err != nil {
		if errors.Is(err, domain.ErrDuplicate) {
			utils.FailResponseWithStatus(c, http.StatusConflict, "Category already exists")
			return
		}
		utils.ErrorResponse(c, err.Error())
		return
	}

	utils.SuccessResponseWithStatus(c, http.StatusCreated, category, "Category created successfully")
}

func (h *CategoryHandler) ListCategories(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}

	categories, err := h.categoryService.ListCategories(c.Request.Context(), userID)
	if err != nil {
		utils.ErrorResponse(c, "Failed to retrieve categories")
		return
	}

	utils.SuccessResponse(c, categories, "Categories retrieved successfully")
}

func (h *CategoryHandler) GetCategoryByID(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.FailResponseWithStatus(c, http.StatusBadRequest, "Invalid category ID")
		return
	}

	category, err := h.categoryService.GetCategoryByID(c.Request.Context(), id, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			utils.NotFoundResponse(c, "Category not found")
			return
		}
		utils.ErrorResponse(c, "Failed to retrieve category")
		return
	}

	utils.SuccessResponse(c, category, "Category retrieved successfully")
}

func (h *CategoryHandler) UpdateCategory(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.FailResponseWithStatus(c, http.StatusBadRequest, "Invalid category ID")
		return
	}

	var input struct {
		Name string `json:"name" binding:"required"`
		Icon string `json:"icon"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.FailResponseWithDetails(c, "Invalid request body", err.Error())
		return
	}

	category, err := h.categoryService.UpdateCategory(c.Request.Context(), id, userID, input.Name, input.Icon)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			utils.NotFoundResponse(c, "Category not found")
			return
		}
		utils.ErrorResponse(c, err.Error())
		return
	}

	utils.SuccessResponse(c, category, "Category updated successfully")
}

func (h *CategoryHandler) DeleteCategory(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.FailResponseWithStatus(c, http.StatusBadRequest, "Invalid category ID")
		return
	}

	if err := h.categoryService.DeleteCategory(c.Request.Context(), id, userID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			utils.NotFoundResponse(c, "Category not found")
			return
		}
		utils.ErrorResponse(c, "Failed to delete category")
		return
	}

	c.Status(http.StatusNoContent)
}
