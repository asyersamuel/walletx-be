package transaction

import (
	"time"

	"github.com/google/uuid"
)

type CreateTransactionInput struct {
	Amount          float64    `json:"amount" binding:"required,gt=0"`
	Merchant        string     `json:"merchant" binding:"required"`
	Note            string     `json:"note"`
	CategoryID      *uuid.UUID `json:"category_id"`
	TransactionDate time.Time  `json:"transaction_date" binding:"required"`
}

type UpdateTransactionInput struct {
	Amount          *float64   `json:"amount"`
	Merchant        *string    `json:"merchant"`
	Note            *string    `json:"note"`
	CategoryID      *uuid.UUID `json:"category_id"`
	TransactionDate *time.Time `json:"transaction_date"`
}

// TransactionFilters are the query filters accepted by the list/search endpoints.
type TransactionFilters struct {
	DateFrom       string
	DateTo         string
	AmountMin      *float64
	AmountMax      *float64
	LastUpdated    string
	ExcludeDeleted bool
}

// SortOption defines the sort field and direction for list queries.
type SortOption struct {
	Field     string
	Direction string
}
