package jobs_test

import (
	"walletx-be/internal/modules/transactions/jobs"
	"walletx-be/internal/modules/transactions/service"
)

func init() {
	jobs.NewTransactionService = service.NewTransactionService
}
