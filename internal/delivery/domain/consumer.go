package domainreconciliation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	domainmodule "github.com/coffeyvidzro/dugble/server/internal/modules/domain"
)

type repository interface {
	ClaimPendingReconciliations(context.Context, string, int32, time.Time) ([]domainmodule.ReconciliationClaim, error)
	CompleteReconciliation(context.Context, uuid.UUID, string, string, []domainmodule.VerificationRecord, time.Time) (domainmodule.SenderDomain, error)
	RecordReconciliationFailure(context.Context, uuid.UUID, string, error, time.Time) (domainmodule.SenderDomain, error)
	CompleteHealthCheck(context.Context, uuid.UUID, string, time.Time) (domainmodule.SenderDomain, error)
	RecordHealthFailure(context.Context, uuid.UUID, string, error, int32, time.Time) (domainmodule.SenderDomain, error)
}

type checker interface {
	Check(context.Context, domainmodule.SenderDomain) (domainmodule.ReconciliationResult, error)
}

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

type Consumer struct {
	repository repository
	checker    checker
	config     Config
	workerID   string
	now        func() time.Time
}

func NewConsumer(repository repository, checker checker, config Config, workerID string) *Consumer {
	return &Consumer{repository: repository, checker: checker, config: config, workerID: workerID, now: func() time.Time { return time.Now().UTC() }}
}

func (c *Consumer) Run(ctx context.Context) error {
	if c == nil || c.repository == nil || c.checker == nil {
		return errors.New("sender domain reconciliation consumer is not configured")
	}
	if c.workerID == "" || c.config.PollInterval <= 0 || c.config.BatchSize <= 0 || c.config.Concurrency <= 0 || c.config.LockTimeout <= 0 || c.config.CheckTimeout <= 0 || c.config.HealthCheckInterval <= 0 || c.config.HealthRetryInterval <= 0 || c.config.HealthFailureThreshold <= 0 {
		return errors.New("sender domain reconciliation configuration is invalid")
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
	claims, err := c.repository.ClaimPendingReconciliations(ctx, c.workerID, c.config.BatchSize, now.Add(-c.config.LockTimeout))
	if err != nil {
		return err
	}
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
				slog.Error("sender domain reconciliation failed", "domain_id", claim.Domain.ID, "attempt", claim.Attempt, "error", err)
			}
		}()
	}
	wait.Wait()
	return ctx.Err()
}

func (c *Consumer) reconcile(ctx context.Context, claim domainmodule.ReconciliationClaim) error {
	id, err := uuid.Parse(claim.Domain.ID)
	if err != nil {
		return fmt.Errorf("parse sender domain id: %w", err)
	}
	checkCtx, cancel := context.WithTimeout(ctx, c.config.CheckTimeout)
	defer cancel()
	result, checkErr := c.checker.Check(checkCtx, claim.Domain)
	if claim.Domain.Status == domainmodule.StatusVerified {
		return c.completeHealthCheck(ctx, id, result, checkErr)
	}
	// Claims contain the incremented, one-based attempt count, while the
	// backoff schedule is indexed from zero.
	delay := nextCheckDelay(max(claim.Attempt-1, 0), id)
	if checkErr == nil && result.Status == domainmodule.StatusVerified {
		delay = jitter(c.config.HealthCheckInterval, id)
	}
	nextCheckAt := c.now().Add(delay)
	if checkErr != nil {
		_, recordErr := c.repository.RecordReconciliationFailure(ctx, id, c.workerID, checkErr, nextCheckAt)
		return errors.Join(checkErr, recordErr)
	}
	_, err = c.repository.CompleteReconciliation(ctx, id, c.workerID, result.Status, result.VerificationRecords, nextCheckAt)
	return err
}

func (c *Consumer) completeHealthCheck(ctx context.Context, id uuid.UUID, result domainmodule.ReconciliationResult, checkErr error) error {
	if checkErr == nil && result.Status == domainmodule.StatusVerified {
		_, err := c.repository.CompleteHealthCheck(ctx, id, c.workerID, c.now().Add(jitter(c.config.HealthCheckInterval, id)))
		return err
	}
	if checkErr == nil {
		checkErr = errors.New("sender domain verification checks no longer pass")
	}
	_, recordErr := c.repository.RecordHealthFailure(ctx, id, c.workerID, checkErr, c.config.HealthFailureThreshold, c.now().Add(jitter(c.config.HealthRetryInterval, id)))
	return errors.Join(checkErr, recordErr)
}

func nextCheckDelay(attempt int32, id uuid.UUID) time.Duration {
	var delay time.Duration
	switch attempt {
	case 0, 1:
		delay = time.Minute
	case 2:
		delay = 2 * time.Minute
	case 3:
		delay = 5 * time.Minute
	case 4:
		delay = 10 * time.Minute
	case 5:
		delay = 30 * time.Minute
	default:
		delay = time.Hour << min(attempt-6, 2)
		if delay > 6*time.Hour {
			delay = 6 * time.Hour
		}
	}
	return jitter(delay, id)
}

func jitter(delay time.Duration, id uuid.UUID) time.Duration {
	jitterPercent := int(id[0])%21 - 10
	return delay + time.Duration(jitterPercent)*delay/100
}
