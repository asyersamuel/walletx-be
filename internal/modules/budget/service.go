package budget

import (
	"context"
	"errors"
	"time"

	"walletx-be/internal/platform/logger"
	"walletx-be/internal/shared/errors"

	"github.com/google/uuid"
)

// Service encapsulate budget business logic.
type Service interface {
	Create(ctx context.Context, userID uuid.UUID, input CreateInput) (*CategoryLimit, error)
	List(ctx context.Context, userID uuid.UUID) ([]CategoryLimit, error)
	GetByID(ctx context.Context, id, userID uuid.UUID) (*CategoryLimit, error)
	GetProgress(ctx context.Context, userID uuid.UUID, targetDate time.Time) ([]ProgressItem, error)
	Update(ctx context.Context, id, userID uuid.UUID, input UpdateInput) (*CategoryLimit, error)
	Delete(ctx context.Context, id, userID uuid.UUID) error
}

type service struct {
	repo         Repository
	summaryRepo  SummaryRepository
	logger       logger.Logger
}

func NewService(repo Repository, summaryRepo SummaryRepository, logger logger.Logger) Service {
	return &service{
		repo:        repo,
		summaryRepo: summaryRepo,
		logger:      logger,
	}
}

func (s *service) Create(ctx context.Context, userID uuid.UUID, input CreateInput) (*CategoryLimit, error) {
	if input.LimitAmount <= 0 {
		return nil, errors.New("limit_amount must be greater than 0")
	}

	existing, err := s.repo.FindByCategory(ctx, userID, input.CategoryID)
	if err != nil && !errors.Is(err, apperrors.ErrNotFound) {
		return nil, err
	}

	if existing != nil && existing.IsActive {
		return nil, errors.New("conflict: budget already exists for this category")
	}

	var limit *CategoryLimit
	if existing != nil {
		existing.LimitAmount = input.LimitAmount
		existing.IsActive = true
		if err := s.repo.Update(ctx, existing); err != nil {
			return nil, err
		}
		limit = existing
	} else {
		limit = &CategoryLimit{
			UserID:      userID,
			CategoryID:  input.CategoryID,
			LimitAmount: input.LimitAmount,
			IsActive:    true,
		}
		if err := s.repo.Create(ctx, limit); err != nil {
			return nil, err
		}
	}

	return limit, nil
}

func (s *service) List(ctx context.Context, userID uuid.UUID) ([]CategoryLimit, error) {
	return s.repo.List(ctx, userID)
}

func (s *service) GetByID(ctx context.Context, id, userID uuid.UUID) (*CategoryLimit, error) {
	return s.repo.GetByID(ctx, id, userID)
}

func (s *service) GetProgress(ctx context.Context, userID uuid.UUID, targetDate time.Time) ([]ProgressItem, error) {
	monthStart := time.Date(targetDate.Year(), targetDate.Month(), 1, 0, 0, 0, 0, targetDate.Location())
	monthEnd := monthStart.AddDate(0, 1, 0).AddDate(0, 0, -1)
	monthEnd = time.Date(monthEnd.Year(), monthEnd.Month(), monthEnd.Day(), 23, 59, 59, 999999999, monthEnd.Location())

	limits, err := s.repo.GetActiveByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if len(limits) == 0 {
		return []ProgressItem{}, nil
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

	var results []ProgressItem
	for _, limit := range limits {
		results = append(results, ProgressItem{
			CategoryID:   limit.CategoryID,
			CategoryName: limit.Category.Name,
			LimitAmount:  limit.LimitAmount,
			SpentAmount:  monthlyMap[limit.CategoryID],
		})
	}

	return results, nil
}

func (s *service) Update(ctx context.Context, id, userID uuid.UUID, input UpdateInput) (*CategoryLimit, error) {
	if input.LimitAmount <= 0 {
		return nil, errors.New("limit_amount must be greater than 0")
	}

	limit, err := s.repo.GetByID(ctx, id, userID)
	if err != nil {
		return nil, err
	}

	limit.LimitAmount = input.LimitAmount
	limit.IsActive = input.IsActive

	if err := s.repo.Update(ctx, limit); err != nil {
		return nil, err
	}

	return limit, nil
}

func (s *service) Delete(ctx context.Context, id, userID uuid.UUID) error {
	return s.repo.Delete(ctx, id, userID)
}
