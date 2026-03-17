package services

import (
	"walletx-be/internal/repository"

	"github.com/google/uuid"
)

// BudgetSummaryItem is the response DTO for a single budget line in the dashboard
type BudgetSummaryItem struct {
	CategoryID      *uuid.UUID `json:"category_id"`
	LimitAmount     float64    `json:"limit_amount"`
	SpentAmount     float64    `json:"spent_amount"`
	RemainingBudget float64    `json:"remaining_budget"`
	Period          string     `json:"period"`
}

// DashboardService defines the contract for dashboard-level aggregations
type DashboardService interface {
	GetBudgetSummary(userID uuid.UUID) ([]BudgetSummaryItem, error)
	GetDailyTotal(userID uuid.UUID, month int, year int) ([]repository.DailyTotalDTO, error)
}

type dashboardService struct {
	budgetRepo  repository.BudgetRepository
	summaryRepo repository.SummaryRepository
}

func NewDashboardService(budgetRepo repository.BudgetRepository, summaryRepo repository.SummaryRepository) DashboardService {
	return &dashboardService{
		budgetRepo:  budgetRepo,
		summaryRepo: summaryRepo,
	}
}

// GetBudgetSummary merges the user's active budget limits with their actual spending
// from the daily_expense_summary VIEW, computing the remaining budget per category.
func (s *dashboardService) GetBudgetSummary(userID uuid.UUID) ([]BudgetSummaryItem, error) {
	// Fetch all active budget limits for this user
	limits, err := s.budgetRepo.GetActiveByUserID(userID)
	if err != nil {
		return nil, err
	}

	if len(limits) == 0 {
		return []BudgetSummaryItem{}, nil
	}

	// Pre-fetch spending summaries for all relevant periods
	// Use a compound key map (CategoryID + Period) to prevent data overwriting
	// and to avoid executing database queries inside the loop (solving the N+1 problem).
	spendMap := make(map[string]float64)

	for _, period := range []string{"monthly", "weekly"} {
		summaries, err := s.summaryRepo.GetSummary(userID, period)
		if err != nil {
			return nil, err
		}

		for _, sum := range summaries {
			if sum.CategoryID == nil {
				continue
			}
			// Create a unique key combining Category UUID and Period string
			// Example: "550e8400-e29b-41d4-a716-446655440000:monthly"
			key := sum.CategoryID.String() + ":" + period
			spendMap[key] = sum.TotalAmount
		}
	}

	// Build the response by merging limits with the pre-fetched spending data
	result := make([]BudgetSummaryItem, 0, len(limits))
	for _, limit := range limits {
		// Construct the same compound key to look up the spent amount in memory
		key := limit.CategoryID.String() + ":" + limit.Period
		
		// If the key doesn't exist in the map, spent will automatically be 0.0
		spent := spendMap[key]

		catID := limit.CategoryID
		result = append(result, BudgetSummaryItem{
			CategoryID:      &catID,
			LimitAmount:     limit.LimitAmount,
			SpentAmount:     spent,
			RemainingBudget: limit.LimitAmount - spent,
			Period:          limit.Period,
		})
	}

	return result, nil
}

// GetDailyTotal fetches aggregated daily spending for a given month and year
func (s *dashboardService) GetDailyTotal(userID uuid.UUID, month int, year int) ([]repository.DailyTotalDTO, error) {
	return s.summaryRepo.GetDailyTotal(userID, month, year)
}