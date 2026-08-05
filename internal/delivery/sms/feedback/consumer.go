package feedback

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

type ConsumerConfig struct {
	PollInterval time.Duration
}

type Consumer struct {
	reconciler *Reconciler
	config     ConsumerConfig
}

func NewConsumer(reconciler *Reconciler, config ConsumerConfig) *Consumer {
	if config.PollInterval <= 0 {
		config.PollInterval = 30 * time.Second
	}
	return &Consumer{reconciler: reconciler, config: config}
}

func (consumer *Consumer) Run(ctx context.Context) error {
	if consumer == nil || consumer.reconciler == nil {
		return ErrConsumerNotConfigured
	}
	ticker := time.NewTicker(consumer.config.PollInterval)
	defer ticker.Stop()
	for {
		processed, err := consumer.reconciler.ReconcileBatch(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			slog.WarnContext(ctx, "SMS feedback reconciliation failed", "processed", processed, "error", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
