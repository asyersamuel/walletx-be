package users

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"walletx-be/internal/platform/logger"
	apperrors "walletx-be/internal/shared/errors"

	"github.com/google/uuid"
)

const (
	maxNameLength    = 100
	maxPictureLength = 2048
)

// TokenRevoker is the narrow capability needed to invalidate the active JWT
// after account deletion. It is satisfied by the auth service.
type TokenRevoker interface {
	Logout(ctx context.Context, tokenString string) error
}

// Service encapsulates user profile management: self-service edit and account deletion.
type Service interface {
	UpdateProfile(ctx context.Context, userID uuid.UUID, input UpdateUserRequest) (*User, error)
	DeleteAccount(ctx context.Context, userID uuid.UUID, activeToken string) error
}

type service struct {
	repo    UserRepository
	revoker TokenRevoker
	logger  logger.Logger
}

func NewService(repo UserRepository, revoker TokenRevoker, logger logger.Logger) Service {
	return &service{repo: repo, revoker: revoker, logger: logger}
}

func (s *service) UpdateProfile(ctx context.Context, userID uuid.UUID, input UpdateUserRequest) (*User, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: name must not be empty", apperrors.ErrInvalidInput)
	}
	if len(name) > maxNameLength {
		return nil, fmt.Errorf("%w: name must not exceed %d characters", apperrors.ErrInvalidInput, maxNameLength)
	}

	user, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	picture := user.Picture
	if input.Picture != nil {
		candidate := strings.TrimSpace(*input.Picture)
		if candidate == "" {
			picture = ""
		} else {
			if len(candidate) > maxPictureLength {
				return nil, fmt.Errorf("%w: picture URL must not exceed %d characters", apperrors.ErrInvalidInput, maxPictureLength)
			}
			parsed, parseErr := url.Parse(candidate)
			if parseErr != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
				return nil, fmt.Errorf("%w: picture must be a valid http/https URL", apperrors.ErrInvalidInput)
			}
			picture = candidate
		}
	}

	return s.repo.UpdateProfile(ctx, userID, name, &picture)
}

// DeleteAccount soft-deletes the account and revokes the active token.
// The operation is idempotent: deleting an already-deleted account succeeds.
func (s *service) DeleteAccount(ctx context.Context, userID uuid.UUID, activeToken string) error {
	if err := s.repo.SoftDelete(ctx, userID); err != nil {
		return err
	}

	if activeToken != "" {
		if err := s.revoker.Logout(ctx, activeToken); err != nil {
			s.logger.WithError(err).WithField("operation", "delete_account_token_revoke").
				Warn("Account deleted but active token revocation failed")
		}
	}

	return nil
}
