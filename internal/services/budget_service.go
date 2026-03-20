package services

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"walletx-be/internal/models"
	"walletx-be/internal/repository"

	"github.com/google/uuid"
)

// BudgetService defines the business logic contract for category limits
type BudgetService interface {
	CreateBudget(userID uuid.UUID, input CreateBudgetInput) (*models.CategoryLimit, error)
	ListBudgets(userID uuid.UUID) ([]models.CategoryLimit, error)
	GetBudgetByID(id, userID uuid.UUID) (*models.CategoryLimit, error)
	GetBudgetProgress(userID uuid.UUID, targetDate time.Time) ([]BudgetProgressItem, error)
	UpdateBudget(id, userID uuid.UUID, input UpdateBudgetInput) (*models.CategoryLimit, error)
	DeleteBudget(id, userID uuid.UUID) error
}

type BudgetProgressItem struct {
	CategoryID   uuid.UUID `json:"category_id"`
	CategoryName string    `json:"category_name"`
	LimitAmount  float64   `json:"limit_amount"`
	SpentAmount  float64   `json:"spent_amount"`
}

// CreateBudgetInput is the DTO for creating a new category limit
type CreateBudgetInput struct {
	CategoryID  uuid.UUID `json:"category_id" binding:"required"`
	LimitAmount float64   `json:"limit_amount" binding:"required,gt=0"`
}

// UpdateBudgetInput is the DTO for updating an existing category limit
type UpdateBudgetInput struct {
	LimitAmount float64 `json:"limit_amount" binding:"required,gt=0"`
	IsActive    bool    `json:"is_active"`
}

type budgetService struct {
	budgetRepo  repository.BudgetRepository
	summaryRepo repository.SummaryRepository
	db          *gorm.DB
}

func NewBudgetService(budgetRepo repository.BudgetRepository, summaryRepo repository.SummaryRepository, db *gorm.DB) BudgetService {
	return &budgetService{
		budgetRepo:  budgetRepo,
		summaryRepo: summaryRepo,
		db:          db,
	}
}

func (s *budgetService) CreateBudget(userID uuid.UUID, input CreateBudgetInput) (*models.CategoryLimit, error) {
	if input.LimitAmount <= 0 {
		return nil, errors.New("limit_amount must be greater than 0")
	}

	existing, err := s.budgetRepo.FindByCategory(userID, input.CategoryID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	if existing != nil && existing.IsActive {
		return nil, errors.New("conflict: budget already exists for this category")
	}

	var limit *models.CategoryLimit
	if existing != nil {
		existing.LimitAmount = input.LimitAmount
		existing.IsActive = true
		if err := s.budgetRepo.Update(existing); err != nil {
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
		if err := s.budgetRepo.Create(limit); err != nil {
			return nil, err
		}
	}

	return limit, nil
}

func (s *budgetService) ListBudgets(userID uuid.UUID) ([]models.CategoryLimit, error) {
	return s.budgetRepo.List(userID)
}

func (s *budgetService) GetBudgetByID(id, userID uuid.UUID) (*models.CategoryLimit, error) {
	return s.budgetRepo.GetByID(id, userID)
}

func (s *budgetService) GetBudgetProgress(userID uuid.UUID, targetDate time.Time) ([]BudgetProgressItem, error) {
	// Monthly: 1st of month to last of month
	monthStart := time.Date(targetDate.Year(), targetDate.Month(), 1, 0, 0, 0, 0, targetDate.Location())
	monthEnd := monthStart.AddDate(0, 1, 0).AddDate(0, 0, -1)
	monthEnd = time.Date(monthEnd.Year(), monthEnd.Month(), monthEnd.Day(), 23, 59, 59, 999999999, monthEnd.Location())

	// Fetch active budgets for user (preloads Category)
	limits, err := s.budgetRepo.GetActiveByUserID(userID)
	if err != nil {
		return nil, err
	}

	if len(limits) == 0 {
		return []BudgetProgressItem{}, nil
	}

	monthlySpends, err := s.summaryRepo.GetSpendingByCategoryAndDateRange(userID, monthStart, monthEnd)
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

func (s *budgetService) UpdateBudget(id, userID uuid.UUID, input UpdateBudgetInput) (*models.CategoryLimit, error) {
	if input.LimitAmount <= 0 {
		return nil, errors.New("limit_amount must be greater than 0")
	}

	limit, err := s.budgetRepo.GetByID(id, userID)
	if err != nil {
		return nil, err
	}

	limit.LimitAmount = input.LimitAmount
	limit.IsActive = input.IsActive

	if err := s.budgetRepo.Update(limit); err != nil {
		return nil, err
	}

	return limit, nil
}

func (s *budgetService) DeleteBudget(id, userID uuid.UUID) error {
	return s.budgetRepo.Delete(id, userID)
}
