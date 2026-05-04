package services

import (
	"context"
	"fmt"
	"time"

	"walletx-be/internal/ports"
	"walletx-be/pkg/repository"

	"github.com/google/uuid"
)

type BudgetSummaryItem struct {
	CategoryID      *uuid.UUID `json:"category_id"`
	LimitAmount     float64    `json:"limit_amount"`
	SpentAmount     float64    `json:"spent_amount"`
	RemainingBudget float64    `json:"remaining_budget"`
}

type DashboardService interface {
	GetBudgetSummary(ctx context.Context, userID uuid.UUID) ([]BudgetSummaryItem, error)
	GetDailyTotal(ctx context.Context, userID uuid.UUID, month int, year int) ([]repository.DailyTotalDTO, error)
}

type dashboardService struct {
	budgetRepo  repository.BudgetRepository
	summaryRepo repository.SummaryRepository
	cacheRepo   repository.CacheRepository
	logger      ports.Logger
}

func NewDashboardService(budgetRepo repository.BudgetRepository, summaryRepo repository.SummaryRepository, cacheRepo repository.CacheRepository, logger ports.Logger) DashboardService {
	return &dashboardService{
		budgetRepo:  budgetRepo,
		summaryRepo: summaryRepo,
		cacheRepo:   cacheRepo,
		logger:      logger,
	}
}

func (s *dashboardService) GetBudgetSummary(ctx context.Context, userID uuid.UUID) ([]BudgetSummaryItem, error) {
	cacheKey := fmt.Sprintf("cache:dashboard:budget_summary:%s", userID.String())

	var cached []BudgetSummaryItem
	if err := s.cacheRepo.GetCache(ctx, cacheKey, &cached); err == nil {
		return cached, nil
	}

	limits, err := s.budgetRepo.GetActiveByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if len(limits) == 0 {
		return []BudgetSummaryItem{}, nil
	}

	spendMap := make(map[uuid.UUID]float64)
	summaries, err := s.summaryRepo.GetSummary(ctx, userID, "monthly")
	if err != nil {
		return nil, err
	}

	for _, sum := range summaries {
		if sum.CategoryID != nil {
			spendMap[*sum.CategoryID] = sum.TotalAmount
		}
	}

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

	_ = s.cacheRepo.SetCache(ctx, cacheKey, result, 1*time.Hour)

	return result, nil
}

func (s *dashboardService) GetDailyTotal(ctx context.Context, userID uuid.UUID, month int, year int) ([]repository.DailyTotalDTO, error) {
	cacheKey := fmt.Sprintf("cache:dashboard:daily_total:%s:%d:%d", userID.String(), month, year)

	var cached []repository.DailyTotalDTO
	if err := s.cacheRepo.GetCache(ctx, cacheKey, &cached); err == nil {
		return cached, nil
	}

	result, err := s.summaryRepo.GetDailyTotal(ctx, userID, month, year)
	if err != nil {
		return nil, err
	}

	_ = s.cacheRepo.SetCache(ctx, cacheKey, result, 1*time.Hour)

	return result, nil
}
