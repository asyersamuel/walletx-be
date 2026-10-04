package jobs

import (
	"context"
	"errors"
	"fmt"
	"time"
	domain "walletx-be/internal/modules/transactions/contracts"
	"walletx-be/internal/platform/logger"

	"github.com/google/uuid"
)

// errInvalidEmbeddingDimensions is returned when the provider returns a vector
// whose length does not equal EmbeddingDimension. This error is never retried
// because retrying a structurally wrong response cannot produce a valid one.
var errInvalidEmbeddingDimensions = errors.New("embedding provider returned wrong number of dimensions")

// EmbeddingJobConfig controls the retry policy for a single embedding job.
// Zero values are safe: MaxAttempts of 0 is treated as 1 (one attempt), and
// zero Backoff means no delay between retries.
type EmbeddingJobConfig struct {
	// MaxAttempts is the total number of times Generate is called before the
	// job gives up. Must be >= 1; a value of 0 is normalised to 1 internally.
	MaxAttempts int

	// Backoff is the fixed delay inserted between consecutive attempts.
	// Use time.Nanosecond in tests so the suite stays fast.
	Backoff time.Duration
}

// EmbeddingJobInput carries all immutable data the async job needs. Every
// field is copied by value into the goroutine so the job never aliases the
// caller's memory after launch.
type EmbeddingJobInput struct {
	TransactionID uuid.UUID
	UserID        uuid.UUID
	// UpdatedAt is the transaction's updated_at at the moment the job was
	// enqueued. It is forwarded verbatim to SaveEmbedding so the database's
	// version guard can reject stale writes.
	UpdatedAt time.Time
	// Text is the pre-formatted embedding input string.
	Text string
}

// EmbeddingJob orchestrates a single async embedding attempt: generate vector,
// validate dimensions, and persist with stale-job protection.
type EmbeddingJob struct {
	repo     domain.TransactionRepository
	provider domain.EmbeddingProvider
	logger   logger.Logger
	cfg      EmbeddingJobConfig
}

// NewEmbeddingJob constructs an orchestrator with the supplied dependencies.
func NewEmbeddingJob(
	repo domain.TransactionRepository,
	provider domain.EmbeddingProvider,
	appLogger logger.Logger,
	cfg EmbeddingJobConfig,
) *EmbeddingJob {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 1
	}
	return &EmbeddingJob{
		repo:     repo,
		provider: provider,
		logger:   appLogger,
		cfg:      cfg,
	}
}

// run executes the embedding pipeline synchronously. It is always called from
// inside a goroutine by the service layer. The method returns an error so tests
// can assert specific outcomes without relying on log output.
//
// Retry policy:
//   - Transient provider errors are retried up to cfg.MaxAttempts times with
//     cfg.Backoff between each pair of consecutive attempts.
//   - errInvalidEmbeddingDimensions is non-retryable: the provider returned a
//     structurally wrong response that repeated calls cannot fix.
//   - Context cancellation/deadline is respected between attempts.
func (j *EmbeddingJob) Run(ctx context.Context, input EmbeddingJobInput) error {
	vector, err := j.generateWithRetry(ctx, input.Text)
	if err != nil {
		j.logger.
			WithError(err).
			WithFields(map[string]interface{}{
				"operation":      "generate_transaction_embedding",
				"transaction_id": input.TransactionID,
				"attempts":       j.cfg.MaxAttempts,
			}).
			Error("transaction embedding generation failed after all attempts")
		return err
	}

	if saveErr := j.repo.SaveEmbedding(
		ctx,
		input.TransactionID,
		input.UserID,
		vector,
		input.UpdatedAt,
	); saveErr != nil {
		j.logger.
			WithError(saveErr).
			WithField("operation", "save_transaction_embedding").
			Error("transaction embedding persistence failed")
		return saveErr
	}

	return nil
}

// generateWithRetry calls the provider up to cfg.MaxAttempts times. It stops
// immediately and propagates errInvalidEmbeddingDimensions without retrying.
func (j *EmbeddingJob) generateWithRetry(ctx context.Context, text string) ([]float32, error) {
	var lastErr error

	for attempt := 1; attempt <= j.cfg.MaxAttempts; attempt++ {
		vector, err := j.provider.Generate(ctx, text)
		if err != nil {
			lastErr = err

			// Honour context cancellation immediately without sleeping.
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}

			if attempt < j.cfg.MaxAttempts {
				j.sleep(ctx)
			}
			continue
		}

		// Validate dimensions before persisting. A wrong dimension count is a
		// permanent failure: the provider is misconfigured, so retrying it will
		// produce the same invalid result.
		if len(vector) != domain.EmbeddingDimension {
			return nil, fmt.Errorf(
				"%w: got %d, want %d",
				errInvalidEmbeddingDimensions,
				len(vector),
				domain.EmbeddingDimension,
			)
		}

		return vector, nil
	}

	return nil, fmt.Errorf(
		"embedding generation failed after %d attempt(s): %w",
		j.cfg.MaxAttempts,
		lastErr,
	)
}

// sleep waits for the configured backoff duration, returning early if the
// context is cancelled. It is a no-op when Backoff is zero.
func (j *EmbeddingJob) sleep(ctx context.Context) {
	if j.cfg.Backoff <= 0 {
		return
	}
	timer := time.NewTimer(j.cfg.Backoff)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
}
