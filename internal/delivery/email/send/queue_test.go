package emaildelivery

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/coffeyvidzro/dugble/server/internal/messaging/outbox"
)

type recordingStore struct {
	event     outbox.Event
	deleted   uuid.UUID
	updated   uuid.UUID
	updatedAt time.Time
	usedTx    bool
	enqueue   int
}

func (s *recordingStore) UpdatePendingAvailableAtTx(_ context.Context, _ pgx.Tx, eventID uuid.UUID, availableAt time.Time) error {
	s.updated, s.updatedAt = eventID, availableAt
	return nil
}

func (s *recordingStore) DeletePendingTx(_ context.Context, _ pgx.Tx, eventID uuid.UUID) error {
	s.deleted = eventID
	return nil
}

func (s *recordingStore) Enqueue(_ context.Context, event outbox.Event) (uuid.UUID, error) {
	s.event = event
	s.enqueue++
	return event.ID, nil
}

func (s *recordingStore) EnqueueTx(_ context.Context, _ pgx.Tx, event outbox.Event) (uuid.UUID, error) {
	s.event = event
	s.usedTx = true
	s.enqueue++
	return event.ID, nil
}

func TestQueueEnqueueEmailDeliveryTx(t *testing.T) {
	messageID := uuid.New()
	teamID := uuid.New()
	store := &recordingStore{}

	err := NewQueue(store).EnqueueEmailDeliveryTx(context.Background(), nil, messageID, teamID)
	if err != nil {
		t.Fatalf("enqueue email delivery: %v", err)
	}
	if !store.usedTx || store.enqueue != 1 {
		t.Fatalf("expected one transactional enqueue, usedTx=%v count=%d", store.usedTx, store.enqueue)
	}
	if store.event.Subject != DeliverSubject {
		t.Fatalf("subject = %q, want %q", store.event.Subject, DeliverSubject)
	}
	if store.event.AggregateType != "email_message" || store.event.AggregateID != messageID {
		t.Fatalf("unexpected aggregate: %q %s", store.event.AggregateType, store.event.AggregateID)
	}
	if store.event.Headers["Dugble-Event-Type"] != "email.send.requested.v1" {
		t.Fatalf("unexpected event type header: %q", store.event.Headers["Dugble-Event-Type"])
	}

	var command DeliverCommand
	if err := json.Unmarshal(store.event.Payload, &command); err != nil {
		t.Fatalf("decode command: %v", err)
	}
	if command.EventID != store.event.ID || command.MessageID != messageID || command.TeamID != teamID || command.SchemaVersion != 1 {
		t.Fatalf("unexpected command: %+v", command)
	}
}

func TestQueueEnqueueEmailDeliveryAtTxPreservesSchedule(t *testing.T) {
	store := &recordingStore{}
	scheduledAt := time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)

	err := NewQueue(store).EnqueueEmailDeliveryAtTx(
		context.Background(), nil, uuid.New(), uuid.New(), scheduledAt,
	)
	if err != nil {
		t.Fatalf("enqueue scheduled email delivery: %v", err)
	}
	if !store.event.AvailableAt.Equal(scheduledAt) {
		t.Fatalf("available at = %s, want %s", store.event.AvailableAt, scheduledAt)
	}
}

func TestQueueCancelEmailDeliveryTxDeletesDeterministicEvent(t *testing.T) {
	store := &recordingStore{}
	messageID := uuid.New()

	if err := NewQueue(store).CancelEmailDeliveryTx(context.Background(), nil, messageID, uuid.New()); err != nil {
		t.Fatalf("cancel email delivery: %v", err)
	}
	want := uuid.NewSHA1(uuid.NameSpaceURL, []byte(deliveryNamespace+messageID.String()))
	if store.deleted != want {
		t.Fatalf("deleted event = %s, want %s", store.deleted, want)
	}
}

func TestQueueRescheduleEmailDeliveryTxUpdatesDeterministicEvent(t *testing.T) {
	store := &recordingStore{}
	messageID := uuid.New()
	availableAt := time.Now().UTC().Add(2 * time.Hour)

	if err := NewQueue(store).RescheduleEmailDeliveryTx(context.Background(), nil, messageID, uuid.New(), availableAt); err != nil {
		t.Fatalf("reschedule email delivery: %v", err)
	}
	want := uuid.NewSHA1(uuid.NameSpaceURL, []byte(deliveryNamespace+messageID.String()))
	if store.updated != want || !store.updatedAt.Equal(availableAt) {
		t.Fatalf("updated event = %s at %s, want %s at %s", store.updated, store.updatedAt, want, availableAt)
	}
}

func TestQueueUsesDeterministicEventID(t *testing.T) {
	messageID := uuid.New()
	teamID := uuid.New()
	first, err := newDeliveryEvent(messageID, teamID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := newDeliveryEvent(messageID, teamID)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("event IDs differ: %s != %s", first.ID, second.ID)
	}
}

func TestQueueRequiresStore(t *testing.T) {
	if err := (*Queue)(nil).EnqueueEmailDelivery(context.Background(), uuid.New(), uuid.New()); err == nil {
		t.Fatal("expected an error for an unconfigured queue")
	}
}
