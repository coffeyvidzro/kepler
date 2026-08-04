package smsdelivery

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/coffeyvidzro/dugble/server/internal/messaging/outbox"
)

func TestQueueEnqueueTxCreatesDeterministicOutboxEvent(t *testing.T) {
	messageID := uuid.New()
	teamID := uuid.New()
	store := &fakeEventStore{}
	queue := NewQueue(store)

	if err := queue.EnqueueSMSDeliveryTx(context.Background(), nil, messageID, teamID); err != nil {
		t.Fatalf("EnqueueSMSDeliveryTx returned error: %v", err)
	}
	if len(store.events) != 1 {
		t.Fatalf("events = %d, want 1", len(store.events))
	}

	event := store.events[0]
	if event.Subject != DeliverSubject {
		t.Fatalf("subject = %q, want %q", event.Subject, DeliverSubject)
	}
	if event.AggregateID != messageID {
		t.Fatalf("aggregate ID = %s, want %s", event.AggregateID, messageID)
	}
	if event.ID == uuid.Nil {
		t.Fatal("event ID is nil")
	}

	second, err := newDeliveryEvent(messageID, teamID)
	if err != nil {
		t.Fatalf("newDeliveryEvent returned error: %v", err)
	}
	if second.ID != event.ID {
		t.Fatalf("event ID = %s, second ID = %s; want deterministic IDs", event.ID, second.ID)
	}
}

type fakeEventStore struct {
	events    []outbox.Event
	deleted   uuid.UUID
	updated   uuid.UUID
	updatedAt time.Time
}

func (s *fakeEventStore) DeletePendingTx(_ context.Context, _ pgx.Tx, id uuid.UUID) error {
	s.deleted = id
	return nil
}

func (s *fakeEventStore) UpdatePendingAvailableAtTx(_ context.Context, _ pgx.Tx, id uuid.UUID, availableAt time.Time) error {
	s.updated, s.updatedAt = id, availableAt
	return nil
}

func TestQueueSchedulesReschedulesAndCancelsDelivery(t *testing.T) {
	store := &fakeEventStore{}
	queue := NewQueue(store)
	messageID, teamID := uuid.New(), uuid.New()
	first := time.Now().UTC().Add(time.Hour)
	second := first.Add(time.Hour)
	if err := queue.EnqueueSMSDeliveryAtTx(context.Background(), nil, messageID, teamID, first); err != nil {
		t.Fatal(err)
	}
	if !store.events[0].AvailableAt.Equal(first) {
		t.Fatalf("available at = %s, want %s", store.events[0].AvailableAt, first)
	}
	if err := queue.RescheduleSMSDeliveryTx(context.Background(), nil, messageID, teamID, second); err != nil {
		t.Fatal(err)
	}
	wantID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(deliveryEventNamespace+messageID.String()))
	if store.updated != wantID || !store.updatedAt.Equal(second) {
		t.Fatalf("unexpected reschedule: %s %s", store.updated, store.updatedAt)
	}
	if err := queue.CancelSMSDeliveryTx(context.Background(), nil, messageID, teamID); err != nil {
		t.Fatal(err)
	}
	if store.deleted != wantID {
		t.Fatalf("deleted = %s, want %s", store.deleted, wantID)
	}
}

func (s *fakeEventStore) Enqueue(_ context.Context, event outbox.Event) (uuid.UUID, error) {
	s.events = append(s.events, event)
	return event.ID, nil
}

func (s *fakeEventStore) EnqueueTx(_ context.Context, _ pgx.Tx, event outbox.Event) (uuid.UUID, error) {
	s.events = append(s.events, event)
	return event.ID, nil
}
