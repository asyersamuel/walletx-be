package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
	domain "walletx-be/internal/modules/transactions/contracts"
	"walletx-be/internal/platform/database"
	sqlc "walletx-be/internal/platform/database/sqlc"
	apperrors "walletx-be/internal/shared/errors"
)

type pendingSelectionDB struct {
	sqlc.DBTX
	rows  *pendingSelectionRows
	err   error
	query string
	args  []interface{}
	calls int
}

func (d *pendingSelectionDB) Query(_ context.Context, query string, args ...interface{}) (pgx.Rows, error) {
	d.calls++
	d.query, d.args = query, args
	return d.rows, d.err
}

type pendingSelectionRows struct {
	pgx.Rows
	items   []sqlc.GetPendingTransactionsRow
	index   int
	closed  bool
	err     error
	scanErr error
}

func (r *pendingSelectionRows) Next() bool {
	if r.index >= len(r.items) {
		return false
	}
	r.index++
	return true
}
func (r *pendingSelectionRows) Close()     { r.closed = true }
func (r *pendingSelectionRows) Err() error { return r.err }
func (r *pendingSelectionRows) Scan(dest ...interface{}) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	row := r.items[r.index-1]
	*dest[0].(*pgtype.UUID) = row.ID
	*dest[1].(*pgtype.UUID) = row.UserID
	*dest[2].(**string) = row.ReceiptImageUrl
	*dest[3].(*pgtype.Timestamptz) = row.CreatedAt
	*dest[4].(*pgtype.Timestamptz) = row.UpdatedAt
	*dest[5].(*string) = row.Status
	return nil
}

func pendingSelectionRow() sqlc.GetPendingTransactionsRow {
	return sqlc.GetPendingTransactionsRow{
		ID: database.UUIDParam(uuid.New()), UserID: database.UUIDParam(uuid.New()),
		ReceiptImageUrl: strPtr("owned/image.png"), Status: "pending",
		CreatedAt: database.TimeParam(time.Now().UTC()), UpdatedAt: database.TimeParam(time.Now().UTC()),
	}
}

func TestPendingSelectionQueryAndMapping(t *testing.T) {
	first, second := pendingSelectionRow(), pendingSelectionRow()
	rows := &pendingSelectionRows{items: []sqlc.GetPendingTransactionsRow{first, second}}
	db := &pendingSelectionDB{rows: rows}
	reader := NewPendingReceiptReader(sqlc.New(db))
	got, err := reader.GetPendingTransactions(context.Background(), 10)
	require.NoError(t, err)
	require.True(t, rows.closed)
	require.Contains(t, db.query, "where status = 'pending'")
	require.Contains(t, db.query, "order by created_at asc, id asc")
	require.Contains(t, db.query, "limit $1::int")
	require.Equal(t, []interface{}{int32(10)}, db.args)
	require.Len(t, got, 2)
	for i, row := range []sqlc.GetPendingTransactionsRow{first, second} {
		require.Equal(t, uuid.UUID(row.ID.Bytes), got[i].ID)
		require.Equal(t, uuid.UUID(row.UserID.Bytes), got[i].UserID)
		require.Equal(t, *row.ReceiptImageUrl, got[i].ReceiptImageURL)
		require.True(t, row.CreatedAt.Time.Equal(got[i].CreatedAt))
		require.True(t, row.UpdatedAt.Time.Equal(got[i].UpdatedAt))
	}
}

func TestPendingSelectionEmptyAndFailures(t *testing.T) {
	cause := errors.New("database read failed")
	for _, name := range []string{"empty", "query", "scan", "iteration", "malformed"} {
		t.Run(name, func(t *testing.T) {
			rows := &pendingSelectionRows{}
			db := &pendingSelectionDB{rows: rows}
			want := cause
			switch name {
			case "empty":
				want = nil
			case "query":
				db.err = cause
			case "scan":
				rows.items, rows.scanErr = []sqlc.GetPendingTransactionsRow{pendingSelectionRow()}, cause
			case "iteration":
				rows.items, rows.err = []sqlc.GetPendingTransactionsRow{pendingSelectionRow()}, cause
			case "malformed":
				bad := pendingSelectionRow()
				bad.ReceiptImageUrl = nil
				rows.items = []sqlc.GetPendingTransactionsRow{pendingSelectionRow(), bad}
				want = apperrors.ErrInternal
			}
			got, err := NewPendingReceiptReader(sqlc.New(db)).GetPendingTransactions(context.Background(), 20)
			if want == nil {
				require.NoError(t, err)
				require.NotNil(t, got)
				require.Empty(t, got)
			} else {
				require.ErrorIs(t, err, want)
				require.Nil(t, got, "never return a partially mapped batch")
			}
			if name != "query" {
				require.True(t, rows.closed)
			}
		})
	}
}

func TestPendingSelectionRejectsInvalidLimitsAndContexts(t *testing.T) {
	db := &pendingSelectionDB{}
	reader := NewPendingReceiptReader(sqlc.New(db))
	for _, limit := range []int{-1, 0, domain.MaxPendingReceiptBatchSize + 1, 1 << 32} {
		got, err := reader.GetPendingTransactions(context.Background(), limit)
		require.ErrorIs(t, err, apperrors.ErrInvalidInput)
		require.Nil(t, got)
	}
	_, err := reader.GetPendingTransactions(nil, 10)
	require.ErrorIs(t, err, apperrors.ErrInvalidInput)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = reader.GetPendingTransactions(ctx, 10)
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, db.calls)
}

func TestPendingSelectionIntegration(t *testing.T) {
	skipIfNoDatabase(t)
	truncateTables(t)
	ctx := context.Background()
	repo := newTestRepository(t)
	owner, otherOwner := seedUser(t), seedUser(t)
	category := seedCategory(t, owner, "Food", "expense")
	seedTransaction(t, repo, owner, category, "Completed", 10, time.Now().UTC())
	first, second, third := seedPending(t, repo, owner), seedPending(t, repo, otherOwner), seedPending(t, repo, owner)
	// Tie timestamps on two rows to exercise the UUID tie breaker.
	_, err := testPool.Exec(ctx, "update public.transactions set created_at = '2026-01-01' where id = $1 or id = $2", first.ID, second.ID)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, "update public.transactions set created_at = '2026-01-02' where id = $1", third.ID)
	require.NoError(t, err)
	reader := NewPendingReceiptReader(sqlc.New(testPool))
	got, err := reader.GetPendingTransactions(ctx, 2)
	require.NoError(t, err)
	require.Len(t, got, 2)
	if first.ID.String() > second.ID.String() {
		first, second = second, first
	}
	require.Equal(t, first.ID, got[0].ID)
	require.Equal(t, second.ID, got[1].ID)
	all, err := reader.GetPendingTransactions(ctx, 20)
	require.NoError(t, err)
	require.Len(t, all, 3)
	require.Equal(t, third.ID, all[2].ID)
}
