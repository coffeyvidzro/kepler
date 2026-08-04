package cache

import (
	"context"
	"fmt"
	"time"

	nrredis "github.com/newrelic/go-agent/v3/integrations/nrredis-v9"
	"github.com/redis/go-redis/v9"
)

func NewRedis(
	ctx context.Context,
	redisURL string,
) (*redis.Client, error) {
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse REDIS_URL: %w", err)
	}

	options.MaxRetries = 3
	options.DialTimeout = 5 * time.Second
	options.ReadTimeout = 3 * time.Second
	options.WriteTimeout = 3 * time.Second

	// PoolSize is per application process. Each application process gets
	// its own connection pool.
	options.PoolSize = 20
	options.MinIdleConns = 2
	options.ConnMaxIdleTime = 5 * time.Minute

	client := redis.NewClient(options)
	client.AddHook(nrredis.NewHook(options))

	pingContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := client.Ping(pingContext).Err(); err != nil {
		_ = client.Close()

		return nil, fmt.Errorf("ping Redis: %w", err)
	}

	return client, nil
}
