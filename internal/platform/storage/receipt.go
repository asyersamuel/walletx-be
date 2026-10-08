package storage

import (
	"fmt"
	"net/http"

	domain "walletx-be/internal/modules/transactions/contracts"
	apperrors "walletx-be/internal/shared/errors"
)

// Validate stored bytes rather than trusting an extension or HTTP content type.
func validateStoredReceipt(image []byte) error {
	if len(image) == 0 || len(image) > domain.MaxReceiptBytes {
		return fmt.Errorf("stored receipt has invalid size: %w", apperrors.ErrInternal)
	}
	if receiptExtension(http.DetectContentType(image)) == "" {
		return fmt.Errorf("stored receipt is not a supported image: %w", apperrors.ErrInternal)
	}
	return nil
}
