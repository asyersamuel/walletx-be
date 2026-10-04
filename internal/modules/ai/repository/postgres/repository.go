package postgres

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"walletx-be/internal/modules/ai"
	"walletx-be/internal/platform/database"
	db "walletx-be/internal/platform/database/sqlc"
	apperrors "walletx-be/internal/shared/errors"
)

type Queries interface {
	MatchTransactions(context.Context, db.MatchTransactionsParams) ([]db.MatchTransactionsRow, error)
	ListCategoriesByUser(context.Context, db.ListCategoriesByUserParams) ([]db.Category, error)
}

// Repository implements AI-owned read projections without exposing SQL types.
type Repository struct{ queries Queries }

func NewRepository(queries Queries) *Repository { return &Repository{queries: queries} }

var _ ai.TransactionReader = (*Repository)(nil)
var _ ai.CategoryReader = (*Repository)(nil)

func (r *Repository) MatchTransactions(ctx context.Context, userID uuid.UUID, embedding []float32, threshold float64, count int) ([]ai.RetrievedTransaction, error) {
	if userID == uuid.Nil || len(embedding) != 768 || math.IsNaN(threshold) || math.IsInf(threshold, 0) || threshold < 0 || threshold > 1 || count <= 0 || int64(count) > math.MaxInt32 {
		return nil, fmt.Errorf("invalid retrieval parameters: %w", apperrors.ErrInvalidInput)
	}
	var vector strings.Builder
	vector.WriteByte('[')
	nonzero := false
	for i, value := range embedding {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return nil, fmt.Errorf("invalid embedding: %w", apperrors.ErrInvalidInput)
		}
		nonzero = nonzero || value != 0
		if i > 0 {
			vector.WriteByte(',')
		}
		vector.WriteString(strconv.FormatFloat(float64(value), 'g', -1, 32))
	}
	if !nonzero {
		return nil, fmt.Errorf("zero embedding: %w", apperrors.ErrInvalidInput)
	}
	vector.WriteByte(']')
	rows, err := r.queries.MatchTransactions(ctx, db.MatchTransactionsParams{QueryEmbedding: vector.String(), MatchThreshold: threshold, MatchCount: int32(count), UserID: database.UUIDParam(userID)})
	if err != nil {
		return nil, fmt.Errorf("match AI transactions: %w", err)
	}
	result := make([]ai.RetrievedTransaction, 0, len(rows))
	for _, row := range rows {
		var notes *string
		if !row.NotesIsNull {
			value := row.Notes
			notes = &value
		}
		result = append(result, ai.RetrievedTransaction{ID: database.UUIDValue(row.ID), Amount: row.Amount, Merchant: row.MerchantName, CategoryName: row.CategoryName, Notes: notes, Date: database.TimeValue(row.TransactionDate)})
	}
	return result, nil
}

func (r *Repository) ListByUser(ctx context.Context, userID uuid.UUID) ([]ai.CategoryOption, error) {
	if userID == uuid.Nil {
		return nil, fmt.Errorf("missing category owner: %w", apperrors.ErrInvalidInput)
	}
	rows, err := r.queries.ListCategoriesByUser(ctx, db.ListCategoriesByUserParams{UserID: database.UUIDParam(userID)})
	if err != nil {
		return nil, fmt.Errorf("list AI categories: %w", err)
	}
	result := make([]ai.CategoryOption, 0, len(rows))
	for _, row := range rows {
		result = append(result, ai.CategoryOption{ID: database.UUIDValue(row.ID), Name: row.Name, Type: row.Type})
	}
	return result, nil
}
