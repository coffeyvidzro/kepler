package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/newrelic/go-agent/v3/integrations/nrpgx5"
)

func NewPostgres(
	ctx context.Context,
	databaseURL string,
) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}

	poolConfig.ConnConfig.Tracer = nrpgx5.NewTracer(
		nrpgx5.WithQueryParameters(false),
	)
	poolConfig.MaxConns = 20
	poolConfig.MinConns = 2
	poolConfig.MaxConnLifetime = time.Hour
	poolConfig.MaxConnIdleTime = 30 * time.Minute
	poolConfig.HealthCheckPeriod = time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf(
			"create PostgreSQL connection pool: %w",
			err,
		)
	}

	pingContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := pool.Ping(pingContext); err != nil {
		pool.Close()

		return nil, fmt.Errorf("ping PostgreSQL: %w", err)
	}

	return pool, nil
}
