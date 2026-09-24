package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apperrors "walletx-be/internal/shared/errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

// HTTP tests for the auth handler. The Service is mocked; no real service,
// repository, database, or Google call occurs. The JWT middleware is replaced
// by a controlled test middleware injecting a known user ID, per planning.md
// Phase 3. nopLogger is reused from repository_test.go (same test package).

type mockAuthService struct {
	mock.Mock
}

func (m *mockAuthService) ProcessGoogleAuth(ctx context.Context, input GoogleAuthInput) (*User, bool, error) {
	args := m.Called(ctx, input)
	user, _ := args.Get(0).(*User)
	return user, args.Bool(1), args.Error(2)
}

func (m *mockAuthService) GenerateJWT(user *User) (string, error) {
	args := m.Called(user)
	return args.String(0), args.Error(1)
}

func (m *mockAuthService) Logout(ctx context.Context, tokenString string) error {
	return m.Called(ctx, tokenString).Error(0)
}

var _ Service = (*mockAuthService)(nil)

// fakeAuthMiddleware simulates middleware.AuthMiddleware by injecting the
// verified identity into the Gin context before protected handlers run.
func fakeAuthMiddleware(userID string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("user_id", userID)
		c.Set("email", "user@example.com")
		c.Next()
	}
}

// newTestRouter mirrors internal/app/router.go wiring for the auth module:
// public routes registered directly (including dev-mode OAuth helpers when
// devMode is true), protected routes behind auth middleware.
func newTestRouter(h *Handler, authedUserID string, devMode bool) *gin.Engine {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	api := r.Group("/api/v1")

	RegisterRoutes(api, h, devMode)

	protected := api.Group("")
	protected.Use(fakeAuthMiddleware(authedUserID))
	RegisterProtectedRoutes(protected, h)

	return r
}

func doJSON(r http.Handler, method, path, payload string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// envelope decodes the shared response.APIResponse shape.
type envelope struct {
	Status  string      `json:"status"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

// authEnvelope decodes the success payload of POST /auth/google.
type authEnvelope struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Data    struct {
		User struct {
			ID       string `json:"id"`
			GoogleID string `json:"google_id"`
			Email    string `json:"email"`
			Name     string `json:"name"`
			Picture  string `json:"picture"`
		} `json:"user"`
		Token string `json:"token"`
	} `json:"data"`
}

func TestHandleGoogleAuth_Success(t *testing.T) {
	user := &User{
		ID:       uuid.New(),
		GoogleID: "google-123",
		Email:    "user@example.com",
		Name:     "WalletX User",
		Picture:  "https://example.com/avatar.jpg",
	}

	t.Run("existing user returns 200 with user and internal token", func(t *testing.T) {
		svc := new(mockAuthService)
		svc.On("ProcessGoogleAuth", mock.Anything, GoogleAuthInput{IDToken: "valid-google-id-token"}).
			Return(user, false, nil).Once()
		svc.On("GenerateJWT", user).Return("signed-internal-jwt", nil).Once()
		r := newTestRouter(NewHandler(svc, nopLogger{}, nil), uuid.NewString(), false)

		w := doJSON(r, http.MethodPost, "/api/v1/auth/google", `{"id_token":"valid-google-id-token"}`)

		require.Equal(t, http.StatusOK, w.Code)

		var body authEnvelope
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "success", body.Status)
		assert.Equal(t, "Login successful", body.Message)
		assert.Equal(t, user.ID.String(), body.Data.User.ID)
		assert.Equal(t, "google-123", body.Data.User.GoogleID)
		assert.Equal(t, "user@example.com", body.Data.User.Email)
		assert.Equal(t, "WalletX User", body.Data.User.Name)
		assert.Equal(t, "https://example.com/avatar.jpg", body.Data.User.Picture)
		assert.Equal(t, "signed-internal-jwt", body.Data.Token)

		svc.AssertExpectations(t)
	})

	t.Run("new user returns 201 registration", func(t *testing.T) {
		svc := new(mockAuthService)
		svc.On("ProcessGoogleAuth", mock.Anything, GoogleAuthInput{IDToken: "valid-google-id-token"}).
			Return(user, true, nil).Once()
		svc.On("GenerateJWT", user).Return("signed-internal-jwt", nil).Once()
		r := newTestRouter(NewHandler(svc, nopLogger{}, nil), uuid.NewString(), false)

		w := doJSON(r, http.MethodPost, "/api/v1/auth/google", `{"id_token":"valid-google-id-token"}`)

		require.Equal(t, http.StatusCreated, w.Code)

		var body authEnvelope
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "success", body.Status)
		assert.Equal(t, "Registration successful", body.Message)
		assert.Equal(t, "signed-internal-jwt", body.Data.Token)

		svc.AssertExpectations(t)
	})
}

func TestHandleGoogleAuth_BadRequest_DoesNotCallService(t *testing.T) {
	tests := []struct {
		name            string
		payload         string
		expectedMessage string
	}{
		{name: "malformed json", payload: `{"id_token":`, expectedMessage: "Invalid request body"},
		{name: "empty json object", payload: `{}`, expectedMessage: "id_token is required"},
		{name: "blank id_token", payload: `{"id_token":"   "}`, expectedMessage: "id_token is required"},
		{name: "unknown field rejected", payload: `{"id_token":"valid","unexpected":"field"}`, expectedMessage: "Invalid request body"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := new(mockAuthService)
			svc.On("ProcessGoogleAuth", mock.Anything, mock.Anything).Return(nil, false, nil).Maybe()
			svc.On("GenerateJWT", mock.Anything).Return("", nil).Maybe()
			r := newTestRouter(NewHandler(svc, nopLogger{}, nil), uuid.NewString(), false)

			w := doJSON(r, http.MethodPost, "/api/v1/auth/google", tt.payload)

			require.Equal(t, http.StatusBadRequest, w.Code)

			var body envelope
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			assert.Equal(t, "fail", body.Status)
			assert.Equal(t, tt.expectedMessage, body.Message)
			assert.Nil(t, body.Data, "error responses must not carry a data payload")

			svc.AssertNotCalled(t, "ProcessGoogleAuth", mock.Anything, mock.Anything)
			svc.AssertNotCalled(t, "GenerateJWT", mock.Anything)
		})
	}
}

func TestHandleGoogleAuth_ServiceUnauthorized_Maps401WithoutInternalDetails(t *testing.T) {
	svc := new(mockAuthService)
	internalErr := fmt.Errorf("%w: google signature check failed: internal detail XYZ", apperrors.ErrUnauthorized)
	svc.On("ProcessGoogleAuth", mock.Anything, GoogleAuthInput{IDToken: "bad-google-token"}).
		Return(nil, false, internalErr).Once()
	svc.On("GenerateJWT", mock.Anything).Return("", nil).Maybe()
	r := newTestRouter(NewHandler(svc, nopLogger{}, nil), uuid.NewString(), false)

	w := doJSON(r, http.MethodPost, "/api/v1/auth/google", `{"id_token":"bad-google-token"}`)

	require.Equal(t, http.StatusUnauthorized, w.Code)

	var body envelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "fail", body.Status)
	assert.Equal(t, "Invalid or expired Google token", body.Message)

	assert.NotContains(t, w.Body.String(), "XYZ", "internal error detail must never reach the client")
	assert.NotContains(t, w.Body.String(), "internal detail")
	assert.NotContains(t, w.Body.String(), "google signature check failed")

	svc.AssertExpectations(t)
	svc.AssertNotCalled(t, "GenerateJWT", mock.Anything)
}

func TestLogout_AuthenticatedContext_Returns204(t *testing.T) {
	svc := new(mockAuthService)
	svc.On("Logout", mock.Anything, "active-internal-jwt").Return(nil).Once()

	authedUserID := uuid.NewString()
	r := newTestRouter(NewHandler(svc, nopLogger{}, nil), authedUserID, false)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer active-internal-jwt")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
	assert.Empty(t, w.Body.String(), "204 responses must carry no body")

	svc.AssertExpectations(t)
}

func TestLogout_MissingBearerToken_Returns401WithoutCallingService(t *testing.T) {
	svc := new(mockAuthService)
	svc.On("Logout", mock.Anything, mock.Anything).Return(nil).Maybe()
	r := newTestRouter(NewHandler(svc, nopLogger{}, nil), uuid.NewString(), false)

	// Reachable in production when the middleware accepted a ?token= query
	// param (extractToken supports it) but no Authorization header is present.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code,
		"missing bearer token must match the middleware's unauthorized behavior")

	var body envelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "fail", body.Status)
	assert.Equal(t, "Token required", body.Message)

	svc.AssertNotCalled(t, "Logout", mock.Anything, mock.Anything)
}

// testOAuthConfig is a valid, non-nil OAuth configuration so dev-mode helper
// routes can render their redirect without dereferencing nil.
func testOAuthConfig() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURL:  "http://localhost:8080/api/v1/auth/google/callback",
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://accounts.google.com/o/oauth2/auth",
			TokenURL: "https://oauth2.googleapis.com/token",
		},
		Scopes: []string{"openid", "email", "profile"},
	}
}

func TestGoogleOAuthHelperRoutes_DevModeEnabled_RoutesRegistered(t *testing.T) {
	svc := new(mockAuthService)
	svc.On("ProcessGoogleAuth", mock.Anything, mock.Anything).Return(nil, false, nil).Maybe()
	svc.On("GenerateJWT", mock.Anything).Return("", nil).Maybe()
	r := newTestRouter(NewHandler(svc, nopLogger{}, testOAuthConfig()), uuid.NewString(), true)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/google/test-login", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.NotEqual(t, http.StatusNotFound, w.Code, "test-login route must exist when dev mode is enabled")
	require.Equal(t, http.StatusTemporaryRedirect, w.Code,
		"GoogleLoginTest issues an OAuth redirect via http.StatusTemporaryRedirect")

	location := w.Header().Get("Location")
	require.NotEmpty(t, location, "redirect must carry the Google authorization URL")
	assert.Contains(t, location, "https://accounts.google.com/o/oauth2/auth")
	assert.Contains(t, location, "client_id=test-client-id")
	assert.Contains(t, location, "redirect_uri=")
	assert.Contains(t, location, "state=random-state-string")

	svc.AssertNotCalled(t, "ProcessGoogleAuth", mock.Anything, mock.Anything)
	svc.AssertNotCalled(t, "GenerateJWT", mock.Anything)
}

func TestGoogleOAuthHelperRoutes_DevModeDisabled_RoutesNotRegistered(t *testing.T) {
	// A valid OAuth config is supplied on purpose: the DEV_MODE flag, not the
	// presence of a config, must be the gate that closes this attack surface.
	svc := new(mockAuthService)
	r := newTestRouter(NewHandler(svc, nopLogger{}, testOAuthConfig()), uuid.NewString(), false)

	for _, path := range []string{
		"/api/v1/auth/google/test-login",
		"/api/v1/auth/google/callback",
	} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusNotFound, w.Code,
				"OAuth helper routes must not be registered outside dev mode")
		})
	}
}
