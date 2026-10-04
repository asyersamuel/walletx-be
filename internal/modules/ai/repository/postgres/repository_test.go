package postgres

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
	"walletx-be/internal/platform/database"
	db "walletx-be/internal/platform/database/sqlc"
	apperrors "walletx-be/internal/shared/errors"
)

type fakeQueries struct {
	match      func(context.Context, db.MatchTransactionsParams) ([]db.MatchTransactionsRow, error)
	categories func(context.Context, db.ListCategoriesByUserParams) ([]db.Category, error)
}

func (q fakeQueries) MatchTransactions(ctx context.Context, p db.MatchTransactionsParams) ([]db.MatchTransactionsRow, error) {
	return q.match(ctx, p)
}
func (q fakeQueries) ListCategoriesByUser(ctx context.Context, p db.ListCategoriesByUserParams) ([]db.Category, error) {
	return q.categories(ctx, p)
}
func vector() []float32 { v := make([]float32, 768); v[0] = 0.5; v[767] = -0.25; return v }

func TestMatchTransactions(t *testing.T) {
	userID, id := uuid.New(), uuid.New()
	date := time.Date(2026, 10, 3, 1, 2, 3, 0, time.UTC)
	ctx := context.WithValue(context.Background(), struct{}{}, "test")
	q := fakeQueries{match: func(actual context.Context, p db.MatchTransactionsParams) ([]db.MatchTransactionsRow, error) {
		require.Equal(t, ctx, actual)
		require.Equal(t, database.UUIDParam(userID), p.UserID)
		require.Equal(t, 0.7, p.MatchThreshold)
		require.Equal(t, int32(5), p.MatchCount)
		require.True(t, strings.HasPrefix(p.QueryEmbedding, "[0.5,0,"))
		require.True(t, strings.HasSuffix(p.QueryEmbedding, ",-0.25]"))
		require.Len(t, strings.Split(p.QueryEmbedding, ","), 768)
		return []db.MatchTransactionsRow{
			{ID: database.UUIDParam(id), MerchantName: "Coffee", CategoryName: "Food", Amount: 12.5, TransactionDate: database.TimeParam(date), Notes: "receipt"},
			{ID: database.UUIDParam(id), TransactionDate: database.TimeParam(date), NotesIsNull: true},
			{ID: database.UUIDParam(id), TransactionDate: database.TimeParam(date), Notes: "", NotesIsNull: false},
		}, nil
	}}
	result, err := NewRepository(q).MatchTransactions(ctx, userID, vector(), 0.7, 5)
	require.NoError(t, err)
	require.Len(t, result, 3)
	require.Equal(t, id, result[0].ID)
	require.Equal(t, "Coffee", result[0].Merchant)
	require.Equal(t, "Food", result[0].CategoryName)
	require.Equal(t, 12.5, result[0].Amount)
	require.Equal(t, date, result[0].Date)
	require.Equal(t, "receipt", *result[0].Notes)
	require.Nil(t, result[1].Notes)
	require.NotNil(t, result[2].Notes)
	require.Empty(t, *result[2].Notes)
}

func TestMatchTransactionsInvalidParameters(t *testing.T) {
	for _, name := range []string{"owner", "dimensions", "zero vector", "nan vector", "infinite vector", "negative threshold", "high threshold", "nan threshold", "infinite threshold", "count zero", "negative count", "overflow count"} {
		t.Run(name, func(t *testing.T) {
			id := uuid.New()
			v := vector()
			threshold := 0.7
			count := 5
			switch name {
			case "owner":
				id = uuid.Nil
			case "dimensions":
				v = v[:767]
			case "zero vector":
				v = make([]float32, 768)
			case "nan vector":
				v[0] = float32(math.NaN())
			case "infinite vector":
				v[0] = float32(math.Inf(1))
			case "negative threshold":
				threshold = -1
			case "high threshold":
				threshold = 1.1
			case "nan threshold":
				threshold = math.NaN()
			case "infinite threshold":
				threshold = math.Inf(1)
			case "count zero":
				count = 0
			case "negative count":
				count = -1
			case "overflow count":
				count = math.MaxInt32 + 1
			}
			_, err := NewRepository(fakeQueries{}).MatchTransactions(context.Background(), id, v, threshold, count)
			require.ErrorIs(t, err, apperrors.ErrInvalidInput)
		})
	}
}

func TestRepositoryEmptyResultsAndFailures(t *testing.T) {
	for _, cause := range []error{nil, errors.New("database unavailable"), context.Canceled} {
		q := fakeQueries{
			match: func(context.Context, db.MatchTransactionsParams) ([]db.MatchTransactionsRow, error) {
				return nil, cause
			},
			categories: func(context.Context, db.ListCategoriesByUserParams) ([]db.Category, error) { return nil, cause },
		}
		repo := NewRepository(q)
		transactions, err := repo.MatchTransactions(context.Background(), uuid.New(), vector(), 0, 5)
		if cause != nil {
			require.ErrorIs(t, err, cause)
		} else {
			require.NoError(t, err)
			require.NotNil(t, transactions)
			require.Empty(t, transactions)
		}
		categories, err := repo.ListByUser(context.Background(), uuid.New())
		if cause != nil {
			require.ErrorIs(t, err, cause)
		} else {
			require.NoError(t, err)
			require.NotNil(t, categories)
			require.Empty(t, categories)
		}
	}
}

func TestListCategories(t *testing.T) {
	userID, id := uuid.New(), uuid.New()
	q := fakeQueries{categories: func(ctx context.Context, p db.ListCategoriesByUserParams) ([]db.Category, error) {
		require.Equal(t, database.UUIDParam(userID), p.UserID)
		require.Nil(t, p.CategoryType)
		return []db.Category{{ID: database.UUIDParam(id), Name: "Salary", Type: "income"}}, nil
	}}
	rows, err := NewRepository(q).ListByUser(context.Background(), userID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, id, rows[0].ID)
	require.Equal(t, "Salary", rows[0].Name)
	require.Equal(t, "income", rows[0].Type)
	_, err = NewRepository(fakeQueries{}).ListByUser(context.Background(), uuid.Nil)
	require.ErrorIs(t, err, apperrors.ErrInvalidInput)
}

// Exercise the generated SQL and scan order with pgx's actual DBTX contract.
type rpcDB struct {
	db.DBTX
	t     *testing.T
	owner uuid.UUID
	rows  *rpcRows
}

func (d rpcDB) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	require.Contains(d.t, sql, "FROM public.match_walletx_transactions(")
	require.Contains(d.t, sql, "$1::text::extensions.vector")
	require.Contains(d.t, sql, "$4::uuid")
	require.Len(d.t, args, 4)
	require.IsType(d.t, "", args[0])
	require.Equal(d.t, 0.7, args[1])
	require.Equal(d.t, int32(5), args[2])
	require.Equal(d.t, database.UUIDParam(d.owner), args[3])
	return d.rows, nil
}

type rpcRows struct {
	pgx.Rows
	row     db.MatchTransactionsRow
	next    bool
	closed  bool
	err     error
	scanErr error
}

func (r *rpcRows) Next() bool {
	if r.next {
		return false
	}
	r.next = true
	return true
}
func (r *rpcRows) Close()     { r.closed = true }
func (r *rpcRows) Err() error { return r.err }
func (r *rpcRows) Scan(dest ...interface{}) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	// Assign fields in the order defined by the RPC query projection.
	values := []interface{}{r.row.ID, r.row.MerchantName, r.row.CategoryName, r.row.Amount, r.row.TransactionDate, r.row.Notes, r.row.NotesIsNull}
	for i, v := range values {
		switch target := dest[i].(type) {
		case *string:
			*target = v.(string)
		case *float64:
			*target = v.(float64)
		case *bool:
			*target = v.(bool)
		default:
			if i == 0 {
				*dest[i].(*pgtype.UUID) = r.row.ID
			} else {
				*dest[i].(*pgtype.Timestamptz) = r.row.TransactionDate
			}
		}
	}
	return nil
}

func TestGeneratedRPCQueryAndScan(t *testing.T) {
	for _, name := range []string{"success", "scan failure", "iteration failure"} {
		t.Run(name, func(t *testing.T) {
			owner := uuid.New()
			rows := &rpcRows{row: db.MatchTransactionsRow{ID: database.UUIDParam(uuid.New()), MerchantName: "Merchant", CategoryName: "Food", Amount: 20, TransactionDate: database.TimeParam(time.Now()), NotesIsNull: true}}
			cause := errors.New("read failure")
			if name == "scan failure" {
				rows.scanErr = cause
			}
			if name == "iteration failure" {
				rows.err = cause
			}
			repo := NewRepository(db.New(rpcDB{t: t, owner: owner, rows: rows}))
			result, err := repo.MatchTransactions(context.Background(), owner, vector(), 0.7, 5)
			require.True(t, rows.closed)
			if name != "success" {
				require.ErrorIs(t, err, cause)
			} else {
				require.NoError(t, err)
				require.Len(t, result, 1)
				require.Equal(t, "Merchant", result[0].Merchant)
				require.Nil(t, result[0].Notes)
			}
		})
	}
}
