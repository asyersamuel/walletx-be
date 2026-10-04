package http

import domain "walletx-be/internal/modules/transactions/contracts"

type Handler struct {
	service domain.TransactionService
	agent   domain.AgentService
	receipt domain.ReceiptService
}
