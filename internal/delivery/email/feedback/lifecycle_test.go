package feedback

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	awsses "github.com/coffeyvidzro/dugble/server/internal/integration/aws/ses"
	platformwebhook "github.com/coffeyvidzro/dugble/server/internal/platform/webhook"
)

func TestEmailLifecycleWebhookEvent(t *testing.T) {
	providerEventID := uuid.New()
	messageID := uuid.New()
	teamID := uuid.New()
	occurredAt := time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC)
	recipientDetails := []emailLifecycleRecipient{{
		Email:          "a@example.com",
		Status:         "bounced",
		Action:         "failed",
		StatusCode:     "5.1.1",
		DiagnosticCode: "smtp; 550 user unknown",
	}}

	event, ok, err := emailLifecycleWebhookEvent(
		providerEventID,
		messageID,
		teamID,
		"partially_failed",
		recipientDetails,
		awsses.FeedbackEvent{
			EventType:         "bounce",
			ProviderMessageID: "ses-message-id",
			OccurredAt:        occurredAt,
			Recipients:        []string{" A@Example.com ", "a@example.com", "b@example.com"},
			Diagnostics: awsses.EventDiagnostics{
				BounceType:    "Permanent",
				BounceSubType: "General",
				ReportingMTA:  "dsn; example.com",
			},
		},
	)
	if err != nil {
		t.Fatalf("create email lifecycle webhook: %v", err)
	}
	if !ok {
		t.Fatal("bounce event was not mapped to a webhook")
	}
	if event.ID != uuid.NewSHA1(webhookEventNamespace, []byte(providerEventID.String())) {
		t.Fatalf("event ID = %s, want deterministic provider-event ID", event.ID)
	}
	if event.TeamID != teamID || event.Type != platformwebhook.EventEmailBounced || event.ObjectType != "email" {
		t.Fatalf("unexpected webhook event: %+v", event)
	}
	if event.ObjectID == nil || *event.ObjectID != messageID || !event.OccurredAt.Equal(occurredAt) {
		t.Fatalf("unexpected webhook identity or timestamp: %+v", event)
	}

	var payload emailLifecyclePayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("decode email webhook payload: %v", err)
	}
	if payload.Object != "email" || payload.ID != messageID.String() || payload.Provider != ProviderSES {
		t.Fatalf("unexpected email webhook payload: %+v", payload)
	}
	if payload.Status != "partially_failed" || payload.ProviderEventID != providerEventID.String() {
		t.Fatalf("unexpected status or provider event identity: %+v", payload)
	}
	if payload.ProviderMessageID != "ses-message-id" || payload.LastEvent != "bounce" {
		t.Fatalf("unexpected provider payload fields: %+v", payload)
	}
	if want := []string{"a@example.com", "b@example.com"}; !reflect.DeepEqual(payload.Recipients, want) {
		t.Fatalf("recipients = %v, want %v", payload.Recipients, want)
	}
	if !reflect.DeepEqual(payload.RecipientDetails, recipientDetails) {
		t.Fatalf("recipient details = %#v, want %#v", payload.RecipientDetails, recipientDetails)
	}
	if payload.Diagnostics.BounceType != "Permanent" || payload.Diagnostics.ReportingMTA != "dsn; example.com" {
		t.Fatalf("diagnostics = %#v", payload.Diagnostics)
	}
}

func TestEmailWebhookEventTypes(t *testing.T) {
	tests := map[string]string{
		"send":              platformwebhook.EventEmailSubmitted,
		"delivery":          platformwebhook.EventEmailDelivered,
		"delivery_delay":    platformwebhook.EventEmailDelayed,
		"bounce":            platformwebhook.EventEmailBounced,
		"complaint":         platformwebhook.EventEmailComplained,
		"reject":            platformwebhook.EventEmailRejected,
		"rendering_failure": platformwebhook.EventEmailFailed,
	}
	for providerType, want := range tests {
		got, ok := emailWebhookEventType(providerType)
		if !ok || got != want {
			t.Fatalf("emailWebhookEventType(%q) = %q, %v; want %q, true", providerType, got, ok, want)
		}
	}
	if got, ok := emailWebhookEventType("unknown"); ok || got != "" {
		t.Fatalf("unknown event mapping = %q, %v; want empty, false", got, ok)
	}
}

func TestNormalizedRecipients(t *testing.T) {
	got := normalizedRecipients([]string{" A@Example.com ", "a@example.com", "", "B@example.com"})
	want := []string{"a@example.com", "b@example.com"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalized recipients = %v, want %v", got, want)
	}
}
