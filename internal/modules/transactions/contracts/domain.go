package contracts

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// EmbeddingDimension matches the extensions.vector(768) column defined in
// supabase/migrations. It is the contract for the asynchronous embedding job
// implemented in a later iteration.
const EmbeddingDimension = 768

// DefaultListLimit is the page size the repository falls back to when a list
// request does not specify a positive limit. It is exported so the service /
// handler layers can surface the same default consistently.
const DefaultListLimit = 20

const MaxReceiptBytes = 10 << 20

// Transaction represents either a pending or completed transaction. For pending
// rows only, empty MerchantName, zero Amount and zero TransactionDate represent
// absent extraction fields. These sentinels cannot be valid success values and
// the HTTP projection exposes them as null, rather than invented business data.
//
// The transaction "type" (income/expense) is intentionally NOT stored here:
// it is derived from the categories row referenced by CategoryID
// (categories.type is 'expense' or 'income'). The Category domain model is
// already defined in internal/modules/categories and is not duplicated here.
//
// Present amounts are finite and strictly positive. The repository validates
// nullable fields according to Status and preserves the existing scalar contract.
type Transaction struct {
	ID              uuid.UUID         `json:"id"`
	UserID          uuid.UUID         `json:"user_id"`
	Status          TransactionStatus `json:"status"`
	CategoryID      *uuid.UUID        `json:"category_id"`       // nullable: categories(id) ON DELETE SET NULL
	MerchantName    string            `json:"merchant_name"`     // required for a completed transaction
	Amount          float64           `json:"amount"`            // finite, positive for a completed transaction
	TransactionDate time.Time         `json:"transaction_date"`  // required, finite timestamptz
	ReceiptImageUrl *string           `json:"receipt_image_url"` // optional, NULL when absent
	Notes           *string           `json:"notes"`             // optional, NULL when absent
	Embedding       []float32         `json:"embedding"`         // NULL until the async embedding job fills it
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

type TransactionStatus string

const (
	TransactionStatusPending TransactionStatus = "pending"
	TransactionStatusSuccess TransactionStatus = "success"
)

// PendingTransaction contains persisted receipt metadata without inventing
// values for details that have not yet been extracted.
type PendingTransaction struct {
	ID              uuid.UUID
	UserID          uuid.UUID
	ReceiptImageURL string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type CreatePendingTransactionInput struct {
	UserID          uuid.UUID
	ReceiptImageURL string
}

// CompletePendingTransactionInput is a full, ownership-scoped transition to
// success, rather than a partial edit of an incomplete transaction.
type CompletePendingTransactionInput struct {
	UserID          uuid.UUID
	TransactionID   uuid.UUID
	CategoryID      uuid.UUID
	CategoryName    string // service embedding context; not persisted on transactions
	MerchantName    string
	Amount          float64
	TransactionDate time.Time
	Notes           *string
}

// ReceiptResult identifies the persisted receipt. Transaction is populated only
// for success; pending results contain no fabricated transaction details.
type ReceiptResult struct {
	Status          TransactionStatus
	TransactionID   uuid.UUID
	ReceiptImageURL string
	Transaction     *Transaction
}

// ReceiptStorage returns a durable, opaque reference after persisting the image.
// DeleteReceipt is idempotent and cleans unreferenced uploads/deleted transactions.
type ReceiptStorage interface {
	SaveReceipt(ctx context.Context, userID uuid.UUID, contentType, filename string, image []byte) (string, error)
	DeleteReceipt(ctx context.Context, userID uuid.UUID, reference string) error
}

// ReceiptTransactionWriter isolates receipt lifecycle operations from ordinary
// transaction CRUD. Each write commits before returning; extraction runs outside
// database transactions.
type ReceiptTransactionWriter interface {
	CreatePendingTransaction(ctx context.Context, userID uuid.UUID, input CreatePendingTransactionInput) (*PendingTransaction, error)
	CompletePendingTransaction(ctx context.Context, userID, transactionID uuid.UUID, input CompletePendingTransactionInput) (*Transaction, error)
}

// CreateTransactionInput carries everything the repository needs to persist a
// new transaction. UserID and CategoryID come from the verified JWT context
// and the user-owned category relation; they are never client-controlled
// ownership overrides.
type CreateTransactionInput struct {
	UserID          uuid.UUID // authenticated user; stored in transactions.user_id
	CategoryID      uuid.UUID // must exist AND belong to UserID
	CategoryName    string    // resolved user-owned category name for embedding context
	MerchantName    string    // required for successful creation
	Amount          float64   // required, must be > 0
	TransactionDate time.Time // required, stored as timestamptz (timezone instant preserved)
	ReceiptImageUrl *string   // optional; nil persists SQL NULL
	Notes           *string   // optional; nil persists SQL NULL
}

// ListTransactionsInput carries pagination and optional inclusive date-range
// filters for listing a single user's transactions.
//
//   - UserID scopes the query; it comes from the verified JWT context, never
//     from a client-controlled filter.
//   - StartDate/EndDate are inclusive bounds compared against transaction_date
//     by absolute instant (timestamptz). nil means unbounded on that side.
//   - Limit/Offset: the repository defensively normalizes Limit <= 0 to
//     DefaultListLimit and Offset < 0 to 0, so an invalid value can never reach
//     PostgreSQL (LIMIT/OFFSET reject negative numbers). The service/handler
//     layer remains responsible for user-facing validation (e.g. HTTP 400 for a
//     non-numeric, zero, or over-large limit) before the repository is called.
type ListTransactionsInput struct {
	UserID    uuid.UUID
	Limit     int
	Offset    int
	StartDate *time.Time
	EndDate   *time.Time
}

// UpdateTransactionInput carries a partial, ownership-scoped update. Only the
// fields with non-nil values are written; nil fields are left unchanged. The
// repository turns each non-nil field into a set-flag for the SQL CASE guard,
// so this is a true partial update (callers send only what they want to
// change). UserID and TransactionID locate the row and are always required;
// UserID itself is never modified (ownership cannot be reassigned).
//
// CategoryID, when provided, is re-validated to exist AND belong to UserID; a
// missing or foreign category makes the update affect zero rows (ErrNotFound),
// exactly like Create's ownership guard.
type UpdateTransactionInput struct {
	UserID          uuid.UUID  // authenticated user; WHERE guard (ownership), never modified
	TransactionID   uuid.UUID  // target transaction; WHERE guard
	MerchantName    *string    // nil = keep current
	Amount          *float64   // nil = keep current; non-nil must satisfy CHECK (> 0)
	TransactionDate *time.Time // nil = keep current
	CategoryID      *uuid.UUID // nil = keep; non-nil must exist AND belong to UserID
	ReceiptImageUrl *string    // nil = keep current
	Notes           *string    // nil = keep current
	ClearReceiptURL bool       // when true, explicitly set receipt_image_url to SQL NULL
	ClearNotes      bool       // when true, explicitly set notes to SQL NULL
}

// SearchSimilarTransactionsInput describes a user-scoped cosine-similarity
// search. QueryEmbedding must contain exactly EmbeddingDimension components.
type SearchSimilarTransactionsInput struct {
	UserID         uuid.UUID
	QueryEmbedding []float32
	MatchThreshold float64
	StartDate      *time.Time
	EndDate        *time.Time
	Limit          int
}

// SimilarTransaction is the business projection returned by the RAG vector
// search. The raw embedding is intentionally not returned.
type SimilarTransaction struct {
	ID              uuid.UUID  `json:"id"`
	UserID          uuid.UUID  `json:"user_id"`
	CategoryID      *uuid.UUID `json:"category_id"`
	MerchantName    string     `json:"merchant_name"`
	Amount          float64    `json:"amount"`
	TransactionDate time.Time  `json:"transaction_date"`
	Notes           *string    `json:"notes"`
	CategoryName    *string    `json:"category_name"`
	CategoryType    *string    `json:"category_type"`
	Similarity      float64    `json:"similarity"`
}

// TransactionRepository defines the persistence interface for transactions.
//
// Includes completed-transaction CRUD, receipt pending/completion operations,
// embedding persistence, and user-scoped vector search.
type TransactionRepository interface {
	// CreatePendingTransaction persists the receipt with NULL extraction fields.
	CreatePendingTransaction(ctx context.Context, input CreatePendingTransactionInput) (*PendingTransaction, error)

	// CompletePendingTransaction atomically completes a pending row owned by
	// input.UserID. Missing, foreign, or already completed rows yield ErrNotFound.
	CompletePendingTransaction(ctx context.Context, input CompletePendingTransactionInput) (*Transaction, error)

	// Create persists a new transaction owned by input.UserID and returns the
	// fully populated entity, including the database-generated id, created_at,
	// and updated_at. The embedding column is left NULL on purpose; it is
	// filled later by an asynchronous background job.
	Create(ctx context.Context, input CreateTransactionInput) (*Transaction, error)

	// GetTransactionByID returns a successful transaction owned by userID.
	// Pending, foreign and missing transactions yield ErrNotFound.
	// The embedding is not loaded in this projection.
	GetTransactionByID(ctx context.Context, userID, transactionID uuid.UUID) (*Transaction, error)

	// GetTransactionForDeletion includes pending rows for owner-scoped cleanup.
	// Foreign and missing transactions yield ErrNotFound.
	GetTransactionForDeletion(ctx context.Context, userID, transactionID uuid.UUID) (*Transaction, error)

	// ListTransactions returns the caller's successful transactions, newest first
	// (transaction_date desc, id desc), optionally bounded by an inclusive date
	// range and windowed by Limit/Offset. It never returns another user's rows.
	// The result is always a non-nil slice (empty when there are no matches).
	ListTransactions(ctx context.Context, input ListTransactionsInput) ([]Transaction, error)

	// UpdateTransaction applies a partial update to a successful transaction the
	// caller owns: only non-nil fields of input are written, nil fields are kept.
	// A transaction owned by another user (or a missing one) yields
	// apperrors.ErrNotFound and is left untouched. When CategoryID is supplied,
	// the new category must exist AND belong to the caller; otherwise zero rows
	// change (also ErrNotFound), so a caller cannot attach another user's
	// category. An amount that violates CHECK (amount > 0) yields
	// apperrors.ErrInvalidInput. Returns the updated entity.
	UpdateTransaction(ctx context.Context, input UpdateTransactionInput) (*Transaction, error)

	// DeleteTransaction removes a transaction the caller owns. Deleting another
	// user's transaction (or a missing one) affects zero rows and returns
	// apperrors.ErrNotFound. The related category is never touched.
	DeleteTransaction(ctx context.Context, userID, transactionID uuid.UUID) error

	// SaveEmbedding replaces only the embedding for a transaction owned by
	// userID. The vector must have exactly EmbeddingDimension components.
	// updatedAt is part of the optimistic version check so stale asynchronous
	// jobs cannot overwrite an embedding generated for a newer transaction.
	SaveEmbedding(ctx context.Context, transactionID, userID uuid.UUID, embedding []float32, updatedAt time.Time) error

	// SearchSimilarTransactions performs a user-scoped cosine-distance search.
	SearchSimilarTransactions(ctx context.Context, input SearchSimilarTransactionsInput) ([]SimilarTransaction, error)
}

// TransactionService defines transaction business rules above persistence.
type TransactionService interface {
	ReceiptTransactionWriter
	CreateTransaction(ctx context.Context, userID uuid.UUID, input CreateTransactionInput) (*Transaction, error)
	GetTransaction(ctx context.Context, userID, transactionID uuid.UUID) (*Transaction, error)
	ListTransactions(ctx context.Context, userID uuid.UUID, input ListTransactionsInput) ([]Transaction, error)
	UpdateTransaction(ctx context.Context, userID, transactionID uuid.UUID, input UpdateTransactionInput) (*Transaction, error)
	DeleteTransaction(ctx context.Context, userID, transactionID uuid.UUID) error
}

// LLM abstracts Gemini chat/function-calling responses.
type LLM interface {
	Generate(ctx context.Context, prompt string) (LLMResponse, error)
}

type LLMResponse struct {
	Text     string
	ToolCall *LLMToolCall
}

type LLMToolCall struct {
	Name      string
	Arguments json.RawMessage
}

// CategoryResolver resolves a category name only within the authenticated
// user's category namespace.
type CategoryResolver interface {
	ResolveOwnedCategory(ctx context.Context, userID uuid.UUID, name string) (uuid.UUID, string, error)
}

type AgentResponse struct {
	Text         string
	Transaction  *Transaction
	SimilarItems []SimilarTransaction
}

type CircuitBreakerConfig struct {
	FailureThreshold int
	Cooldown         time.Duration
}

type AgentService interface {
	Handle(ctx context.Context, userID uuid.UUID, prompt string) (AgentResponse, error)
}

// ReceiptService abstracts multimodal receipt extraction. The handler owns
// transport validation; this boundary receives only validated image bytes.
type ReceiptService interface {
	ProcessReceipt(ctx context.Context, userID uuid.UUID, contentType, filename string, image []byte) (ReceiptResult, error)
}

// EmbeddingProvider abstracts Gemini (or another provider) text embedding
// generation from transaction business logic.
type EmbeddingProvider interface {
	Generate(ctx context.Context, text string) ([]float32, error)
}

// EmbeddingService is retained as the domain-facing name used by the current
// transaction service. New code should prefer EmbeddingProvider.
type EmbeddingService = EmbeddingProvider
