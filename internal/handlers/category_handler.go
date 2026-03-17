package handlers

import (
	"errors"
	"net/http"

	"walletx-be/internal/services"
	"walletx-be/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type CategoryHandler struct {
	categoryService services.CategoryService
}

func NewCategoryHandler(categoryService services.CategoryService) *CategoryHandler {
	return &CategoryHandler{categoryService: categoryService}
}

// parseUserID is a shared helper to extract and validate the JWT user ID from context
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

// CreateCategory POST /api/v1/categories
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

	category, err := h.categoryService.CreateCategory(userID, input.Name, input.Icon)
	if err != nil {
		utils.ErrorResponse(c, err.Error())
		return
	}

	utils.SuccessResponseWithStatus(c, http.StatusCreated, category, "Category created successfully")
}

// ListCategories GET /api/v1/categories
func (h *CategoryHandler) ListCategories(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}

	categories, err := h.categoryService.ListCategories(userID)
	if err != nil {
		utils.ErrorResponse(c, "Failed to retrieve categories")
		return
	}

	utils.SuccessResponse(c, categories, "Categories retrieved successfully")
}

// GetCategoryByID GET /api/v1/categories/:id
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

	category, err := h.categoryService.GetCategoryByID(id, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.NotFoundResponse(c, "Category not found")
			return
		}
		utils.ErrorResponse(c, "Failed to retrieve category")
		return
	}

	utils.SuccessResponse(c, category, "Category retrieved successfully")
}

// UpdateCategory PUT /api/v1/categories/:id
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

	category, err := h.categoryService.UpdateCategory(id, userID, input.Name, input.Icon)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.NotFoundResponse(c, "Category not found")
			return
		}
		utils.ErrorResponse(c, err.Error())
		return
	}

	utils.SuccessResponse(c, category, "Category updated successfully")
}

// DeleteCategory DELETE /api/v1/categories/:id
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

	if err := h.categoryService.DeleteCategory(id, userID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.NotFoundResponse(c, "Category not found")
			return
		}
		utils.ErrorResponse(c, "Failed to delete category")
		return
	}

	utils.SuccessResponse(c, nil, "Category deleted successfully")
}
