package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	apperrors "walletx-be/internal/shared/errors"

	"github.com/google/uuid"
)

const (
	queryEmbeddingDimensions = 768
	defaultMatchThreshold    = 0.7
	defaultMatchCount        = 5
	defaultMaxContextBytes   = 4096
)

type aiService struct {
	provider     AIChatProvider
	transactions TransactionReader
	categories   CategoryReader
	retrieval    RetrievalConfig
}

var _ AIService = (*aiService)(nil)

// NewAIService injects the provider and repository ports. Omitting config or
// supplying its zero value uses default retrieval settings. A supplied config
// preserves an explicit zero similarity threshold; zero count and context
// budget use their defaults. Errors are returned to the handling boundary,
// which owns logging and transport-specific sanitization.
func NewAIService(provider AIChatProvider, transactions TransactionReader, categories CategoryReader, config ...RetrievalConfig) AIService {
	retrieval := RetrievalConfig{
		MatchThreshold:  defaultMatchThreshold,
		MatchCount:      defaultMatchCount,
		MaxContextBytes: defaultMaxContextBytes,
	}
	if len(config) > 0 && config[0] != (RetrievalConfig{}) {
		retrieval = config[0]
		if retrieval.MatchCount == 0 {
			retrieval.MatchCount = defaultMatchCount
		}
		if retrieval.MaxContextBytes == 0 {
			retrieval.MaxContextBytes = defaultMaxContextBytes
		}
	}
	return &aiService{
		provider: provider, transactions: transactions, categories: categories,
		retrieval: retrieval,
	}
}

func (s *aiService) Chat(ctx context.Context, request ChatRequest) (ChatStream, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	if request.UserID == uuid.Nil {
		return nil, apperrors.ErrUnauthorized
	}
	if strings.TrimSpace(request.Prompt) == "" {
		return nil, fmt.Errorf("chat prompt is required: %w", apperrors.ErrInvalidInput)
	}
	for _, message := range request.History {
		if !validHistoryMessage(message) {
			return nil, fmt.Errorf("invalid chat history: %w", apperrors.ErrInvalidInput)
		}
	}
	if s.provider == nil {
		return nil, fmt.Errorf("AI provider is unavailable: %w", apperrors.ErrInternal)
	}

	providerRequest := ProviderChatRequest{
		History: cloneHistory(request.History), Prompt: request.Prompt,
		IsThinking: request.IsThinking,
	}
	if request.Personalized {
		if s.transactions == nil || !validRetrievalConfig(s.retrieval) {
			return nil, fmt.Errorf("AI retrieval is unavailable or misconfigured: %w", apperrors.ErrInternal)
		}
		embedding, err := s.provider.GenerateEmbedding(ctx, request.Prompt)
		if err != nil {
			return nil, fmt.Errorf("generate chat embedding: %w", err)
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !validQueryEmbedding(embedding) {
			return nil, fmt.Errorf("invalid query embedding: %w", apperrors.ErrInternal)
		}
		matches, err := s.transactions.MatchTransactions(ctx, request.UserID, embedding, s.retrieval.MatchThreshold, s.retrieval.MatchCount)
		if err != nil {
			return nil, fmt.Errorf("retrieve chat transactions: %w", err)
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		providerRequest.RAGContext, err = formatRAGContext(matches, s.retrieval.MaxContextBytes)
		if err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// A child context lets terminal events stop an upstream producer even when
	// the caller's request context remains active.
	streamContext, cancel := context.WithCancel(ctx)
	upstream, err := s.provider.StreamChat(streamContext, providerRequest)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("open AI chat stream: %w", err)
	}
	if err := streamContext.Err(); err != nil {
		cancel()
		return nil, err
	}
	if upstream == nil {
		cancel()
		return nil, fmt.Errorf("AI provider returned a nil stream: %w", apperrors.ErrInternal)
	}
	output := make(chan ChatEvent)
	go relayChatStream(streamContext, cancel, upstream, output, request.IsThinking)
	return output, nil
}

func (s *aiService) ExtractTransaction(ctx context.Context, input ExtractionInput) (ExtractedTransaction, error) {
	if err := checkContext(ctx); err != nil {
		return ExtractedTransaction{}, err
	}
	if input.UserID == uuid.Nil {
		return ExtractedTransaction{}, apperrors.ErrUnauthorized
	}
	if err := validateExtractionInput(input); err != nil {
		return ExtractedTransaction{}, err
	}
	if s.provider == nil || s.categories == nil {
		return ExtractedTransaction{}, fmt.Errorf("AI extraction dependencies are unavailable: %w", apperrors.ErrInternal)
	}
	categories, err := s.categories.ListByUser(ctx, input.UserID)
	if err != nil {
		return ExtractedTransaction{}, fmt.Errorf("load extraction categories: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return ExtractedTransaction{}, err
	}
	if len(categories) == 0 {
		return ExtractedTransaction{}, fmt.Errorf("transaction categories are required: %w", apperrors.ErrInvalidInput)
	}

	// Snapshot allowed IDs before calling the provider. The provider's category
	// slice must not be able to expand the allowed set during extraction.
	allowed := make(map[uuid.UUID]struct{}, len(categories))
	for _, category := range categories {
		allowed[category.ID] = struct{}{}
	}
	providerRequest := ProviderExtractionRequest{
		Text: input.Text, Image: input.Image,
		Categories:    append([]CategoryOption(nil), categories...),
		ReferenceTime: input.ReferenceTime, TimeZone: input.TimeZone,
	}
	if len(input.Image) > 0 {
		providerRequest.MIMEType = input.MIMEType
	}
	extracted, err := s.provider.ExtractTransaction(ctx, providerRequest)
	if err != nil {
		return ExtractedTransaction{}, fmt.Errorf("extract transaction: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return ExtractedTransaction{}, err
	}
	if !validExtractedTransaction(extracted, allowed) {
		return ExtractedTransaction{}, fmt.Errorf("invalid extracted transaction: %w", apperrors.ErrInternal)
	}
	return extracted, nil
}

func checkContext(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("request context is required: %w", apperrors.ErrInvalidInput)
	}
	return ctx.Err()
}

func validHistoryMessage(message ChatMessage) bool {
	if message.Role != ChatRoleUser && message.Role != ChatRoleAssistant {
		return false
	}
	if len(message.Parts) == 0 {
		return false
	}
	for _, part := range message.Parts {
		if message.Role == ChatRoleUser && (part.Thought || len(part.ThoughtSignature) > 0) {
			return false
		}
		// A signature-only model part is valid continuation metadata.
		if strings.TrimSpace(part.Text) == "" && len(part.ThoughtSignature) == 0 {
			return false
		}
	}
	return true
}

func cloneHistory(history []ChatMessage) []ChatMessage {
	if history == nil {
		return nil
	}
	result := make([]ChatMessage, len(history))
	for i, message := range history {
		result[i] = ChatMessage{Role: message.Role, Parts: make([]ChatPart, len(message.Parts))}
		for j, part := range message.Parts {
			result[i].Parts[j] = part
			if part.ThoughtSignature != nil {
				result[i].Parts[j].ThoughtSignature = append([]byte{}, part.ThoughtSignature...)
			}
		}
	}
	return result
}

func validRetrievalConfig(config RetrievalConfig) bool {
	return !math.IsNaN(config.MatchThreshold) && !math.IsInf(config.MatchThreshold, 0) &&
		config.MatchThreshold >= 0 && config.MatchThreshold <= 1 &&
		config.MatchCount > 0 && config.MaxContextBytes > 0
}

func validQueryEmbedding(embedding []float32) bool {
	if len(embedding) != queryEmbeddingDimensions {
		return false
	}
	var normSquared float64
	for _, component := range embedding {
		value := float64(component)
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
		normSquared += value * value
	}
	return normSquared > 0
}

// JSON lines keep transaction text quoted as data and retain repository rank
// order with a deterministic field order. Only complete records are included;
// oversized records are omitted rather than truncating JSON or UTF-8 text.
func formatRAGContext(matches []RetrievedTransaction, maxBytes int) (string, error) {
	var result strings.Builder
	for _, transaction := range matches {
		record := struct {
			ID       uuid.UUID `json:"id"`
			Amount   float64   `json:"amount"`
			Merchant string    `json:"merchant"`
			Category string    `json:"category"`
			Notes    *string   `json:"notes"`
			Date     string    `json:"date"`
		}{
			ID: transaction.ID, Amount: transaction.Amount, Merchant: transaction.Merchant,
			Category: transaction.CategoryName, Notes: transaction.Notes,
			Date: transaction.Date.UTC().Format(time.RFC3339Nano),
		}
		encoded, err := json.Marshal(record)
		if err != nil {
			return "", fmt.Errorf("encode retrieved transaction: %w: %w", apperrors.ErrInternal, err)
		}
		separator := 0
		if result.Len() > 0 {
			separator = 1
		}
		if len(encoded) > maxBytes-result.Len()-separator {
			continue
		}
		if separator > 0 {
			result.WriteByte('\n')
		}
		result.Write(encoded)
	}
	return result.String(), nil
}

func relayChatStream(ctx context.Context, cancel context.CancelFunc, upstream ChatStream, output chan<- ChatEvent, thinking bool) {
	defer close(output)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return
		case event, open := <-upstream:
			if ctx.Err() != nil {
				return
			}
			if !open {
				sendChatEvent(ctx, output, chatProtocolError("AI stream closed without a terminal event"))
				return
			}
			if !validChatEvent(event) {
				sendChatEvent(ctx, output, chatProtocolError("AI stream returned an invalid event"))
				return
			}
			if event.Kind == ChatEventThought && !thinking {
				continue
			}
			if !sendChatEvent(ctx, output, event) {
				return
			}
			if event.Kind == ChatEventComplete || event.Kind == ChatEventError {
				return
			}
		}
	}
}

func sendChatEvent(ctx context.Context, output chan<- ChatEvent, event ChatEvent) bool {
	if ctx.Err() != nil {
		return false
	}
	select {
	case <-ctx.Done():
		return false
	case output <- event:
		return true
	}
}

func chatProtocolError(message string) ChatEvent {
	return ChatEvent{Kind: ChatEventError, Err: fmt.Errorf("%s: %w", message, apperrors.ErrInternal)}
}

func validChatEvent(event ChatEvent) bool {
	switch event.Kind {
	case ChatEventThought, ChatEventAnswer:
		return event.Err == nil && event.Message == nil
	case ChatEventError:
		return event.Err != nil && event.Message == nil && event.Text == ""
	case ChatEventComplete:
		if event.Err != nil || event.Text != "" || event.Message == nil || event.Message.Role != ChatRoleAssistant || len(event.Message.Parts) == 0 {
			return false
		}
		for _, part := range event.Message.Parts {
			if !part.Thought && strings.TrimSpace(part.Text) != "" {
				return true
			}
		}
	}
	return false
}

func validateExtractionInput(input ExtractionInput) error {
	hasText := strings.TrimSpace(input.Text) != ""
	hasImage := len(input.Image) > 0
	if hasText == hasImage {
		return fmt.Errorf("exactly one extraction input is required: %w", apperrors.ErrInvalidInput)
	}
	if hasImage {
		switch input.MIMEType {
		case "image/jpeg", "image/png", "image/webp", "image/gif", "image/heic", "image/heif":
		default:
			return fmt.Errorf("a supported image MIME type is required: %w", apperrors.ErrInvalidInput)
		}
	}
	if input.ReferenceTime.IsZero() || input.TimeZone == "" || input.TimeZone == "Local" {
		return fmt.Errorf("extraction date context is required: %w", apperrors.ErrInvalidInput)
	}
	if _, err := time.LoadLocation(input.TimeZone); err != nil {
		return fmt.Errorf("invalid extraction timezone: %w", apperrors.ErrInvalidInput)
	}
	return nil
}

func validExtractedTransaction(transaction ExtractedTransaction, allowed map[uuid.UUID]struct{}) bool {
	if math.IsNaN(transaction.Amount) || math.IsInf(transaction.Amount, 0) || transaction.Amount <= 0 ||
		strings.TrimSpace(transaction.Merchant) == "" || transaction.CategoryID == uuid.Nil {
		return false
	}
	if _, exists := allowed[transaction.CategoryID]; !exists {
		return false
	}
	date, err := time.Parse(time.DateOnly, transaction.Date)
	return err == nil && date.Format(time.DateOnly) == transaction.Date
}
