package jobs

import domain "walletx-be/internal/modules/transactions/contracts"

// The external test bridge binds the real service constructor. Keeping the
// import outside package jobs avoids the service -> jobs -> service test cycle.
var NewTransactionService func(domain.TransactionRepository, ...domain.EmbeddingService) domain.TransactionService
