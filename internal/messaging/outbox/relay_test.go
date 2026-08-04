package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

type relayStoreStub struct {
	events        []Event
	published     []uuid.UUID
	released      []uuid.UUID
	releasedError string
}

func (s *relayStoreStub) ClaimBatch(context.Context, string, int, time.Time) ([]Event, error) {
	events := s.events
	s.events = nil
	return events, nil
}

func (s *relayStoreStub) MarkPublished(_ context.Context, id uuid.UUID, _ string) error {
	s.published = append(s.published, id)
	return nil
}

func (s *relayStoreStub) Release(_ context.Context, id uuid.UUID, _ string, _ time.Time, lastError string) error {
	s.released = append(s.released, id)
	s.releasedError = lastError
	return nil
}

type publisherStub struct {
	err       error
	messageID string
	headers   map[string]string
}

func (p *publisherStub) Publish(_ context.Context, _ string, _ []byte, headers map[string]string, messageID string) error {
	p.messageID = messageID
	p.headers = headers
	return p.err
}

func TestRelayMarksPublishedAfterJetStreamAck(t *testing.T) {
	t.Parallel()

	eventID := uuid.New()
	aggregateID := uuid.New()
	store := &relayStoreStub{events: []Event{{
		ID:            eventID,
		Subject:       "dugble.job.email.send.v1",
		AggregateType: "email",
		AggregateID:   aggregateID,
		Payload:       json.RawMessage(`{"message_id":"test"}`),
		Headers:       map[string]string{"Trace-Id": "trace"},
		Attempts:      1,
	}}}
	publisher := &publisherStub{}
	relay := NewRelay(store, publisher, Config{BatchSize: 10})

	processed, err := relay.processBatch(context.Background())
	if err != nil {
		t.Fatalf("process outbox batch: %v", err)
	}
	if processed != 1 || len(store.published) != 1 || store.published[0] != eventID {
		t.Fatalf("expected event %s to be published", eventID)
	}
	if len(store.released) != 0 {
		t.Fatalf("successful event must not be released")
	}
	if publisher.messageID != eventID.String() {
		t.Fatalf("expected message id %s, got %s", eventID, publisher.messageID)
	}
	if publisher.headers["Dugble-Aggregate-Id"] != aggregateID.String() {
		t.Fatalf("aggregate correlation header was not included")
	}
}

func TestRelayReleasesFailedPublish(t *testing.T) {
	t.Parallel()

	eventID := uuid.New()
	store := &relayStoreStub{events: []Event{{
		ID:            eventID,
		Subject:       "dugble.job.sms.deliver.v1",
		AggregateType: "sms",
		AggregateID:   uuid.New(),
		Payload:       json.RawMessage(`{"message_id":"test"}`),
		Attempts:      3,
	}}}
	publisher := &publisherStub{err: errors.New("NATS unavailable")}
	relay := NewRelay(store, publisher, Config{BatchSize: 10})

	processed, err := relay.processBatch(context.Background())
	if err != nil {
		t.Fatalf("process outbox batch: %v", err)
	}
	if processed != 1 || len(store.released) != 1 || store.released[0] != eventID {
		t.Fatalf("expected event %s to be released for retry", eventID)
	}
	if len(store.published) != 0 {
		t.Fatalf("failed event must not be marked published")
	}
	if store.releasedError != "NATS unavailable" {
		t.Fatalf("unexpected persisted error: %q", store.releasedError)
	}
}

func TestRetryBackoffIsBounded(t *testing.T) {
	t.Parallel()

	if got := retryBackoff(1); got != time.Second {
		t.Fatalf("first retry backoff = %s, want 1s", got)
	}
	if got := retryBackoff(100); got > 15*time.Minute {
		t.Fatalf("retry backoff exceeded cap: %s", got)
	}
}
