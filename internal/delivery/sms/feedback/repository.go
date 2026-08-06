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

	messagingfeedback "github.com/coffeyvidzro/dugble/server/internal/delivery/messaging/feedback"
	smsmodule "github.com/coffeyvidzro/dugble/server/internal/modules/sms"
	platformdelivery "github.com/coffeyvidzro/dugble/server/internal/platform/messaging/delivery"
	platformfeedback "github.com/coffeyvidzro/dugble/server/internal/platform/messaging/feedback"
	smsapi "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
)

type PendingMessage struct {
	AttemptID         uuid.UUID
	ID                uuid.UUID
	TeamID            uuid.UUID
	ProviderID        string
	ProviderMessageID string
	Status            string
	ReconcileAttempts int32
	UpdatedAt         time.Time
}

type Repository struct {
	db       *pgxpool.Pool
	messages *smsmodule.Repository
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db, messages: smsmodule.NewRepository(db)}
}

func NewRepositoryWithMessageRepository(
	db *pgxpool.Pool,
	messages *smsmodule.Repository,
) *Repository {
	if messages == nil {
		messages = smsmodule.NewRepository(db)
	}
	return &Repository{db: db, messages: messages}
}

func (repository *Repository) ListPending(ctx context.Context, limit int32) ([]PendingMessage, error) {
	if repository == nil || repository.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := repository.db.Query(ctx, `
		SELECT attempt.id, attempt.sms_message_id, attempt.team_id,
			attempt.provider, attempt.provider_message_id, attempt.status,
			attempt.reconcile_attempts, attempt.updated_at
		FROM message_delivery_attempts AS attempt
		JOIN sms_messages AS message
		  ON message.id = attempt.sms_message_id
		 AND message.team_id = attempt.team_id
		WHERE attempt.channel = 'sms'
		  AND attempt.sms_message_id IS NOT NULL
		  AND attempt.provider IS NOT NULL
		  AND attempt.provider_message_id IS NOT NULL
		  AND attempt.status IN ('submission_unknown', 'submitted', 'accepted', 'sent', 'unknown')
		  AND message.status NOT IN ('delivered', 'undelivered', 'rejected', 'failed', 'expired', 'canceled')
		ORDER BY COALESCE(attempt.last_reconciled_at, attempt.created_at), attempt.id
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list SMS attempts pending feedback: %w", err)
	}
	defer rows.Close()

	messages := make([]PendingMessage, 0, limit)
	for rows.Next() {
		var message PendingMessage
		if err := rows.Scan(
			&message.AttemptID,
			&message.ID,
			&message.TeamID,
			&message.ProviderID,
			&message.ProviderMessageID,
			&message.Status,
			&message.ReconcileAttempts,
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

func (repository *Repository) Apply(
	ctx context.Context,
	event platformfeedback.Event,
) (platformfeedback.Result, error) {
	if repository == nil || repository.db == nil || repository.messages == nil {
		return platformfeedback.Result{}, ErrRepositoryNotConfigured
	}
	tx, err := repository.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return platformfeedback.Result{}, fmt.Errorf("begin SMS feedback transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	processor, err := platformfeedback.NewProcessor(messagingfeedback.NewRepository(tx))
	if err != nil {
		return platformfeedback.Result{}, err
	}
	result, err := processor.Process(ctx, event)
	if err != nil {
		return platformfeedback.Result{}, err
	}
	if result.Duplicate || !result.Transitioned {
		if err := tx.Commit(ctx); err != nil {
			return platformfeedback.Result{}, fmt.Errorf("commit SMS feedback observation: %w", err)
		}
		return result, nil
	}

	var messageID uuid.UUID
	var teamID uuid.UUID
	if err := tx.QueryRow(ctx, `
		SELECT sms_message_id, team_id
		FROM message_delivery_attempts
		WHERE id = $1 AND channel = 'sms'
	`, result.AttemptID).Scan(&messageID, &teamID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return platformfeedback.Result{}, platformfeedback.ErrAttemptNotFound
		}
		return platformfeedback.Result{}, fmt.Errorf("load SMS message for feedback: %w", err)
	}
	messageStatus, ok := smsMessageStatus(event)
	if ok {
		if _, err := repository.messages.WithTx(tx).ApplyFeedback(
			ctx,
			messageID,
			teamID,
			messageStatus,
			event.ErrorMessage,
			event.OccurredAt,
		); err != nil {
			return platformfeedback.Result{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return platformfeedback.Result{}, fmt.Errorf("commit SMS feedback transaction: %w", err)
	}
	return result, nil
}

func smsMessageStatus(event platformfeedback.Event) (string, bool) {
	switch event.Status {
	case platformdelivery.StatusSubmitted, platformdelivery.StatusAccepted:
		return smsmodule.StatusSubmitted, true
	case platformdelivery.StatusSent:
		return smsmodule.StatusSent, true
	case platformdelivery.StatusDelivered:
		return smsmodule.StatusDelivered, true
	case platformdelivery.StatusPermanentFailure:
		if strings.HasSuffix(event.EventType, "."+smsapi.StatusUndelivered) {
			return smsmodule.StatusUndelivered, true
		}
		return smsmodule.StatusFailed, true
	case platformdelivery.StatusRejected:
		return smsmodule.StatusRejected, true
	case platformdelivery.StatusExpired:
		return smsmodule.StatusExpired, true
	case platformdelivery.StatusUnknown:
		return smsmodule.StatusUnknown, true
	default:
		return "", false
	}
}
