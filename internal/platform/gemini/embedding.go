// Package gemini contains the isolated Gemini SDK integration used by the
// application's embedding provider.
package gemini

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"walletx-be/internal/modules/ai"

	"google.golang.org/genai"
)

const (
	defaultEmbeddingModel = "gemini-embedding-2"
	embeddingDimensions   = 768
)

var (
	// ErrInvalidEmbeddingDimensions indicates that Gemini returned a vector
	// incompatible with the pgvector column contract.
	ErrInvalidEmbeddingDimensions = errors.New("Gemini returned invalid embedding dimensions")
	// ErrEmptyEmbeddingResponse indicates that Gemini returned no embedding.
	ErrEmptyEmbeddingResponse = errors.New("Gemini returned an empty embedding response")
)

// EmbeddingClient is a concrete Gemini embedding provider.
//
// The client owns the underlying genai.Client and must be closed when the
// application shuts down.
type EmbeddingClient struct {
	client     *genai.Client
	modelName  string
	dimensions int
}

// ChatProvider shares the SDK client and embedding model used to index
// transactions, keeping personalized retrieval in the same vector space.
func (c *EmbeddingClient) ChatProvider(chatModel string) ai.AIChatProvider {
	return NewProvider(c.client, chatModel, c.modelName)
}

// NewEmbeddingClient creates a Gemini embedding client using the configured
// API key. The key is passed directly to the SDK and is never stored in logs.
func NewEmbeddingClient(apiKey string) (*EmbeddingClient, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("Gemini API key must not be empty")
	}

	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{APIKey: apiKey})
	if err != nil {
		return nil, fmt.Errorf("initialize Gemini client: %w", err)
	}

	return &EmbeddingClient{
		client:     client,
		modelName:  defaultEmbeddingModel,
		dimensions: embeddingDimensions,
	}, nil
}

// Generate creates a 768-dimensional embedding for text.
func (c *EmbeddingClient) Generate(ctx context.Context, text string) ([]float32, error) {
	if c == nil || c.client == nil {
		return nil, errors.New("Gemini embedding client is not initialized")
	}
	if ctx == nil {
		return nil, errors.New("Gemini embedding context must not be nil")
	}

	contents := genai.Text(text)
	dimensions := int32(c.dimensions)
	config := &genai.EmbedContentConfig{
		OutputDimensionality: &dimensions,
	}
	response, err := c.client.Models.EmbedContent(ctx, c.modelName, contents, config)
	if err != nil {
		return nil, fmt.Errorf("generate Gemini embedding with model %q: %w", c.modelName, err)
	}
	if response == nil || len(response.Embeddings) == 0 || response.Embeddings[0] == nil || len(response.Embeddings[0].Values) == 0 {
		return nil, ErrEmptyEmbeddingResponse
	}

	values := response.Embeddings[0].Values
	if len(values) != c.dimensions {
		return nil, fmt.Errorf(
			"%w: got %d, want %d",
			ErrInvalidEmbeddingDimensions,
			len(values),
			c.dimensions,
		)
	}

	return values, nil
}

// Close is retained for lifecycle compatibility. The new GenAI SDK does not
// expose a persistent connection that requires explicit shutdown.
func (c *EmbeddingClient) Close() error {
	return nil
}
