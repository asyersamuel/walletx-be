package middleware

import (
	"errors"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidToken   = errors.New("invalid or expired token")
	ErrTokenRevoked   = errors.New("token has been revoked")
	ErrInvalidClaims  = errors.New("invalid token claims")
)

func parseJWT(tokenString string, secret []byte) (*jwt.Token, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return secret, nil
	})
	if err != nil || !token.Valid {
		return nil, ErrInvalidToken
	}
	return token, nil
}

func extractClaims(token *jwt.Token) (*TokenClaims, error) {
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrInvalidClaims
	}

	result := &TokenClaims{}

	if userID, exists := claims["user_id"].(string); exists {
		result.UserID = userID
	}

	if email, exists := claims["email"].(string); exists {
		result.Email = email
	}

	if role, exists := claims["role"].(string); exists {
		result.Role = role
	}

	if jti, exists := claims["jti"].(string); exists {
		result.JTI = jti
	}

	return result, nil
}
