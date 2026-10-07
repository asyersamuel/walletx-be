package postgres

import (
	"math"
	"math/big"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
	domain "walletx-be/internal/modules/transactions/contracts"
	"walletx-be/internal/platform/database"
	sqlc "walletx-be/internal/platform/database/sqlc"
	apperrors "walletx-be/internal/shared/errors"
)

type mappingFixture struct {
	id, userID, categoryID     pgtype.UUID
	merchant                   *string
	amount                     pgtype.Numeric
	date, createdAt, updatedAt pgtype.Timestamptz
	url, notes                 *string
	status                     string
}

func validMappingFixture() mappingFixture {
	now := database.TimeParam(time.Date(2026, 10, 6, 1, 2, 3, 0, time.FixedZone("Jakarta", 7*60*60)))
	return mappingFixture{
		id: database.UUIDParam(uuid.New()), userID: database.UUIDParam(uuid.New()),
		categoryID: database.UUIDParam(uuid.New()), merchant: strPtr("Coffee"),
		amount: pgtype.Numeric{Int: big.NewInt(12345), Exp: -2, Valid: true},
		date:   now, createdAt: now, updatedAt: now, status: "success",
	}
}

func (f mappingFixture) mapTransaction() (domain.Transaction, error) {
	return transactionFrom(f.id, f.userID, f.categoryID, f.merchant, f.amount,
		f.date, f.url, f.notes, f.createdAt, f.updatedAt, f.status)
}

func TestTransactionMapping_ValidAndOptionalFields(t *testing.T) {
	f := validMappingFixture()
	f.categoryID = pgtype.UUID{}
	f.url, f.notes = strPtr("https://storage.example/receipt"), strPtr("")
	got, err := f.mapTransaction()
	require.NoError(t, err)
	require.Equal(t, uuid.UUID(f.id.Bytes), got.ID)
	require.Equal(t, uuid.UUID(f.userID.Bytes), got.UserID)
	require.Nil(t, got.CategoryID)
	require.Equal(t, 123.45, got.Amount)
	require.True(t, f.date.Time.Equal(got.TransactionDate))
	require.Equal(t, f.url, got.ReceiptImageUrl)
	require.Equal(t, f.notes, got.Notes)
	require.Nil(t, got.Embedding)
	f.categoryID = database.UUIDParam(uuid.New())
	f.url, f.notes = nil, nil
	got, err = f.mapTransaction()
	require.NoError(t, err)
	require.Equal(t, uuid.UUID(f.categoryID.Bytes), *got.CategoryID)
	require.Nil(t, got.ReceiptImageUrl)
	require.Nil(t, got.Notes)
}

func TestTransactionMapping_RejectsMalformedRows(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*mappingFixture)
	}{
		{"missing id", func(f *mappingFixture) { f.id.Valid = false }},
		{"nil id", func(f *mappingFixture) { f.id.Bytes = [16]byte{} }},
		{"missing owner", func(f *mappingFixture) { f.userID.Valid = false }},
		{"nil owner", func(f *mappingFixture) { f.userID.Bytes = [16]byte{} }},
		{"nil present category", func(f *mappingFixture) { f.categoryID.Bytes = [16]byte{} }},
		{"missing merchant", func(f *mappingFixture) { f.merchant = nil }},
		{"blank merchant", func(f *mappingFixture) { f.merchant = strPtr("  ") }},
		{"missing amount", func(f *mappingFixture) { f.amount.Valid = false }},
		{"missing coefficient", func(f *mappingFixture) { f.amount.Int = nil }},
		{"zero amount", func(f *mappingFixture) { f.amount.Int = big.NewInt(0) }},
		{"negative amount", func(f *mappingFixture) { f.amount.Int = big.NewInt(-1) }},
		{"nan amount", func(f *mappingFixture) { f.amount.NaN = true }},
		{"infinite amount", func(f *mappingFixture) { f.amount.InfinityModifier = pgtype.Infinity }},
		{"overflow amount", func(f *mappingFixture) { f.amount.Exp = 10000 }},
		{"underflow amount", func(f *mappingFixture) { f.amount.Exp = -10000 }},
		{"missing date", func(f *mappingFixture) { f.date.Valid = false }},
		{"zero date", func(f *mappingFixture) { f.date.Time = time.Time{} }},
		{"infinite date", func(f *mappingFixture) { f.date.InfinityModifier = pgtype.Infinity }},
		{"missing created at", func(f *mappingFixture) { f.createdAt.Valid = false }},
		{"infinite updated at", func(f *mappingFixture) { f.updatedAt.InfinityModifier = pgtype.NegativeInfinity }},
		{"unknown status", func(f *mappingFixture) { f.status = "failed" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := validMappingFixture()
			tc.mutate(&f)
			got, err := f.mapTransaction()
			require.ErrorIs(t, err, apperrors.ErrInternal)
			require.Equal(t, domain.Transaction{}, got)
		})
	}
}

func TestTransactionMapping_PendingAllowsMissingExtractionFields(t *testing.T) {
	f := validMappingFixture()
	f.status = "pending"
	f.categoryID, f.merchant, f.amount, f.date = pgtype.UUID{}, nil, pgtype.Numeric{}, pgtype.Timestamptz{}
	f.url = strPtr(uuid.UUID(f.userID.Bytes).String() + "/" + uuid.NewString() + "_123.png")
	got, err := f.mapTransaction()
	require.NoError(t, err)
	require.Equal(t, domain.TransactionStatusPending, got.Status)
	require.Nil(t, got.CategoryID)
	require.Empty(t, got.MerchantName)
	require.Zero(t, got.Amount)
	require.True(t, got.TransactionDate.IsZero())
	require.Equal(t, f.url, got.ReceiptImageUrl)
	// Non-NULL fields remain validated even on a pending row.
	f.amount = pgtype.Numeric{Int: big.NewInt(-1), Valid: true}
	_, err = f.mapTransaction()
	require.ErrorIs(t, err, apperrors.ErrInternal)
}

func TestAmountMapping_CoefficientAndExponent(t *testing.T) {
	for _, tc := range []struct {
		coefficient int64
		exponent    int32
		want        float64
	}{
		{12345, -2, 123.45}, {123, 2, 12300}, {1, -6, 0.000001}, {25, 0, 25},
	} {
		got, err := amountValue(pgtype.Numeric{Int: big.NewInt(tc.coefficient), Exp: tc.exponent, Valid: true})
		require.NoError(t, err)
		require.Equal(t, tc.want, got)
	}
}

func TestAmountParam_RoundTripAndInvalidValues(t *testing.T) {
	for _, amount := range []float64{123.45, 25000, 0.000001, 1e20} {
		param, err := amountParam(amount)
		require.NoError(t, err)
		got, err := amountValue(param)
		require.NoError(t, err)
		require.Equal(t, amount, got)
	}
	for _, amount := range []float64{0, -1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		_, err := amountParam(amount)
		require.ErrorIs(t, err, apperrors.ErrInvalidInput)
	}
}

func TestPendingMapping_ValidatesMetadata(t *testing.T) {
	f := validMappingFixture()
	url := strPtr("https://storage.example/receipt")
	got, err := pendingTransactionFrom(f.id, f.userID, url, f.createdAt, f.updatedAt, "pending")
	require.NoError(t, err)
	require.Equal(t, *url, got.ReceiptImageURL)
	require.Equal(t, uuid.UUID(f.id.Bytes), got.ID)
	for _, tc := range []struct {
		name   string
		url    *string
		status string
	}{
		{"missing receipt", nil, "pending"}, {"blank receipt", strPtr(" "), "pending"},
		{"wrong status", url, "success"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := pendingTransactionFrom(f.id, f.userID, tc.url, f.createdAt, f.updatedAt, tc.status)
			require.ErrorIs(t, err, apperrors.ErrInternal)
			require.Equal(t, domain.PendingTransaction{}, got)
		})
	}
}

func TestSimilarMapping_CheckedDetailsAndOptionalCategory(t *testing.T) {
	f := validMappingFixture()
	row := sqlc.SearchSimilarTransactionsRow{
		ID: f.id, UserID: f.userID, CategoryID: pgtype.UUID{}, MerchantName: f.merchant,
		Amount: f.amount, TransactionDate: f.date, Status: f.status, Similarity: 0.75,
		Notes: strPtr(""),
	}
	got, err := similarTransactionFrom(row)
	require.NoError(t, err)
	require.Equal(t, 123.45, got.Amount)
	require.Equal(t, 0.75, got.Similarity)
	require.Nil(t, got.CategoryID)
	require.Nil(t, got.CategoryName)
	require.Nil(t, got.CategoryType)
	require.Equal(t, strPtr(""), got.Notes)
	for _, tc := range []struct {
		name   string
		mutate func(*sqlc.SearchSimilarTransactionsRow)
	}{
		{"missing amount", func(r *sqlc.SearchSimilarTransactionsRow) { r.Amount.Valid = false }},
		{"missing date", func(r *sqlc.SearchSimilarTransactionsRow) { r.TransactionDate.Valid = false }},
		{"pending", func(r *sqlc.SearchSimilarTransactionsRow) { r.Status = "pending" }},
		{"nan similarity", func(r *sqlc.SearchSimilarTransactionsRow) { r.Similarity = math.NaN() }},
		{"infinite similarity", func(r *sqlc.SearchSimilarTransactionsRow) { r.Similarity = math.Inf(1) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := row
			tc.mutate(&bad)
			got, err := similarTransactionFrom(bad)
			require.ErrorIs(t, err, apperrors.ErrInternal)
			require.Equal(t, domain.SimilarTransaction{}, got)
		})
	}
}
