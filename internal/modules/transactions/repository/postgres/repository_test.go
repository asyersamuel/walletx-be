package postgres

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	domain "walletx-be/internal/modules/transactions/contracts"
	"walletx-be/internal/platform/database"

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

// Integration tests require a real PostgreSQL instance with the transactions
// schema applied (see supabase/migrations), including the pgvector extension
// and the CHECK (amount > 0) constraint. Set TEST_DATABASE_URL (or
// DATABASE_URL) before running, e.g.:
//
//	$env:TEST_DATABASE_URL = "postgres://postgres:postgres@127.0.0.1:54322/postgres"
//	go test ./internal/modules/transactions/ -run TestTransactionRepository -v
//
// When the variable is not set, tests are skipped so unit-only runs stay fast.

var testPool *pgxpool.Pool

// integrationLockKey is SHARED with the other integration suites (auth,
// categories) on purpose. go test runs package binaries in parallel and every
// suite TRUNCATEs the shared users/categories/transactions tables, so all of
// them take the same PostgreSQL advisory lock to serialize destructive setup.
const integrationLockKey int64 = 742_001

// nopLogger is a silent logger.Logger so tests never emit noise.
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
		t.Skip("TEST_DATABASE_URL/DATABASE_URL not set; skipping transaction repository integration tests")
	}
}

func newTestRepository(t *testing.T) domain.TransactionRepository {
	t.Helper()
	return NewTransactionRepository(sqlc.New(testPool), nopLogger{})
}

// truncateTables wipes every table the suite touches, in FK-safe order.
func truncateTables(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := testPool.Exec(ctx, `TRUNCATE public.transactions, public.categories, public.users CASCADE`)
	require.NoError(t, err, "truncate test data")
}

// seedUser inserts a user row so categories/transactions can reference it and
// returns the generated user ID.
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

// seedCategory inserts a category owned by the given user and returns its ID.
// categoryType must be "expense" or "income".
func seedCategory(t *testing.T, userID uuid.UUID, name, categoryType string) uuid.UUID {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var id uuid.UUID
	err := testPool.QueryRow(ctx,
		`INSERT INTO public.categories (user_id, name, type)
		 VALUES ($1, $2, $3)
		 RETURNING id`,
		userID, name, categoryType,
	).Scan(&id)
	require.NoError(t, err, "seed category")
	require.NotEqual(t, uuid.Nil, id)
	return id
}

func strPtr(value string) *string { return &value }

// countTransactionsForUser is an independent read-back used to assert that a
// failed create left no row behind, without trusting repository output.
func countTransactionsForUser(t *testing.T, userID uuid.UUID) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var count int
	err := testPool.QueryRow(ctx,
		`SELECT count(*) FROM public.transactions WHERE user_id = $1`, userID,
	).Scan(&count)
	require.NoError(t, err, "count transactions")
	return count
}

// storedTransaction is the raw persisted row, read straight back from
// PostgreSQL to verify exactly what landed (including embedding NULL-ness).
type storedTransaction struct {
	userID          uuid.UUID
	categoryID      uuid.UUID
	merchantName    string
	amount          float64
	transactionDate time.Time
	receiptImageUrl *string
	notes           *string
	embeddingIsNull bool
	createdAt       time.Time
	updatedAt       time.Time
}

func fetchStoredTransaction(t *testing.T, id uuid.UUID) storedTransaction {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var s storedTransaction
	err := testPool.QueryRow(ctx,
		`SELECT user_id, category_id, merchant_name, amount, transaction_date,
		        receipt_image_url, notes, (embedding IS NULL), created_at, updated_at
		 FROM public.transactions WHERE id = $1`, id,
	).Scan(
		&s.userID, &s.categoryID, &s.merchantName, &s.amount, &s.transactionDate,
		&s.receiptImageUrl, &s.notes, &s.embeddingIsNull, &s.createdAt, &s.updatedAt,
	)
	require.NoError(t, err, "read back transaction")
	return s
}

// ---------------------------------------------------------------------------
// Happy paths
// ---------------------------------------------------------------------------

func TestTransactionRepository_Create_HappyPaths(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx := context.Background()
	repo := newTestRepository(t)

	// One user owns both an expense and an income category; a second user with
	// her own category exists to prove isolation is not accidentally required
	// for the happy path (different users may use identical category names).
	userID := seedUser(t)
	expenseCat := seedCategory(t, userID, "Food", "expense")
	incomeCat := seedCategory(t, userID, "Salary", "income")

	fixedDate := time.Date(2026, 9, 20, 14, 30, 0, 0, time.UTC)
	before := time.Now().Add(-time.Minute)

	tests := []struct {
		name           string
		input          domain.CreateTransactionInput
		wantCategoryID uuid.UUID
		wantReceipt    *string
		wantNotes      *string
	}{
		{
			name: "expense transaction, optional fields omitted",
			input: domain.CreateTransactionInput{
				UserID:          userID,
				CategoryID:      expenseCat,
				MerchantName:    "AWANN COSTA",
				Amount:          50000,
				TransactionDate: fixedDate,
			},
			wantCategoryID: expenseCat,
			wantReceipt:    nil,
			wantNotes:      nil,
		},
		{
			name: "income transaction with positive amount",
			input: domain.CreateTransactionInput{
				UserID:          userID,
				CategoryID:      incomeCat,
				MerchantName:    "Acme Payroll",
				Amount:          1250000.5,
				TransactionDate: fixedDate,
			},
			wantCategoryID: incomeCat,
			wantReceipt:    nil,
			wantNotes:      nil,
		},
		{
			name: "all optional fields populated",
			input: domain.CreateTransactionInput{
				UserID:          userID,
				CategoryID:      expenseCat,
				MerchantName:    "Tokopedia",
				Amount:          0.01,
				TransactionDate: fixedDate,
				ReceiptImageUrl: strPtr("https://cdn.example.com/receipts/abc.png?v=2"),
				Notes:           strPtr("Kopi & croissant — meeting dgn klien"),
			},
			wantCategoryID: expenseCat,
			wantReceipt:    strPtr("https://cdn.example.com/receipts/abc.png?v=2"),
			wantNotes:      strPtr("Kopi & croissant — meeting dgn klien"),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			created, err := repo.Create(ctx, tt.input)

			require.NoError(t, err)
			require.NotNil(t, created)

			// Returned entity must be fully populated from RETURNING.
			assert.NotEqual(t, uuid.Nil, created.ID)
			assert.Equal(t, tt.input.UserID, created.UserID)
			require.NotNil(t, created.CategoryID)
			assert.Equal(t, tt.wantCategoryID, *created.CategoryID)
			assert.Equal(t, tt.input.MerchantName, created.MerchantName)
			assert.InDelta(t, tt.input.Amount, created.Amount, 1e-9)
			assert.True(t, created.TransactionDate.Equal(tt.input.TransactionDate), "transaction_date instant must be preserved")
			assert.False(t, created.CreatedAt.IsZero())
			assert.False(t, created.UpdatedAt.IsZero())
			assert.True(t, created.CreatedAt.After(before), "created_at should be recent")
			assert.True(t, created.CreatedAt.Before(time.Now().Add(time.Minute)), "created_at should not be in the future")

			// embedding is not selected on create and is generated async later.
			assert.Nil(t, created.Embedding)

			// Optional pointer fields.
			if tt.wantReceipt == nil {
				assert.Nil(t, created.ReceiptImageUrl)
			} else {
				require.NotNil(t, created.ReceiptImageUrl)
				assert.Equal(t, *tt.wantReceipt, *created.ReceiptImageUrl)
			}
			if tt.wantNotes == nil {
				assert.Nil(t, created.Notes)
			} else {
				require.NotNil(t, created.Notes)
				assert.Equal(t, *tt.wantNotes, *created.Notes)
			}

			// Independent read-back proves real persistence, not just the return.
			stored := fetchStoredTransaction(t, created.ID)
			assert.Equal(t, tt.input.UserID, stored.userID)
			assert.Equal(t, tt.wantCategoryID, stored.categoryID)
			assert.Equal(t, tt.input.MerchantName, stored.merchantName)
			assert.InDelta(t, tt.input.Amount, stored.amount, 1e-9)
			assert.True(t, stored.transactionDate.Equal(tt.input.TransactionDate))
			assert.True(t, stored.embeddingIsNull, "embedding must be NULL until the async job runs")
		})
	}
}

// ---------------------------------------------------------------------------
// Amount CHECK constraint (amount > 0)
// ---------------------------------------------------------------------------

func TestTransactionRepository_Create_AmountMustBePositive(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx := context.Background()
	repo := newTestRepository(t)

	userID := seedUser(t)
	categoryID := seedCategory(t, userID, "Food", "expense")
	date := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name   string
		amount float64
	}{
		{name: "zero amount rejected", amount: 0},
		{name: "negative amount rejected", amount: -25000},
		{name: "tiny negative amount rejected", amount: -0.01},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			created, err := repo.Create(ctx, domain.CreateTransactionInput{
				UserID:          userID,
				CategoryID:      categoryID,
				MerchantName:    "Invalid Merchant",
				Amount:          tt.amount,
				TransactionDate: date,
			})

			assert.Nil(t, created)
			require.Error(t, err)
			assert.ErrorIs(t, err, apperrors.ErrInvalidInput, "CHECK (amount > 0) must map to ErrInvalidInput")

			// The failed insert must not have created a partial row.
			assert.Equal(t, 0, countTransactionsForUser(t, userID))
		})
	}

	// Sanity: a positive amount still succeeds for the same user/category.
	ok, err := repo.Create(ctx, domain.CreateTransactionInput{
		UserID:          userID,
		CategoryID:      categoryID,
		MerchantName:    "Valid Merchant",
		Amount:          50000,
		TransactionDate: date,
	})
	require.NoError(t, err)
	require.NotNil(t, ok)
	assert.Equal(t, 1, countTransactionsForUser(t, userID))
}

// ---------------------------------------------------------------------------
// Category relational integrity + cross-user isolation
// ---------------------------------------------------------------------------

func TestTransactionRepository_Create_CategoryIntegrity(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx := context.Background()
	repo := newTestRepository(t)

	owningUser := seedUser(t)
	ownerCategory := seedCategory(t, owningUser, "Food", "expense")

	actingUser := seedUser(t)
	actingCategory := seedCategory(t, actingUser, "Transport", "expense")
	date := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)

	t.Run("non-existent category id returns not found", func(t *testing.T) {
		created, err := repo.Create(ctx, domain.CreateTransactionInput{
			UserID:          actingUser,
			CategoryID:      uuid.New(), // valid UUID shape, but no such row
			MerchantName:    "Ghost Merchant",
			Amount:          100,
			TransactionDate: date,
		})

		assert.Nil(t, created)
		require.Error(t, err)
		assert.ErrorIs(t, err, apperrors.ErrNotFound, "missing category must map to ErrNotFound")
		assert.Equal(t, 0, countTransactionsForUser(t, actingUser))
	})

	t.Run("cross-user category returns not found and stores nothing", func(t *testing.T) {
		created, err := repo.Create(ctx, domain.CreateTransactionInput{
			UserID:          actingUser,
			CategoryID:      ownerCategory, // belongs to owningUser, not actingUser
			MerchantName:    "Stolen Category",
			Amount:          100,
			TransactionDate: date,
		})

		assert.Nil(t, created)
		require.Error(t, err)
		assert.ErrorIs(t, err, apperrors.ErrNotFound,
			"another user's category must be rejected identically to a missing one, without leaking ownership")

		// Neither the acting user nor the category owner gets a transaction row.
		assert.Equal(t, 0, countTransactionsForUser(t, actingUser))
		assert.Equal(t, 0, countTransactionsForUser(t, owningUser))
	})

	t.Run("own category succeeds (control)", func(t *testing.T) {
		created, err := repo.Create(ctx, domain.CreateTransactionInput{
			UserID:          actingUser,
			CategoryID:      actingCategory,
			MerchantName:    "Bluebird Taxi",
			Amount:          75000,
			TransactionDate: date,
		})

		require.NoError(t, err)
		require.NotNil(t, created)
		assert.Equal(t, 1, countTransactionsForUser(t, actingUser))
		// The owner's data is untouched.
		assert.Equal(t, 0, countTransactionsForUser(t, owningUser))
	})
}

// ---------------------------------------------------------------------------
// Timezone instant preservation for transaction_date (timestamptz)
// ---------------------------------------------------------------------------

func TestTransactionRepository_Create_PreservesTimezoneInstant(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx := context.Background()
	repo := newTestRepository(t)

	userID := seedUser(t)
	categoryID := seedCategory(t, userID, "Food", "expense")

	// 14:30 at UTC+7 == 07:30 UTC. The stored instant must equal the input.
	plusSeven := time.FixedZone("UTC+7", 7*60*60)
	input := time.Date(2026, 9, 20, 14, 30, 0, 0, plusSeven)

	created, err := repo.Create(ctx, domain.CreateTransactionInput{
		UserID:          userID,
		CategoryID:      categoryID,
		MerchantName:    "Timezone Cafe",
		Amount:          42000,
		TransactionDate: input,
	})
	require.NoError(t, err)
	require.NotNil(t, created)

	// .Equal compares instants regardless of the zone the driver returns.
	assert.True(t, created.TransactionDate.Equal(input), "returned transaction_date must match the input instant")

	stored := fetchStoredTransaction(t, created.ID)
	assert.True(t, stored.transactionDate.Equal(input), "persisted transaction_date must match the input instant")
	assert.Equal(t, input.UTC().Unix(), stored.transactionDate.UTC().Unix())
}

// ---------------------------------------------------------------------------
// Schema-level constraints that Create's typed Go API cannot express as NULL
// (merchant_name / transaction_date / amount NOT NULL, FKs, pgvector dim).
// These probe PostgreSQL directly to prove the migration is enforced.
// ---------------------------------------------------------------------------

func TestTransactionSchemaConstraints(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	userID := seedUser(t)
	categoryID := seedCategory(t, userID, "Food", "expense")

	// assertPgCode runs raw INSERT SQL and asserts the resulting SQLSTATE.
	assertPgCode := func(t *testing.T, wantCode string, query string, args ...interface{}) {
		t.Helper()
		_, err := testPool.Exec(ctx, query, args...)
		require.Error(t, err)
		var pgErr *pgconn.PgError
		require.True(t, errors.As(err, &pgErr), "expected a *pgconn.PgError, got %v", err)
		assert.Equal(t, wantCode, pgErr.Code, "unexpected SQLSTATE for query")
	}

	t.Run("merchant_name NOT NULL enforced", func(t *testing.T) {
		assertPgCode(t, "23502",
			`INSERT INTO public.transactions (user_id, category_id, merchant_name, amount, transaction_date)
			 VALUES ($1, $2, NULL, $3, $4)`,
			userID, categoryID, 100.0, time.Now(),
		)
		assert.True(t, database.IsNotNullViolation(errorWithCode("23502")))
	})

	t.Run("transaction_date NOT NULL enforced", func(t *testing.T) {
		assertPgCode(t, "23502",
			`INSERT INTO public.transactions (user_id, category_id, merchant_name, amount, transaction_date)
			 VALUES ($1, $2, $3, $4, NULL)`,
			userID, categoryID, "No Date", 100.0,
		)
	})

	t.Run("amount NOT NULL enforced", func(t *testing.T) {
		assertPgCode(t, "23502",
			`INSERT INTO public.transactions (user_id, category_id, merchant_name, amount, transaction_date)
			 VALUES ($1, $2, $3, NULL, $4)`,
			userID, categoryID, "No Amount", time.Now(),
		)
	})

	t.Run("amount CHECK (> 0) enforced", func(t *testing.T) {
		assertPgCode(t, "23514",
			`INSERT INTO public.transactions (user_id, category_id, merchant_name, amount, transaction_date)
			 VALUES ($1, $2, $3, $4, $5)`,
			userID, categoryID, "Zero", 0.0, time.Now(),
		)
		assert.True(t, database.IsCheckViolation(errorWithCode("23514")))
	})

	t.Run("category_id foreign key enforced", func(t *testing.T) {
		assertPgCode(t, "23503",
			`INSERT INTO public.transactions (user_id, category_id, merchant_name, amount, transaction_date)
			 VALUES ($1, $2, $3, $4, $5)`,
			userID, uuid.New(), "No Category", 100.0, time.Now(),
		)
		assert.True(t, database.IsForeignKeyViolation(errorWithCode("23503")))
	})

	t.Run("user_id foreign key enforced", func(t *testing.T) {
		assertPgCode(t, "23503",
			`INSERT INTO public.transactions (user_id, category_id, merchant_name, amount, transaction_date)
			 VALUES ($1, $2, $3, $4, $5)`,
			uuid.New(), categoryID, "No User", 100.0, time.Now(),
		)
	})

	t.Run("pgvector dimension mismatch rejected", func(t *testing.T) {
		// A 3-dim vector must be rejected by the vector(768) column.
		wrong := "[1,2,3]"
		_, err := testPool.Exec(ctx,
			`INSERT INTO public.transactions (user_id, category_id, merchant_name, amount, transaction_date, embedding)
			 VALUES ($1, $2, $3, $4, $5, $6::extensions.vector)`,
			userID, categoryID, "Bad Vector", 100.0, time.Now(), wrong,
		)
		require.Error(t, err, "wrong-dimension embedding must be rejected")

		// A correctly sized 768-dim zero vector must be accepted.
		right := "[" + strings.TrimSuffix(strings.Repeat("0,", 768), ",") + "]"
		_, err = testPool.Exec(ctx,
			`INSERT INTO public.transactions (user_id, category_id, merchant_name, amount, transaction_date, embedding)
			 VALUES ($1, $2, $3, $4, $5, $6::extensions.vector)`,
			userID, categoryID, "Good Vector", 100.0, time.Now(), right,
		)
		require.NoError(t, err, "768-dim embedding must be accepted")
	})
}

// errorWithCode builds a *pgconn.PgError with the given SQLSTATE so the
// database.Is*Violation helpers can be unit-asserted independent of a live
// error round-trip.
func errorWithCode(code string) error {
	return &pgconn.PgError{Code: code}
}

// ---------------------------------------------------------------------------
// Shared fixture helpers for Get/List tests
// ---------------------------------------------------------------------------

// seedTransaction creates a transaction through the (already green) repository
// Create path and returns the persisted entity. Reusing the tested write path
// keeps List/Get fixtures realistic (real ids, real timestamps) without
// duplicating INSERT SQL.
func seedTransaction(t *testing.T, repo domain.TransactionRepository, userID, categoryID uuid.UUID, merchant string, amount float64, date time.Time) *domain.Transaction {
	t.Helper()
	txn, err := repo.Create(context.Background(), domain.CreateTransactionInput{
		UserID:          userID,
		CategoryID:      categoryID,
		MerchantName:    merchant,
		Amount:          amount,
		TransactionDate: date,
	})
	require.NoError(t, err)
	require.NotNil(t, txn)
	return txn
}

func merchants(list []domain.Transaction) []string {
	out := make([]string, len(list))
	for i, tx := range list {
		out[i] = tx.MerchantName
	}
	return out
}

func ids(list []domain.Transaction) []uuid.UUID {
	out := make([]uuid.UUID, len(list))
	for i, tx := range list {
		out[i] = tx.ID
	}
	return out
}

// ---------------------------------------------------------------------------
// Get-by-ID: happy paths
// ---------------------------------------------------------------------------

func TestTransactionRepository_GetTransactionByID_Own(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx := context.Background()
	repo := newTestRepository(t)

	userID := seedUser(t)
	categoryID := seedCategory(t, userID, "Food", "expense")
	date := time.Date(2026, 9, 20, 14, 30, 0, 0, time.UTC)

	// Required fields only.
	minimal := seedTransaction(t, repo, userID, categoryID, "AWANN COSTA", 50000, date)

	// Fully populated optional fields.
	receipt := "https://cdn.example.com/receipts/xyz.png?v=3"
	notes := "Lunch meeting — klien penting"
	full, err := repo.Create(ctx, domain.CreateTransactionInput{
		UserID:          userID,
		CategoryID:      categoryID,
		MerchantName:    "Tokopedia",
		Amount:          1234.56,
		TransactionDate: date,
		ReceiptImageUrl: strPtr(receipt),
		Notes:           strPtr(notes),
	})
	require.NoError(t, err)
	require.NotNil(t, full)

	tests := []struct {
		name        string
		target      *domain.Transaction
		wantReceipt *string
		wantNotes   *string
	}{
		{name: "required fields only", target: minimal, wantReceipt: nil, wantNotes: nil},
		{name: "all fields populated", target: full, wantReceipt: strPtr(receipt), wantNotes: strPtr(notes)},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got, err := repo.GetTransactionByID(ctx, userID, tt.target.ID)

			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tt.target.ID, got.ID)
			assert.Equal(t, userID, got.UserID)
			require.NotNil(t, got.CategoryID)
			assert.Equal(t, categoryID, *got.CategoryID)
			assert.Equal(t, tt.target.MerchantName, got.MerchantName)
			assert.InDelta(t, tt.target.Amount, got.Amount, 1e-9)
			assert.True(t, got.TransactionDate.Equal(date), "transaction_date instant must be preserved")
			assert.False(t, got.CreatedAt.IsZero())
			assert.False(t, got.UpdatedAt.IsZero())
			assert.Nil(t, got.Embedding, "embedding is not loaded in this iteration")

			if tt.wantReceipt == nil {
				assert.Nil(t, got.ReceiptImageUrl)
			} else {
				require.NotNil(t, got.ReceiptImageUrl)
				assert.Equal(t, *tt.wantReceipt, *got.ReceiptImageUrl)
			}
			if tt.wantNotes == nil {
				assert.Nil(t, got.Notes)
			} else {
				require.NotNil(t, got.Notes)
				assert.Equal(t, *tt.wantNotes, *got.Notes)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Get-by-ID: cross-user isolation + not-found
// ---------------------------------------------------------------------------

func TestTransactionRepository_GetTransactionByID_Isolation(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx := context.Background()
	repo := newTestRepository(t)

	owner := seedUser(t)
	ownerCat := seedCategory(t, owner, "Food", "expense")
	other := seedUser(t)
	date := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)

	ownerTxn := seedTransaction(t, repo, owner, ownerCat, "Owner Private", 999, date)

	t.Run("another user cannot read the transaction", func(t *testing.T) {
		got, err := repo.GetTransactionByID(ctx, other, ownerTxn.ID)

		assert.Nil(t, got, "cross-user read must return no entity (no merchant/amount leak)")
		require.Error(t, err)
		assert.ErrorIs(t, err, apperrors.ErrNotFound,
			"another user's transaction must be indistinguishable from a missing one")
	})

	t.Run("non-existent id returns not found", func(t *testing.T) {
		got, err := repo.GetTransactionByID(ctx, owner, uuid.New())

		assert.Nil(t, got)
		require.Error(t, err)
		assert.ErrorIs(t, err, apperrors.ErrNotFound)
	})

	t.Run("owner can still read own transaction (control)", func(t *testing.T) {
		got, err := repo.GetTransactionByID(ctx, owner, ownerTxn.ID)

		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, ownerTxn.ID, got.ID)
		assert.Equal(t, owner, got.UserID)
	})
}

// ---------------------------------------------------------------------------
// List: scope strictly to the caller (cross-user isolation)
// ---------------------------------------------------------------------------

func TestTransactionRepository_ListTransactions_ScopesToUser(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx := context.Background()
	repo := newTestRepository(t)

	userA := seedUser(t)
	catA := seedCategory(t, userA, "Food", "expense")
	userB := seedUser(t)
	catB := seedCategory(t, userB, "Food", "expense") // same category name, different owner
	base := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)

	seedTransaction(t, repo, userA, catA, "A1", 10, base)
	seedTransaction(t, repo, userA, catA, "A2", 20, base.Add(-time.Hour))
	seedTransaction(t, repo, userA, catA, "A3", 30, base.Add(-2*time.Hour))
	bSecret := seedTransaction(t, repo, userB, catB, "B1-SECRET", 40, base)

	// A date range that would ALSO match userB's row if isolation were broken.
	start := base.Add(-3 * time.Hour)
	end := base

	list, err := repo.ListTransactions(ctx, domain.ListTransactionsInput{
		UserID: userA, Limit: 50, Offset: 0, StartDate: &start, EndDate: &end,
	})

	require.NoError(t, err)
	require.NotNil(t, list)
	require.Len(t, list, 3, "only the caller's transactions must be returned")
	assert.ElementsMatch(t, []string{"A1", "A2", "A3"}, merchants(list))
	for _, tx := range list {
		assert.Equal(t, userA, tx.UserID)
		assert.NotEqual(t, bSecret.ID, tx.ID)
		assert.NotEqual(t, "B1-SECRET", tx.MerchantName, "another user's row must never leak")
	}
}

// ---------------------------------------------------------------------------
// List: pagination windows (deterministic newest-first ordering)
// ---------------------------------------------------------------------------

func TestTransactionRepository_ListTransactions_Pagination(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx := context.Background()
	repo := newTestRepository(t)

	userID := seedUser(t)
	cat := seedCategory(t, userID, "Food", "expense")
	base := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)

	// Strictly descending transaction_date => deterministic newest-first order:
	// M1(12:00) > M2(11:00) > M3(10:00) > M4(09:00) > M5(08:00).
	seededPtrs := []*domain.Transaction{
		seedTransaction(t, repo, userID, cat, "M1", 1, base),
		seedTransaction(t, repo, userID, cat, "M2", 2, base.Add(-1*time.Hour)),
		seedTransaction(t, repo, userID, cat, "M3", 3, base.Add(-2*time.Hour)),
		seedTransaction(t, repo, userID, cat, "M4", 4, base.Add(-3*time.Hour)),
		seedTransaction(t, repo, userID, cat, "M5", 5, base.Add(-4*time.Hour)),
	}
	seeded := make([]domain.Transaction, len(seededPtrs))
	for i, p := range seededPtrs {
		seeded[i] = *p
	}
	fullOrder := merchants(seeded) // == date desc since dates are distinct

	tests := []struct {
		name   string
		limit  int
		offset int
		want   []string
	}{
		{name: "first page", limit: 2, offset: 0, want: []string{"M1", "M2"}},
		{name: "second page", limit: 2, offset: 2, want: []string{"M3", "M4"}},
		{name: "last partial page", limit: 2, offset: 4, want: []string{"M5"}},
		{name: "whole list in one page", limit: 10, offset: 0, want: fullOrder},
		{name: "offset beyond end returns empty", limit: 2, offset: 10, want: []string{}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			page, err := repo.ListTransactions(ctx, domain.ListTransactionsInput{
				UserID: userID, Limit: tt.limit, Offset: tt.offset,
			})
			require.NoError(t, err)
			require.NotNil(t, page, "result must be a non-nil slice even when empty")
			assert.Equal(t, tt.want, merchants(page))
		})
	}

	// No duplicate or skipped rows across the three real pages: their union is
	// exactly the five seeded ids, all distinct.
	p0, err := repo.ListTransactions(ctx, domain.ListTransactionsInput{UserID: userID, Limit: 2, Offset: 0})
	require.NoError(t, err)
	p1, err := repo.ListTransactions(ctx, domain.ListTransactionsInput{UserID: userID, Limit: 2, Offset: 2})
	require.NoError(t, err)
	p2, err := repo.ListTransactions(ctx, domain.ListTransactionsInput{UserID: userID, Limit: 2, Offset: 4})
	require.NoError(t, err)

	union := append(append(ids(p0), ids(p1)...), ids(p2)...)
	assert.Len(t, union, 5)
	assert.ElementsMatch(t, ids(seeded), union)

	seen := make(map[uuid.UUID]bool, len(union))
	for _, id := range union {
		assert.False(t, seen[id], "a row must not appear on more than one page")
		seen[id] = true
	}
}

// ---------------------------------------------------------------------------
// List: deterministic tie-break by id when transaction_date is identical
// ---------------------------------------------------------------------------

func TestTransactionRepository_ListTransactions_TieBreakByID(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx := context.Background()
	repo := newTestRepository(t)

	userID := seedUser(t)
	cat := seedCategory(t, userID, "Food", "expense")
	sameInstant := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)

	a := seedTransaction(t, repo, userID, cat, "TieA", 10, sameInstant)
	b := seedTransaction(t, repo, userID, cat, "TieB", 20, sameInstant)

	list, err := repo.ListTransactions(ctx, domain.ListTransactionsInput{UserID: userID, Limit: 10, Offset: 0})
	require.NoError(t, err)
	require.Len(t, list, 2)

	// With equal transaction_date the contract is id desc.
	var wantIDs []uuid.UUID
	if bytes.Compare(a.ID[:], b.ID[:]) >= 0 {
		wantIDs = []uuid.UUID{a.ID, b.ID}
	} else {
		wantIDs = []uuid.UUID{b.ID, a.ID}
	}
	assert.Equal(t, wantIDs, ids(list), "tie on transaction_date must break by id desc")

	// Stability: an identical query must return an identical ordering.
	again, err := repo.ListTransactions(ctx, domain.ListTransactionsInput{UserID: userID, Limit: 10, Offset: 0})
	require.NoError(t, err)
	assert.Equal(t, ids(list), ids(again), "ordering must be stable across calls")
}

// ---------------------------------------------------------------------------
// List: limit/offset normalization guards against unsafe SQL
// ---------------------------------------------------------------------------

func TestTransactionRepository_ListTransactions_LimitOffsetNormalization(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx := context.Background()
	repo := newTestRepository(t)

	userID := seedUser(t)
	cat := seedCategory(t, userID, "Food", "expense")
	base := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	const total = 3
	seedTransaction(t, repo, userID, cat, "N0", 1, base)
	seedTransaction(t, repo, userID, cat, "N1", 2, base.Add(-time.Hour))
	seedTransaction(t, repo, userID, cat, "N2", 3, base.Add(-2*time.Hour))

	tests := []struct {
		name      string
		limit     int
		offset    int
		wantCount int
	}{
		{name: "zero limit falls back to default (returns all available)", limit: 0, offset: 0, wantCount: total},
		{name: "negative limit falls back to default", limit: -5, offset: 0, wantCount: total},
		{name: "negative offset treated as zero", limit: 10, offset: -5, wantCount: total},
		{name: "very large limit returns available rows without error", limit: 100000, offset: 0, wantCount: total},
		{name: "offset beyond count returns empty", limit: 10, offset: 100, wantCount: 0},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			list, err := repo.ListTransactions(ctx, domain.ListTransactionsInput{
				UserID: userID, Limit: tt.limit, Offset: tt.offset,
			})
			require.NoError(t, err, "normalization must avoid PostgreSQL LIMIT/OFFSET errors")
			require.NotNil(t, list, "result must be a non-nil slice even when empty")
			assert.Len(t, list, tt.wantCount)
		})
	}
}

// ---------------------------------------------------------------------------
// List: inclusive date-range filtering (applied before pagination)
// ---------------------------------------------------------------------------

func TestTransactionRepository_ListTransactions_DateRangeFiltering(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx := context.Background()
	repo := newTestRepository(t)

	userID := seedUser(t)
	cat := seedCategory(t, userID, "Food", "expense")
	base := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)

	// Chronological: D1(08:00) < D2(09:00) < D3(10:00) < D4(11:00) < D5(12:00).
	d1 := base.Add(-4 * time.Hour)
	d2 := base.Add(-3 * time.Hour)
	d3 := base.Add(-2 * time.Hour)
	d4 := base.Add(-1 * time.Hour)
	d5 := base
	seedTransaction(t, repo, userID, cat, "D1", 1, d1)
	seedTransaction(t, repo, userID, cat, "D2", 2, d2)
	seedTransaction(t, repo, userID, cat, "D3", 3, d3)
	seedTransaction(t, repo, userID, cat, "D4", 4, d4)
	seedTransaction(t, repo, userID, cat, "D5", 5, d5)

	at := func(tm time.Time) *time.Time { return &tm }

	tests := []struct {
		name  string
		start *time.Time
		end   *time.Time
		want  []string // membership asserted order-insensitively
	}{
		{name: "start only (inclusive)", start: at(d3), end: nil, want: []string{"D3", "D4", "D5"}},
		{name: "end only (inclusive)", start: nil, end: at(d3), want: []string{"D1", "D2", "D3"}},
		{name: "both bounds (inclusive)", start: at(d2), end: at(d4), want: []string{"D2", "D3", "D4"}},
		{name: "start == end matches the single instant", start: at(d3), end: at(d3), want: []string{"D3"}},
		{name: "start after end returns empty (no misleading rows)", start: at(d4), end: at(d2), want: []string{}},
		{name: "no bounds returns all", start: nil, end: nil, want: []string{"D1", "D2", "D3", "D4", "D5"}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			list, err := repo.ListTransactions(ctx, domain.ListTransactionsInput{
				UserID: userID, Limit: 50, Offset: 0, StartDate: tt.start, EndDate: tt.end,
			})
			require.NoError(t, err)
			require.NotNil(t, list)
			assert.ElementsMatch(t, tt.want, merchants(list))
		})
	}

	// The date filter runs in WHERE, BEFORE LIMIT/OFFSET. With end=d4 the newest
	// row (D5) is excluded, so the first page of size 2 must be D4 then D3 — not
	// D5 (which would appear if pagination ran before filtering).
	page, err := repo.ListTransactions(ctx, domain.ListTransactionsInput{
		UserID: userID, Limit: 2, Offset: 0, EndDate: at(d4),
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"D4", "D3"}, merchants(page),
		"date filter must be applied before pagination")
}

// ---------------------------------------------------------------------------
// List: date bounds compare absolute instants across timezones
// ---------------------------------------------------------------------------

func TestTransactionRepository_ListTransactions_DateFilterUsesAbsoluteInstant(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx := context.Background()
	repo := newTestRepository(t)

	userID := seedUser(t)
	cat := seedCategory(t, userID, "Food", "expense")

	// A single transaction at 12:00 UTC, which is 19:00 at UTC+7 and 07:00 at UTC-5.
	instantUTC := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	seedTransaction(t, repo, userID, cat, "Instant", 100, instantUTC)

	plusSeven := time.FixedZone("UTC+7", 7*3600)
	minusFive := time.FixedZone("UTC-5", -5*3600)
	startSameInstant := time.Date(2026, 9, 20, 19, 0, 0, 0, plusSeven) // == instantUTC
	endSameInstant := time.Date(2026, 9, 20, 7, 0, 0, 0, minusFive)    // == instantUTC

	list, err := repo.ListTransactions(ctx, domain.ListTransactionsInput{
		UserID: userID, Limit: 10, Offset: 0,
		StartDate: &startSameInstant, EndDate: &endSameInstant,
	})
	require.NoError(t, err)
	require.Len(t, list, 1, "bounds in other zones but the same instant must still include the row (inclusive both ends)")
	assert.Equal(t, "Instant", list[0].MerchantName)

	// One second before/after the instant excludes it — proving an exact
	// absolute-instant comparison, not a wall-clock string compare.
	after := instantUTC.Add(time.Second)
	before := instantUTC.Add(-time.Second)

	tooLate, err := repo.ListTransactions(ctx, domain.ListTransactionsInput{UserID: userID, Limit: 10, Offset: 0, StartDate: &after})
	require.NoError(t, err)
	assert.Len(t, tooLate, 0, "start one second after the instant must exclude it")

	tooEarly, err := repo.ListTransactions(ctx, domain.ListTransactionsInput{UserID: userID, Limit: 10, Offset: 0, EndDate: &before})
	require.NoError(t, err)
	assert.Len(t, tooEarly, 0, "end one second before the instant must exclude it")
}

// ---------------------------------------------------------------------------
// Shared fixture helpers for Update/Delete tests
// ---------------------------------------------------------------------------

func float64Ptr(v float64) *float64  { return &v }
func timePtr(v time.Time) *time.Time { return &v }
func uuidPtr(v uuid.UUID) *uuid.UUID { return &v }

// categoryExists reads the categories table directly to prove that deleting or
// updating a transaction never touches its category.
func categoryExists(t *testing.T, categoryID uuid.UUID) bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var exists bool
	err := testPool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM public.categories WHERE id = $1)`, categoryID,
	).Scan(&exists)
	require.NoError(t, err, "check category existence")
	return exists
}

// ---------------------------------------------------------------------------
// Update: partial update writes only supplied fields
// ---------------------------------------------------------------------------

func TestTransactionRepository_UpdateTransaction_PartialUpdate(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx := context.Background()
	repo := newTestRepository(t)

	userID := seedUser(t)
	categoryID := seedCategory(t, userID, "Food", "expense")
	originalDate := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	created := seedTransaction(t, repo, userID, categoryID, "Original Merchant", 100, originalDate)

	// Change ONLY the amount; every other field is left nil (=> keep current).
	updated, err := repo.UpdateTransaction(ctx, domain.UpdateTransactionInput{
		UserID:        userID,
		TransactionID: created.ID,
		Amount:        float64Ptr(250.75),
	})
	require.NoError(t, err)
	require.NotNil(t, updated)

	// Changed field.
	assert.InDelta(t, 250.75, updated.Amount, 1e-9)
	// Preserved fields (nil in input => unchanged).
	assert.Equal(t, created.ID, updated.ID)
	assert.Equal(t, userID, updated.UserID, "ownership must never change")
	require.NotNil(t, updated.CategoryID)
	assert.Equal(t, categoryID, *updated.CategoryID, "category preserved when not supplied")
	assert.Equal(t, "Original Merchant", updated.MerchantName)
	assert.True(t, updated.TransactionDate.Equal(originalDate), "transaction_date preserved when not supplied")
	assert.Nil(t, updated.ReceiptImageUrl)
	assert.Nil(t, updated.Notes)
	assert.Nil(t, updated.Embedding)
	assert.False(t, updated.UpdatedAt.IsZero())
	assert.False(t, updated.UpdatedAt.Before(created.CreatedAt), "updated_at must be refreshed on update")

	// Independent read-back proves persistence.
	stored := fetchStoredTransaction(t, created.ID)
	assert.InDelta(t, 250.75, stored.amount, 1e-9)
	assert.Equal(t, "Original Merchant", stored.merchantName)
	assert.True(t, stored.transactionDate.Equal(originalDate))
	assert.Equal(t, categoryID, stored.categoryID)
	assert.Nil(t, stored.receiptImageUrl)
	assert.Nil(t, stored.notes)
}

// ---------------------------------------------------------------------------
// Update: full update rewrites every mutable field
// ---------------------------------------------------------------------------

func TestTransactionRepository_UpdateTransaction_FullUpdate(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx := context.Background()
	repo := newTestRepository(t)

	userID := seedUser(t)
	firstCat := seedCategory(t, userID, "Food", "expense")
	secondCat := seedCategory(t, userID, "Transport", "expense")
	originalDate := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	created := seedTransaction(t, repo, userID, firstCat, "Original", 100, originalDate)

	newDate := time.Date(2026, 10, 1, 9, 15, 0, 0, time.UTC)
	newReceipt := "https://cdn.example.com/receipts/new.png"
	newNotes := "Updated notes — reimbursement"

	updated, err := repo.UpdateTransaction(ctx, domain.UpdateTransactionInput{
		UserID:          userID,
		TransactionID:   created.ID,
		MerchantName:    strPtr("New Merchant"),
		Amount:          float64Ptr(999.99),
		TransactionDate: timePtr(newDate),
		CategoryID:      uuidPtr(secondCat),
		ReceiptImageUrl: strPtr(newReceipt),
		Notes:           strPtr(newNotes),
	})
	require.NoError(t, err)
	require.NotNil(t, updated)

	assert.Equal(t, created.ID, updated.ID)
	assert.Equal(t, userID, updated.UserID, "ownership must never change")
	assert.Equal(t, "New Merchant", updated.MerchantName)
	assert.InDelta(t, 999.99, updated.Amount, 1e-9)
	assert.True(t, updated.TransactionDate.Equal(newDate))
	require.NotNil(t, updated.CategoryID)
	assert.Equal(t, secondCat, *updated.CategoryID, "category moved to the caller's other category")
	require.NotNil(t, updated.ReceiptImageUrl)
	assert.Equal(t, newReceipt, *updated.ReceiptImageUrl)
	require.NotNil(t, updated.Notes)
	assert.Equal(t, newNotes, *updated.Notes)

	stored := fetchStoredTransaction(t, created.ID)
	assert.Equal(t, "New Merchant", stored.merchantName)
	assert.InDelta(t, 999.99, stored.amount, 1e-9)
	assert.True(t, stored.transactionDate.Equal(newDate))
	assert.Equal(t, secondCat, stored.categoryID)
	require.NotNil(t, stored.receiptImageUrl)
	assert.Equal(t, newReceipt, *stored.receiptImageUrl)
	require.NotNil(t, stored.notes)
	assert.Equal(t, newNotes, *stored.notes)
}

// ---------------------------------------------------------------------------
// Update: amount CHECK (> 0) constraint
// ---------------------------------------------------------------------------

func TestTransactionRepository_UpdateTransaction_AmountMustBePositive(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx := context.Background()
	repo := newTestRepository(t)

	userID := seedUser(t)
	categoryID := seedCategory(t, userID, "Food", "expense")
	created := seedTransaction(t, repo, userID, categoryID, "Merchant", 100, time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC))

	tests := []struct {
		name   string
		amount float64
	}{
		{name: "zero amount rejected", amount: 0},
		{name: "negative amount rejected", amount: -50},
		{name: "tiny negative rejected", amount: -0.01},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			updated, err := repo.UpdateTransaction(ctx, domain.UpdateTransactionInput{
				UserID:        userID,
				TransactionID: created.ID,
				Amount:        float64Ptr(tt.amount),
			})
			assert.Nil(t, updated)
			require.Error(t, err)
			assert.ErrorIs(t, err, apperrors.ErrInvalidInput, "CHECK (amount > 0) must map to ErrInvalidInput")

			// The rejected update must leave the stored amount unchanged.
			stored := fetchStoredTransaction(t, created.ID)
			assert.InDelta(t, 100.0, stored.amount, 1e-9, "amount unchanged after a rejected update")
		})
	}

	// Sanity: a positive amount still succeeds.
	ok, err := repo.UpdateTransaction(ctx, domain.UpdateTransactionInput{
		UserID: userID, TransactionID: created.ID, Amount: float64Ptr(200),
	})
	require.NoError(t, err)
	require.NotNil(t, ok)
	assert.InDelta(t, 200.0, ok.Amount, 1e-9)
}

// ---------------------------------------------------------------------------
// Update: category ownership is enforced when changing category_id
// ---------------------------------------------------------------------------

func TestTransactionRepository_UpdateTransaction_CategoryOwnership(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx := context.Background()
	repo := newTestRepository(t)

	owner := seedUser(t)
	ownerCatA := seedCategory(t, owner, "Food", "expense")
	ownerCatB := seedCategory(t, owner, "Transport", "expense")
	other := seedUser(t)
	otherCat := seedCategory(t, other, "Secret", "expense")
	date := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	tx := seedTransaction(t, repo, owner, ownerCatA, "Merchant", 100, date)

	t.Run("move to another owned category succeeds", func(t *testing.T) {
		updated, err := repo.UpdateTransaction(ctx, domain.UpdateTransactionInput{
			UserID: owner, TransactionID: tx.ID, CategoryID: uuidPtr(ownerCatB),
		})
		require.NoError(t, err)
		require.NotNil(t, updated)
		require.NotNil(t, updated.CategoryID)
		assert.Equal(t, ownerCatB, *updated.CategoryID)
	})

	// Reset back to ownerCatA so the isolation subtests have a stable baseline.
	_, err := repo.UpdateTransaction(ctx, domain.UpdateTransactionInput{
		UserID: owner, TransactionID: tx.ID, CategoryID: uuidPtr(ownerCatA),
	})
	require.NoError(t, err)

	t.Run("move to another user's category is rejected and changes nothing", func(t *testing.T) {
		updated, err := repo.UpdateTransaction(ctx, domain.UpdateTransactionInput{
			UserID: owner, TransactionID: tx.ID, CategoryID: uuidPtr(otherCat),
		})
		assert.Nil(t, updated)
		require.Error(t, err)
		assert.ErrorIs(t, err, apperrors.ErrNotFound,
			"another user's category must be rejected like a missing one, without leaking ownership")
		stored := fetchStoredTransaction(t, tx.ID)
		assert.Equal(t, ownerCatA, stored.categoryID, "category unchanged after a rejected move")
		assert.Equal(t, "Merchant", stored.merchantName)
	})

	t.Run("move to a non-existent category is rejected", func(t *testing.T) {
		updated, err := repo.UpdateTransaction(ctx, domain.UpdateTransactionInput{
			UserID: owner, TransactionID: tx.ID, CategoryID: uuidPtr(uuid.New()),
		})
		assert.Nil(t, updated)
		require.Error(t, err)
		assert.ErrorIs(t, err, apperrors.ErrNotFound)
		stored := fetchStoredTransaction(t, tx.ID)
		assert.Equal(t, ownerCatA, stored.categoryID)
	})

	t.Run("not supplying a category skips the guard and keeps it", func(t *testing.T) {
		updated, err := repo.UpdateTransaction(ctx, domain.UpdateTransactionInput{
			UserID: owner, TransactionID: tx.ID, MerchantName: strPtr("Renamed"),
		})
		require.NoError(t, err)
		require.NotNil(t, updated)
		assert.Equal(t, "Renamed", updated.MerchantName)
		require.NotNil(t, updated.CategoryID)
		assert.Equal(t, ownerCatA, *updated.CategoryID, "category preserved when not supplied")
	})
}

// ---------------------------------------------------------------------------
// Update: cross-user isolation + not-found
// ---------------------------------------------------------------------------

func TestTransactionRepository_UpdateTransaction_CrossUserIsolationAndNotFound(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx := context.Background()
	repo := newTestRepository(t)

	owner := seedUser(t)
	ownerCat := seedCategory(t, owner, "Food", "expense")
	other := seedUser(t)
	date := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	tx := seedTransaction(t, repo, owner, ownerCat, "Owner Merchant", 100, date)

	t.Run("another user cannot update the transaction", func(t *testing.T) {
		updated, err := repo.UpdateTransaction(ctx, domain.UpdateTransactionInput{
			UserID: other, TransactionID: tx.ID, MerchantName: strPtr("Hijacked"),
		})
		assert.Nil(t, updated)
		require.Error(t, err)
		assert.ErrorIs(t, err, apperrors.ErrNotFound)

		// The owner's row is untouched.
		stored := fetchStoredTransaction(t, tx.ID)
		assert.Equal(t, "Owner Merchant", stored.merchantName)
		assert.Equal(t, owner, stored.userID)
	})

	t.Run("updating a non-existent transaction returns not found", func(t *testing.T) {
		updated, err := repo.UpdateTransaction(ctx, domain.UpdateTransactionInput{
			UserID: owner, TransactionID: uuid.New(), MerchantName: strPtr("Ghost"),
		})
		assert.Nil(t, updated)
		require.Error(t, err)
		assert.ErrorIs(t, err, apperrors.ErrNotFound)
	})
}

// ---------------------------------------------------------------------------
// Delete: owner succeeds
// ---------------------------------------------------------------------------

func TestTransactionRepository_DeleteTransaction_OwnSucceeds(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx := context.Background()
	repo := newTestRepository(t)

	userID := seedUser(t)
	categoryID := seedCategory(t, userID, "Food", "expense")
	tx := seedTransaction(t, repo, userID, categoryID, "ToDelete", 100, time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC))
	require.Equal(t, 1, countTransactionsForUser(t, userID))

	err := repo.DeleteTransaction(ctx, userID, tx.ID)
	require.NoError(t, err)

	assert.Equal(t, 0, countTransactionsForUser(t, userID), "row must be removed")

	// A subsequent Get must report not found.
	got, err := repo.GetTransactionByID(ctx, userID, tx.ID)
	assert.Nil(t, got)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

// ---------------------------------------------------------------------------
// Delete: cross-user isolation (zero rows affected)
// ---------------------------------------------------------------------------

func TestTransactionRepository_DeleteTransaction_CrossUserIsolation(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx := context.Background()
	repo := newTestRepository(t)

	owner := seedUser(t)
	ownerCat := seedCategory(t, owner, "Food", "expense")
	other := seedUser(t)
	tx := seedTransaction(t, repo, owner, ownerCat, "Owner Secret", 100, time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC))

	// Another user attempts to delete -> ErrNotFound, zero rows affected.
	err := repo.DeleteTransaction(ctx, other, tx.ID)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
	assert.Equal(t, 1, countTransactionsForUser(t, owner), "owner's row must survive a cross-user delete attempt")

	// The owner can still delete it.
	require.NoError(t, repo.DeleteTransaction(ctx, owner, tx.ID))
	assert.Equal(t, 0, countTransactionsForUser(t, owner))
}

// ---------------------------------------------------------------------------
// Delete: not-found + category and sibling rows untouched
// ---------------------------------------------------------------------------

func TestTransactionRepository_DeleteTransaction_NotFoundAndCategoryUntouched(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx := context.Background()
	repo := newTestRepository(t)

	userID := seedUser(t)
	categoryID := seedCategory(t, userID, "Food", "expense")
	tx1 := seedTransaction(t, repo, userID, categoryID, "Keep", 100, time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC))
	tx2 := seedTransaction(t, repo, userID, categoryID, "Delete", 200, time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC))
	require.Equal(t, 2, countTransactionsForUser(t, userID))

	// Deleting a non-existent id returns not found and affects nothing.
	err := repo.DeleteTransaction(ctx, userID, uuid.New())
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
	assert.Equal(t, 2, countTransactionsForUser(t, userID))

	// Delete one; the sibling and the category remain.
	require.NoError(t, repo.DeleteTransaction(ctx, userID, tx2.ID))
	assert.Equal(t, 1, countTransactionsForUser(t, userID), "only the targeted row is removed")
	assert.True(t, categoryExists(t, categoryID), "deleting a transaction must not delete its category")

	got, err := repo.GetTransactionByID(ctx, userID, tx1.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Keep", got.MerchantName)
}

// ---------------------------------------------------------------------------
// Transactional and concurrency behavior
// ---------------------------------------------------------------------------

func embeddingWithMarker(marker float32) []float32 {
	embedding := make([]float32, domain.EmbeddingDimension)
	embedding[0] = marker
	return embedding
}

func readEmbeddingMarker(t *testing.T, transactionID uuid.UUID) (float64, bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var present bool
	var literal string
	err := testPool.QueryRow(ctx,
		`SELECT embedding IS NOT NULL, COALESCE(embedding::text, '')
		 FROM public.transactions WHERE id = $1`, transactionID,
	).Scan(&present, &literal)
	require.NoError(t, err, "read embedding")
	if !present {
		return 0, false
	}

	components := strings.Split(strings.Trim(literal, "[]"), ",")
	require.NotEmpty(t, components, "vector literal must contain a first component")
	marker, err := strconv.ParseFloat(strings.TrimSpace(components[0]), 64)
	require.NoError(t, err, "parse vector marker")
	return marker, true
}

func countEmbeddingsForUser(t *testing.T, userID uuid.UUID) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var count int
	err := testPool.QueryRow(ctx,
		`SELECT count(*) FROM public.transactions
		 WHERE user_id = $1 AND embedding IS NOT NULL`, userID,
	).Scan(&count)
	require.NoError(t, err, "count embeddings")
	return count
}

func TestTransactionRepository_ConcurrentCreatesProduceDistinctIDs(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	repo := newTestRepository(t)
	userID := seedUser(t)
	categoryID := seedCategory(t, userID, "Concurrent", "expense")

	const workers = 16
	start := make(chan struct{})
	results := make([]*domain.Transaction, workers)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		i := i
		go func() {
			defer wg.Done()
			<-start
			results[i], errs[i] = repo.Create(context.Background(), domain.CreateTransactionInput{
				UserID:          userID,
				CategoryID:      categoryID,
				MerchantName:    "concurrent-" + strconv.Itoa(i),
				Amount:          float64(i + 1),
				TransactionDate: time.Date(2026, 9, 20, 12, i, 0, 0, time.UTC),
			})
		}()
	}
	close(start)
	wg.Wait()

	seen := make(map[uuid.UUID]struct{}, workers)
	for i := 0; i < workers; i++ {
		require.NoError(t, errs[i], "concurrent create %d", i)
		require.NotNil(t, results[i])
		_, duplicate := seen[results[i].ID]
		assert.False(t, duplicate, "concurrent creates must return distinct UUIDs")
		seen[results[i].ID] = struct{}{}
		assert.Equal(t, userID, results[i].UserID)
	}
	assert.Len(t, seen, workers)
	assert.Equal(t, workers, countTransactionsForUser(t, userID))
}

func TestTransactionRepository_ConcurrentCreateEmbeddingAssociation(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	repo := newTestRepository(t)
	userID := seedUser(t)
	categoryID := seedCategory(t, userID, "Concurrent Embeddings", "expense")

	const workers = 12
	type result struct {
		transactionID uuid.UUID
		marker        float64
		err           error
	}
	results := make([]result, workers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		i := i
		go func() {
			defer wg.Done()
			<-start
			created, err := repo.Create(context.Background(), domain.CreateTransactionInput{
				UserID:          userID,
				CategoryID:      categoryID,
				MerchantName:    "create-embed-" + strconv.Itoa(i),
				Amount:          float64(i + 1),
				TransactionDate: time.Date(2026, 9, 20, 13, i, 0, 0, time.UTC),
			})
			if err != nil {
				results[i].err = err
				return
			}
			marker := float32(i + 1)
			results[i].transactionID = created.ID
			results[i].marker = float64(marker)
			results[i].err = repo.SaveEmbedding(context.Background(), created.ID, userID, embeddingWithMarker(marker), created.UpdatedAt)
		}()
	}
	close(start)
	wg.Wait()

	seen := make(map[uuid.UUID]struct{}, workers)
	for i, item := range results {
		require.NoError(t, item.err, "create/save embedding worker %d", i)
		_, duplicate := seen[item.transactionID]
		assert.False(t, duplicate, "each worker must retain its own transaction UUID")
		seen[item.transactionID] = struct{}{}
		actual, present := readEmbeddingMarker(t, item.transactionID)
		require.True(t, present)
		assert.InDelta(t, item.marker, actual, 0.0001,
			"embedding must remain associated with the transaction that created it")
	}
	assert.Len(t, seen, workers)
	assert.Equal(t, workers, countEmbeddingsForUser(t, userID))
}

func TestTransactionRepository_ConcurrentEmbeddingSavesDoNotMixTransactions(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	repo := newTestRepository(t)
	userID := seedUser(t)
	categoryID := seedCategory(t, userID, "Embedding Saves", "expense")

	const workers = 10
	transactions := make([]*domain.Transaction, workers)
	for i := range transactions {
		transactions[i] = seedTransaction(t, repo, userID, categoryID,
			"embedding-target-"+strconv.Itoa(i), float64(i+1),
			time.Date(2026, 9, 20, 14, i, 0, 0, time.UTC))
	}

	errs := make([]error, workers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := range transactions {
		i := i
		go func() {
			defer wg.Done()
			<-start
			errs[i] = repo.SaveEmbedding(context.Background(), transactions[i].ID, userID,
				embeddingWithMarker(float32(100+i)), transactions[i].UpdatedAt)
		}()
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		require.NoError(t, err, "embedding save %d", i)
		actual, present := readEmbeddingMarker(t, transactions[i].ID)
		require.True(t, present)
		assert.InDelta(t, float64(100+i), actual, 0.0001,
			"concurrent embedding saves must not cross-assign vectors")
	}
}

func TestTransactionRepository_ConcurrentDeleteAndEmbeddingSaveCannotAttachToAnotherRow(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	repo := newTestRepository(t)
	userID := seedUser(t)
	categoryID := seedCategory(t, userID, "Delete Race", "expense")
	deleted := seedTransaction(t, repo, userID, categoryID, "Delete Me", 100,
		time.Date(2026, 9, 20, 15, 0, 0, 0, time.UTC))
	survivor := seedTransaction(t, repo, userID, categoryID, "Keep Me", 200,
		time.Date(2026, 9, 20, 15, 1, 0, 0, time.UTC))

	start := make(chan struct{})
	var wg sync.WaitGroup
	var deleteErr, embeddingErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		deleteErr = repo.DeleteTransaction(context.Background(), userID, deleted.ID)
	}()
	go func() {
		defer wg.Done()
		<-start
		embeddingErr = repo.SaveEmbedding(context.Background(), deleted.ID, userID, embeddingWithMarker(777), deleted.UpdatedAt)
	}()
	close(start)
	wg.Wait()

	assert.True(t, deleteErr == nil || errors.Is(deleteErr, apperrors.ErrNotFound))
	assert.True(t, embeddingErr == nil || errors.Is(embeddingErr, apperrors.ErrNotFound))

	got, err := repo.GetTransactionByID(context.Background(), userID, deleted.ID)
	assert.Nil(t, got)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
	marker, present := readEmbeddingMarker(t, survivor.ID)
	assert.False(t, present, "a delete/save race must never attach the vector to another transaction")
	assert.Zero(t, marker)
}

func TestTransactionRepository_FailedCreateLeavesNoOrphanEmbedding(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	repo := newTestRepository(t)
	userID := seedUser(t)

	created, err := repo.Create(context.Background(), domain.CreateTransactionInput{
		UserID:          userID,
		CategoryID:      uuid.New(),
		MerchantName:    "Should Not Exist",
		Amount:          10,
		TransactionDate: time.Date(2026, 9, 20, 16, 0, 0, 0, time.UTC),
	})
	assert.Nil(t, created)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
	assert.Equal(t, 0, countTransactionsForUser(t, userID))
	assert.Equal(t, 0, countEmbeddingsForUser(t, userID))
}

func TestTransactionRepository_CreateCategoryRaceCannotInsertWithDeletedCategory(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	repo := newTestRepository(t)
	userID := seedUser(t)
	categoryID := seedCategory(t, userID, "Race Category", "expense")

	ctx := context.Background()
	lockTx, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer lockTx.Rollback(ctx)
	_, err = lockTx.Exec(ctx, `SELECT id FROM public.categories WHERE id = $1 FOR UPDATE`, categoryID)
	require.NoError(t, err)

	result := make(chan error, 1)
	go func() {
		_, createErr := repo.Create(context.Background(), domain.CreateTransactionInput{
			UserID:          userID,
			CategoryID:      categoryID,
			MerchantName:    "Race Create",
			Amount:          25,
			TransactionDate: time.Date(2026, 9, 20, 17, 0, 0, 0, time.UTC),
		})
		result <- createErr
	}()

	// The Create query's FOR SHARE category validation cannot pass while this
	// transaction owns the row lock. Delete and commit the category first; the
	// waiting create must then observe no valid owned category and insert nothing.
	_, err = lockTx.Exec(ctx, `DELETE FROM public.categories WHERE id = $1`, categoryID)
	require.NoError(t, err)
	require.NoError(t, lockTx.Commit(ctx))

	select {
	case createErr := <-result:
		assert.ErrorIs(t, createErr, apperrors.ErrNotFound)
	case <-time.After(5 * time.Second):
		t.Fatal("create did not finish after category lock was released")
	}
	assert.Equal(t, 0, countTransactionsForUser(t, userID),
		"category validation and insertion must be atomic with respect to deletion")
}

func TestTransactionRepository_CanceledQueriesReturnContextErrorWithoutCommit(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	repo := newTestRepository(t)
	userID := seedUser(t)
	categoryID := seedCategory(t, userID, "Canceled", "expense")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	created, err := repo.Create(ctx, domain.CreateTransactionInput{
		UserID:          userID,
		CategoryID:      categoryID,
		MerchantName:    "Canceled Create",
		Amount:          10,
		TransactionDate: time.Date(2026, 9, 20, 18, 0, 0, 0, time.UTC),
	})
	assert.Nil(t, created)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 0, countTransactionsForUser(t, userID), "canceled create must not commit")

	timeoutCtx, timeoutCancel := context.WithTimeout(context.Background(), 0)
	defer timeoutCancel()
	list, err := repo.ListTransactions(timeoutCtx, domain.ListTransactionsInput{UserID: userID, Limit: 10})
	assert.Nil(t, list)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestTransactionRepository_QueryFailureDoesNotLeakRowsOrConnections(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	repo := newTestRepository(t)
	userID := seedUser(t)
	categoryID := seedCategory(t, userID, "Resource Cleanup", "expense")
	seedTransaction(t, repo, userID, categoryID, "Valid", 10,
		time.Date(2026, 9, 20, 19, 0, 0, 0, time.UTC))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	failed, err := repo.ListTransactions(ctx, domain.ListTransactionsInput{UserID: userID, Limit: 10})
	assert.Nil(t, failed)
	assert.ErrorIs(t, err, context.Canceled)

	// A subsequent query must still work, proving the canceled query did not
	// retain a connection or leave rows/resources open in the pool.
	working, err := repo.ListTransactions(context.Background(), domain.ListTransactionsInput{UserID: userID, Limit: 10})
	require.NoError(t, err)
	require.Len(t, working, 1)
	assert.Equal(t, "Valid", working[0].MerchantName)
}
