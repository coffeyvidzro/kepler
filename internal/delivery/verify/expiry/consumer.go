package expiry

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/coffeyvidzro/dugble/server/internal/monitoring/verifymetrics"
)

type batchExpirer interface {
	ExpireBatch(context.Context, int32) (int, error)
}

type Consumer struct {
	repository batchExpirer
	config     Config
}

func NewConsumer(repository batchExpirer, config Config) *Consumer {
	return &Consumer{repository: repository, config: normalizeConfig(config)}
}

func (consumer *Consumer) Run(ctx context.Context) error {
	if consumer == nil || consumer.repository == nil {
		return errors.New("verification expiry consumer is not configured")
	}
	ticker := time.NewTicker(consumer.config.PollInterval)
	defer ticker.Stop()
	for {
		consumer.poll(ctx)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (consumer *Consumer) poll(ctx context.Context) {
	for {
		started := time.Now()
		batchCtx, cancel := context.WithTimeout(ctx, consumer.config.BatchTimeout)
		expired, err := consumer.repository.ExpireBatch(batchCtx, consumer.config.BatchSize)
		cancel()
		verifymetrics.Default.Observe("expiry_batch", verifymetrics.Outcome(err), time.Since(started))
		verifymetrics.Default.AddExpired(expired)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				slog.Error("verification expiry batch failed", "error", err)
			}
			return
		}
		if expired > 0 {
			slog.InfoContext(ctx, "verification expiry batch completed", "expired", expired)
		}
		if expired < int(consumer.config.BatchSize) {
			return
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}
