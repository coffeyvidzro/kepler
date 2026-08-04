package feedback

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const defaultReconciliationMaxAttempts = 12

type ReconcileClaim struct {
	EventID      uuid.UUID
	AttemptCount int
}

type ReconcilerConfig struct {
	PollInterval  time.Duration
	BatchSize     int
	Concurrency   int
	LeaseDuration time.Duration
	HandleTimeout time.Duration
}

type Reconciler struct {
	repository *Repository
	config     ReconcilerConfig
}

func NewReconciler(repository *Repository, config ReconcilerConfig) *Reconciler {
	return &Reconciler{repository: repository, config: normalizeReconcilerConfig(config)}
}

func normalizeReconcilerConfig(config ReconcilerConfig) ReconcilerConfig {
	if config.PollInterval <= 0 {
		config.PollInterval = 5 * time.Second
	}
	if config.BatchSize <= 0 {
		config.BatchSize = 25
	}
	if config.Concurrency <= 0 {
		config.Concurrency = 5
	}
	if config.LeaseDuration <= 0 {
		config.LeaseDuration = 2 * time.Minute
	}
	if config.HandleTimeout <= 0 {
		config.HandleTimeout = 30 * time.Second
	}
	return config
}

func (r *Reconciler) Run(ctx context.Context) error {
	if r == nil || r.repository == nil || r.repository.db == nil {
		return errors.New("email feedback reconciler is not configured")
	}
	ticker := time.NewTicker(r.config.PollInterval)
	defer ticker.Stop()

	for {
		if err := r.runBatch(ctx); err != nil && ctx.Err() == nil {
			slog.Error("email feedback reconciliation batch failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (r *Reconciler) runBatch(ctx context.Context) error {
	claims, err := r.repository.ClaimDue(ctx, r.config.BatchSize, r.config.LeaseDuration)
	if err != nil {
		return err
	}
	if len(claims) == 0 {
		return nil
	}

	semaphore := make(chan struct{}, r.config.Concurrency)
	var waitGroup sync.WaitGroup
	for _, claim := range claims {
		claim := claim
		select {
		case <-ctx.Done():
			waitGroup.Wait()
			return nil
		case semaphore <- struct{}{}:
		}
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			defer func() { <-semaphore }()

			handleCtx, cancel := context.WithTimeout(ctx, r.config.HandleTimeout)
			err := r.repository.processClaimed(handleCtx, claim)
			cancel()
			if err == nil {
				return
			}
			if recordErr := r.repository.RecordReconcileFailure(ctx, claim, err); recordErr != nil {
				slog.Error("failed to persist email feedback reconciliation failure", "event_id", claim.EventID, "error", recordErr, "cause", err)
				return
			}
			slog.Warn("email feedback event rescheduled", "event_id", claim.EventID, "attempt", claim.AttemptCount, "error", err)
		}()
	}
	waitGroup.Wait()
	return nil
}

func (r *Repository) ClaimDue(ctx context.Context, batchSize int, leaseDuration time.Duration) ([]ReconcileClaim, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("email feedback repository is not configured")
	}
	if batchSize <= 0 {
		batchSize = 25
	}
	if leaseDuration <= 0 {
		leaseDuration = 2 * time.Minute
	}
	rows, err := r.db.Query(ctx, `
		WITH due AS (
			SELECT id
			FROM email_provider_events
			WHERE processed_at IS NULL
			  AND dead_lettered_at IS NULL
			  AND next_attempt_at IS NOT NULL
			  AND next_attempt_at <= now()
			ORDER BY next_attempt_at, id
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		)
		UPDATE email_provider_events AS event
		SET attempt_count = event.attempt_count + 1,
			last_attempt_at = now(),
			next_attempt_at = now() + ($2 * interval '1 second')
		FROM due
		WHERE event.id = due.id
		RETURNING event.id, event.attempt_count
	`, batchSize, durationSeconds(leaseDuration))
	if err != nil {
		return nil, fmt.Errorf("claim due email provider events: %w", err)
	}
	defer rows.Close()

	claims := make([]ReconcileClaim, 0, batchSize)
	for rows.Next() {
		var claim ReconcileClaim
		if err := rows.Scan(&claim.EventID, &claim.AttemptCount); err != nil {
			return nil, fmt.Errorf("scan claimed email provider event: %w", err)
		}
		claims = append(claims, claim)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate claimed email provider events: %w", err)
	}
	return claims, nil
}

func (r *Repository) claimSpecific(ctx context.Context, eventID uuid.UUID, leaseDuration time.Duration) (ReconcileClaim, bool, error) {
	if leaseDuration <= 0 {
		leaseDuration = 2 * time.Minute
	}
	var claim ReconcileClaim
	err := r.db.QueryRow(ctx, `
		UPDATE email_provider_events
		SET attempt_count = attempt_count + 1,
			last_attempt_at = now(),
			next_attempt_at = now() + ($2 * interval '1 second')
		WHERE id = $1
		  AND processed_at IS NULL
		  AND dead_lettered_at IS NULL
		  AND (next_attempt_at IS NULL OR next_attempt_at <= now())
		RETURNING id, attempt_count
	`, eventID, durationSeconds(leaseDuration)).Scan(&claim.EventID, &claim.AttemptCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReconcileClaim{}, false, nil
	}
	if err != nil {
		return ReconcileClaim{}, false, fmt.Errorf("claim email provider event %s: %w", eventID, err)
	}
	return claim, true, nil
}

func (r *Repository) RecordReconcileFailure(ctx context.Context, claim ReconcileClaim, cause error) error {
	if claim.EventID == uuid.Nil {
		return errors.New("email provider event ID is required")
	}
	reason := truncateReconciliationError(cause)
	if claim.AttemptCount >= defaultReconciliationMaxAttempts {
		_, err := r.db.Exec(ctx, `
			UPDATE email_provider_events
			SET dead_lettered_at = COALESCE(dead_lettered_at, now()),
				next_attempt_at = NULL,
				last_error = $2
			WHERE id = $1
			  AND processed_at IS NULL
		`, claim.EventID, reason)
		if err != nil {
			return fmt.Errorf("dead-letter email provider event %s: %w", claim.EventID, err)
		}
		return nil
	}
	_, err := r.db.Exec(ctx, `
		UPDATE email_provider_events
		SET next_attempt_at = now() + ($2 * interval '1 second'),
			last_error = $3
		WHERE id = $1
		  AND processed_at IS NULL
		  AND dead_lettered_at IS NULL
	`, claim.EventID, durationSeconds(reconciliationDelay(claim.AttemptCount)), reason)
	if err != nil {
		return fmt.Errorf("reschedule email provider event %s: %w", claim.EventID, err)
	}
	return nil
}

func reconciliationDelay(attempt int) time.Duration {
	delays := [...]time.Duration{
		5 * time.Second,
		30 * time.Second,
		2 * time.Minute,
		10 * time.Minute,
		30 * time.Minute,
		1 * time.Hour,
		3 * time.Hour,
		6 * time.Hour,
		12 * time.Hour,
		24 * time.Hour,
		48 * time.Hour,
		72 * time.Hour,
	}
	if attempt <= 0 {
		return delays[0]
	}
	index := attempt - 1
	if index >= len(delays) {
		index = len(delays) - 1
	}
	return delays[index]
}

func durationSeconds(value time.Duration) int64 {
	seconds := int64(value / time.Second)
	if seconds < 1 {
		return 1
	}
	return seconds
}

func truncateReconciliationError(err error) string {
	if err == nil {
		return "unknown email feedback reconciliation failure"
	}
	reason := err.Error()
	if len(reason) > 1024 {
		return reason[:1024]
	}
	return reason
}
