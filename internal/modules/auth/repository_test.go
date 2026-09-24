package auth

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	sqlc "walletx-be/internal/platform/database/sqlc"
	"walletx-be/internal/platform/logger"
	apperrors "walletx-be/internal/shared/errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Integration tests require a real PostgreSQL instance with the users schema
// applied (see supabase/migrations). Set TEST_DATABASE_URL (or DATABASE_URL)
// before running, e.g.:
//
//	$env:TEST_DATABASE_URL = "postgres://postgres:postgres@127.0.0.1:54322/postgres"
//	go test ./internal/modules/auth/ -run TestUserRepository -v
//
// When the variable is not set, tests are skipped.

var testPool *pgxpool.Pool

// integrationLockKey is shared by every integration test package that hits the
// same database (auth, categories). go test runs package binaries in parallel
// and both suites TRUNCATE shared tables, so TestMain holds this PostgreSQL
// advisory lock to serialize them.
const integrationLockKey int64 = 742_001

type nopLogger struct{}

func (nopLogger) Info(string)                                       {}
func (nopLogger) Warn(string)                                       {}
func (nopLogger) Error(string)                                      {}
func (nopLogger) Debug(string)                                      {}
func (l nopLogger) WithField(string, interface{}) logger.Logger     { return l }
func (l nopLogger) WithError(error) logger.Logger                   { return l }
func (l nopLogger) WithFields(map[string]interface{}) logger.Logger { return l }

func TestMain(m *testing.M) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}
	if databaseURL == "" {
		os.Exit(m.Run())
	}

	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		panic("failed to parse test database URL: " + err.Error())
	}
	poolConfig.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		panic("failed to create test pool: " + err.Error())
	}
	if err := pool.Ping(ctx); err != nil {
		panic("failed to ping test database: " + err.Error())
	}
	testPool = pool

	// Serialize integration suites that share this database (see
	// integrationLockKey). The lock lives on a dedicated connection and is
	// explicitly released before exit because os.Exit skips defers.
	lockConn, err := testPool.Acquire(context.Background())
	if err != nil {
		panic("failed to acquire connection for integration lock: " + err.Error())
	}
	if _, err := lockConn.Exec(context.Background(), "SELECT pg_advisory_lock($1)", integrationLockKey); err != nil {
		panic("failed to acquire integration lock: " + err.Error())
	}

	code := m.Run()

	unlockCtx, unlockCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer unlockCancel()
	if _, err := lockConn.Exec(unlockCtx, "SELECT pg_advisory_unlock($1)", integrationLockKey); err != nil {
		panic("failed to release integration lock: " + err.Error())
	}
	lockConn.Release()
	testPool.Close()

	os.Exit(code)
}

func skipIfNoDatabase(t *testing.T) {
	t.Helper()
	if testPool == nil {
		t.Skip("TEST_DATABASE_URL/DATABASE_URL not set; skipping repository integration tests")
	}
}

func newTestRepository(t *testing.T) UserRepository {
	t.Helper()
	return NewUserRepository(testPool, sqlc.New(testPool), nopLogger{})
}

func truncateTables(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := testPool.Exec(ctx, `TRUNCATE public.users CASCADE`)
	require.NoError(t, err, "truncate test data")
}

// seededUser captures the full identity inserted directly into the database so
// FindByGoogleID assertions compare against independent ground truth instead of
// repository output.
type seededUser struct {
	ID        uuid.UUID
	GoogleID  string
	Email     string
	Name      string
	Picture   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// seedUser inserts a user row via raw SQL and returns its persisted identity.
func seedUser(t *testing.T) seededUser {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := uuid.NewString()
	expected := seededUser{
		GoogleID: "google-" + suffix,
		Email:    "user-" + suffix + "@example.com",
		Name:     "Test User",
		Picture:  "https://example.com/picture.jpg",
	}

	err := testPool.QueryRow(ctx,
		`INSERT INTO public.users (google_id, email, name, picture)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, created_at, updated_at`,
		expected.GoogleID, expected.Email, expected.Name, expected.Picture,
	).Scan(&expected.ID, &expected.CreatedAt, &expected.UpdatedAt)
	require.NoError(t, err, "seed user")
	require.NotEqual(t, uuid.Nil, expected.ID)
	return expected
}

// fetchUserRow is an independent read-back helper used to verify persisted
// state directly against the database instead of trusting repository output.
func fetchUserRow(t *testing.T, googleID string) User {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var (
		row       User
		picture   *string
		deletedAt *time.Time
	)
	err := testPool.QueryRow(ctx,
		`SELECT id, google_id, email, name, picture, created_at, updated_at, deleted_at
		 FROM public.users
		 WHERE google_id = $1`,
		googleID,
	).Scan(&row.ID, &row.GoogleID, &row.Email, &row.Name, &picture,
		&row.CreatedAt, &row.UpdatedAt, &deletedAt)
	require.NoError(t, err, "read back user row")
	require.NotNil(t, picture)
	row.Picture = *picture
	row.DeletedAt = deletedAt
	return row
}

func TestUserRepository_Create(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx := context.Background()
	repo := newTestRepository(t)

	suffix := uuid.NewString()
	user := &User{
		GoogleID: "google-" + suffix,
		Email:    "user-" + suffix + "@example.com",
		Name:     "WalletX User",
		Picture:  "https://example.com/avatar.jpg",
	}
	before := time.Now().Add(-time.Minute)

	err := repo.Create(ctx, user)

	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, user.ID, "database must assign the user ID")
	assert.Equal(t, "google-"+suffix, user.GoogleID)
	assert.Equal(t, "user-"+suffix+"@example.com", user.Email)
	assert.Equal(t, "WalletX User", user.Name)
	assert.Equal(t, "https://example.com/avatar.jpg", user.Picture)
	assert.False(t, user.CreatedAt.IsZero())
	assert.False(t, user.UpdatedAt.IsZero())
	assert.True(t, user.CreatedAt.After(before), "created_at should be recent")
	assert.True(t, user.CreatedAt.Before(time.Now().Add(time.Minute)), "created_at should not be in the future")
	assert.Nil(t, user.DeletedAt)

	persisted := fetchUserRow(t, user.GoogleID)
	assert.Equal(t, user.ID, persisted.ID)
	assert.Equal(t, user.GoogleID, persisted.GoogleID)
	assert.Equal(t, user.Email, persisted.Email)
	assert.Equal(t, user.Name, persisted.Name)
	assert.Equal(t, user.Picture, persisted.Picture)
	assert.True(t, user.CreatedAt.Equal(persisted.CreatedAt))
	assert.True(t, user.UpdatedAt.Equal(persisted.UpdatedAt))
	assert.Nil(t, persisted.DeletedAt)
}

func TestUserRepository_FindByGoogleID(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx := context.Background()
	repo := newTestRepository(t)
	expected := seedUser(t)

	found, err := repo.FindByGoogleID(ctx, expected.GoogleID)

	require.NoError(t, err)
	require.NotNil(t, found, "existing google_id must return the persisted user")
	assert.Equal(t, expected.ID, found.ID)
	assert.Equal(t, expected.GoogleID, found.GoogleID)
	assert.Equal(t, expected.Email, found.Email)
	assert.Equal(t, expected.Name, found.Name)
	assert.Equal(t, expected.Picture, found.Picture)
	assert.True(t, expected.CreatedAt.Equal(found.CreatedAt))
	assert.True(t, expected.UpdatedAt.Equal(found.UpdatedAt))
	assert.Nil(t, found.DeletedAt)
}

// countUsers is an independent read-back helper proving whether an operation
// created an additional row instead of trusting repository output only.
func countUsers(t *testing.T) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var count int
	err := testPool.QueryRow(ctx, `SELECT count(*) FROM public.users`).Scan(&count)
	require.NoError(t, err, "count users")
	return count
}

// assertNotRawPgError proves the repository translated a PostgreSQL failure
// into an application error instead of leaking the driver error to callers.
func assertNotRawPgError(t *testing.T, err error) {
	t.Helper()
	var pgErr *pgconn.PgError
	assert.False(t, errors.As(err, &pgErr),
		"raw PostgreSQL error must not leak from the repository")
}

func TestUserRepository_UpsertExistingGoogleIdentity_UpdatesProfileWithoutDuplicate(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx := context.Background()
	repo := newTestRepository(t)
	existing := seedUser(t)

	// Repository-level upsert flow used by Google sign-in: resolve the
	// identity, then update only the permitted profile fields (name, picture).
	found, err := repo.FindByGoogleID(ctx, existing.GoogleID)
	require.NoError(t, err)
	require.NotNil(t, found)

	updated, err := repo.UpdateGoogleProfile(ctx, found.ID, "Renamed User", "https://example.com/renamed.jpg")

	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, existing.ID, updated.ID, "upsert must reuse the existing row")
	assert.Equal(t, existing.GoogleID, updated.GoogleID, "google_id is not a permitted profile field")
	assert.Equal(t, existing.Email, updated.Email, "email is not a permitted profile field")
	assert.Equal(t, "Renamed User", updated.Name)
	assert.Equal(t, "https://example.com/renamed.jpg", updated.Picture)
	assert.Nil(t, updated.DeletedAt)
	assert.False(t, updated.UpdatedAt.Before(existing.UpdatedAt), "updated_at must not regress")

	persisted := fetchUserRow(t, existing.GoogleID)
	assert.Equal(t, existing.ID, persisted.ID)
	assert.Equal(t, "Renamed User", persisted.Name)
	assert.Equal(t, "https://example.com/renamed.jpg", persisted.Picture)

	assert.Equal(t, 1, countUsers(t), "upsert must not create a second user row")
}

func TestUserRepository_Create_DuplicateGoogleID_ReturnsApplicationError(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx := context.Background()
	repo := newTestRepository(t)
	existing := seedUser(t)

	duplicate := &User{
		GoogleID: existing.GoogleID, // violates users_google_id_key
		Email:    "other-" + uuid.NewString() + "@example.com",
		Name:     "Second Identity",
		Picture:  "https://example.com/second.jpg",
	}

	err := repo.Create(ctx, duplicate)

	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrConflict,
		"unique google_id violation must map to the application conflict error")
	assertNotRawPgError(t, err)
	assert.Equal(t, uuid.Nil, duplicate.ID, "failed create must not assign an ID")
	assert.Equal(t, 1, countUsers(t), "rejected insert must not add a row")
}

func TestUserRepository_Create_DuplicateEmail_DifferentGoogleID_ReturnsApplicationError(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx := context.Background()
	repo := newTestRepository(t)
	existing := seedUser(t)

	duplicate := &User{
		GoogleID: "google-" + uuid.NewString(),
		Email:    existing.Email, // violates users_email_key
		Name:     "Same Email Other Provider",
		Picture:  "https://example.com/same-email.jpg",
	}

	err := repo.Create(ctx, duplicate)

	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrConflict,
		"unique email violation must map to the application conflict error")
	assertNotRawPgError(t, err)
	assert.Equal(t, uuid.Nil, duplicate.ID, "failed create must not assign an ID")
	assert.Equal(t, 1, countUsers(t), "rejected insert must not add a row")

	original := fetchUserRow(t, existing.GoogleID)
	assert.Equal(t, existing.ID, original.ID)
	assert.Equal(t, existing.Name, original.Name,
		"rejected insert must not mutate the existing user")
}

func TestUserRepository_MissingIdentity_ReturnsApplicationErrors(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx := context.Background()
	repo := newTestRepository(t)

	t.Run("GetByID returns ErrNotFound", func(t *testing.T) {
		user, err := repo.GetByID(ctx, uuid.New())

		assert.Nil(t, user)
		require.Error(t, err)
		assert.ErrorIs(t, err, apperrors.ErrNotFound)
		assertNotRawPgError(t, err)
	})

	t.Run("FindByEmail returns ErrNotFound", func(t *testing.T) {
		user, err := repo.FindByEmail(ctx, "missing-"+uuid.NewString()+"@example.com")

		assert.Nil(t, user)
		require.Error(t, err)
		assert.ErrorIs(t, err, apperrors.ErrNotFound)
		assertNotRawPgError(t, err)
	})

	t.Run("FindByGoogleID returns nil user without error", func(t *testing.T) {
		// Contract: a missing google_id is the normal new-registration signal
		// for the auth service, so it returns (nil, nil) instead of ErrNotFound.
		user, err := repo.FindByGoogleID(ctx, "google-missing-"+uuid.NewString())

		require.NoError(t, err)
		assert.Nil(t, user)
	})
}
