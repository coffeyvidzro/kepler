package redis

import (
	"context"

	legacy "github.com/coffeyvidzro/dugble/server/internal/platform/cache"
	goredis "github.com/redis/go-redis/v9"
)

// New creates and verifies a Redis client.
func New(ctx context.Context, redisURL string) (*goredis.Client, error) {
	return legacy.NewRedis(ctx, redisURL)
}
