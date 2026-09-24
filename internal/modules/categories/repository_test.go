package categories

import (
	"context"
	"os"
	"testing"
	"time"

	sqlc "walletx-be/internal/platform/database/sqlc"
	"walletx-be/internal/platform/logger"
	apperrors "walletx-be/internal/shared/errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Integration tests require a real PostgreSQL instance with the categories
// schema applied (see supabase/migrations). Set TEST_DATABASE_URL (or
// DATABASE_URL) before running, e.g.:
//
//	$env:TEST_DATABASE_URL = "postgres://postgres:postgres@127.0.0.1:54322/postgres"
//	go test ./internal/modules/categories/ -run TestCategoryRepository -v
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

func newTestRepository(t *testing.T) CategoryRepository {
	t.Helper()
	return NewCategoryRepository(sqlc.New(testPool), nopLogger{})
}

func truncateTables(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := testPool.Exec(ctx, `TRUNCATE public.categories, public.users CASCADE`)
	require.NoError(t, err, "truncate test data")
}

// seedUser inserts a user row so categories can reference it via the
// user_id foreign key and returns the generated user ID.
func seedUser(t *testing.T) uuid.UUID {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := uuid.NewString()
	var id uuid.UUID
	err := testPool.QueryRow(ctx,
		`INSERT INTO public.users (google_id, email, name)
		 VALUES ($1, $2, $3)
		 RETURNING id`,
		"google-"+suffix, "user-"+suffix+"@example.com", "Test User",
	).Scan(&id)
	require.NoError(t, err, "seed user")
	require.NotEqual(t, uuid.Nil, id)
	return id
}

func strPtr(value string) *string {
	return &value
}

// countCategories is an independent read-back helper used to verify state
// directly against the database instead of trusting repository output only.
func countCategories(t *testing.T, userID uuid.UUID, name, categoryType string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var count int
	err := testPool.QueryRow(ctx,
		`SELECT count(*) FROM public.categories
		 WHERE user_id = $1 AND name = $2 AND type = $3`,
		userID, name, categoryType,
	).Scan(&count)
	require.NoError(t, err, "count categories")
	return count
}

func TestCategoryRepository_Create(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx := context.Background()
	repo := newTestRepository(t)
	userID := seedUser(t)

	tests := []struct {
		name         string
		categoryName string
		categoryType string
	}{
		{
			name:         "create expense category",
			categoryName: "Food",
			categoryType: CategoryTypeExpense,
		},
		{
			name:         "create income category",
			categoryName: "Salary",
			categoryType: CategoryTypeIncome,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := time.Now().Add(-time.Minute)

			created, err := repo.Create(ctx, userID, tt.categoryName, tt.categoryType)

			require.NoError(t, err)
			require.NotNil(t, created)
			assert.NotEqual(t, uuid.Nil, created.ID)
			assert.Equal(t, userID, created.UserID)
			assert.Equal(t, tt.categoryName, created.Name)
			assert.Equal(t, tt.categoryType, created.Type)
			assert.False(t, created.CreatedAt.IsZero())
			assert.False(t, created.UpdatedAt.IsZero())
			assert.True(t, created.CreatedAt.After(before), "created_at should be recent")
			assert.True(t, created.CreatedAt.Before(time.Now().Add(time.Minute)), "created_at should not be in the future")

			assert.Equal(t, 1, countCategories(t, userID, tt.categoryName, tt.categoryType))
		})
	}
}

func TestCategoryRepository_Create_SameNameDifferentType_Allowed(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx := context.Background()
	repo := newTestRepository(t)
	userID := seedUser(t)

	expense, err := repo.Create(ctx, userID, "Food", CategoryTypeExpense)
	require.NoError(t, err)
	require.NotNil(t, expense)
	assert.Equal(t, CategoryTypeExpense, expense.Type)

	income, err := repo.Create(ctx, userID, "Food", CategoryTypeIncome)
	require.NoError(t, err)
	require.NotNil(t, income)
	assert.Equal(t, CategoryTypeIncome, income.Type)

	assert.NotEqual(t, expense.ID, income.ID, "same name with different type must be distinct rows")
	assert.Equal(t, userID, income.UserID)
	assert.Equal(t, "Food", income.Name)

	assert.Equal(t, 1, countCategories(t, userID, "Food", CategoryTypeExpense))
	assert.Equal(t, 1, countCategories(t, userID, "Food", CategoryTypeIncome))
}

func TestCategoryRepository_Create_DuplicateUserNameType_ReturnsConflict(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx := context.Background()
	repo := newTestRepository(t)
	userID := seedUser(t)

	first, err := repo.Create(ctx, userID, "Food", CategoryTypeExpense)
	require.NoError(t, err)
	require.NotNil(t, first)

	// Violates categories_user_name_type_key unique (user_id, name, type).
	duplicate, err := repo.Create(ctx, userID, "Food", CategoryTypeExpense)

	assert.Nil(t, duplicate)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrConflict, "unique (user_id, name, type) violation must map to ErrConflict")
	assert.Equal(t, 1, countCategories(t, userID, "Food", CategoryTypeExpense))
}

func TestCategoryRepository_Create_SameNameAndType_DifferentUser_Allowed(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx := context.Background()
	repo := newTestRepository(t)
	userA := seedUser(t)
	userB := seedUser(t)

	createdA, err := repo.Create(ctx, userA, "Food", CategoryTypeExpense)
	require.NoError(t, err)
	require.NotNil(t, createdA)

	createdB, err := repo.Create(ctx, userB, "Food", CategoryTypeExpense)
	require.NoError(t, err)
	require.NotNil(t, createdB)

	assert.NotEqual(t, createdA.ID, createdB.ID)
	assert.Equal(t, userA, createdA.UserID)
	assert.Equal(t, userB, createdB.UserID)
	assert.Equal(t, 1, countCategories(t, userA, "Food", CategoryTypeExpense))
	assert.Equal(t, 1, countCategories(t, userB, "Food", CategoryTypeExpense))
}

func TestCategoryRepository_ListByUser_NilFilter_ReturnsAllTypes(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx := context.Background()
	repo := newTestRepository(t)
	userID := seedUser(t)
	otherUserID := seedUser(t)

	_, err := repo.Create(ctx, userID, "Salary", CategoryTypeIncome)
	require.NoError(t, err)
	_, err = repo.Create(ctx, userID, "Transport", CategoryTypeExpense)
	require.NoError(t, err)
	_, err = repo.Create(ctx, userID, "Food", CategoryTypeExpense)
	require.NoError(t, err)
	_, err = repo.Create(ctx, otherUserID, "Bills", CategoryTypeExpense)
	require.NoError(t, err)

	// A nil *string filter is passed straight to the nullable SQLC parameter
	// (NULL => no type predicate), so both types must be returned.
	list, err := repo.ListByUser(ctx, userID, nil)

	require.NoError(t, err)
	require.Len(t, list, 3, "only the authenticated user's categories must be returned")

	// order by type asc, name asc, id asc => expense rows before income rows.
	expected := []struct {
		name         string
		categoryType string
	}{
		{name: "Food", categoryType: CategoryTypeExpense},
		{name: "Transport", categoryType: CategoryTypeExpense},
		{name: "Salary", categoryType: CategoryTypeIncome},
	}
	for i, want := range expected {
		assert.Equal(t, want.name, list[i].Name, "unexpected order at index %d", i)
		assert.Equal(t, want.categoryType, list[i].Type, "unexpected type at index %d", i)
		assert.Equal(t, userID, list[i].UserID)
		assert.NotEqual(t, uuid.Nil, list[i].ID)
	}

	for _, category := range list {
		assert.NotEqual(t, "Bills", category.Name, "another user's category must never leak")
	}
}

func TestCategoryRepository_ListByUser_FilterByType_ReturnsOnlyMatchingType(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx := context.Background()
	repo := newTestRepository(t)
	userID := seedUser(t)
	otherUserID := seedUser(t)

	_, err := repo.Create(ctx, userID, "Food", CategoryTypeExpense)
	require.NoError(t, err)
	_, err = repo.Create(ctx, userID, "Transport", CategoryTypeExpense)
	require.NoError(t, err)
	_, err = repo.Create(ctx, userID, "Salary", CategoryTypeIncome)
	require.NoError(t, err)
	_, err = repo.Create(ctx, otherUserID, "Bills", CategoryTypeExpense)
	require.NoError(t, err)

	tests := []struct {
		name          string
		filterType    string
		expectedNames []string
	}{
		{
			name:          "filter expense",
			filterType:    CategoryTypeExpense,
			expectedNames: []string{"Food", "Transport"},
		},
		{
			name:          "filter income",
			filterType:    CategoryTypeIncome,
			expectedNames: []string{"Salary"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list, err := repo.ListByUser(ctx, userID, strPtr(tt.filterType))

			require.NoError(t, err)
			require.Len(t, list, len(tt.expectedNames))

			for i, wantName := range tt.expectedNames {
				assert.Equal(t, wantName, list[i].Name)
				assert.Equal(t, tt.filterType, list[i].Type)
				assert.Equal(t, userID, list[i].UserID)
			}
		})
	}
}

func TestCategoryRepository_ListByUser_FilterTypeWithoutMatch_ReturnsEmpty(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx := context.Background()
	repo := newTestRepository(t)
	userID := seedUser(t)

	_, err := repo.Create(ctx, userID, "Food", CategoryTypeExpense)
	require.NoError(t, err)

	list, err := repo.ListByUser(ctx, userID, strPtr(CategoryTypeIncome))

	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestCategoryRepository_ListByUser_NoCategories_ReturnsEmpty(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx := context.Background()
	repo := newTestRepository(t)
	userID := seedUser(t)

	withNilFilter, err := repo.ListByUser(ctx, userID, nil)
	require.NoError(t, err)
	assert.Empty(t, withNilFilter)

	withFilter, err := repo.ListByUser(ctx, userID, strPtr(CategoryTypeExpense))
	require.NoError(t, err)
	assert.Empty(t, withFilter)
}
