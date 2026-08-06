package redis

import (
	"context"
	"fmt"
	"strings"
	"time"

	nrredis "github.com/newrelic/go-agent/v3/integrations/nrredis-v9"
	goredis "github.com/redis/go-redis/v9"
)

// New creates and verifies a Redis client.
func New(ctx context.Context, redisURL string) (*goredis.Client, error) {
	if ctx == nil {
		return nil, fmt.Errorf("redis context is required")
	}
	redisURL = strings.TrimSpace(redisURL)
	if redisURL == "" {
		return nil, fmt.Errorf("REDIS_URL is required")
	}
	options, err := goredis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse REDIS_URL: %w", err)
	}
	options.MaxRetries = 3
	options.DialTimeout = 5 * time.Second
	options.ReadTimeout = 3 * time.Second
	options.WriteTimeout = 3 * time.Second
	options.PoolSize = 20
	options.MinIdleConns = 2
	options.ConnMaxIdleTime = 5 * time.Minute
	client := goredis.NewClient(options)
	client.AddHook(nrredis.NewHook(options))
	pingContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.Ping(pingContext).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping Redis: %w", err)
	}
	return client, nil
}
