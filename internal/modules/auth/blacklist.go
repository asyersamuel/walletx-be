package auth

import (
	"context"
	"sync"
	"time"

	"walletx-be/internal/middleware"
)

// inMemoryBlacklistRepository keeps revoked tokens local to one API process.
type inMemoryBlacklistRepository struct {
	mu     sync.RWMutex
	tokens map[string]time.Time
}

// NewInMemoryBlacklistRepository creates a token blacklist for the current process. 
func NewInMemoryBlacklistRepository() middleware.TokenBlacklistRepository {
	return &inMemoryBlacklistRepository{tokens: make(map[string]time.Time)}
}

func (r *inMemoryBlacklistRepository) BlacklistToken(ctx context.Context, jti string, ttl time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if jti == "" || ttl <= 0 {
		return nil
	}

	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()

	for tokenID, expiresAt := range r.tokens {
		if !expiresAt.After(now) {
			delete(r.tokens, tokenID)
		}
	}
	r.tokens[jti] = now.Add(ttl)
	return nil
}

func (r *inMemoryBlacklistRepository) IsTokenBlacklisted(ctx context.Context, jti string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	now := time.Now()
	r.mu.RLock()
	expiresAt, exists := r.tokens[jti]
	r.mu.RUnlock()

	if !exists || !expiresAt.After(now) {
		if exists {
			r.mu.Lock()
			if currentExpiry, stillExists := r.tokens[jti]; stillExists && !currentExpiry.After(now) {
				delete(r.tokens, jti)
			}
			r.mu.Unlock()
		}
		return false, nil
	}

	return true, nil
}
