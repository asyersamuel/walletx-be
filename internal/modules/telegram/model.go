package telegram

// VerifyData is stored in Redis during the account-linking email flow.
type VerifyData struct {
	Email  string `json:"email"`
	ChatID int64  `json:"chat_id"`
}

// PendingTxData is stored in Redis while the user picks a category.
type PendingTxData struct {
	Merchant string  `json:"merchant"`
	Amount   float64 `json:"amount"`
	ChatID   int64   `json:"chat_id"`
}

// CategoryState is stored in Redis between /tambah_category Step 1 and Step 2.
type CategoryState struct {
	Step         string `json:"step"`                     // "AWAITING_NAME" or "AWAITING_LIMIT"
	CategoryName string `json:"category_name,omitempty"`
	PendingTxKey string `json:"pending_tx_key,omitempty"` // For linking the transaction after creation
}
