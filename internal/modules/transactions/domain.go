package transactions

// Domain aliases preserve the root API while leaf packages depend only on
// contracts, avoiding cycles through the module composition root.
import domain "walletx-be/internal/modules/transactions/contracts"

const EmbeddingDimension = domain.EmbeddingDimension
const DefaultListLimit = domain.DefaultListLimit

type Transaction = domain.Transaction
type CreateTransactionInput = domain.CreateTransactionInput
type ListTransactionsInput = domain.ListTransactionsInput
type UpdateTransactionInput = domain.UpdateTransactionInput
type SearchSimilarTransactionsInput = domain.SearchSimilarTransactionsInput
type SimilarTransaction = domain.SimilarTransaction
type TransactionRepository = domain.TransactionRepository
type TransactionService = domain.TransactionService
type LLM = domain.LLM
type LLMResponse = domain.LLMResponse
type LLMToolCall = domain.LLMToolCall
type CategoryResolver = domain.CategoryResolver
type AgentResponse = domain.AgentResponse
type CircuitBreakerConfig = domain.CircuitBreakerConfig
type AgentService = domain.AgentService
type ReceiptService = domain.ReceiptService
type EmbeddingProvider = domain.EmbeddingProvider
type EmbeddingService = domain.EmbeddingService
