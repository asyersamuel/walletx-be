package budget

import "github.com/google/uuid"

type CreateInput struct {
	CategoryID  uuid.UUID `json:"category_id" binding:"required"`
	LimitAmount float64   `json:"limit_amount" binding:"required,gt=0"`
}

type UpdateInput struct {
	LimitAmount float64 `json:"limit_amount" binding:"required,gt=0"`
	IsActive    bool    `json:"is_active"`
}

// ProgressItem is a single row of the budget-progress report.
type ProgressItem struct {
	CategoryID   uuid.UUID `json:"category_id"`
	CategoryName string    `json:"category_name"`
	LimitAmount  float64   `json:"limit_amount"`
	SpentAmount  float64   `json:"spent_amount"`
}
