package middleware

import (
	"context"
	"time"
)

type TokenClaims struct {
	UserID string
	Email  string
	Role   string
	JTI    string
}

type TokenValidator interface {
	ValidateToken(ctx context.Context, tokenString string) (*TokenClaims, error)
}

type jwtValidator struct {
	secret         string
	blacklistRepo  TokenBlacklistRepository
}

type TokenBlacklistRepository interface {
	BlacklistToken(ctx context.Context, jti string, ttl time.Duration) error
	IsTokenBlacklisted(ctx context.Context, jti string) (bool, error)
}

func NewJWTValidator(secret string, blacklistRepo TokenBlacklistRepository) TokenValidator {
	return &jwtValidator{
		secret:        secret,
		blacklistRepo: blacklistRepo,
	}
}

func (v *jwtValidator) ValidateToken(ctx context.Context, tokenString string) (*TokenClaims, error) {
	claims, err := v.parseAndValidate(tokenString)
	if err != nil {
		return nil, err
	}

	if err := v.checkBlacklist(ctx, claims.JTI); err != nil {
		return nil, err
	}

	return claims, nil
}

func (v *jwtValidator) parseAndValidate(tokenString string) (*TokenClaims, error) {
	token, err := parseJWT(tokenString, []byte(v.secret))
	if err != nil {
		return nil, err
	}

	claims, err := extractClaims(token)
	if err != nil {
		return nil, err
	}

	return claims, nil
}

func (v *jwtValidator) checkBlacklist(ctx context.Context, jti string) error {
	if jti == "" {
		return nil
	}

	blacklisted, err := v.blacklistRepo.IsTokenBlacklisted(ctx, jti)
	if err != nil {
		return err
	}

	if blacklisted {
		return ErrTokenRevoked
	}

	return nil
}
