package services

import (
	"context"
	"fmt"
	"time"

	"walletx-be/configs"
	"walletx-be/pkg/models"
	"walletx-be/pkg/repository"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"google.golang.org/api/idtoken"
)

// DTO hanya menerima ID Token dari Frontend
type GoogleAuthInput struct {
	IDToken string `json:"id_token" binding:"required"`
}

type AuthService struct {
	userRepo repository.UserRepository
	cfg      *configs.Config
	repo     repository.TokenBlacklistRepository
}

func NewAuthService(userRepo repository.UserRepository, cfg *configs.Config, repo repository.TokenBlacklistRepository) *AuthService {
	return &AuthService{
		userRepo: userRepo,
		cfg:      cfg,
		repo:     repo,
	}
}

func (s *AuthService) ProcessGoogleAuth(input GoogleAuthInput) (*models.User, bool, error) {
	// Validate idToken to Google Server
	payload, err := idtoken.Validate(context.Background(), input.IDToken, s.cfg.OAuth.ClientID)
	if err != nil {
		return nil, false, fmt.Errorf("invalid google id_token: %v", err)
	}

	// Extract Data from Google
	googleID := payload.Subject
	email := fmt.Sprintf("%v", payload.Claims["email"])
	name := fmt.Sprintf("%v", payload.Claims["name"])
	picture := ""
	if payload.Claims["picture"] != nil {
		picture = fmt.Sprintf("%v", payload.Claims["picture"])
	}

	// Check if user exist on database
	existingUser, err := s.userRepo.FindByGoogleID(googleID)
	if err != nil {
		return nil, false, err
	}

	if existingUser != nil {
		existingUser.Name = name
		existingUser.Picture = picture
		s.userRepo.Update(existingUser)
		return existingUser, false, nil
	}

	newUser := &models.User{
		GoogleID: googleID,
		Email:    email,
		Name:     name,
		Picture:  picture,
	}

	err = s.userRepo.Create(newUser)
	if err != nil {
		return nil, false, err
	}

	return newUser, true, nil
}

func (s *AuthService) GenerateJWT(user *models.User) (string, error) {
	jti := uuid.New().String()
	claims := jwt.MapClaims{
		"user_id": user.ID.String(),
		"email":   user.Email,
		"jti":     jti,
		"exp":     time.Now().Add(time.Hour * time.Duration(s.cfg.JWT.Expiration)).Unix(),
		"iat":     time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.cfg.JWT.Secret))
}

func (s *AuthService) Logout(ctx context.Context, tokenString string) error {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return []byte(s.cfg.JWT.Secret), nil
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

	return s.repo.BlacklistToken(ctx, jti, ttl)
}