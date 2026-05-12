package services

import (
	"context"
	"errors"
	"time"

	"walletx-be/core/domain"
	"walletx-be/core/domain/dto"
	"walletx-be/core/ports"
	"walletx-be/pkg/models"
	"walletx-be/pkg/repository"

	"github.com/google/uuid"
)

type CreateBudgetInput = dto.CreateBudgetInput
type UpdateBudgetInput = dto.UpdateBudgetInput

type BudgetService interface {
	CreateBudget(ctx context.Context, userID uuid.UUID, input CreateBudgetInput) (*models.CategoryLimit, error)
	ListBudgets(ctx context.Context, userID uuid.UUID) ([]models.CategoryLimit, error)
	GetBudgetByID(ctx context.Context, id, userID uuid.UUID) (*models.CategoryLimit, error)
	GetBudgetProgress(ctx context.Context, userID uuid.UUID, targetDate time.Time) ([]BudgetProgressItem, error)
	UpdateBudget(ctx context.Context, id, userID uuid.UUID, input UpdateBudgetInput) (*models.CategoryLimit, error)
	DeleteBudget(ctx context.Context, id, userID uuid.UUID) error
}

type BudgetProgressItem struct {
	CategoryID   uuid.UUID `json:"category_id"`
	CategoryName string    `json:"category_name"`
	LimitAmount  float64   `json:"limit_amount"`
	SpentAmount  float64   `json:"spent_amount"`
}

type budgetService struct {
	budgetRepo  repository.BudgetRepository
	summaryRepo repository.SummaryRepository
	logger      ports.Logger
}

func NewBudgetService(budgetRepo repository.BudgetRepository, summaryRepo repository.SummaryRepository, logger ports.Logger) BudgetService {
	return &budgetService{
		budgetRepo:  budgetRepo,
		summaryRepo: summaryRepo,
		logger:      logger,
	}
}

func (s *budgetService) CreateBudget(ctx context.Context, userID uuid.UUID, input CreateBudgetInput) (*models.CategoryLimit, error) {
	if input.LimitAmount <= 0 {
		return nil, errors.New("limit_amount must be greater than 0")
	}

	existing, err := s.budgetRepo.FindByCategory(ctx, userID, input.CategoryID)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}

	if existing != nil && existing.IsActive {
		return nil, errors.New("conflict: budget already exists for this category")
	}

	var limit *models.CategoryLimit
	if existing != nil {
		existing.LimitAmount = input.LimitAmount
		existing.IsActive = true
		if err := s.budgetRepo.Update(ctx, existing); err != nil {
			return nil, err
		}
		limit = existing
	} else {
		limit = &models.CategoryLimit{
			UserID:      userID,
			CategoryID:  input.CategoryID,
			LimitAmount: input.LimitAmount,
			IsActive:    true,
		}
		if err := s.budgetRepo.Create(ctx, limit); err != nil {
			return nil, err
		}
	}

	return limit, nil
}

func (s *budgetService) ListBudgets(ctx context.Context, userID uuid.UUID) ([]models.CategoryLimit, error) {
	return s.budgetRepo.List(ctx, userID)
}

func (s *budgetService) GetBudgetByID(ctx context.Context, id, userID uuid.UUID) (*models.CategoryLimit, error) {
	return s.budgetRepo.GetByID(ctx, id, userID)
}

func (s *budgetService) GetBudgetProgress(ctx context.Context, userID uuid.UUID, targetDate time.Time) ([]BudgetProgressItem, error) {
	monthStart := time.Date(targetDate.Year(), targetDate.Month(), 1, 0, 0, 0, 0, targetDate.Location())
	monthEnd := monthStart.AddDate(0, 1, 0).AddDate(0, 0, -1)
	monthEnd = time.Date(monthEnd.Year(), monthEnd.Month(), monthEnd.Day(), 23, 59, 59, 999999999, monthEnd.Location())

	limits, err := s.budgetRepo.GetActiveByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if len(limits) == 0 {
		return []BudgetProgressItem{}, nil
	}

	monthlySpends, err := s.summaryRepo.GetSpendingByCategoryAndDateRange(ctx, userID, monthStart, monthEnd)
	if err != nil {
		return nil, err
	}

	monthlyMap := make(map[uuid.UUID]float64)
	for _, ms := range monthlySpends {
		if ms.CategoryID != nil {
			monthlyMap[*ms.CategoryID] = ms.TotalAmount
		}
	}

	var results []BudgetProgressItem
	for _, limit := range limits {
		results = append(results, BudgetProgressItem{
			CategoryID:   limit.CategoryID,
			CategoryName: limit.Category.Name,
			LimitAmount:  limit.LimitAmount,
			SpentAmount:  monthlyMap[limit.CategoryID],
		})
	}

	return results, nil
}

func (s *budgetService) UpdateBudget(ctx context.Context, id, userID uuid.UUID, input UpdateBudgetInput) (*models.CategoryLimit, error) {
	if input.LimitAmount <= 0 {
		return nil, errors.New("limit_amount must be greater than 0")
	}

	limit, err := s.budgetRepo.GetByID(ctx, id, userID)
	if err != nil {
		return nil, err
	}

	limit.LimitAmount = input.LimitAmount
	limit.IsActive = input.IsActive

	if err := s.budgetRepo.Update(ctx, limit); err != nil {
		return nil, err
	}

	return limit, nil
}

func (s *budgetService) DeleteBudget(ctx context.Context, id, userID uuid.UUID) error {
	return s.budgetRepo.Delete(ctx, id, userID)
}
