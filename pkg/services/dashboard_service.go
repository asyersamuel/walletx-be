package services

import (
	"context"
	"fmt"
	"time"
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
	GetBudgetSummary(ctx context.Context, userID uuid.UUID) ([]BudgetSummaryItem, error)
	GetDailyTotal(ctx context.Context, userID uuid.UUID, month int, year int) ([]repository.DailyTotalDTO, error)
}

type dashboardService struct {
	budgetRepo  repository.BudgetRepository
	summaryRepo repository.SummaryRepository
	cacheRepo   repository.CacheRepository
}

func NewDashboardService(budgetRepo repository.BudgetRepository, summaryRepo repository.SummaryRepository, cacheRepo repository.CacheRepository) DashboardService {
	return &dashboardService{
		budgetRepo:  budgetRepo,
		summaryRepo: summaryRepo,
		cacheRepo:   cacheRepo,
	}
}

// GetBudgetSummary merges the user's active budget limits with their actual spending
// from the daily_expense_summary VIEW, computing the remaining budget per category.
func (s *dashboardService) GetBudgetSummary(ctx context.Context, userID uuid.UUID) ([]BudgetSummaryItem, error) {
	cacheKey := fmt.Sprintf("cache:dashboard:budget_summary:%s", userID.String())

	// Try cache first
	var cached []BudgetSummaryItem
	if err := s.cacheRepo.GetCache(ctx, cacheKey, &cached); err == nil {
		return cached, nil
	}

	// Cache miss - fetch from DB
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

	// Store in cache with 1 hour TTL (ignore cache errors)
	_ = s.cacheRepo.SetCache(ctx, cacheKey, result, 1*time.Hour)

	return result, nil
}

// GetDailyTotal fetches aggregated daily spending for a given month and year
func (s *dashboardService) GetDailyTotal(ctx context.Context, userID uuid.UUID, month int, year int) ([]repository.DailyTotalDTO, error) {
	cacheKey := fmt.Sprintf("cache:dashboard:daily_total:%s:%d:%d", userID.String(), month, year)

	// Try cache first
	var cached []repository.DailyTotalDTO
	if err := s.cacheRepo.GetCache(ctx, cacheKey, &cached); err == nil {
		return cached, nil
	}

	// Cache miss - fetch from DB
	result, err := s.summaryRepo.GetDailyTotal(userID, month, year)
	if err != nil {
		return nil, err
	}

	// Store in cache with 1 hour TTL (ignore cache errors)
	_ = s.cacheRepo.SetCache(ctx, cacheKey, result, 1*time.Hour)

	return result, nil
}