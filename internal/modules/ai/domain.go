// Package ai owns the contracts for AI advice and transaction extraction.
// Infrastructure adapters depend on this package; SDK, HTTP, and database
// types must not cross this boundary.
package ai

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type ChatRole string

const (
	ChatRoleUser      ChatRole = "user"
	ChatRoleAssistant ChatRole = "assistant"
)

// System instructions are server-controlled and excluded from client history.
type ChatMessage struct {
	Role  ChatRole   `json:"role"`
	Parts []ChatPart `json:"parts"`
}

type ChatPart struct {
	Text    string `json:"text"`
	Thought bool   `json:"thought,omitempty"`

	// Opaque continuation metadata belongs to its original part. Never render
	// or log it, and do not merge signed parts during history replay.
	ThoughtSignature []byte `json:"thoughtSignature,omitempty"`
}

type ChatRequest struct {
	// Supplied by the authenticated boundary, never by a request body.
	UserID uuid.UUID `json:"-"`

	History      []ChatMessage `json:"history"`
	Prompt       string        `json:"prompt"`
	IsThinking   bool          `json:"isThinking"`
	Personalized bool          `json:"personalized"`
}

// Retrieval settings are trusted server configuration.
type RetrievalConfig struct {
	MatchThreshold  float64
	MatchCount      int
	MaxContextBytes int
}

type ProviderChatRequest struct {
	History    []ChatMessage
	Prompt     string
	RAGContext string
	IsThinking bool
}

type ChatEventKind string

const (
	ChatEventThought  ChatEventKind = "thought"
	ChatEventAnswer   ChatEventKind = "answer"
	ChatEventComplete ChatEventKind = "complete"
	ChatEventError    ChatEventKind = "error"
)

type ChatEvent struct {
	Kind ChatEventKind

	// Incremental raw Markdown for thought and answer events.
	Text string

	// Required on complete events; authoritative assistant turn for replay.
	Message *ChatMessage

	// Required on error events. The HTTP adapter must sanitize it rather than
	// serializing it directly to clients.
	Err error
}

// ChatStream preserves content and terminal-event ordering. The producer owns
// closure and sends exactly one terminal complete or error event, except that
// cancellation may close the stream without a terminal event. Setup failures
// use the method's error return. Closure without completion is interrupted
// generation. All sends observe cancellation; consumers cancel their context
// when they stop reading, without needing to drain the channel.
type ChatStream <-chan ChatEvent

type ExtractionInput struct {
	UserID uuid.UUID

	// Exactly one of Text or Image must be populated.
	Text     string
	Image    []byte
	MIMEType string

	// Trusted context for relative dates, e.g. TimeZone "Asia/Jakarta".
	ReferenceTime time.Time
	TimeZone      string
}

type CategoryOption struct {
	ID   uuid.UUID
	Name string
	Type string // "expense" or "income"
}

// Only the service supplies Categories after fetching the user's allowed set.
type ProviderExtractionRequest struct {
	Text          string
	Image         []byte
	MIMEType      string
	Categories    []CategoryOption
	ReferenceTime time.Time
	TimeZone      string
}

type ExtractedTransaction struct {
	Amount     float64   `json:"amount"`
	Merchant   string    `json:"merchant"`
	CategoryID uuid.UUID `json:"category_id"`
	Notes      *string   `json:"notes"`
	// Calendar date, YYYY-MM-DD; receipt dates do not imply a timestamp.
	Date string `json:"date"`
}

type AIChatProvider interface {
	GenerateEmbedding(ctx context.Context, text string) ([]float32, error)
	StreamChat(ctx context.Context, request ProviderChatRequest) (ChatStream, error)
	ExtractTransaction(ctx context.Context, request ProviderExtractionRequest) (ExtractedTransaction, error)
}

type AIService interface {
	Chat(ctx context.Context, request ChatRequest) (ChatStream, error)
	ExtractTransaction(ctx context.Context, input ExtractionInput) (ExtractedTransaction, error)
}

// RetrievedTransaction is the AI-owned search projection. Outer adapters map
// transaction entities to it without exposing embeddings or infrastructure.
type RetrievedTransaction struct {
	ID           uuid.UUID
	Amount       float64
	Merchant     string
	CategoryName string
	Notes        *string
	Date         time.Time
}

type TransactionReader interface {
	// The adapter must pass userID into the RPC's ownership filter.
	MatchTransactions(ctx context.Context, userID uuid.UUID, embedding []float32, matchThreshold float64, matchCount int) ([]RetrievedTransaction, error)
}

type CategoryReader interface {
	ListByUser(ctx context.Context, userID uuid.UUID) ([]CategoryOption, error)
}
