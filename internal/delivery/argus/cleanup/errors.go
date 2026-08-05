package cleanup

import (
	"errors"
	"time"
)

var ErrWorkerNotConfigured = errors.New("verification cleanup worker is not configured")

type Config struct {
	PollInterval time.Duration
	Retention    time.Duration
	BatchSize    int32
	BatchTimeout time.Duration
}

func DefaultConfig() Config {
	return Config{
		PollInterval: time.Hour,
		Retention:    30 * 24 * time.Hour,
		BatchSize:    500,
		BatchTimeout: 30 * time.Second,
	}
}

func normalizeConfig(config Config) Config {
	defaults := DefaultConfig()
	if config.PollInterval <= 0 {
		config.PollInterval = defaults.PollInterval
	}
	if config.Retention <= 0 {
		config.Retention = defaults.Retention
	}
	if config.BatchSize <= 0 {
		config.BatchSize = defaults.BatchSize
	}
	if config.BatchTimeout <= 0 {
		config.BatchTimeout = defaults.BatchTimeout
	}
	return config
}
