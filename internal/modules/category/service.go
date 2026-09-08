package category

import (
	"context"
	"errors"

	"walletx-be/internal/platform/logger"

	"github.com/google/uuid"
)

// Service encapsulate category CRUD business logic.
type Service interface {
	Create(ctx context.Context, userID uuid.UUID, input CreateInput) (*Category, error)
	List(ctx context.Context, userID uuid.UUID) ([]Category, error)
	GetByID(ctx context.Context, id, userID uuid.UUID) (*Category, error)
	Update(ctx context.Context, id, userID uuid.UUID, input UpdateInput) (*Category, error)
	Delete(ctx context.Context, id, userID uuid.UUID) error
}

type service struct {
	repo   Repository
	logger logger.Logger
}

func NewService(repo Repository, logger logger.Logger) Service {
	return &service{
		repo:   repo,
		logger: logger,
	}
}

func (s *service) Create(ctx context.Context, userID uuid.UUID, input CreateInput) (*Category, error) {
	if input.Name == "" {
		return nil, errors.New("category name cannot be empty")
	}

	category := &Category{
		UserID: userID,
		Name:   input.Name,
		Icon:   input.Icon,
	}

	if err := s.repo.Create(ctx, category); err != nil {
		return nil, err
	}
	return category, nil
}

func (s *service) List(ctx context.Context, userID uuid.UUID) ([]Category, error) {
	return s.repo.List(ctx, userID)
}

func (s *service) GetByID(ctx context.Context, id, userID uuid.UUID) (*Category, error) {
	return s.repo.GetByID(ctx, id, userID)
}

func (s *service) Update(ctx context.Context, id, userID uuid.UUID, input UpdateInput) (*Category, error) {
	if input.Name == "" {
		return nil, errors.New("category name cannot be empty")
	}

	category, err := s.repo.GetByID(ctx, id, userID)
	if err != nil {
		return nil, err
	}

	category.Name = input.Name
	category.Icon = input.Icon

	if err := s.repo.Update(ctx, category); err != nil {
		return nil, err
	}
	return category, nil
}

func (s *service) Delete(ctx context.Context, id, userID uuid.UUID) error {
	return s.repo.Delete(ctx, id, userID)
}
