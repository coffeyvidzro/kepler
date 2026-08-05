package webhook

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	platformevent "github.com/coffeyvidzro/dugble/server/internal/platform/event"
)

type emitterTestStore struct{}

func (emitterTestStore) CreateEventTx(context.Context, pgx.Tx, Event) (uuid.UUID, error) {
	return uuid.Nil, nil
}

func (emitterTestStore) CreateDeliveriesForEventTx(context.Context, pgx.Tx, uuid.UUID, time.Time) (int64, error) {
	return 0, nil
}

func (emitterTestStore) CreateDeliveryTx(context.Context, pgx.Tx, uuid.UUID, uuid.UUID, time.Time) (uuid.UUID, error) {
	return uuid.Nil, nil
}

func TestNormalizeEnvelopeUsesPlatformEventDefaults(t *testing.T) {
	fixed := time.Date(2026, time.August, 5, 7, 30, 0, 0, time.FixedZone("test", 2*60*60))
	emitter := &Emitter{store: emitterTestStore{}, now: func() time.Time { return fixed }}
	objectID := uuid.New()

	normalized, err := emitter.normalizeEnvelope(Event{
		TeamID:     uuid.New(),
		Type:       EventSMSSubmitted,
		ObjectType: "sms",
		ObjectID:   &objectID,
		Payload:    json.RawMessage(`{"status":"submitted"}`),
	}.envelope())
	if err != nil {
		t.Fatalf("normalize envelope: %v", err)
	}
	if normalized.ID == uuid.Nil {
		t.Fatal("expected generated event id")
	}
	if normalized.Version != platformevent.CurrentVersion {
		t.Fatalf("expected version %q, got %q", platformevent.CurrentVersion, normalized.Version)
	}
	if !normalized.OccurredAt.Equal(fixed.UTC()) {
		t.Fatalf("expected occurrence time %s, got %s", fixed.UTC(), normalized.OccurredAt)
	}
}

func TestNormalizeEnvelopeRejectsUnknownLegacyEvent(t *testing.T) {
	emitter := &Emitter{store: emitterTestStore{}, now: time.Now}

	_, err := emitter.normalizeEnvelope(Event{
		TeamID:     uuid.New(),
		Type:       "custom.created",
		ObjectType: "custom",
		Payload:    json.RawMessage(`{}`),
	}.envelope())
	if err == nil || !strings.Contains(err.Error(), "unknown event type") {
		t.Fatalf("expected unknown event type error, got %v", err)
	}
}

func TestNormalizeEnvelopeRejectsCatalogObjectMismatch(t *testing.T) {
	emitter := &Emitter{store: emitterTestStore{}, now: time.Now}
	objectID := uuid.New()

	_, err := emitter.normalizeEnvelope(Event{
		TeamID:     uuid.New(),
		Type:       EventSMSDelivered,
		ObjectType: "email",
		ObjectID:   &objectID,
		Payload:    json.RawMessage(`{}`),
	}.envelope())
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("expected object type mismatch error, got %v", err)
	}
}
