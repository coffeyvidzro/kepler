package dispatch

import (
	"errors"
	"time"
)

var (
	ErrNotFound               = errors.New("verification dispatch resource not found")
	ErrConsumerNotConfigured  = errors.New("verification dispatch consumer is not fully configured")
	ErrProcessorNotConfigured = errors.New("verification dispatch processor is not configured")
)

var defaultRetryDelays = []time.Duration{
	5 * time.Second,
	30 * time.Second,
	2 * time.Minute,
	10 * time.Minute,
	time.Hour,
	6 * time.Hour,
}

type ConsumerConfig struct {
	Concurrency    int
	AckWait        time.Duration
	HandlerTimeout time.Duration
	MaxDeliver     int
	RetryDelays    []time.Duration
}

func DefaultConsumerConfig() ConsumerConfig {
	return ConsumerConfig{
		Concurrency: 5, AckWait: 2 * time.Minute, HandlerTimeout: 45 * time.Second,
		MaxDeliver:  len(defaultRetryDelays),
		RetryDelays: append([]time.Duration(nil), defaultRetryDelays...),
	}
}

func normalizeConsumerConfig(config ConsumerConfig) ConsumerConfig {
	defaults := DefaultConsumerConfig()
	if config.Concurrency <= 0 {
		config.Concurrency = defaults.Concurrency
	}
	if config.AckWait <= 0 {
		config.AckWait = defaults.AckWait
	}
	if config.HandlerTimeout <= 0 {
		config.HandlerTimeout = defaults.HandlerTimeout
	}
	if config.MaxDeliver <= 0 {
		config.MaxDeliver = defaults.MaxDeliver
	}
	if len(config.RetryDelays) == 0 {
		config.RetryDelays = defaults.RetryDelays
	} else {
		config.RetryDelays = append([]time.Duration(nil), config.RetryDelays...)
	}
	return config
}
