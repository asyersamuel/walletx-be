package auth

import (
	"context"
	"fmt"
	"time"

	"walletx-be/internal/middleware"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"google.golang.org/api/idtoken"
)

// UserStore is the narrow capability the auth service needs from persistence.
type UserStore interface {
	FindByGoogleID(ctx context.Context, googleID string) (*User, error)
	Create(ctx context.Context, user *User) error
	Update(ctx context.Context, user *User) error
}

// Service encapsulates authentication concerns: Google SSO sign-in, JWT
// issuance, and token invalidation.
type Service interface {
	ProcessGoogleAuth(ctx context.Context, input GoogleAuthInput) (*User, bool, error)
	GenerateJWT(user *User) (string, error)
	Logout(ctx context.Context, tokenString string) error
}

type service struct {
	userStore      UserStore
	oauthClientID  string
	jwtSecret      string
	jwtExpiration  int
	blacklistRepo  middleware.TokenBlacklistRepository
}

func NewService(userStore UserStore, oauthClientID, jwtSecret string, jwtExpiration int, blacklistRepo middleware.TokenBlacklistRepository) Service {
	return &service{
		userStore:      userStore,
		oauthClientID:  oauthClientID,
		jwtSecret:      jwtSecret,
		jwtExpiration:  jwtExpiration,
		blacklistRepo:  blacklistRepo,
	}
}

func (s *service) ProcessGoogleAuth(ctx context.Context, input GoogleAuthInput) (*User, bool, error) {
	payload, err := idtoken.Validate(ctx, input.IDToken, s.oauthClientID)
	if err != nil {
		return nil, false, fmt.Errorf("invalid google id_token: %v", err)
	}

	googleID := payload.Subject
	email := fmt.Sprintf("%v", payload.Claims["email"])
	name := fmt.Sprintf("%v", payload.Claims["name"])
	picture := ""
	if payload.Claims["picture"] != nil {
		picture = fmt.Sprintf("%v", payload.Claims["picture"])
	}

	existingUser, err := s.userStore.FindByGoogleID(ctx, googleID)
	if err != nil {
		return nil, false, err
	}

	if existingUser != nil {
		existingUser.Name = name
		existingUser.Picture = picture
		s.userStore.Update(ctx, existingUser)
		return existingUser, false, nil
	}

	newUser := &User{
		GoogleID: googleID,
		Email:    email,
		Name:     name,
		Picture:  picture,
	}

	err = s.userStore.Create(ctx, newUser)
	if err != nil {
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
