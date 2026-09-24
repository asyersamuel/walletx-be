package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// HTTP tests for AuthMiddleware + jwtValidator using the real middleware chain
// with a test secret. Only the blacklist repository is mocked; the dummy
// endpoint proves whether the guard actually lets a request through and what
// identity it injects into the Gin context.

const testSecret = "middleware-unit-test-secret"

type mockBlacklistRepository struct {
	mock.Mock
}

func (m *mockBlacklistRepository) BlacklistToken(ctx context.Context, jti string, ttl time.Duration) error {
	return m.Called(ctx, jti, ttl).Error(0)
}

func (m *mockBlacklistRepository) IsTokenBlacklisted(ctx context.Context, jti string) (bool, error) {
	args := m.Called(ctx, jti)
	return args.Bool(0), args.Error(1)
}

var _ TokenBlacklistRepository = (*mockBlacklistRepository)(nil)

// newProtectedRouter mounts GET /protected behind the real AuthMiddleware.
// The dummy endpoint echoes the identity extracted from the Gin context so
// tests can prove the middleware injected it.
func newProtectedRouter(validator TokenValidator) *gin.Engine {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.GET("/protected", AuthMiddleware(validator), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"user_id": c.GetString("user_id"),
			"email":   c.GetString("email"),
		})
	})
	return r
}

func signTestToken(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
	require.NoError(t, err)
	return signed
}

func doRequest(r http.Handler, headerToken string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	if headerToken != "" {
		req.Header.Set("Authorization", "Bearer "+headerToken)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func requireEnvelope(t *testing.T, w *httptest.ResponseRecorder, expectedStatus, expectedMessage string) {
	t.Helper()
	var body struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, expectedStatus, body.Status)
	assert.Equal(t, expectedMessage, body.Message)
}

func TestAuthMiddleware_MissingToken_Returns401(t *testing.T) {
	blacklist := new(mockBlacklistRepository)
	r := newProtectedRouter(NewJWTValidator(testSecret, blacklist))

	w := doRequest(r, "")

	require.Equal(t, http.StatusUnauthorized, w.Code)
	requireEnvelope(t, w, "fail", "Token required")
	blacklist.AssertNotCalled(t, "IsTokenBlacklisted", mock.Anything, mock.Anything)
}

func TestAuthMiddleware_MalformedToken_Returns401(t *testing.T) {
	blacklist := new(mockBlacklistRepository)
	r := newProtectedRouter(NewJWTValidator(testSecret, blacklist))

	w := doRequest(r, "invalid.token.format")

	require.Equal(t, http.StatusUnauthorized, w.Code)
	requireEnvelope(t, w, "fail", "Invalid or expired token")
	blacklist.AssertNotCalled(t, "IsTokenBlacklisted", mock.Anything, mock.Anything)
}

func TestAuthMiddleware_ExpiredToken_Returns401(t *testing.T) {
	blacklist := new(mockBlacklistRepository)
	r := newProtectedRouter(NewJWTValidator(testSecret, blacklist))

	// Validly signed with the correct secret, but exp lies in the past;
	// jwt/v5 exp validation must reject it before any blacklist lookup.
	expired := signTestToken(t, jwt.MapClaims{
		"user_id": uuid.NewString(),
		"email":   "user@example.com",
		"jti":     "jti-expired",
		"iat":     time.Now().Add(-2 * time.Hour).Unix(),
		"exp":     time.Now().Add(-time.Hour).Unix(),
	})

	w := doRequest(r, expired)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	requireEnvelope(t, w, "fail", "Invalid or expired token")
	blacklist.AssertNotCalled(t, "IsTokenBlacklisted", mock.Anything, mock.Anything)
}

func TestAuthMiddleware_RevokedToken_Returns401(t *testing.T) {
	jti := "jti-revoked-" + uuid.NewString()
	blacklist := new(mockBlacklistRepository)
	blacklist.On("IsTokenBlacklisted", mock.Anything, jti).Return(true, nil).Once()
	r := newProtectedRouter(NewJWTValidator(testSecret, blacklist))

	// Signature and exp are valid; revocation comes from the blacklist.
	validButRevoked := signTestToken(t, jwt.MapClaims{
		"user_id": uuid.NewString(),
		"email":   "user@example.com",
		"jti":     jti,
		"iat":     time.Now().Unix(),
		"exp":     time.Now().Add(time.Hour).Unix(),
	})

	w := doRequest(r, validButRevoked)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	requireEnvelope(t, w, "fail", "Token has been revoked")
	blacklist.AssertExpectations(t)
}

func TestAuthMiddleware_ValidToken_ReachesEndpointAndExtractsUserID(t *testing.T) {
	userID := uuid.NewString()
	jti := "jti-active-" + uuid.NewString()
	blacklist := new(mockBlacklistRepository)
	blacklist.On("IsTokenBlacklisted", mock.Anything, jti).Return(false, nil).Once()
	r := newProtectedRouter(NewJWTValidator(testSecret, blacklist))

	valid := signTestToken(t, jwt.MapClaims{
		"user_id": userID,
		"email":   "user@example.com",
		"jti":     jti,
		"iat":     time.Now().Unix(),
		"exp":     time.Now().Add(time.Hour).Unix(),
	})

	w := doRequest(r, valid)

	require.Equal(t, http.StatusOK, w.Code, "dummy endpoint must be reached with a fully valid token")

	var body struct {
		UserID string `json:"user_id"`
		Email  string `json:"email"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, userID, body.UserID, "user_id must be extracted into the Gin context")
	assert.Equal(t, "user@example.com", body.Email)

	blacklist.AssertExpectations(t)
}
