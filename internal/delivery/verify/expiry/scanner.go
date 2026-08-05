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

type Scanner struct {
	processor batchExpirer
	config    Config
}

type Consumer = Scanner

func NewScanner(processor batchExpirer, config Config) *Scanner {
	return &Scanner{processor: processor, config: normalizeConfig(config)}
}

func NewConsumer(processor batchExpirer, config Config) *Scanner {
	return NewScanner(processor, config)
}

func (scanner *Scanner) Run(ctx context.Context) error {
	if scanner == nil || scanner.processor == nil {
		return ErrScannerNotConfigured
	}
	ticker := time.NewTicker(scanner.config.PollInterval)
	defer ticker.Stop()
	for {
		scanner.scan(ctx)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (scanner *Scanner) scan(ctx context.Context) {
	for {
		started := time.Now()
		batchCtx, cancel := context.WithTimeout(ctx, scanner.config.BatchTimeout)
		expired, err := scanner.processor.ExpireBatch(batchCtx, scanner.config.BatchSize)
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
		if expired < int(scanner.config.BatchSize) {
			return
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}
