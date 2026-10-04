package jobs

import domain "walletx-be/internal/modules/transactions/contracts"

// EmbeddingProvider abstracts Gemini (or another provider) text embedding
// generation from transaction business logic.
type EmbeddingProvider = domain.EmbeddingProvider

// EmbeddingService retains the existing alias for embedding providers.
type EmbeddingService = domain.EmbeddingService
