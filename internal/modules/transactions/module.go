package transactions

import (
	"fmt"
	"time"
	"walletx-be/configs"

	"walletx-be/internal/modules/ai"
	transactionhttp "walletx-be/internal/modules/transactions/delivery/http"
	"walletx-be/internal/modules/transactions/infrastructure"
	"walletx-be/internal/modules/transactions/repository/postgres"
	transactionservice "walletx-be/internal/modules/transactions/service"
	db "walletx-be/internal/platform/database/sqlc"
	"walletx-be/internal/platform/logger"
)

// Module exposes transaction dependencies to the application wiring layer.
type Module struct {
	Handler         *transactionhttp.Handler
	Service         TransactionService
	Repository      TransactionRepository
	CronHandler     *transactionhttp.CronHandler
	PendingReceipts PendingReceiptProcessor
}

// NewModule wires transaction dependencies: repository -> service -> handler.
func NewModule(queries *db.Queries, appLogger logger.Logger, embedding EmbeddingProvider, toolGenerator infrastructure.LLMGenerator, extractor transactionservice.ReceiptExtractor, categoryReader ai.CategoryReader, receiptStorage ReceiptStorage, extractionTimeout time.Duration, receiptReader ReceiptReader, cronConfig configs.CronConfig) (*Module, error) {
	if err := cronConfig.Validate(); err != nil {
		return nil, err
	}
	location, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return nil, fmt.Errorf("load receipt timezone: %w", err)
	}
	repo := postgres.NewTransactionRepository(queries, appLogger)
	service := transactionservice.NewTransactionServiceWithLogger(repo, embedding, appLogger, receiptStorage)
	receiptService, err := transactionservice.NewReceiptService(extractor, categoryReader, service, receiptStorage, appLogger, extractionTimeout, location, time.Now)
	if err != nil {
		return nil, fmt.Errorf("initialize receipt service: %w", err)
	}
	// Cron completion stays inside the invocation; the regular service's
	// in-memory embedding queue cannot reliably survive a serverless response.
	cronCompletion := transactionservice.NewTransactionServiceWithLogger(repo, nil, appLogger, receiptStorage)
	pendingService, err := transactionservice.NewPendingReceiptService(postgres.NewPendingReceiptReader(queries), receiptReader, cronCompletion, extractor, categoryReader, appLogger, transactionservice.PendingReceiptOptions{
		BatchSize: cronConfig.ReceiptBatchSize, Timeout: cronConfig.ReceiptTimeout,
		DownloadTimeout: 5 * time.Second, ExtractionTimeout: extractionTimeout, CompletionReserve: time.Second,
	}, location, time.Now)
	if err != nil {
		return nil, fmt.Errorf("initialize pending receipt service: %w", err)
	}
	llmAdapter := infrastructure.NewLLMAdapter(toolGenerator)
	categoryResolver := infrastructure.NewCategoryResolver(queries)
	agentService := transactionservice.NewAgentService(llmAdapter, service, repo, categoryResolver, embedding, CircuitBreakerConfig{
		FailureThreshold: 3,
		Cooldown:         time.Minute,
	}, categoryReader, location, time.Now, appLogger)

	return &Module{
		Handler:         transactionhttp.NewHandler(service, agentService, receiptService),
		Service:         service,
		Repository:      repo,
		PendingReceipts: pendingService,
		CronHandler:     transactionhttp.NewCronHandler(pendingService),
	}, nil
}
