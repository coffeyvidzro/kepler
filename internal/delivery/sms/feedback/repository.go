package feedback

import (
	"context"
	"errors"
	"fmt"
	attempt "github.com/coffeyvidzro/dugble/server/internal/delivery/attempt"
	feedback "github.com/coffeyvidzro/dugble/server/internal/delivery/feedback"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	smsmodule "github.com/coffeyvidzro/dugble/server/internal/modules/sms"
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
	event feedback.Event,
) (feedback.Result, error) {
	if repository == nil || repository.db == nil || repository.messages == nil {
		return feedback.Result{}, ErrRepositoryNotConfigured
	}
	tx, err := repository.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return feedback.Result{}, fmt.Errorf("begin SMS feedback transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	processor, err := feedback.NewProcessor(feedback.NewSQLRepository(tx))
	if err != nil {
		return feedback.Result{}, err
	}
	result, err := processor.Process(ctx, event)
	if err != nil {
		return feedback.Result{}, err
	}
	if result.Duplicate || !result.Transitioned {
		if err := tx.Commit(ctx); err != nil {
			return feedback.Result{}, fmt.Errorf("commit SMS feedback observation: %w", err)
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
			return feedback.Result{}, feedback.ErrAttemptNotFound
		}
		return feedback.Result{}, fmt.Errorf("load SMS message for feedback: %w", err)
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
			return feedback.Result{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return feedback.Result{}, fmt.Errorf("commit SMS feedback transaction: %w", err)
	}
	return result, nil
}

func smsMessageStatus(event feedback.Event) (string, bool) {
	switch event.Status {
	case attempt.StatusSubmitted, attempt.StatusAccepted:
		return smsmodule.StatusSubmitted, true
	case attempt.StatusSent:
		return smsmodule.StatusSent, true
	case attempt.StatusDelivered:
		return smsmodule.StatusDelivered, true
	case attempt.StatusPermanentFailure:
		if strings.HasSuffix(event.EventType, "."+smsapi.StatusUndelivered) {
			return smsmodule.StatusUndelivered, true
		}
		return smsmodule.StatusFailed, true
	case attempt.StatusRejected:
		return smsmodule.StatusRejected, true
	case attempt.StatusExpired:
		return smsmodule.StatusExpired, true
	case attempt.StatusUnknown:
		return smsmodule.StatusUnknown, true
	default:
		return "", false
	}
}
