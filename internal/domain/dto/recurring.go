package dto

import (
	"time"

	"github.com/google/uuid"
)

type CreateRecurringInput struct {
	CategoryID uuid.UUID `json:"category_id" binding:"required"`
	Amount     float64   `json:"amount" binding:"required,gt=0"`
	Frequency  string    `json:"frequency" binding:"required"`
	StartDate  time.Time `json:"start_date" binding:"required"`
}

type UpdateRecurringInput struct {
	Amount    float64   `json:"amount" binding:"required,gt=0"`
	Frequency string    `json:"frequency" binding:"required"`
	StartDate time.Time `json:"start_date"`
}
