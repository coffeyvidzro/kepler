package broadcastexecution

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

type Config struct {
	PollInterval time.Duration
	BatchSize    int
}

type Consumer struct {
	processor *Processor
	config    Config
}

func NewConsumer(processor *Processor, config Config) *Consumer {
	if config.PollInterval <= 0 {
		config.PollInterval = time.Second
	}
	if config.BatchSize <= 0 {
		config.BatchSize = 100
	}
	return &Consumer{processor: processor, config: config}
}

func (c *Consumer) Run(ctx context.Context) error {
	if c == nil || c.processor == nil {
		return ErrConsumerNotConfigured
	}

	for {
		if err := c.poll(ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("broadcast execution poll failed", "error", err)
		}

		timer := time.NewTimer(c.config.PollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil
		case <-timer.C:
		}
	}
}

func (c *Consumer) poll(ctx context.Context) error {
	if c == nil || c.processor == nil {
		return ErrConsumerNotConfigured
	}
	return c.processor.ProcessBatch(ctx, c.config.BatchSize)
}
