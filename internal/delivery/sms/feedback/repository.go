package feedback

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

type PendingMessage struct {
	ID                uuid.UUID
	TeamID            uuid.UUID
	ProviderID        string
	ProviderMessageID string
	Status            string
	UpdatedAt         time.Time
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (repository *Repository) ListPending(ctx context.Context, limit int32) ([]PendingMessage, error) {
	if repository == nil || repository.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := repository.db.Query(ctx, `
		SELECT id, team_id, provider_id, provider_message_id, status, updated_at
		FROM sms_messages
		WHERE provider_id IS NOT NULL
		  AND provider_message_id IS NOT NULL
		  AND status IN ('submitted', 'sent', 'unknown')
		ORDER BY updated_at, id
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list SMS messages pending feedback: %w", err)
	}
	defer rows.Close()

	messages := make([]PendingMessage, 0, limit)
	for rows.Next() {
		var message PendingMessage
		if err := rows.Scan(
			&message.ID,
			&message.TeamID,
			&message.ProviderID,
			&message.ProviderMessageID,
			&message.Status,
			&message.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan SMS feedback candidate: %w", err)
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate SMS feedback candidates: %w", err)
	}
	return messages, nil
}

func (repository *Repository) Apply(ctx context.Context, event Event) error {
	if repository == nil || repository.db == nil {
		return ErrRepositoryNotConfigured
	}
	event = event.Normalize()
	if err := event.Validate(); err != nil {
		return err
	}

	tx, err := repository.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin SMS feedback transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var currentStatus string
	err = tx.QueryRow(ctx, `
		SELECT status
		FROM sms_messages
		WHERE provider_id = $1 AND provider_message_id = $2
		FOR UPDATE
	`, event.ProviderID, event.ProviderMessageID).Scan(&currentStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return tx.Commit(ctx)
	}
	if err != nil {
		return fmt.Errorf("load SMS message for feedback: %w", err)
	}
	nextStatus := monotonicStatus(currentStatus, event.Status)
	if nextStatus == strings.ToLower(strings.TrimSpace(currentStatus)) {
		return tx.Commit(ctx)
	}

	_, err = tx.Exec(ctx, `
		UPDATE sms_messages
		SET status = $3,
			delivered_at = CASE WHEN $3 = 'delivered' THEN COALESCE(delivered_at, $4) ELSE delivered_at END,
			error_message = CASE
				WHEN $3 IN ('undelivered', 'rejected', 'failed', 'expired') THEN NULLIF($5, '')
				WHEN $3 = 'delivered' THEN NULL
				ELSE error_message
			END,
			updated_at = now()
		WHERE provider_id = $1 AND provider_message_id = $2
	`, event.ProviderID, event.ProviderMessageID, nextStatus, event.OccurredAt, event.ErrorMessage)
	if err != nil {
		return fmt.Errorf("apply SMS feedback status: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit SMS feedback status: %w", err)
	}
	return nil
}

func monotonicStatus(current, next string) string {
	current = strings.ToLower(strings.TrimSpace(current))
	next = strings.ToLower(strings.TrimSpace(next))
	if terminalStatus(current) || next == "unknown" {
		return current
	}
	currentRank, currentProgress := progressRank(current)
	nextRank, nextProgress := progressRank(next)
	if currentProgress && nextProgress && nextRank < currentRank {
		return current
	}
	return next
}

func terminalStatus(status string) bool {
	switch status {
	case "delivered", "undelivered", "rejected", "failed", "expired", "canceled":
		return true
	default:
		return false
	}
}

func progressRank(status string) (int, bool) {
	switch status {
	case "queued":
		return 0, true
	case "processing":
		return 1, true
	case "submitted":
		return 2, true
	case "sent":
		return 3, true
	default:
		return 0, false
	}
}
