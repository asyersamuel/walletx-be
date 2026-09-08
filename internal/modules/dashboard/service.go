package dashboard

import (
	"context"
	"fmt"
	"time"

	"walletx-be/internal/modules/budget"
	"walletx-be/internal/platform/cache"
	"walletx-be/internal/platform/logger"

	"github.com/google/uuid"
)

// Service encapsulate dashboard/read-model business logic.
type Service interface {
	GetBudgetSummary(ctx context.Context, userID uuid.UUID) ([]SummaryItem, error)
	GetDailyTotal(ctx context.Context, userID uuid.UUID, month int, year int) ([]budget.DailyTotalDTO, error)
}

type service struct {
	budgetRepo   budget.Repository
	summaryRepo  budget.SummaryRepository
	cacheRepo    cache.CacheRepository
	logger       logger.Logger
}

func NewService(budgetRepo budget.Repository, summaryRepo budget.SummaryRepository, cacheRepo cache.CacheRepository, logger logger.Logger) Service {
	return &service{
		budgetRepo:  budgetRepo,
		summaryRepo: summaryRepo,
		cacheRepo:   cacheRepo,
		logger:      logger,
	}
}

func (s *service) GetBudgetSummary(ctx context.Context, userID uuid.UUID) ([]SummaryItem, error) {
	cacheKey := fmt.Sprintf("cache:dashboard:budget_summary:%s", userID.String())

	var cached []SummaryItem
	if err := s.cacheRepo.GetCache(ctx, cacheKey, &cached); err == nil {
		return cached, nil
	}

	limits, err := s.budgetRepo.GetActiveByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if len(limits) == 0 {
		return []SummaryItem{}, nil
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

	result := make([]SummaryItem, 0, len(limits))
	for _, limit := range limits {
		spent := spendMap[limit.CategoryID]
		catID := limit.CategoryID

		result = append(result, SummaryItem{
			CategoryID:      &catID,
			LimitAmount:     limit.LimitAmount,
			SpentAmount:     spent,
			RemainingBudget: limit.LimitAmount - spent,
		})
	}

	_ = s.cacheRepo.SetCache(ctx, cacheKey, result, 1*time.Hour)

	return result, nil
}

func (s *service) GetDailyTotal(ctx context.Context, userID uuid.UUID, month int, year int) ([]budget.DailyTotalDTO, error) {
	cacheKey := fmt.Sprintf("cache:dashboard:daily_total:%s:%d:%d", userID.String(), month, year)

	var cached []budget.DailyTotalDTO
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
