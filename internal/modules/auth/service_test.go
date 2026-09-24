package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"walletx-be/internal/middleware"
	apperrors "walletx-be/internal/shared/errors"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/idtoken"
)

// Unit tests for the auth service. All dependencies (user store, Google token
// verifier, JWT blacklist) are mocked; no database, Gin, or network I/O occurs.
// nopLogger is reused from repository_test.go (same test package).
//
// RED-phase contract driven by this file (planning.md Priority 2, Step 1):
// service.go hardcodes idtoken.Validate at the moment, so these tests define
// the injectable Google verifier seam that must be introduced to go green:
//
//	type TokenVerifier interface {
//		Validate(ctx context.Context, idToken string) (*idtoken.Payload, error)
//	}
//
//	func NewGoogleTokenVerifier(oauthClientID string) TokenVerifier // wraps idtoken.Validate
//
//	func NewService(userStore UserStore, verifier TokenVerifier, appLogger logger.Logger,
//		jwtSecret string, jwtExpiration int, blacklistRepo middleware.TokenBlacklistRepository) Service
//
// ProcessGoogleAuth must call s.verifier.Validate(ctx, input.IDToken) instead of
// idtoken.Validate directly; oauthClientID moves into the concrete verifier.
// Until that refactor lands, this package intentionally fails to compile (Red).

const (
	testJWTSecret          = "unit-test-secret"
	testJWTExpirationHours = 12
)

type mockUserStore struct {
	mock.Mock
}

func (m *mockUserStore) FindByGoogleID(ctx context.Context, googleID string) (*User, error) {
	args := m.Called(ctx, googleID)
	user, _ := args.Get(0).(*User)
	return user, args.Error(1)
}

func (m *mockUserStore) FindByGoogleIDAny(ctx context.Context, googleID string) (*User, error) {
	args := m.Called(ctx, googleID)
	user, _ := args.Get(0).(*User)
	return user, args.Error(1)
}

func (m *mockUserStore) FindByEmail(ctx context.Context, email string) (*User, error) {
	args := m.Called(ctx, email)
	user, _ := args.Get(0).(*User)
	return user, args.Error(1)
}

func (m *mockUserStore) CreateWithDefaultCategories(ctx context.Context, user *User) error {
	return m.Called(ctx, user).Error(0)
}

func (m *mockUserStore) UpdateGoogleProfile(ctx context.Context, userID uuid.UUID, name, picture string) (*User, error) {
	args := m.Called(ctx, userID, name, picture)
	user, _ := args.Get(0).(*User)
	return user, args.Error(1)
}

var _ UserStore = (*mockUserStore)(nil)

type mockTokenVerifier struct {
	mock.Mock
}

func (m *mockTokenVerifier) Validate(ctx context.Context, token string) (*idtoken.Payload, error) {
	args := m.Called(ctx, token)
	payload, _ := args.Get(0).(*idtoken.Payload)
	return payload, args.Error(1)
}

var _ TokenVerifier = (*mockTokenVerifier)(nil)

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

var _ middleware.TokenBlacklistRepository = (*mockBlacklistRepository)(nil)

func newTestService(store UserStore, verifier TokenVerifier, blacklist middleware.TokenBlacklistRepository) Service {
	return NewService(store, verifier, nopLogger{}, testJWTSecret, testJWTExpirationHours, blacklist)
}

// validGoogleClaims builds a fully valid Google identity claim set. A nil
// override value deletes the claim to simulate a missing field.
func validGoogleClaims(overrides map[string]interface{}) map[string]interface{} {
	claims := map[string]interface{}{
		"email":          "user@example.com",
		"email_verified": true,
		"name":           "WalletX User",
		"picture":        "https://example.com/avatar.jpg",
	}
	for key, value := range overrides {
		if value == nil {
			delete(claims, key)
			continue
		}
		claims[key] = value
	}
	return claims
}

// newValidUserFlow wires verifier and store mocks whose identity resolves as a
// brand-new user. onCreate is invoked inside CreateWithDefaultCategories to
// simulate persistence side effects (ID assignment); createErr is the error the
// create call returns.
func newValidUserFlow(t *testing.T, ctx context.Context, googleID string, claims map[string]interface{}, onCreate func(*User), createErr error) (*mockUserStore, *mockTokenVerifier, *mockBlacklistRepository) {
	t.Helper()

	store := new(mockUserStore)
	verifier := new(mockTokenVerifier)
	blacklist := new(mockBlacklistRepository)

	payload := &idtoken.Payload{Subject: googleID, Audience: "test-client-id", Claims: claims}
	verifier.On("Validate", ctx, "google-id-token").Return(payload, nil).Once()
	store.On("FindByGoogleID", ctx, googleID).Return(nil, nil).Once()
	store.On("FindByGoogleIDAny", ctx, googleID).Return(nil, nil).Once()
	store.On("FindByEmail", ctx, "user@example.com").Return(nil, apperrors.ErrNotFound).Once()
	store.On("CreateWithDefaultCategories", ctx, mock.AnythingOfType("*auth.User")).
		Run(func(args mock.Arguments) {
			if onCreate != nil {
				onCreate(args.Get(1).(*User))
			}
		}).
		Return(createErr).Once()

	return store, verifier, blacklist
}

func TestProcessGoogleAuth_NewUser_CreatesUserAndIssuesJWT(t *testing.T) {
	ctx := context.Background()
	persistedID := uuid.New()

	var createInput *User
	store, verifier, blacklist := newValidUserFlow(t, ctx, "google-123", validGoogleClaims(map[string]interface{}{
		"email":   "  User@Example.COM ",
		"name":    "  WalletX User ",
		"picture": " https://example.com/avatar.jpg ",
	}), func(u *User) {
		createInput = &User{GoogleID: u.GoogleID, Email: u.Email, Name: u.Name, Picture: u.Picture}
		u.ID = persistedID
		u.CreatedAt = time.Now()
		u.UpdatedAt = time.Now()
	}, nil)

	svc := newTestService(store, verifier, blacklist)

	user, isNewUser, err := svc.ProcessGoogleAuth(ctx, GoogleAuthInput{IDToken: "google-id-token"})

	require.NoError(t, err)
	require.NotNil(t, user)
	assert.True(t, isNewUser, "first sign-in must be reported as a new user")

	require.NotNil(t, createInput)
	assert.Equal(t, "google-123", createInput.GoogleID)
	assert.Equal(t, "user@example.com", createInput.Email, "email must be lowercased and trimmed before persistence")
	assert.Equal(t, "WalletX User", createInput.Name, "name must be trimmed before persistence")
	assert.Equal(t, "https://example.com/avatar.jpg", createInput.Picture, "picture must be trimmed before persistence")

	assert.Equal(t, persistedID, user.ID, "service must return the persisted identity")

	token, err := svc.GenerateJWT(user)
	require.NoError(t, err)
	require.NotEmpty(t, token, "successful authentication must yield a signed internal JWT")

	parsed, err := jwt.Parse(token, func(*jwt.Token) (interface{}, error) {
		return []byte(testJWTSecret), nil
	})
	require.NoError(t, err)
	require.True(t, parsed.Valid)
	assert.Equal(t, jwt.SigningMethodHS256.Alg(), parsed.Method.Alg())

	claims, ok := parsed.Claims.(jwt.MapClaims)
	require.True(t, ok)
	assert.Equal(t, persistedID.String(), claims["user_id"])
	assert.Equal(t, "user@example.com", claims["email"])
	assert.NotEmpty(t, claims["jti"])

	exp, err := claims.GetExpirationTime()
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now().Add(testJWTExpirationHours*time.Hour), exp.Time, time.Minute)

	verifier.AssertExpectations(t)
	store.AssertExpectations(t)
	store.AssertNotCalled(t, "UpdateGoogleProfile", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestProcessGoogleAuth_ExistingUser_ReusesWithoutCreating(t *testing.T) {
	ctx := context.Background()
	existingID := uuid.New()

	t.Run("unchanged profile is reused as-is", func(t *testing.T) {
		store := new(mockUserStore)
		verifier := new(mockTokenVerifier)
		blacklist := new(mockBlacklistRepository)
		svc := newTestService(store, verifier, blacklist)

		existing := &User{
			ID:        existingID,
			GoogleID:  "google-123",
			Email:     "user@example.com",
			Name:      "WalletX User",
			Picture:   "https://example.com/avatar.jpg",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		payload := &idtoken.Payload{Subject: "google-123", Claims: validGoogleClaims(nil)}
		verifier.On("Validate", ctx, "google-id-token").Return(payload, nil).Once()
		store.On("FindByGoogleID", ctx, "google-123").Return(existing, nil).Once()

		user, isNewUser, err := svc.ProcessGoogleAuth(ctx, GoogleAuthInput{IDToken: "google-id-token"})

		require.NoError(t, err)
		require.NotNil(t, user)
		assert.False(t, isNewUser, "returning user must not be reported as new")
		assert.Equal(t, existingID, user.ID)
		assert.Equal(t, "WalletX User", user.Name)
		assert.Equal(t, "https://example.com/avatar.jpg", user.Picture)

		verifier.AssertExpectations(t)
		store.AssertExpectations(t)
		store.AssertNotCalled(t, "UpdateGoogleProfile", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		store.AssertNotCalled(t, "CreateWithDefaultCategories", mock.Anything, mock.Anything)
		store.AssertNotCalled(t, "FindByEmail", mock.Anything, mock.Anything)
	})

	t.Run("changed profile is updated without creating a new record", func(t *testing.T) {
		store := new(mockUserStore)
		verifier := new(mockTokenVerifier)
		blacklist := new(mockBlacklistRepository)
		svc := newTestService(store, verifier, blacklist)

		existing := &User{
			ID:        existingID,
			GoogleID:  "google-123",
			Email:     "user@example.com",
			Name:      "Old Name",
			Picture:   "https://example.com/old.jpg",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		updated := *existing
		updated.Name = "New Name"
		updated.Picture = "https://example.com/new.jpg"

		payload := &idtoken.Payload{Subject: "google-123", Claims: validGoogleClaims(map[string]interface{}{
			"name":    "New Name",
			"picture": "https://example.com/new.jpg",
		})}
		verifier.On("Validate", ctx, "google-id-token").Return(payload, nil).Once()
		store.On("FindByGoogleID", ctx, "google-123").Return(existing, nil).Once()
		store.On("UpdateGoogleProfile", ctx, existingID, "New Name", "https://example.com/new.jpg").
			Return(&updated, nil).Once()

		user, isNewUser, err := svc.ProcessGoogleAuth(ctx, GoogleAuthInput{IDToken: "google-id-token"})

		require.NoError(t, err)
		require.NotNil(t, user)
		assert.False(t, isNewUser)
		assert.Equal(t, existingID, user.ID, "update must reuse the existing row, never create a new one")
		assert.Equal(t, "New Name", user.Name)
		assert.Equal(t, "https://example.com/new.jpg", user.Picture)

		verifier.AssertExpectations(t)
		store.AssertExpectations(t)
		store.AssertNotCalled(t, "CreateWithDefaultCategories", mock.Anything, mock.Anything)
		store.AssertNotCalled(t, "FindByEmail", mock.Anything, mock.Anything)
	})
}

func TestProcessGoogleAuth_VerifierFailure_ReturnsUnauthorizedAndSkipsRepository(t *testing.T) {
	ctx := context.Background()
	store := new(mockUserStore)
	verifier := new(mockTokenVerifier)
	blacklist := new(mockBlacklistRepository)
	svc := newTestService(store, verifier, blacklist)

	verifier.On("Validate", ctx, "bad-token").Return(nil, errors.New("google: token expired")).Once()

	// Maybe() expectations make the "repository never called" requirement
	// explicit; any actual call would also fail AssertNotCalled below.
	store.On("FindByGoogleID", mock.Anything, mock.Anything).Return(nil, nil).Maybe()
	store.On("FindByGoogleIDAny", mock.Anything, mock.Anything).Return(nil, nil).Maybe()
	store.On("FindByEmail", mock.Anything, mock.Anything).Return(nil, nil).Maybe()
	store.On("CreateWithDefaultCategories", mock.Anything, mock.Anything).Return(nil).Maybe()
	store.On("UpdateGoogleProfile", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Maybe()

	user, isNewUser, err := svc.ProcessGoogleAuth(ctx, GoogleAuthInput{IDToken: "bad-token"})

	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrUnauthorized,
		"provider failure must surface as the application unauthorized error")
	assert.Nil(t, user)
	assert.False(t, isNewUser)

	verifier.AssertExpectations(t)
	store.AssertNotCalled(t, "FindByGoogleID", mock.Anything, mock.Anything)
	store.AssertNotCalled(t, "FindByGoogleIDAny", mock.Anything, mock.Anything)
	store.AssertNotCalled(t, "FindByEmail", mock.Anything, mock.Anything)
	store.AssertNotCalled(t, "CreateWithDefaultCategories", mock.Anything, mock.Anything)
	store.AssertNotCalled(t, "UpdateGoogleProfile", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestProcessGoogleAuth_RepositoryFailure_PropagatesApplicationError(t *testing.T) {
	ctx := context.Background()

	t.Run("lookup failure propagates unchanged", func(t *testing.T) {
		store := new(mockUserStore)
		verifier := new(mockTokenVerifier)
		blacklist := new(mockBlacklistRepository)
		svc := newTestService(store, verifier, blacklist)

		lookupErr := errors.New("connection refused")
		payload := &idtoken.Payload{Subject: "google-123", Claims: validGoogleClaims(nil)}
		verifier.On("Validate", ctx, "google-id-token").Return(payload, nil).Once()
		store.On("FindByGoogleID", ctx, "google-123").Return(nil, lookupErr).Once()

		user, isNewUser, err := svc.ProcessGoogleAuth(ctx, GoogleAuthInput{IDToken: "google-id-token"})

		require.Error(t, err)
		assert.ErrorIs(t, err, lookupErr)
		assert.Nil(t, user)
		assert.False(t, isNewUser)

		store.AssertExpectations(t)
		store.AssertNotCalled(t, "CreateWithDefaultCategories", mock.Anything, mock.Anything)
	})

	t.Run("email lookup failure propagates unchanged", func(t *testing.T) {
		store := new(mockUserStore)
		verifier := new(mockTokenVerifier)
		blacklist := new(mockBlacklistRepository)
		svc := newTestService(store, verifier, blacklist)

		emailErr := errors.New("connection reset")
		payload := &idtoken.Payload{Subject: "google-123", Claims: validGoogleClaims(nil)}
		verifier.On("Validate", ctx, "google-id-token").Return(payload, nil).Once()
		store.On("FindByGoogleID", ctx, "google-123").Return(nil, nil).Once()
		store.On("FindByGoogleIDAny", ctx, "google-123").Return(nil, nil).Once()
		store.On("FindByEmail", ctx, "user@example.com").Return(nil, emailErr).Once()

		user, isNewUser, err := svc.ProcessGoogleAuth(ctx, GoogleAuthInput{IDToken: "google-id-token"})

		require.Error(t, err)
		assert.ErrorIs(t, err, emailErr)
		assert.Nil(t, user)
		assert.False(t, isNewUser)

		store.AssertExpectations(t)
		store.AssertNotCalled(t, "CreateWithDefaultCategories", mock.Anything, mock.Anything)
	})

	t.Run("duplicate create failure keeps ErrConflict classification", func(t *testing.T) {
		store, verifier, blacklist := newValidUserFlow(t, ctx, "google-123", validGoogleClaims(nil), nil, apperrors.ErrConflict)
		svc := newTestService(store, verifier, blacklist)

		user, isNewUser, err := svc.ProcessGoogleAuth(ctx, GoogleAuthInput{IDToken: "google-id-token"})

		require.Error(t, err)
		assert.ErrorIs(t, err, apperrors.ErrConflict,
			"repository conflict classification must survive the service boundary for 409 mapping")
		assert.Nil(t, user)
		assert.False(t, isNewUser)

		store.AssertExpectations(t)
	})
}

func TestProcessGoogleAuth_InvalidGoogleClaims_ReturnsUnauthorized(t *testing.T) {
	tests := []struct {
		name      string
		subject   string
		overrides map[string]interface{}
	}{
		{name: "missing subject", subject: "", overrides: nil},
		{name: "missing email claim", subject: "google-123", overrides: map[string]interface{}{"email": nil}},
		{name: "blank email claim", subject: "google-123", overrides: map[string]interface{}{"email": "   "}},
		{name: "email not verified", subject: "google-123", overrides: map[string]interface{}{"email_verified": false}},
		{name: "missing name claim", subject: "google-123", overrides: map[string]interface{}{"name": nil}},
		{name: "blank name claim", subject: "google-123", overrides: map[string]interface{}{"name": "   "}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			store := new(mockUserStore)
			verifier := new(mockTokenVerifier)
			blacklist := new(mockBlacklistRepository)
			svc := newTestService(store, verifier, blacklist)

			payload := &idtoken.Payload{Subject: tt.subject, Claims: validGoogleClaims(tt.overrides)}
			verifier.On("Validate", ctx, "google-id-token").Return(payload, nil).Once()
			store.On("FindByGoogleID", mock.Anything, mock.Anything).Return(nil, nil).Maybe()
			store.On("FindByGoogleIDAny", mock.Anything, mock.Anything).Return(nil, nil).Maybe()
			store.On("FindByEmail", mock.Anything, mock.Anything).Return(nil, nil).Maybe()
			store.On("CreateWithDefaultCategories", mock.Anything, mock.Anything).Return(nil).Maybe()
			store.On("UpdateGoogleProfile", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Maybe()

			user, isNewUser, err := svc.ProcessGoogleAuth(ctx, GoogleAuthInput{IDToken: "google-id-token"})

			require.Error(t, err)
			assert.ErrorIs(t, err, apperrors.ErrUnauthorized)
			assert.Nil(t, user)
			assert.False(t, isNewUser)

			verifier.AssertExpectations(t)
			store.AssertNotCalled(t, "CreateWithDefaultCategories", mock.Anything, mock.Anything)
			store.AssertNotCalled(t, "UpdateGoogleProfile", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func TestGenerateJWT_IssuesSignedTokenWithExpectedClaims(t *testing.T) {
	svc := newTestService(new(mockUserStore), new(mockTokenVerifier), new(mockBlacklistRepository))
	before := time.Now()
	user := &User{ID: uuid.New(), GoogleID: "google-123", Email: "user@example.com", Name: "WalletX User"}

	token, err := svc.GenerateJWT(user)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	parsed, err := jwt.Parse(token, func(*jwt.Token) (interface{}, error) {
		return []byte(testJWTSecret), nil
	})
	require.NoError(t, err)
	require.True(t, parsed.Valid)
	assert.Equal(t, jwt.SigningMethodHS256.Alg(), parsed.Method.Alg())

	claims, ok := parsed.Claims.(jwt.MapClaims)
	require.True(t, ok)
	assert.Equal(t, user.ID.String(), claims["user_id"])
	assert.Equal(t, user.Email, claims["email"])

	jti, ok := claims["jti"].(string)
	require.True(t, ok, "token must carry a JTI claim for logout revocation")
	assert.NotEmpty(t, jti)

	exp, err := claims.GetExpirationTime()
	require.NoError(t, err)
	assert.WithinDuration(t, before.Add(testJWTExpirationHours*time.Hour), exp.Time, time.Minute)

	iat, err := claims.GetIssuedAt()
	require.NoError(t, err)
	assert.WithinDuration(t, before, iat.Time, time.Minute)

	t.Run("token signed with a different secret is rejected", func(t *testing.T) {
		_, err := jwt.Parse(token, func(*jwt.Token) (interface{}, error) {
			return []byte("attacker-secret"), nil
		})
		assert.Error(t, err)
	})

	t.Run("each issued token gets a unique JTI", func(t *testing.T) {
		second, err := svc.GenerateJWT(user)
		require.NoError(t, err)
		assert.NotEqual(t, token, second)

		parsedSecond, err := jwt.Parse(second, func(*jwt.Token) (interface{}, error) {
			return []byte(testJWTSecret), nil
		})
		require.NoError(t, err)
		secondClaims, ok := parsedSecond.Claims.(jwt.MapClaims)
		require.True(t, ok)
		assert.NotEqual(t, jti, secondClaims["jti"], "revocation requires a distinct JTI per token")
	})
}
