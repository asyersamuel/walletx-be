package middleware

import (
	"context"
	"errors"
	"time"

	"walletx-be/internal/shared/response"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidToken   = errors.New("invalid or expired token")
	ErrTokenRevoked   = errors.New("token has been revoked")
	ErrInvalidClaims  = errors.New("invalid token claims")
)

// TokenClaims holds the parsed claims extracted from a JWT.
type TokenClaims struct {
	UserID string
	Email  string
	Role   string
	JTI    string
}

// TokenValidator validates and extracts claims from a JWT string.
type TokenValidator interface {
	ValidateToken(ctx context.Context, tokenString string) (*TokenClaims, error)
}

// TokenBlacklistRepository checks whether a JWT (by its jti claim) has been revoked.
type TokenBlacklistRepository interface {
	BlacklistToken(ctx context.Context, jti string, ttl time.Duration) error
	IsTokenBlacklisted(ctx context.Context, jti string) (bool, error)
}

type jwtValidator struct {
	secret         string
	blacklistRepo  TokenBlacklistRepository
}

func NewJWTValidator(secret string, blacklistRepo TokenBlacklistRepository) TokenValidator {
	return &jwtValidator{
		secret:        secret,
		blacklistRepo: blacklistRepo,
	}
}

// AuthMiddleware is the cross-cutting JWT authentication guard placed around
// all protected routes. It populates the Gin context with the authenticated
// user's identity (user_id, email, role).
func AuthMiddleware(validator TokenValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString := extractToken(c)
		if tokenString == "" {
			response.Unauthorized(c, "Token required")
			c.Abort()
			return
		}

		claims, err := validator.ValidateToken(c.Request.Context(), tokenString)
		if err != nil {
			if err == ErrTokenRevoked {
				response.Unauthorized(c, "Token has been revoked")
			} else {
				response.Unauthorized(c, "Invalid or expired token")
			}
			c.Abort()
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("email", claims.Email)
		if claims.Role != "" {
			c.Set("role", claims.Role)
		}

		c.Next()
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

func extractToken(c *gin.Context) string {
	authHeader := c.GetHeader("Authorization")
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		return authHeader[7:]
	}

	if token := c.Query("token"); token != "" {
		return token
	}

	return ""
}

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
