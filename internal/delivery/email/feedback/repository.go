package feedback

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	awsses "github.com/coffeyvidzro/dugble/server/internal/integration/aws/ses"
	awssns "github.com/coffeyvidzro/dugble/server/internal/integration/aws/sns"
	"github.com/coffeyvidzro/dugble/server/internal/messaging/outbox"
)

var ErrProviderEventUnlinked = errors.New("email provider event is not linked to a message")

type Repository struct {
	db      *pgxpool.Pool
	outbox  *outbox.Repository
	emitter webhookEmitter
	now     func() time.Time
}

func NewRepository(db *pgxpool.Pool, outboxRepository *outbox.Repository) *Repository {
	return &Repository{db: db, outbox: outboxRepository, now: time.Now}
}

func (r *Repository) Ingest(ctx context.Context, envelope awssns.Envelope) error {
	if r == nil || r.db == nil || r.outbox == nil {
		return errors.New("email feedback repository is not configured")
	}

	providerEvent, err := awsses.ParseFeedbackEvent(envelope.Message)
	if err != nil {
		return err
	}
	normalizedPayload, err := json.Marshal(providerEvent)
	if err != nil {
		return fmt.Errorf("encode normalized SES event: %w", err)
	}

	eventID := uuid.NewSHA1(eventNamespace, []byte(envelope.TopicARN+":"+envelope.MessageID))
	receivedAt := r.currentTime().UTC()

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin email provider event transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	commandTag, err := tx.Exec(ctx, `
		INSERT INTO email_provider_events (
			id, email_message_id, provider, transport, provider_notification_id,
			provider_message_id, event_type, occurred_at, received_at,
			normalized_payload, provider_payload, next_attempt_at
		)
		VALUES (
			$1,
			(
				SELECT id
				FROM email_messages
				WHERE provider = $2
				  AND provider_message_id = $3
			),
			$2, $4, $5, $3, $6, $7, $8, $9, $10, $8
		)
		ON CONFLICT (provider, transport, provider_notification_id) DO NOTHING
	`,
		eventID,
		ProviderSES,
		providerEvent.ProviderMessageID,
		TransportSNS,
		envelope.MessageID,
		providerEvent.EventType,
		providerEvent.OccurredAt,
		receivedAt,
		normalizedPayload,
		providerEvent.Payload,
	)
	if err != nil {
		return fmt.Errorf("insert email provider event: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		return tx.Commit(ctx)
	}

	outboxPayload, err := encodeProviderEventReference(eventID)
	if err != nil {
		return fmt.Errorf("encode email provider event reference: %w", err)
	}
	if _, err := r.outbox.EnqueueTx(ctx, tx, outbox.Event{
		ID:            eventID,
		Subject:       ProviderEventTopic,
		AggregateType: "email_provider_event",
		AggregateID:   eventID,
		Payload:       outboxPayload,
		Headers: map[string]string{
			"Dugble-Event-Id":            eventID.String(),
			"Dugble-Provider":            ProviderSES,
			"Dugble-Transport":           TransportSNS,
			"AWS-SNS-Message-Id":         envelope.MessageID,
			"AWS-SNS-Topic-Arn":          envelope.TopicARN,
			"Dugble-Provider-Event-Type": providerEvent.EventType,
		},
		AvailableAt: receivedAt,
	}); err != nil {
		return fmt.Errorf("enqueue email provider event: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit email provider event transaction: %w", err)
	}
	return nil
}

// Process claims one event for the initial JetStream delivery. Once claimed,
// PostgreSQL owns all retries; successfully persisting a reschedule is treated
// as successful handling so JetStream can acknowledge the wake-up message.
func (r *Repository) Process(ctx context.Context, eventID uuid.UUID) error {
	if r == nil || r.db == nil {
		return errors.New("email feedback repository is not configured")
	}
	if eventID == uuid.Nil {
		return errors.New("email provider event ID is required")
	}
	claim, claimed, err := r.claimSpecific(ctx, eventID, 2*time.Minute)
	if err != nil || !claimed {
		return err
	}
	if err := r.processClaimed(ctx, claim); err != nil {
		if recordErr := r.RecordReconcileFailure(ctx, claim, err); recordErr != nil {
			return fmt.Errorf("process email provider event %s: %v; persist retry: %w", eventID, err, recordErr)
		}
	}
	return nil
}

func (r *Repository) processClaimed(ctx context.Context, claim ReconcileClaim) error {
	if claim.EventID == uuid.Nil {
		return errors.New("email provider event ID is required")
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin email feedback transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var emailMessageID *uuid.UUID
	var providerMessageID string
	var eventType string
	var occurredAt time.Time
	var normalizedPayload []byte
	var processedAt, deadLetteredAt pgtype.Timestamptz
	if err := tx.QueryRow(ctx, `
		SELECT email_message_id, provider_message_id, event_type, occurred_at,
			normalized_payload, processed_at, dead_lettered_at
		FROM email_provider_events
		WHERE id = $1
		  AND provider = $2
		  AND transport = $3
		FOR UPDATE
	`, claim.EventID, ProviderSES, TransportSNS).Scan(
		&emailMessageID,
		&providerMessageID,
		&eventType,
		&occurredAt,
		&normalizedPayload,
		&processedAt,
		&deadLetteredAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("email provider event %s not found", claim.EventID)
		}
		return fmt.Errorf("load email provider event %s: %w", claim.EventID, err)
	}
	if processedAt.Valid || deadLetteredAt.Valid {
		return tx.Commit(ctx)
	}

	providerEvent := awsses.FeedbackEvent{
		EventType:         eventType,
		ProviderMessageID: providerMessageID,
		OccurredAt:        occurredAt,
	}
	if err := json.Unmarshal(normalizedPayload, &providerEvent); err != nil {
		return fmt.Errorf("decode normalized email provider event %s: %w", claim.EventID, err)
	}
	providerEvent.EventType = eventType
	providerEvent.ProviderMessageID = providerMessageID
	providerEvent.OccurredAt = occurredAt

	messageID, currentStatus, err := linkAndLockMessage(ctx, tx, claim.EventID, emailMessageID, providerEvent)
	if err != nil {
		if errors.Is(err, ErrProviderEventUnlinked) {
			if err := scheduleClaimTx(ctx, tx, claim, err); err != nil {
				return err
			}
			return tx.Commit(ctx)
		}
		return err
	}
	var teamID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT team_id FROM email_messages WHERE id = $1`, messageID).Scan(&teamID); err != nil {
		return fmt.Errorf("load team for email message %s: %w", messageID, err)
	}
	if err := applyRecipientCurrentState(ctx, tx, messageID, providerEvent); err != nil {
		return err
	}

	aggregate, err := aggregateRecipientMessageStatus(ctx, tx, messageID, currentStatus)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE email_messages
		SET status = $2,
			delivered_at = $3,
			failed_at = $4,
			error_code = $5,
			error_message = $6,
			updated_at = now()
		WHERE id = $1
	`,
		messageID,
		aggregate.status,
		aggregate.deliveredAt,
		aggregate.failedAt,
		aggregate.errorCode,
		aggregate.errorMessage,
	); err != nil {
		return fmt.Errorf("apply recipient aggregate status to email %s: %w", messageID, err)
	}

	if err := r.emitLifecycleWebhook(ctx, tx, claim.EventID, messageID, teamID, providerEvent); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE email_provider_events
		SET processed_at = COALESCE(processed_at, now()),
			next_attempt_at = NULL,
			last_error = NULL
		WHERE id = $1
	`, claim.EventID); err != nil {
		return fmt.Errorf("mark email provider event %s processed: %w", claim.EventID, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit email feedback transaction: %w", err)
	}
	return nil
}

func linkAndLockMessage(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	emailMessageID *uuid.UUID,
	providerEvent awsses.FeedbackEvent,
) (uuid.UUID, string, error) {
	var messageID uuid.UUID
	var currentStatus string

	if emailMessageID != nil {
		err := tx.QueryRow(ctx, `
			SELECT id, status
			FROM email_messages
			WHERE id = $1
			FOR UPDATE
		`, *emailMessageID).Scan(&messageID, &currentStatus)
		if err == nil {
			return messageID, currentStatus, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, "", fmt.Errorf("lock email message %s: %w", *emailMessageID, err)
		}
	}

	providerMessageID := strings.TrimSpace(providerEvent.ProviderMessageID)
	err := tx.QueryRow(ctx, `
		SELECT id, status
		FROM email_messages
		WHERE provider = $1
		  AND provider_message_id = $2
		FOR UPDATE
	`, ProviderSES, providerMessageID).Scan(&messageID, &currentStatus)
	if err == nil {
		if err := linkProviderEvent(ctx, tx, eventID, messageID); err != nil {
			return uuid.Nil, "", err
		}
		return messageID, currentStatus, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, "", fmt.Errorf("find email by provider message %q: %w", providerMessageID, err)
	}

	internalMessageID, parseErr := uuid.Parse(strings.TrimSpace(providerEvent.InternalMessageID))
	if parseErr != nil {
		return uuid.Nil, "", fmt.Errorf("%w: provider message %q", ErrProviderEventUnlinked, providerMessageID)
	}
	if err := tx.QueryRow(ctx, `
		SELECT id, status
		FROM email_messages
		WHERE id = $1
		FOR UPDATE
	`, internalMessageID).Scan(&messageID, &currentStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, "", fmt.Errorf("%w: internal message %q", ErrProviderEventUnlinked, internalMessageID)
		}
		return uuid.Nil, "", fmt.Errorf("lock internally tagged email message %s: %w", internalMessageID, err)
	}

	attemptID, attemptErr := uuid.Parse(strings.TrimSpace(providerEvent.InternalAttemptID))
	if attemptErr == nil {
		var attemptExists bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM email_delivery_attempts
				WHERE id = $1
				  AND email_message_id = $2
			)
		`, attemptID, messageID).Scan(&attemptExists); err != nil {
			return uuid.Nil, "", fmt.Errorf("verify tagged email delivery attempt %s: %w", attemptID, err)
		}
		if !attemptExists {
			return uuid.Nil, "", fmt.Errorf("%w: internal attempt %q", ErrProviderEventUnlinked, attemptID)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE email_delivery_attempts
			SET status = 'submitted',
				provider_message_id = COALESCE(provider_message_id, $2),
				error_code = NULL,
				error_message = NULL,
				completed_at = COALESCE(completed_at, now()),
				updated_at = now()
			WHERE id = $1
		`, attemptID, providerMessageID); err != nil {
			return uuid.Nil, "", fmt.Errorf("reconcile tagged email delivery attempt %s: %w", attemptID, err)
		}
	}

	if err := tx.QueryRow(ctx, `
		UPDATE email_messages
		SET provider = COALESCE(provider, $2),
			provider_message_id = COALESCE(provider_message_id, $3),
			status = CASE WHEN status = 'submission_unknown' THEN 'submitted' ELSE status END,
			submitted_at = COALESCE(submitted_at, now()),
			error_code = CASE WHEN status = 'submission_unknown' THEN NULL ELSE error_code END,
			error_message = CASE WHEN status = 'submission_unknown' THEN NULL ELSE error_message END,
			updated_at = now()
		WHERE id = $1
		RETURNING status
	`, messageID, ProviderSES, providerMessageID).Scan(&currentStatus); err != nil {
		return uuid.Nil, "", fmt.Errorf("reconcile internally tagged email message %s: %w", messageID, err)
	}
	if err := linkProviderEvent(ctx, tx, eventID, messageID); err != nil {
		return uuid.Nil, "", err
	}
	return messageID, currentStatus, nil
}

func linkProviderEvent(ctx context.Context, tx pgx.Tx, eventID, messageID uuid.UUID) error {
	if _, err := tx.Exec(ctx, `
		UPDATE email_provider_events
		SET email_message_id = $2
		WHERE id = $1
		  AND email_message_id IS NULL
	`, eventID, messageID); err != nil {
		return fmt.Errorf("link email provider event %s: %w", eventID, err)
	}
	return nil
}

func scheduleClaimTx(ctx context.Context, tx pgx.Tx, claim ReconcileClaim, cause error) error {
	reason := truncateReconciliationError(cause)
	if claim.AttemptCount >= defaultReconciliationMaxAttempts {
		if _, err := tx.Exec(ctx, `
			UPDATE email_provider_events
			SET dead_lettered_at = COALESCE(dead_lettered_at, now()),
				next_attempt_at = NULL,
				last_error = $2
			WHERE id = $1
		`, claim.EventID, reason); err != nil {
			return fmt.Errorf("dead-letter unlinked email provider event %s: %w", claim.EventID, err)
		}
		return nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE email_provider_events
		SET next_attempt_at = now() + $2::interval,
			last_error = $3
		WHERE id = $1
	`, claim.EventID, reconciliationDelay(claim.AttemptCount).String(), reason); err != nil {
		return fmt.Errorf("reschedule unlinked email provider event %s: %w", claim.EventID, err)
	}
	return nil
}

func (r *Repository) currentTime() time.Time {
	if r != nil && r.now != nil {
		return r.now()
	}
	return time.Now()
}
