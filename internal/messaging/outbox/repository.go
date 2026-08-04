package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrClaimLost = errors.New("outbox event claim was lost")

type queryExecutor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Enqueue(ctx context.Context, event Event) (uuid.UUID, error) {
	if r == nil || r.pool == nil {
		return uuid.Nil, errors.New("outbox repository is not configured")
	}
	return enqueue(ctx, r.pool, event)
}

func (r *Repository) EnqueueTx(ctx context.Context, tx pgx.Tx, event Event) (uuid.UUID, error) {
	if tx == nil {
		return uuid.Nil, errors.New("outbox transaction is required")
	}
	return enqueue(ctx, tx, event)
}

func (r *Repository) DeletePendingTx(ctx context.Context, tx pgx.Tx, eventID uuid.UUID) error {
	if tx == nil {
		return errors.New("outbox transaction is required")
	}
	result, err := tx.Exec(ctx, `
		DELETE FROM outbox_events
		WHERE id = $1 AND published_at IS NULL
	`, eventID)
	if err != nil {
		return fmt.Errorf("delete pending outbox event: %w", err)
	}
	if result.RowsAffected() != 1 {
		return errors.New("pending outbox event not found")
	}
	return nil
}

func (r *Repository) UpdatePendingAvailableAtTx(ctx context.Context, tx pgx.Tx, eventID uuid.UUID, availableAt time.Time) error {
	if tx == nil {
		return errors.New("outbox transaction is required")
	}
	result, err := tx.Exec(ctx, `
		UPDATE outbox_events
		SET available_at = $2, updated_at = now()
		WHERE id = $1 AND published_at IS NULL
	`, eventID, availableAt)
	if err != nil {
		return fmt.Errorf("reschedule pending outbox event: %w", err)
	}
	if result.RowsAffected() != 1 {
		return errors.New("pending outbox event not found")
	}
	return nil
}

func enqueue(ctx context.Context, executor queryExecutor, event Event) (uuid.UUID, error) {
	event.Subject = strings.TrimSpace(event.Subject)
	event.AggregateType = strings.TrimSpace(event.AggregateType)
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	if event.Subject == "" {
		return uuid.Nil, errors.New("outbox subject is required")
	}
	if event.AggregateType == "" {
		return uuid.Nil, errors.New("outbox aggregate type is required")
	}
	if event.AggregateID == uuid.Nil {
		return uuid.Nil, errors.New("outbox aggregate id is required")
	}
	if !json.Valid(event.Payload) {
		return uuid.Nil, errors.New("outbox payload must be valid JSON")
	}
	if event.Headers == nil {
		event.Headers = map[string]string{}
	}
	headers, err := json.Marshal(event.Headers)
	if err != nil {
		return uuid.Nil, fmt.Errorf("encode outbox headers: %w", err)
	}
	if event.AvailableAt.IsZero() {
		event.AvailableAt = time.Now().UTC()
	}

	_, err = executor.Exec(ctx, `
		INSERT INTO outbox_events (
			id,
			subject,
			aggregate_type,
			aggregate_id,
			payload,
			headers,
			available_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO NOTHING
	`, event.ID, event.Subject, event.AggregateType, event.AggregateID, event.Payload, headers, event.AvailableAt)
	if err != nil {
		return uuid.Nil, fmt.Errorf("enqueue outbox event: %w", err)
	}

	return event.ID, nil
}

func (r *Repository) ClaimBatch(
	ctx context.Context,
	workerID string,
	limit int,
	staleBefore time.Time,
) ([]Event, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("outbox repository is not configured")
	}
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		return nil, errors.New("outbox worker id is required")
	}
	if limit <= 0 {
		return nil, errors.New("outbox claim limit must be positive")
	}

	rows, err := r.pool.Query(ctx, `
		WITH candidates AS (
			SELECT id
			FROM outbox_events
			WHERE published_at IS NULL
			  AND available_at <= now()
			  AND (locked_at IS NULL OR locked_at < $3)
			ORDER BY available_at, created_at
			FOR UPDATE SKIP LOCKED
			LIMIT $2
		)
		UPDATE outbox_events AS event
		SET locked_at = now(),
			locked_by = $1,
			attempts = event.attempts + 1,
			updated_at = now()
		FROM candidates
		WHERE event.id = candidates.id
		RETURNING
			event.id,
			event.subject,
			event.aggregate_type,
			event.aggregate_id,
			event.payload,
			event.headers,
			event.available_at,
			event.attempts,
			event.created_at
	`, workerID, limit, staleBefore)
	if err != nil {
		return nil, fmt.Errorf("claim outbox events: %w", err)
	}
	defer rows.Close()

	events := make([]Event, 0, limit)
	for rows.Next() {
		var event Event
		var headersJSON []byte
		if err := rows.Scan(
			&event.ID,
			&event.Subject,
			&event.AggregateType,
			&event.AggregateID,
			&event.Payload,
			&headersJSON,
			&event.AvailableAt,
			&event.Attempts,
			&event.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan outbox event: %w", err)
		}
		if err := json.Unmarshal(headersJSON, &event.Headers); err != nil {
			return nil, fmt.Errorf("decode outbox headers for %s: %w", event.ID, err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate outbox events: %w", err)
	}

	return events, nil
}

func (r *Repository) MarkPublished(ctx context.Context, eventID uuid.UUID, workerID string) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE outbox_events
		SET published_at = now(),
			locked_at = NULL,
			locked_by = NULL,
			last_error = NULL,
			updated_at = now()
		WHERE id = $1
		  AND published_at IS NULL
		  AND locked_by = $2
	`, eventID, workerID)
	if err != nil {
		return fmt.Errorf("mark outbox event %s published: %w", eventID, err)
	}
	if result.RowsAffected() != 1 {
		return ErrClaimLost
	}
	return nil
}

func (r *Repository) Release(
	ctx context.Context,
	eventID uuid.UUID,
	workerID string,
	nextAttempt time.Time,
	lastError string,
) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE outbox_events
		SET available_at = $3,
			locked_at = NULL,
			locked_by = NULL,
			last_error = $4,
			updated_at = now()
		WHERE id = $1
		  AND published_at IS NULL
		  AND locked_by = $2
	`, eventID, workerID, nextAttempt, lastError)
	if err != nil {
		return fmt.Errorf("release outbox event %s: %w", eventID, err)
	}
	if result.RowsAffected() != 1 {
		return ErrClaimLost
	}
	return nil
}
