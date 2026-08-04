package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/newrelic/go-agent/v3/integrations/nrpgx5"
)

// New creates and verifies a PostgreSQL connection pool.
func New(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	if ctx == nil { return nil, fmt.Errorf("PostgreSQL context is required") }
	databaseURL = strings.TrimSpace(databaseURL)
	if databaseURL == "" { return nil, fmt.Errorf("DATABASE_URL is required") }
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil { return nil, fmt.Errorf("parse DATABASE_URL: %w", err) }
	config.ConnConfig.Tracer = nrpgx5.NewTracer(nrpgx5.WithQueryParameters(false))
	config.MaxConns = 20
	config.MinConns = 2
	config.MaxConnLifetime = time.Hour
	config.MaxConnIdleTime = 30 * time.Minute
	config.HealthCheckPeriod = time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil { return nil, fmt.Errorf("create PostgreSQL connection pool: %w", err) }
	pingContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingContext); err != nil { pool.Close(); return nil, fmt.Errorf("ping PostgreSQL: %w", err) }
	return pool, nil
}
