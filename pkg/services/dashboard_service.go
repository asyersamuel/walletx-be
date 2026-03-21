package services

import (
	"walletx-be/pkg/repository"

	"github.com/google/uuid"
)

// BudgetSummaryItem is the response DTO for a single budget line in the dashboard
type BudgetSummaryItem struct {
	CategoryID      *uuid.UUID `json:"category_id"`
	LimitAmount     float64    `json:"limit_amount"`
	SpentAmount     float64    `json:"spent_amount"`
	RemainingBudget float64    `json:"remaining_budget"`
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
	// Fetch all active budget limits
	limits, err := s.budgetRepo.GetActiveByUserID(userID)
	if err != nil {
		return nil, err
	}

	if len(limits) == 0 {
		return []BudgetSummaryItem{}, nil
	}

	// Fetch spending summaries for this month
	spendMap := make(map[uuid.UUID]float64)
	summaries, err := s.summaryRepo.GetSummary(userID, "monthly")
	if err != nil {
		return nil, err
	}

	for _, sum := range summaries {
		if sum.CategoryID != nil {
			spendMap[*sum.CategoryID] = sum.TotalAmount
		}
	}

	// Build the response by merging limits with the pre-fetched monthly spending data
	result := make([]BudgetSummaryItem, 0, len(limits))
	for _, limit := range limits {
		spent := spendMap[limit.CategoryID]
		catID := limit.CategoryID

		result = append(result, BudgetSummaryItem{
			CategoryID:      &catID,
			LimitAmount:     limit.LimitAmount,
			SpentAmount:     spent,
			RemainingBudget: limit.LimitAmount - spent,
		})
	}

	return result, nil
}

// GetDailyTotal fetches aggregated daily spending for a given month and year
func (s *dashboardService) GetDailyTotal(userID uuid.UUID, month int, year int) ([]repository.DailyTotalDTO, error) {
	return s.summaryRepo.GetDailyTotal(userID, month, year)
}