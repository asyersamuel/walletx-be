package services

import (
	"errors"
	"fmt"

	"walletx-be/internal/models"
	"walletx-be/internal/repository"

	"github.com/google/uuid"
)

var validPeriods = map[string]bool{"weekly": true, "monthly": true}

// BudgetService defines the business logic contract for category limits
type BudgetService interface {
	CreateBudget(userID uuid.UUID, input CreateBudgetInput) (*models.CategoryLimit, error)
	ListBudgets(userID uuid.UUID) ([]models.CategoryLimit, error)
	GetBudgetByID(id, userID uuid.UUID) (*models.CategoryLimit, error)
	UpdateBudget(id, userID uuid.UUID, input UpdateBudgetInput) (*models.CategoryLimit, error)
	DeleteBudget(id, userID uuid.UUID) error
}

// CreateBudgetInput is the DTO for creating a new category limit
type CreateBudgetInput struct {
	CategoryID  uuid.UUID `json:"category_id" binding:"required"`
	LimitAmount float64   `json:"limit_amount" binding:"required,gt=0"`
	Period      string    `json:"period" binding:"required"`
}

// UpdateBudgetInput is the DTO for updating an existing category limit
type UpdateBudgetInput struct {
	LimitAmount float64 `json:"limit_amount" binding:"required,gt=0"`
	Period      string  `json:"period" binding:"required"`
	IsActive    bool    `json:"is_active"`
}

type budgetService struct {
	budgetRepo repository.BudgetRepository
}

func NewBudgetService(budgetRepo repository.BudgetRepository) BudgetService {
	return &budgetService{budgetRepo: budgetRepo}
}

func (s *budgetService) CreateBudget(userID uuid.UUID, input CreateBudgetInput) (*models.CategoryLimit, error) {
	if !validPeriods[input.Period] {
		return nil, fmt.Errorf("invalid period '%s': must be 'weekly' or 'monthly'", input.Period)
	}
	if input.LimitAmount <= 0 {
		return nil, errors.New("limit_amount must be greater than 0")
	}

	limit := &models.CategoryLimit{
		UserID:      userID,
		CategoryID:  input.CategoryID,
		LimitAmount: input.LimitAmount,
		Period:      input.Period,
		IsActive:    true,
	}

	if err := s.budgetRepo.Create(limit); err != nil {
		return nil, err
	}
	return limit, nil
}

func (s *budgetService) ListBudgets(userID uuid.UUID) ([]models.CategoryLimit, error) {
	return s.budgetRepo.List(userID)
}

func (s *budgetService) GetBudgetByID(id, userID uuid.UUID) (*models.CategoryLimit, error) {
	return s.budgetRepo.GetByID(id, userID)
}

func (s *budgetService) UpdateBudget(id, userID uuid.UUID, input UpdateBudgetInput) (*models.CategoryLimit, error) {
	if !validPeriods[input.Period] {
		return nil, fmt.Errorf("invalid period '%s': must be 'weekly' or 'monthly'", input.Period)
	}
	if input.LimitAmount <= 0 {
		return nil, errors.New("limit_amount must be greater than 0")
	}

	limit, err := s.budgetRepo.GetByID(id, userID)
	if err != nil {
		return nil, err
	}

	limit.LimitAmount = input.LimitAmount
	limit.Period = input.Period
	limit.IsActive = input.IsActive

	if err := s.budgetRepo.Update(limit); err != nil {
		return nil, err
	}
	return limit, nil
}

func (s *budgetService) DeleteBudget(id, userID uuid.UUID) error {
	return s.budgetRepo.Delete(id, userID)
}
