package feedback

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	awsses "github.com/coffeyvidzro/dugble/server/internal/integration/aws/ses"
	platformwebhook "github.com/coffeyvidzro/dugble/server/internal/platform/webhook"
)

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
