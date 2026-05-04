package parser

import "time"

type ParsedTransactionData struct {
	Amount          float64
	Merchant        string
	TransactionDate *time.Time
}
