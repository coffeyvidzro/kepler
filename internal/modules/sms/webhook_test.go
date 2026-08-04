package sms

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	platformwebhook "github.com/coffeyvidzro/dugble/server/internal/platform/webhook"
)

func TestSMSLifecycleEventBuildsPublicPayload(t *testing.T) {
	messageID, teamID := uuid.New(), uuid.New()
	occurredAt := time.Date(2026, time.July, 27, 12, 0, 0, 0, time.UTC)
	message := Message{
		ID: messageID.String(), TeamID: teamID.String(), To: "+233241234567", From: "DUGBLE",
		Body: "hello", Status: StatusDelivered, Metadata: json.RawMessage(`{"order_id":"123"}`),
		UpdatedAt: occurredAt,
	}

	event, ok, err := smsLifecycleEvent(message)
	if err != nil {
		t.Fatalf("smsLifecycleEvent() error = %v", err)
	}
	if !ok {
		t.Fatal("smsLifecycleEvent() did not create a delivered event")
	}
	if event.TeamID != teamID || event.Type != platformwebhook.EventSMSDelivered || event.ObjectType != "sms" || event.ObjectID == nil || *event.ObjectID != messageID {
		t.Fatalf("smsLifecycleEvent() identifiers = %+v", event)
	}
	if !event.OccurredAt.Equal(occurredAt) {
		t.Fatalf("smsLifecycleEvent() occurred at = %v, want %v", event.OccurredAt, occurredAt)
	}
	var payload SMSResponse
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("decode SMS webhook payload: %v", err)
	}
	if payload.Object != "sms" || payload.ID != messageID.String() || payload.Status != StatusDelivered || payload.To != message.To {
		t.Fatalf("SMS webhook payload = %+v", payload)
	}
}

func TestSMSLifecycleEventSupportsPublishedStatuses(t *testing.T) {
	tests := map[string]string{
		StatusSubmitted:   platformwebhook.EventSMSSubmitted,
		StatusSent:        platformwebhook.EventSMSSent,
		StatusDelivered:   platformwebhook.EventSMSDelivered,
		StatusUndelivered: platformwebhook.EventSMSUndelivered,
		StatusFailed:      platformwebhook.EventSMSFailed,
	}
	for status, wantType := range tests {
		t.Run(status, func(t *testing.T) {
			event, ok, err := smsLifecycleEvent(Message{ID: uuid.NewString(), TeamID: uuid.NewString(), Status: status})
			if err != nil || !ok || event.Type != wantType {
				t.Fatalf("smsLifecycleEvent() = (%q, %t, %v), want (%q, true, nil)", event.Type, ok, err, wantType)
			}
		})
	}

	if _, ok, err := smsLifecycleEvent(Message{Status: StatusProcessing}); err != nil || ok {
		t.Fatalf("processing smsLifecycleEvent() = (_, %t, %v), want (_, false, nil)", ok, err)
	}
}
