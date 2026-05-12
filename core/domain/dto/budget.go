package dto

import "github.com/google/uuid"

type CreateBudgetInput struct {
	CategoryID  uuid.UUID `json:"category_id" binding:"required"`
	LimitAmount float64   `json:"limit_amount" binding:"required,gt=0"`
}

type UpdateBudgetInput struct {
	LimitAmount float64 `json:"limit_amount" binding:"required,gt=0"`
	IsActive    bool    `json:"is_active"`
}
