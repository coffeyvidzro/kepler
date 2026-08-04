package event

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestEnvelopeNormalizeAppliesDefaults(t *testing.T) {
	teamID := uuid.New()
	objectID := uuid.New()
	now := time.Date(2026, time.August, 4, 12, 30, 0, 0, time.FixedZone("test", 2*60*60))

	normalized, err := (Envelope{
		Type:       Type(" sms.sent "),
		TeamID:     teamID,
		ObjectType: " sms ",
		ObjectID:   &objectID,
		Data:       json.RawMessage(`{"status":"sent"}`),
	}).Normalize(func() time.Time { return now })
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if normalized.ID == uuid.Nil {
		t.Fatal("Normalize() did not create an event id")
	}
	if normalized.Version != CurrentVersion {
		t.Fatalf("Normalize().Version = %q, want %q", normalized.Version, CurrentVersion)
	}
	if normalized.Type != TypeSMSSent || normalized.ObjectType != "sms" {
		t.Fatalf("Normalize() type/object = %q/%q", normalized.Type, normalized.ObjectType)
	}
	if !normalized.OccurredAt.Equal(now.UTC()) || normalized.OccurredAt.Location() != time.UTC {
		t.Fatalf("Normalize().OccurredAt = %v, want %v", normalized.OccurredAt, now.UTC())
	}
}

func TestEnvelopeValidateRejectsMismatchedObjectType(t *testing.T) {
	objectID := uuid.New()
	envelope := validEnvelope(TypeEmailDelivered, "sms", &objectID)
	if err := envelope.Validate(); err == nil {
		t.Fatal("Validate() accepted a mismatched object type")
	}
}

func TestEnvelopeValidateRequiresProductObjectID(t *testing.T) {
	envelope := validEnvelope(TypeVerificationCreated, "verification", nil)
	if err := envelope.Validate(); err == nil {
		t.Fatal("Validate() accepted a verification event without an object id")
	}
}

func TestEnvelopeValidateRequiresJSONObjectData(t *testing.T) {
	objectID := uuid.New()
	envelope := validEnvelope(TypeSMSDelivered, "sms", &objectID)
	envelope.Data = json.RawMessage(`[]`)
	if err := envelope.Validate(); err == nil {
		t.Fatal("Validate() accepted non-object data")
	}
}

func validEnvelope(eventType Type, objectType string, objectID *uuid.UUID) Envelope {
	return Envelope{
		ID:         uuid.New(),
		Type:       eventType,
		Version:    CurrentVersion,
		TeamID:     uuid.New(),
		ObjectType: objectType,
		ObjectID:   objectID,
		Data:       json.RawMessage(`{}`),
		OccurredAt: time.Now().UTC(),
	}
}
