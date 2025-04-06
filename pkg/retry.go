package pkg

import (
	"time"

	gocache "github.com/eko/gocache/lib/v4/cache"
	v1 "github.com/flanksource/batch-runner/pkg/apis/batch/v1"
	"github.com/flanksource/duty/cache"
	"github.com/flanksource/duty/context"
)

type RetryCache struct {
	items gocache.CacheInterface[*RetryItem]
}

type RetryItem struct {
	LastAttempt time.Time
	Count       int
}

func NewRetryCache() *RetryCache {
	return &RetryCache{
		items: cache.NewCache[*RetryItem]("retry-cache", 24*time.Hour),
	}
}

func (rc *RetryCache) GetBackoff(ctx context.Context, messageID string, retry *v1.Retry) *time.Duration {
	if retry == nil {
		retry = &v1.Retry{
			Attempts: 3,
			Delay:    30,
		}
	}
	baseDelay := time.Duration(retry.Delay) * time.Second

	item, _ := rc.items.Get(ctx, messageID)
	if item == nil {
		rc.items.Set(ctx, messageID, &RetryItem{
			LastAttempt: time.Now(),
			Count:       1,
		})
		ctx.Warnf("Retrying in %s (1 of %d)", baseDelay, retry.Attempts)
		return &baseDelay
	}

	item.Count++
	item.LastAttempt = time.Now()

	if item.Count > retry.Attempts {
		ctx.Errorf("Max retries exceeded (%d)", retry.Attempts)
		rc.items.Delete(ctx, messageID)
		return nil
	}

	ctx.Warnf("Retrying in %s (%d of %d)", baseDelay, item.Count, retry.Attempts)

	return &baseDelay
}

func (rc *RetryCache) Remove(ctx context.Context, messageID string) {
	rc.items.Delete(ctx, messageID)
}
