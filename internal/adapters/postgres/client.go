package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/newrelic/go-agent/v3/integrations/nrpgx5"
)

const defaultPingTimeout = 5 * time.Second

type Config struct {
	URL               string
	MaxConnections    int32
	MinConnections    int32
	MaxLifetime       time.Duration
	MaxIdleTime       time.Duration
	HealthCheckPeriod time.Duration
}

func DefaultConfig(databaseURL string) Config {
	return Config{
		URL:               strings.TrimSpace(databaseURL),
		MaxConnections:    20,
		MinConnections:    2,
		MaxLifetime:       time.Hour,
		MaxIdleTime:       30 * time.Minute,
		HealthCheckPeriod: time.Minute,
	}
}

// New creates and verifies a PostgreSQL connection pool using production-safe
// defaults.
func New(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	return Open(ctx, DefaultConfig(databaseURL))
}

func Open(ctx context.Context, configuration Config) (*pgxpool.Pool, error) {
	if ctx == nil {
		return nil, errors.New("PostgreSQL context is required")
	}
	configuration = normalizeConfig(configuration)
	if configuration.URL == "" {
		return nil, errors.New("DATABASE_URL is required")
	}

	poolConfig, err := pgxpool.ParseConfig(configuration.URL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	poolConfig.ConnConfig.Tracer = nrpgx5.NewTracer(nrpgx5.WithQueryParameters(false))
	poolConfig.MaxConns = configuration.MaxConnections
	poolConfig.MinConns = configuration.MinConnections
	poolConfig.MaxConnLifetime = configuration.MaxLifetime
	poolConfig.MaxConnIdleTime = configuration.MaxIdleTime
	poolConfig.HealthCheckPeriod = configuration.HealthCheckPeriod

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create PostgreSQL connection pool: %w", err)
	}
	pingContext, cancel := context.WithTimeout(ctx, defaultPingTimeout)
	defer cancel()
	if err := pool.Ping(pingContext); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping PostgreSQL: %w", err)
	}
	return pool, nil
}

func normalizeConfig(configuration Config) Config {
	defaults := DefaultConfig(configuration.URL)
	configuration.URL = strings.TrimSpace(configuration.URL)
	if configuration.MaxConnections <= 0 {
		configuration.MaxConnections = defaults.MaxConnections
	}
	if configuration.MinConnections < 0 {
		configuration.MinConnections = 0
	}
	if configuration.MinConnections > configuration.MaxConnections {
		configuration.MinConnections = configuration.MaxConnections
	}
	if configuration.MaxLifetime <= 0 {
		configuration.MaxLifetime = defaults.MaxLifetime
	}
	if configuration.MaxIdleTime <= 0 {
		configuration.MaxIdleTime = defaults.MaxIdleTime
	}
	if configuration.HealthCheckPeriod <= 0 {
		configuration.HealthCheckPeriod = defaults.HealthCheckPeriod
	}
	return configuration
}
