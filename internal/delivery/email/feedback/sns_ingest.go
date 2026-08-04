package feedback

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	awsses "github.com/coffeyvidzro/dugble/server/internal/adapters/amazon/ses"
	awssns "github.com/coffeyvidzro/dugble/server/internal/adapters/amazon/sns"
	"github.com/coffeyvidzro/dugble/server/internal/messaging/outbox"
)

// IngestSNS persists a verified SNS envelope using only the refactored AWS adapters.
func (repository *Repository) IngestSNS(ctx context.Context, envelope awssns.Envelope) error {
	if repository == nil || repository.db == nil || repository.outbox == nil {
		return fmt.Errorf("email feedback repository is not configured")
	}
	providerEvent, err := awsses.ParseFeedbackEvent(envelope.Message)
	if err != nil { return err }
	normalizedPayload, err := json.Marshal(providerEvent)
	if err != nil { return fmt.Errorf("encode normalized SES event: %w", err) }
	eventID := uuid.NewSHA1(eventNamespace, []byte(envelope.TopicARN+":"+envelope.MessageID))
	receivedAt := repository.currentTime().UTC()
	tx, err := repository.db.Begin(ctx)
	if err != nil { return fmt.Errorf("begin email provider event transaction: %w", err) }
	defer func(){ _ = tx.Rollback(ctx) }()
	commandTag, err := tx.Exec(ctx, `
		INSERT INTO email_provider_events (
			id, email_message_id, provider, transport, provider_notification_id,
			provider_message_id, event_type, occurred_at, received_at,
			normalized_payload, provider_payload, next_attempt_at
		)
		VALUES (
			$1,
			(SELECT id FROM email_messages WHERE provider = $2 AND provider_message_id = $3),
			$2, $4, $5, $3, $6, $7, $8, $9, $10, $8
		)
		ON CONFLICT (provider, transport, provider_notification_id) DO NOTHING
	`, eventID, ProviderSES, providerEvent.ProviderMessageID, TransportSNS, envelope.MessageID, providerEvent.EventType, providerEvent.OccurredAt, receivedAt, normalizedPayload, providerEvent.Payload)
	if err != nil { return fmt.Errorf("insert email provider event: %w", err) }
	if commandTag.RowsAffected() == 0 { return tx.Commit(ctx) }
	outboxPayload, err := encodeProviderEventReference(eventID)
	if err != nil { return fmt.Errorf("encode email provider event reference: %w", err) }
	if _, err := repository.outbox.EnqueueTx(ctx, tx, outbox.Event{ID:eventID,Subject:ProviderEventTopic,AggregateType:"email_provider_event",AggregateID:eventID,Payload:outboxPayload,Headers:map[string]string{"Dugble-Event-Id":eventID.String(),"Dugble-Provider":ProviderSES,"Dugble-Transport":TransportSNS,"AWS-SNS-Message-Id":envelope.MessageID,"AWS-SNS-Topic-Arn":envelope.TopicARN,"Dugble-Provider-Event-Type":providerEvent.EventType},AvailableAt:receivedAt}); err != nil { return fmt.Errorf("enqueue email provider event: %w", err) }
	if err := tx.Commit(ctx); err != nil { return fmt.Errorf("commit email feedback transaction: %w", err) }
	return nil
}

var _ = time.Time{}
