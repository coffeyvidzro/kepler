package webhook

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const claimDeliveriesSQL = `
WITH candidates AS (
    SELECT delivery.id
    FROM webhook_deliveries AS delivery
    JOIN webhook_endpoints AS endpoint ON endpoint.id = delivery.endpoint_id
    JOIN webhook_events AS event ON event.id = delivery.event_id
    JOIN teams AS team ON team.id = event.team_id
    WHERE delivery.status IN ('pending', 'retrying')
      AND delivery.next_attempt_at <= now()
      AND endpoint.enabled = true
      AND endpoint.disabled_at IS NULL
      AND team.status = 'active'
      AND (delivery.locked_at IS NULL OR delivery.locked_at < $2)
    ORDER BY delivery.next_attempt_at, delivery.created_at
    FOR UPDATE OF delivery SKIP LOCKED
    LIMIT $3
)
UPDATE webhook_deliveries AS delivery
SET locked_at = now(),
    locked_by = $1,
    attempt_count = delivery.attempt_count + 1,
    last_attempt_at = now(),
    updated_at = now()
FROM candidates, webhook_events AS event, webhook_endpoints AS endpoint
WHERE delivery.id = candidates.id
  AND event.id = delivery.event_id
  AND endpoint.id = delivery.endpoint_id
RETURNING delivery.id, delivery.event_id, delivery.endpoint_id, delivery.attempt_count,
          event.team_id, event.event_type, event.payload, event.occurred_at,
          endpoint.url, endpoint.signing_secret`

const markSucceededSQL = `
WITH succeeded AS (
    UPDATE webhook_deliveries AS delivery
    SET status = 'succeeded', response_status = $3, response_body = $4,
        last_error = NULL, delivered_at = now(), locked_at = NULL,
        locked_by = NULL, updated_at = now()
    WHERE delivery.id = $1 AND delivery.locked_by = $2
    RETURNING delivery.id, delivery.endpoint_id
), reset_endpoint AS (
    UPDATE webhook_endpoints
    SET consecutive_failures = 0, last_failure_at = NULL,
        disabled_reason = NULL, updated_at = now()
    WHERE id = (SELECT endpoint_id FROM succeeded)
      AND enabled = true AND consecutive_failures > 0
    RETURNING id
)
SELECT id FROM succeeded`

const scheduleRetrySQL = `
UPDATE webhook_deliveries
SET status = 'retrying', next_attempt_at = $3, response_status = $4,
    response_body = $5, last_error = $6, locked_at = NULL,
    locked_by = NULL, updated_at = now()
WHERE id = $1 AND locked_by = $2
RETURNING id`

const markFailedSQL = `
WITH failed AS (
    UPDATE webhook_deliveries AS delivery
    SET status = 'failed', response_status = $3, response_body = $4,
        last_error = $5, locked_at = NULL, locked_by = NULL, updated_at = now()
    WHERE delivery.id = $1 AND delivery.locked_by = $2
    RETURNING delivery.id, delivery.endpoint_id
), update_endpoint AS (
    UPDATE webhook_endpoints
    SET consecutive_failures = consecutive_failures + 1,
        last_failure_at = now(),
        enabled = CASE WHEN consecutive_failures + 1 >= $6 THEN false ELSE enabled END,
        disabled_at = CASE
            WHEN consecutive_failures + 1 >= $6 THEN COALESCE(disabled_at, now())
            ELSE disabled_at
        END,
        disabled_reason = CASE
            WHEN enabled AND consecutive_failures + 1 >= $6 THEN 'failure_threshold'
            ELSE disabled_reason
        END,
        updated_at = now()
    WHERE id = (SELECT endpoint_id FROM failed)
    RETURNING id
)
SELECT id FROM failed`

const releaseClaimSQL = `
UPDATE webhook_deliveries
SET locked_at = NULL, locked_by = NULL, updated_at = now()
WHERE id = $1 AND locked_by = $2`

type Repository struct {
	db               *pgxpool.Pool
	autoDisableAfter int32
}

type RepositoryConfig struct {
	AutoDisableAfter int32
}

func NewRepository(db *pgxpool.Pool, configs ...RepositoryConfig) *Repository {
	config := RepositoryConfig{AutoDisableAfter: 20}
	if len(configs) > 0 {
		config = configs[0]
	}
	if config.AutoDisableAfter <= 0 {
		config.AutoDisableAfter = 20
	}
	return &Repository{db: db, autoDisableAfter: config.AutoDisableAfter}
}

func (repository *Repository) Claim(ctx context.Context, workerID string, limit int32, staleBefore time.Time) ([]ClaimedDelivery, error) {
	workerID, err := repository.validate(workerID)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, errors.New("webhook delivery claim limit must be positive")
	}
	rows, err := repository.db.Query(ctx, claimDeliveriesSQL, workerID, staleBefore.UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("claim webhook deliveries: %w", err)
	}
	defer rows.Close()

	deliveries := make([]ClaimedDelivery, 0, limit)
	for rows.Next() {
		var delivery ClaimedDelivery
		if err := rows.Scan(
			&delivery.ID,
			&delivery.EventID,
			&delivery.EndpointID,
			&delivery.AttemptCount,
			&delivery.TeamID,
			&delivery.EventType,
			&delivery.Payload,
			&delivery.OccurredAt,
			&delivery.URL,
			&delivery.SigningSecret,
		); err != nil {
			return nil, fmt.Errorf("scan claimed webhook delivery: %w", err)
		}
		deliveries = append(deliveries, delivery)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read claimed webhook deliveries: %w", err)
	}
	return deliveries, nil
}

func (repository *Repository) MarkSucceeded(ctx context.Context, id uuid.UUID, workerID string, status int32, body *string) error {
	workerID, err := repository.validateResult(id, workerID)
	if err != nil {
		return err
	}
	return scanResult(repository.db.QueryRow(ctx, markSucceededSQL, id, workerID, status, body), "mark webhook delivery succeeded")
}

func (repository *Repository) ScheduleRetry(ctx context.Context, id uuid.UUID, workerID string, nextAttempt time.Time, status *int32, body *string, lastError string) error {
	workerID, err := repository.validateResult(id, workerID)
	if err != nil {
		return err
	}
	if nextAttempt.IsZero() {
		return errors.New("webhook delivery next attempt is required")
	}
	return scanResult(
		repository.db.QueryRow(ctx, scheduleRetrySQL, id, workerID, nextAttempt.UTC(), status, body, strings.TrimSpace(lastError)),
		"schedule webhook delivery retry",
	)
}

func (repository *Repository) MarkFailed(ctx context.Context, id uuid.UUID, workerID string, status *int32, body *string, lastError string) error {
	workerID, err := repository.validateResult(id, workerID)
	if err != nil {
		return err
	}
	return scanResult(
		repository.db.QueryRow(ctx, markFailedSQL, id, workerID, status, body, strings.TrimSpace(lastError), repository.autoDisableAfter),
		"mark webhook delivery failed",
	)
}

func (repository *Repository) ReleaseClaim(ctx context.Context, id uuid.UUID, workerID string) error {
	workerID, err := repository.validateResult(id, workerID)
	if err != nil {
		return err
	}
	tag, err := repository.db.Exec(ctx, releaseClaimSQL, id, workerID)
	if err != nil {
		return fmt.Errorf("release webhook delivery claim: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrClaimLost
	}
	return nil
}

func (repository *Repository) validate(workerID string) (string, error) {
	if repository == nil || repository.db == nil {
		return "", ErrQueueNotConfigured
	}
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		return "", errors.New("webhook delivery worker id is required")
	}
	return workerID, nil
}

func (repository *Repository) validateResult(id uuid.UUID, workerID string) (string, error) {
	workerID, err := repository.validate(workerID)
	if err != nil {
		return "", err
	}
	if id == uuid.Nil {
		return "", errors.New("webhook delivery id is required")
	}
	return workerID, nil
}

func scanResult(row pgx.Row, operation string) error {
	var id uuid.UUID
	if err := row.Scan(&id); errors.Is(err, pgx.ErrNoRows) {
		return ErrClaimLost
	} else if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}

var _ Queue = (*Repository)(nil)
