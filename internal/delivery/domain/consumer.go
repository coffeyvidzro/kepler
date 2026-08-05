package domainreconciliation

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	domainmodule "github.com/coffeyvidzro/dugble/server/internal/modules/domain"
)

type Config struct {
	PollInterval           time.Duration
	BatchSize              int32
	Concurrency            int
	LockTimeout            time.Duration
	CheckTimeout           time.Duration
	HealthCheckInterval    time.Duration
	HealthRetryInterval    time.Duration
	HealthFailureThreshold int32
}

func (config Config) validate() error {
	if config.PollInterval <= 0 || config.BatchSize <= 0 || config.Concurrency <= 0 ||
		config.LockTimeout <= 0 || config.CheckTimeout <= 0 ||
		config.HealthCheckInterval <= 0 || config.HealthRetryInterval <= 0 ||
		config.HealthFailureThreshold <= 0 {
		return ErrInvalidConfig
	}
	return nil
}

type Consumer struct {
	repository repository
	processor  *Processor
	config     Config
	workerID   string
	now        func() time.Time
}

func NewConsumer(repository repository, checker checker, config Config, workerID string) *Consumer {
	processor := NewProcessor(repository, checker, config, workerID)
	return &Consumer{
		repository: repository,
		processor:  processor,
		config:     config,
		workerID:   workerID,
		now:        func() time.Time { return time.Now().UTC() },
	}
}

func (c *Consumer) Run(ctx context.Context) error {
	if c == nil || c.repository == nil || c.processor == nil || c.processor.checker == nil {
		return ErrConsumerNotConfigured
	}
	if strings.TrimSpace(c.workerID) == "" {
		return ErrWorkerIDRequired
	}
	if err := c.config.validate(); err != nil {
		return err
	}

	ticker := time.NewTicker(c.config.PollInterval)
	defer ticker.Stop()
	for {
		if err := c.poll(ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("sender domain reconciliation poll failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *Consumer) poll(ctx context.Context) error {
	now := c.now()
	claims, err := c.repository.ClaimPendingReconciliations(
		ctx,
		c.workerID,
		c.config.BatchSize,
		now.Add(-c.config.LockTimeout),
	)
	if err != nil {
		return err
	}
	c.processor.now = c.now

	semaphore := make(chan struct{}, c.config.Concurrency)
	var wait sync.WaitGroup
	for _, claim := range claims {
		claim := claim
		wait.Add(1)
		go func() {
			defer wait.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				return
			}
			if err := c.reconcile(ctx, claim); err != nil && !errors.Is(err, context.Canceled) {
				slog.Error(
					"sender domain reconciliation failed",
					"domain_id", claim.Domain.ID,
					"attempt", claim.Attempt,
					"error", err,
				)
			}
		}()
	}
	wait.Wait()
	return ctx.Err()
}

func (c *Consumer) reconcile(ctx context.Context, claim domainmodule.ReconciliationClaim) error {
	return c.processor.Process(ctx, claim)
}
