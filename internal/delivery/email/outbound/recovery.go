package emaildelivery

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type RecoveryDecision string

const (
	RecoveryNotRequired       RecoveryDecision = "not_required"
	RecoveryPending           RecoveryDecision = "pending"
	RecoveryRetry             RecoveryDecision = "retry"
	RecoverySubmissionUnknown RecoveryDecision = "submission_unknown"
)

var ErrMessageRecoveryPending = errors.New("email delivery recovery is pending")

// RecoverStale classifies one interrupted delivery attempt while holding the
// message and attempt locks. A request that had not started can be retried; a
// request that may have reached SES is moved to submission_unknown and must be
// reconciled from provider feedback rather than sent again.
func (r *Repository) RecoverStale(
	ctx context.Context,
	messageID, teamID uuid.UUID,
	staleBefore time.Time,
) (RecoveryDecision, error) {
	if r == nil || r.db == nil {
		return RecoveryNotRequired, errors.New("email delivery repository is not configured")
	}
	if messageID == uuid.Nil || teamID == uuid.Nil {
		return RecoveryNotRequired, errors.New("email message and team IDs are required")
	}
	if staleBefore.IsZero() {
		return RecoveryNotRequired, errors.New("stale email delivery cutoff is required")
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return RecoveryNotRequired, fmt.Errorf("begin stale email recovery: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var messageStatus string
	var processingAt *time.Time
	var attemptID *uuid.UUID
	err = tx.QueryRow(ctx, `
		SELECT status, processing_at, current_delivery_attempt_id
		FROM email_messages
		WHERE id = $1 AND team_id = $2
		FOR UPDATE
	`, messageID, teamID).Scan(&messageStatus, &processingAt, &attemptID)
	if errors.Is(err, pgx.ErrNoRows) {
		return RecoveryNotRequired, nil
	}
	if err != nil {
		return RecoveryNotRequired, fmt.Errorf("lock email message for recovery: %w", err)
	}
	if messageStatus != "processing" {
		return RecoveryNotRequired, tx.Commit(ctx)
	}
	if processingAt == nil || !processingAt.UTC().Before(staleBefore.UTC()) {
		return RecoveryPending, tx.Commit(ctx)
	}
	if attemptID == nil || *attemptID == uuid.Nil {
		return RecoveryNotRequired, fmt.Errorf("processing email %s has no delivery attempt", messageID)
	}

	var attemptStatus string
	if err := tx.QueryRow(ctx, `
		SELECT status
		FROM message_delivery_attempts
		WHERE id = $1
		  AND email_message_id = $2
		  AND team_id = $3
		  AND channel = 'email'
		FOR UPDATE
	`, *attemptID, messageID, teamID).Scan(&attemptStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RecoveryNotRequired, fmt.Errorf("delivery attempt %s for processing email %s was not found", *attemptID, messageID)
		}
		return RecoveryNotRequired, fmt.Errorf("lock email delivery attempt for recovery: %w", err)
	}

	switch attemptStatus {
	case "claimed":
		if _, err := tx.Exec(ctx, `
			UPDATE message_delivery_attempts
			SET status = 'retryable_failure',
				error_code = 'worker_interrupted_before_request',
				error_message = 'Worker stopped before the provider request started',
				request_completed_at = COALESCE(request_completed_at, now()),
				terminal_at = COALESCE(terminal_at, now()), updated_at = now()
			WHERE id = $1 AND channel = 'email'
		`, *attemptID); err != nil {
			return RecoveryNotRequired, fmt.Errorf("recover unstarted email attempt: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE email_messages
			SET status = 'queued',
				current_delivery_attempt_id = NULL,
				processing_at = NULL,
				error_code = 'worker_interrupted_before_request',
				error_message = 'Worker stopped before the provider request started',
				updated_at = now()
			WHERE id = $1 AND team_id = $2
		`, messageID, teamID); err != nil {
			return RecoveryNotRequired, fmt.Errorf("requeue interrupted email: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return RecoveryNotRequired, fmt.Errorf("commit retryable email recovery: %w", err)
		}
		return RecoveryRetry, nil

	case "request_started":
		if _, err := tx.Exec(ctx, `
			UPDATE message_delivery_attempts
			SET status = 'submission_unknown',
				error_code = 'worker_interrupted',
				error_message = 'Worker stopped after the provider request started',
				request_completed_at = COALESCE(request_completed_at, now()),
				updated_at = now()
			WHERE id = $1 AND channel = 'email'
		`, *attemptID); err != nil {
			return RecoveryNotRequired, fmt.Errorf("recover uncertain email attempt: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE email_messages
			SET status = 'submission_unknown',
				error_code = 'worker_interrupted',
				error_message = 'Worker stopped after the provider request started',
				updated_at = now()
			WHERE id = $1 AND team_id = $2
		`, messageID, teamID); err != nil {
			return RecoveryNotRequired, fmt.Errorf("mark interrupted email submission unknown: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return RecoveryNotRequired, fmt.Errorf("commit uncertain email recovery: %w", err)
		}
		return RecoverySubmissionUnknown, nil

	case "submission_unknown":
		if _, err := tx.Exec(ctx, `
			UPDATE email_messages
			SET status = 'submission_unknown', updated_at = now()
			WHERE id = $1 AND team_id = $2
		`, messageID, teamID); err != nil {
			return RecoveryNotRequired, fmt.Errorf("restore uncertain email state: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return RecoveryNotRequired, fmt.Errorf("commit restored email state: %w", err)
		}
		return RecoverySubmissionUnknown, nil

	default:
		return RecoveryNotRequired, fmt.Errorf(
			"cannot recover processing email %s from delivery attempt state %q",
			messageID,
			attemptStatus,
		)
	}
}
