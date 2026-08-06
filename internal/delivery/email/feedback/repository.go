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

	awsses "github.com/coffeyvidzro/dugble/server/internal/adapters/amazon/ses"
	awssns "github.com/coffeyvidzro/dugble/server/internal/adapters/amazon/sns"
	"github.com/coffeyvidzro/dugble/server/internal/platform/outbox"
	platformwebhook "github.com/coffeyvidzro/dugble/server/internal/platform/webhook"
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

func (repository *Repository) IngestSNS(ctx context.Context, envelope awssns.Envelope) error {
	return repository.Ingest(ctx, envelope)
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
				FROM message_delivery_attempts
				WHERE id = $1
				  AND email_message_id = $2
				  AND channel = 'email'
			)
		`, attemptID, messageID).Scan(&attemptExists); err != nil {
			return uuid.Nil, "", fmt.Errorf("verify tagged email delivery attempt %s: %w", attemptID, err)
		}
		if !attemptExists {
			return uuid.Nil, "", fmt.Errorf("%w: internal attempt %q", ErrProviderEventUnlinked, attemptID)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE message_delivery_attempts
			SET status = 'submitted',
				provider_message_id = COALESCE(provider_message_id, $2),
				error_code = NULL,
				error_message = NULL,
				request_completed_at = COALESCE(request_completed_at, now()),
				submitted_at = COALESCE(submitted_at, now()), updated_at = now()
			WHERE id = $1 AND channel = 'email'
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

func aggregateTransitionFromCounts(
	counts map[string]int,
	total int,
	fallbackStatus string,
	latestDeliveredAt *time.Time,
	latestFailedAt *time.Time,
) aggregateTransition {
	if total == 0 {
		return aggregateTransition{status: fallbackStatus}
	}

	delivered := counts[recipientStatusDelivered]
	complained := counts[recipientStatusComplained]
	bounced := counts[recipientStatusBounced]
	rejected := counts[recipientStatusRejected]
	failed := counts[recipientStatusFailed]
	terminalFailures := complained + bounced + rejected + failed

	transition := aggregateTransition{deliveredAt: latestDeliveredAt, failedAt: latestFailedAt}
	switch {
	case complained > 0:
		transition.status = "complained"
		transition.errorCode = stringPointer("ses_complaint")
		transition.errorMessage = stringPointer("SES reported a complaint for at least one recipient")
	case delivered == total:
		transition.status = "delivered"
		transition.failedAt = nil
	case delivered > 0:
		transition.status = "partially_delivered"
		if terminalFailures > 0 {
			transition.errorCode = stringPointer("email_partial_delivery")
			transition.errorMessage = stringPointer("The email was delivered to only some recipients")
		}
	case terminalFailures == total:
		switch {
		case bounced == total:
			transition.status = "bounced"
			transition.errorCode = stringPointer("ses_bounce")
			transition.errorMessage = stringPointer("SES reported a bounce for every recipient")
		case rejected == total:
			transition.status = "rejected"
			transition.errorCode = stringPointer("ses_reject")
			transition.errorMessage = stringPointer("SES rejected every recipient")
		case failed == total:
			transition.status = "failed"
			transition.errorCode = stringPointer("ses_rendering_failure")
			transition.errorMessage = stringPointer("SES could not process the email for any recipient")
		default:
			transition.status = "partially_failed"
			transition.errorCode = stringPointer("email_mixed_recipient_failures")
			transition.errorMessage = stringPointer("Recipients ended in different failure states")
		}
	case terminalFailures > 0:
		transition.status = "partially_failed"
		transition.errorCode = stringPointer("email_partial_failure")
		transition.errorMessage = stringPointer("At least one recipient failed while others remain unresolved")
	case counts[recipientStatusDelayed] > 0:
		transition.status = "delayed"
		transition.errorCode = stringPointer("ses_delivery_delay")
		transition.errorMessage = stringPointer("SES reported a delivery delay for at least one recipient")
	case counts[recipientStatusSubmitted] > 0:
		transition.status = "submitted"
	default:
		transition.status = fallbackStatus
	}
	return transition
}

const (
	recipientStatusPending    = "pending"
	recipientStatusSubmitted  = "submitted"
	recipientStatusDelayed    = "delayed"
	recipientStatusDelivered  = "delivered"
	recipientStatusBounced    = "bounced"
	recipientStatusComplained = "complained"
	recipientStatusRejected   = "rejected"
	recipientStatusFailed     = "failed"
)

type recipientTransition struct {
	status       string
	errorCode    *string
	errorMessage *string
	deliveredAt  *time.Time
	failedAt     *time.Time
}

type aggregateTransition struct {
	status       string
	errorCode    *string
	errorMessage *string
	deliveredAt  *time.Time
	failedAt     *time.Time
}

func applyRecipientCurrentState(ctx context.Context, tx pgx.Tx, messageID uuid.UUID, event awsses.FeedbackEvent) error {
	for _, recipientEmail := range normalizedRecipients(event.Recipients) {
		var currentStatus string
		var lastEventAt *time.Time
		err := tx.QueryRow(ctx, `
			SELECT status, last_event_at
			FROM email_recipients
			WHERE email_message_id = $1 AND recipient_email = $2
			FOR UPDATE
		`, messageID, recipientEmail).Scan(&currentStatus, &lastEventAt)
		if errors.Is(err, pgx.ErrNoRows) {
			if _, err := tx.Exec(ctx, `
				INSERT INTO email_recipients (id, email_message_id, recipient_email, recipient_type, status)
				VALUES ($1, $2, $3, 'unknown', 'pending')
				ON CONFLICT (email_message_id, recipient_email) DO NOTHING
			`, uuid.New(), messageID, recipientEmail); err != nil {
				return fmt.Errorf("create current state for recipient %q: %w", recipientEmail, err)
			}
			currentStatus = recipientStatusPending
			lastEventAt = nil
		} else if err != nil {
			return fmt.Errorf("lock current state for recipient %q: %w", recipientEmail, err)
		}
		occurredAt := event.OccurredAt.UTC()
		if lastEventAt != nil && occurredAt.Before(lastEventAt.UTC()) {
			continue
		}
		transition, apply, err := recipientStatusTransition(currentStatus, event.EventType, occurredAt)
		if err != nil {
			return err
		}
		if !apply {
			continue
		}
		diagnostic := recipientDiagnostic(event.RecipientDiagnostics, recipientEmail)
		if _, err := tx.Exec(ctx, `
			UPDATE email_recipients
			SET status = $3,
				last_event_type = $4,
				last_event_at = $5,
				last_action = $6,
				last_status_code = $7,
				last_diagnostic_code = $8,
				delivered_at = COALESCE($9, delivered_at),
				failed_at = COALESCE($10, failed_at),
				error_code = $11,
				error_message = $12,
				updated_at = now()
			WHERE email_message_id = $1 AND recipient_email = $2
		`, messageID, recipientEmail, transition.status, event.EventType, occurredAt,
			nullableString(diagnostic.Action), nullableString(diagnostic.StatusCode), nullableString(diagnostic.DiagnosticCode),
			transition.deliveredAt, transition.failedAt, transition.errorCode, transition.errorMessage); err != nil {
			return fmt.Errorf("apply %s state to recipient %q: %w", event.EventType, recipientEmail, err)
		}
	}
	return nil
}

func recipientDiagnostic(values []awsses.RecipientDiagnostics, recipientEmail string) awsses.RecipientDiagnostics {
	recipientEmail = strings.ToLower(strings.TrimSpace(recipientEmail))
	for _, value := range values {
		if strings.ToLower(strings.TrimSpace(value.Email)) == recipientEmail {
			return value
		}
	}
	return awsses.RecipientDiagnostics{}
}

func nullableString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func recipientStatusTransition(currentStatus, eventType string, occurredAt time.Time) (recipientTransition, bool, error) {
	currentStatus = strings.TrimSpace(currentStatus)
	eventType = strings.TrimSpace(eventType)
	occurredAt = occurredAt.UTC()
	transition := recipientTransition{}
	providerError := func(code, message string) {
		transition.errorCode = &code
		transition.errorMessage = &message
		transition.failedAt = &occurredAt
	}
	switch eventType {
	case "send":
		if !recipientStatusIn(currentStatus, recipientStatusPending, recipientStatusSubmitted) {
			return recipientTransition{}, false, nil
		}
		transition.status = recipientStatusSubmitted
	case "delivery_delay":
		if !recipientStatusIn(currentStatus, recipientStatusPending, recipientStatusSubmitted, recipientStatusDelayed) {
			return recipientTransition{}, false, nil
		}
		transition.status = recipientStatusDelayed
		transition.errorCode = stringPointer("ses_delivery_delay")
		transition.errorMessage = stringPointer("SES reported a delivery delay")
	case "delivery":
		if !recipientStatusIn(currentStatus, recipientStatusPending, recipientStatusSubmitted, recipientStatusDelayed, recipientStatusDelivered) {
			return recipientTransition{}, false, nil
		}
		transition.status = recipientStatusDelivered
		transition.deliveredAt = &occurredAt
	case "bounce":
		if !recipientStatusIn(currentStatus, recipientStatusPending, recipientStatusSubmitted, recipientStatusDelayed, recipientStatusDelivered, recipientStatusBounced) {
			return recipientTransition{}, false, nil
		}
		transition.status = recipientStatusBounced
		providerError("ses_bounce", "SES reported a bounce")
	case "complaint":
		if currentStatus == recipientStatusComplained {
			transition.status = recipientStatusComplained
			providerError("ses_complaint", "SES reported a complaint")
			return transition, true, nil
		}
		if !recipientStatusIn(currentStatus, recipientStatusPending, recipientStatusSubmitted, recipientStatusDelayed, recipientStatusDelivered, recipientStatusBounced) {
			return recipientTransition{}, false, nil
		}
		transition.status = recipientStatusComplained
		providerError("ses_complaint", "SES reported a complaint")
	case "reject":
		if !recipientStatusIn(currentStatus, recipientStatusPending, recipientStatusSubmitted, recipientStatusDelayed, recipientStatusRejected) {
			return recipientTransition{}, false, nil
		}
		transition.status = recipientStatusRejected
		providerError("ses_reject", "SES rejected the message")
	case "rendering_failure":
		if !recipientStatusIn(currentStatus, recipientStatusPending, recipientStatusSubmitted, recipientStatusDelayed, recipientStatusFailed) {
			return recipientTransition{}, false, nil
		}
		transition.status = recipientStatusFailed
		providerError("ses_rendering_failure", "SES could not render the message")
	case "open", "click", "subscription":
		return recipientTransition{}, false, nil
	default:
		return recipientTransition{}, false, fmt.Errorf("unsupported persisted SES event type %q", eventType)
	}
	return transition, true, nil
}

func aggregateRecipientMessageStatus(ctx context.Context, tx pgx.Tx, messageID uuid.UUID, fallbackStatus string) (aggregateTransition, error) {
	rows, err := tx.Query(ctx, `
		SELECT status, delivered_at, failed_at
		FROM email_recipients
		WHERE email_message_id = $1
		FOR SHARE
	`, messageID)
	if err != nil {
		return aggregateTransition{}, fmt.Errorf("load recipient states for email %s: %w", messageID, err)
	}
	defer rows.Close()
	counts := map[string]int{}
	var total int
	var latestDeliveredAt *time.Time
	var latestFailedAt *time.Time
	for rows.Next() {
		var status string
		var deliveredAt, failedAt *time.Time
		if err := rows.Scan(&status, &deliveredAt, &failedAt); err != nil {
			return aggregateTransition{}, fmt.Errorf("scan recipient state for email %s: %w", messageID, err)
		}
		counts[status]++
		total++
		latestDeliveredAt = laterTime(latestDeliveredAt, deliveredAt)
		latestFailedAt = laterTime(latestFailedAt, failedAt)
	}
	if err := rows.Err(); err != nil {
		return aggregateTransition{}, fmt.Errorf("iterate recipient states for email %s: %w", messageID, err)
	}
	return aggregateTransitionFromCounts(counts, total, fallbackStatus, latestDeliveredAt, latestFailedAt), nil
}

func recipientStatusIn(current string, allowed ...string) bool {
	for _, status := range allowed {
		if current == status {
			return true
		}
	}
	return false
}

func stringPointer(value string) *string { return &value }

func laterTime(current, candidate *time.Time) *time.Time {
	if candidate == nil {
		return current
	}
	candidateUTC := candidate.UTC()
	if current == nil || candidateUTC.After(current.UTC()) {
		return &candidateUTC
	}
	return current
}

var webhookEventNamespace = uuid.MustParse("d90f621c-937d-5fd2-9c85-cd8f55cacaa2")

type webhookEmitter interface {
	EmitTx(context.Context, pgx.Tx, platformwebhook.Event) (uuid.UUID, int64, error)
}

type emailLifecycleRecipient struct {
	Email          string `json:"email"`
	Status         string `json:"status"`
	Action         string `json:"action,omitempty"`
	StatusCode     string `json:"status_code,omitempty"`
	DiagnosticCode string `json:"diagnostic_code,omitempty"`
}

type emailLifecyclePayload struct {
	Object            string                    `json:"object"`
	ID                string                    `json:"id"`
	Status            string                    `json:"status"`
	Provider          string                    `json:"provider"`
	ProviderEventID   string                    `json:"provider_event_id"`
	ProviderMessageID string                    `json:"provider_message_id"`
	LastEvent         string                    `json:"last_event"`
	Recipients        []string                  `json:"recipients"`
	RecipientDetails  []emailLifecycleRecipient `json:"recipient_details,omitempty"`
	Diagnostics       awsses.EventDiagnostics   `json:"diagnostics,omitempty"`
}

func NewRepositoryWithWebhookEmitter(db *pgxpool.Pool, emitter webhookEmitter) *Repository {
	repository := NewRepository(db, nil)
	repository.emitter = emitter
	return repository
}

func (r *Repository) emitLifecycleWebhook(ctx context.Context, tx pgx.Tx, providerEventID, messageID, teamID uuid.UUID, event awsses.FeedbackEvent) error {
	if r == nil || r.emitter == nil {
		return nil
	}
	var messageStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM email_messages WHERE id = $1`, messageID).Scan(&messageStatus); err != nil {
		return fmt.Errorf("load email status for lifecycle webhook %s: %w", messageID, err)
	}
	recipientDetails, err := loadLifecycleRecipients(ctx, tx, messageID, event.Recipients)
	if err != nil {
		return err
	}
	webhookEvent, ok, err := emailLifecycleWebhookEvent(providerEventID, messageID, teamID, messageStatus, recipientDetails, event)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if _, _, err := r.emitter.EmitTx(ctx, tx, webhookEvent); err != nil {
		return fmt.Errorf("emit %s email lifecycle webhook: %w", event.EventType, err)
	}
	return nil
}

func loadLifecycleRecipients(ctx context.Context, tx pgx.Tx, messageID uuid.UUID, recipients []string) ([]emailLifecycleRecipient, error) {
	normalized := normalizedRecipients(recipients)
	if len(normalized) == 0 {
		return nil, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT recipient_email, status, COALESCE(last_action, ''), COALESCE(last_status_code, ''), COALESCE(last_diagnostic_code, '')
		FROM email_recipients
		WHERE email_message_id = $1 AND recipient_email = ANY($2::text[])
		ORDER BY recipient_email
	`, messageID, normalized)
	if err != nil {
		return nil, fmt.Errorf("load recipient diagnostics for email %s: %w", messageID, err)
	}
	defer rows.Close()
	result := make([]emailLifecycleRecipient, 0, len(normalized))
	for rows.Next() {
		var recipient emailLifecycleRecipient
		if err := rows.Scan(&recipient.Email, &recipient.Status, &recipient.Action, &recipient.StatusCode, &recipient.DiagnosticCode); err != nil {
			return nil, fmt.Errorf("scan recipient diagnostics for email %s: %w", messageID, err)
		}
		result = append(result, recipient)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recipient diagnostics for email %s: %w", messageID, err)
	}
	return result, nil
}

func emailLifecycleWebhookEvent(providerEventID, messageID, teamID uuid.UUID, messageStatus string, recipientDetails []emailLifecycleRecipient, event awsses.FeedbackEvent) (platformwebhook.Event, bool, error) {
	eventType, ok := emailWebhookEventType(event.EventType)
	if !ok {
		return platformwebhook.Event{}, false, nil
	}
	payload, err := json.Marshal(emailLifecyclePayload{
		Object: "email", ID: messageID.String(), Status: strings.TrimSpace(messageStatus), Provider: ProviderSES,
		ProviderEventID: providerEventID.String(), ProviderMessageID: strings.TrimSpace(event.ProviderMessageID),
		LastEvent: strings.TrimSpace(event.EventType), Recipients: normalizedRecipients(event.Recipients),
		RecipientDetails: recipientDetails, Diagnostics: event.Diagnostics,
	})
	if err != nil {
		return platformwebhook.Event{}, false, fmt.Errorf("encode email lifecycle webhook payload: %w", err)
	}
	return platformwebhook.Event{
		ID: uuid.NewSHA1(webhookEventNamespace, []byte(providerEventID.String())), TeamID: teamID, Type: eventType,
		ObjectType: "email", ObjectID: &messageID, Payload: payload, OccurredAt: event.OccurredAt,
	}, true, nil
}

func emailWebhookEventType(eventType string) (string, bool) {
	switch strings.TrimSpace(eventType) {
	case "send":
		return platformwebhook.EventEmailSubmitted, true
	case "delivery":
		return platformwebhook.EventEmailDelivered, true
	case "delivery_delay":
		return platformwebhook.EventEmailDelayed, true
	case "bounce":
		return platformwebhook.EventEmailBounced, true
	case "complaint":
		return platformwebhook.EventEmailComplained, true
	case "reject":
		return platformwebhook.EventEmailRejected, true
	case "rendering_failure":
		return platformwebhook.EventEmailFailed, true
	case "open":
		return platformwebhook.EventEmailOpened, true
	case "click":
		return platformwebhook.EventEmailClicked, true
	case "subscription":
		return platformwebhook.EventEmailSubscriptionChanged, true
	default:
		return "", false
	}
}

func normalizedRecipients(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
