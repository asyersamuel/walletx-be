package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"walletx-be/internal/middleware"
	"walletx-be/internal/platform/logger"
	apperrors "walletx-be/internal/shared/errors"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"google.golang.org/api/idtoken"
)

// UserStore is the narrow capability the auth service needs from persistence.
type UserStore interface {
	FindByGoogleID(ctx context.Context, googleID string) (*User, error)
	FindByGoogleIDAny(ctx context.Context, googleID string) (*User, error)
	FindByEmail(ctx context.Context, email string) (*User, error)
	CreateWithDefaultCategories(ctx context.Context, user *User) error
	UpdateGoogleProfile(ctx context.Context, userID uuid.UUID, name, picture string) (*User, error)
}

// TokenVerifier verifies a Google ID token and returns its validated payload.
// It is the seam that keeps the auth service testable without network I/O.
type TokenVerifier interface {
	Validate(ctx context.Context, idToken string) (*idtoken.Payload, error)
}

// googleTokenVerifier is the production TokenVerifier backed by Google's
// idtoken package. The OAuth client ID is the expected token audience.
type googleTokenVerifier struct {
	clientID string
}

func NewGoogleTokenVerifier(oauthClientID string) TokenVerifier {
	return &googleTokenVerifier{clientID: oauthClientID}
}

func (v *googleTokenVerifier) Validate(ctx context.Context, token string) (*idtoken.Payload, error) {
	return idtoken.Validate(ctx, token, v.clientID)
}

// Service encapsulates authentication concerns: Google SSO sign-in, JWT issuance, and token invalidation.
type Service interface {
	ProcessGoogleAuth(ctx context.Context, input GoogleAuthInput) (*User, bool, error)
	GenerateJWT(user *User) (string, error)
	Logout(ctx context.Context, tokenString string) error
}

type service struct {
	userStore     UserStore
	verifier      TokenVerifier
	logger        logger.Logger
	jwtSecret     string
	jwtExpiration int
	blacklistRepo middleware.TokenBlacklistRepository
}

func NewService(userStore UserStore, verifier TokenVerifier, appLogger logger.Logger, jwtSecret string, jwtExpiration int, blacklistRepo middleware.TokenBlacklistRepository) Service {
	return &service{
		userStore:     userStore,
		verifier:      verifier,
		logger:        appLogger,
		jwtSecret:     jwtSecret,
		jwtExpiration: jwtExpiration,
		blacklistRepo: blacklistRepo,
	}
}

func (s *service) ProcessGoogleAuth(ctx context.Context, input GoogleAuthInput) (*User, bool, error) {
	payload, err := s.verifier.Validate(ctx, input.IDToken)
	if err != nil {
		return nil, false, fmt.Errorf("%w: google token validation failed", apperrors.ErrUnauthorized)
	}

	googleID := payload.Subject
	if googleID == "" {
		return nil, false, fmt.Errorf("%w: subject claim missing", apperrors.ErrUnauthorized)
	}

	emailRaw, ok := payload.Claims["email"].(string)
	if !ok || strings.TrimSpace(emailRaw) == "" {
		return nil, false, fmt.Errorf("%w: email claim missing or invalid", apperrors.ErrUnauthorized)
	}
	email := strings.ToLower(strings.TrimSpace(emailRaw))

	emailVerified, _ := payload.Claims["email_verified"].(bool)
	if !emailVerified {
		return nil, false, fmt.Errorf("%w: email not verified by provider", apperrors.ErrUnauthorized)
	}

	nameRaw, ok := payload.Claims["name"].(string)
	if !ok || strings.TrimSpace(nameRaw) == "" {
		return nil, false, fmt.Errorf("%w: name claim missing or invalid", apperrors.ErrUnauthorized)
	}
	name := strings.TrimSpace(nameRaw)

	picture := ""
	if pictureRaw, ok := payload.Claims["picture"].(string); ok {
		picture = strings.TrimSpace(pictureRaw)
	}

	existingUser, err := s.userStore.FindByGoogleID(ctx, googleID)
	if err != nil {
		return nil, false, err
	}

	if existingUser != nil {
		if existingUser.Name != name || existingUser.Picture != picture {
			updated, err := s.userStore.UpdateGoogleProfile(ctx, existingUser.ID, name, picture)
			if err != nil {
				return nil, false, err
			}
			return updated, false, nil
		}
		return existingUser, false, nil
	}

	deletedUser, err := s.userStore.FindByGoogleIDAny(ctx, googleID)
	if err != nil {
		return nil, false, err
	}
	if deletedUser != nil {
		return nil, false, fmt.Errorf("%w: google_id belongs to a deleted account", apperrors.ErrAccountDeleted)
	}

	emailUser, err := s.userStore.FindByEmail(ctx, email)
	if err != nil && !errors.Is(err, apperrors.ErrNotFound) {
		return nil, false, err
	}
	if emailUser != nil {
		return nil, false, fmt.Errorf("%w: email already registered with a different provider identity", apperrors.ErrConflict)
	}

	newUser := &User{
		GoogleID: googleID,
		Email:    email,
		Name:     name,
		Picture:  picture,
	}

	if err := s.userStore.CreateWithDefaultCategories(ctx, newUser); err != nil {
		return nil, false, err
	}

	return newUser, true, nil
}

func (s *service) GenerateJWT(user *User) (string, error) {
	jti := uuid.New().String()
	claims := jwt.MapClaims{
		"user_id": user.ID.String(),
		"email":   user.Email,
		"jti":     jti,
		"exp":     time.Now().Add(time.Hour * time.Duration(s.jwtExpiration)).Unix(),
		"iat":     time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.jwtSecret))
}

func (s *service) Logout(ctx context.Context, tokenString string) error {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return []byte(s.jwtSecret), nil
	})
	if err != nil {
		return fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return fmt.Errorf("invalid token claims")
	}

	jti, exists := claims["jti"].(string)
	if !exists || jti == "" {
		return fmt.Errorf("token missing JTI claim")
	}

	exp, ok := claims["exp"].(float64)
	if !ok {
		return fmt.Errorf("token missing exp claim")
	}

	ttl := time.Until(time.Unix(int64(exp), 0))
	if ttl <= 0 {
		return nil
	}

	return s.blacklistRepo.BlacklistToken(ctx, jti, ttl)
}
