package dispatch

import "time"

var defaultRetryDelays = []time.Duration{5 * time.Second, 30 * time.Second, 2 * time.Minute, 10 * time.Minute, time.Hour, 6 * time.Hour}

type ConsumerConfig struct {
	Concurrency    int
	AckWait        time.Duration
	HandlerTimeout time.Duration
	MaxDeliver     int
	RetryDelays    []time.Duration
}

func DefaultConsumerConfig() ConsumerConfig {
	return ConsumerConfig{Concurrency: 5, AckWait: 2 * time.Minute, HandlerTimeout: 45 * time.Second, MaxDeliver: len(defaultRetryDelays), RetryDelays: append([]time.Duration(nil), defaultRetryDelays...)}
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
	}
	return config
}
