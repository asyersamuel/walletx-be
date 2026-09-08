package dashboard

import "github.com/google/uuid"

// SummaryItem is a single row of the budget-summary read model.
type SummaryItem struct {
	CategoryID      *uuid.UUID `json:"category_id"`
	LimitAmount     float64    `json:"limit_amount"`
	SpentAmount     float64    `json:"spent_amount"`
	RemainingBudget float64    `json:"remaining_budget"`
}
