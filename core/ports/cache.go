package ports

import "context"

type DashboardCacheManager interface {
	InvalidateUserCache(ctx context.Context, userID string) error
}
