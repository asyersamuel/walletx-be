package ports

import (
	"context"

	"walletx-be/internal/domain/dto"
)

type MessageQueueService interface {
	Publish(ctx context.Context, payload dto.EmailProcessingPayload) error
}
