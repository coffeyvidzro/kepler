package expiry

import "time"

type Config struct {
	PollInterval time.Duration
	BatchSize    int32
	BatchTimeout time.Duration
}

func DefaultConfig() Config {
	return Config{
		PollInterval: time.Second,
		BatchSize:    100,
		BatchTimeout: 15 * time.Second,
	}
}

func normalizeConfig(config Config) Config {
	defaults := DefaultConfig()
	if config.PollInterval <= 0 {
		config.PollInterval = defaults.PollInterval
	}
	if config.BatchSize <= 0 {
		config.BatchSize = defaults.BatchSize
	}
	if config.BatchTimeout <= 0 {
		config.BatchTimeout = defaults.BatchTimeout
	}
	return config
}
