package tenantprovision

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	natsjs "github.com/nats-io/nats.go/jetstream"

	"github.com/coffeyvidzro/dugble/server/internal/modules/emailtenant"
)

type consumerHandler struct {
	calls, exhaustedCalls int
	err, exhaustedErr     error
}

func (h *consumerHandler) Handle(context.Context, emailtenant.ProvisionCommand) error {
	h.calls++
	return h.err
}
func (h *consumerHandler) HandleExhausted(context.Context, emailtenant.ProvisionCommand, error) error {
	h.exhaustedCalls++
	return h.exhaustedErr
}

type processedStore struct {
	processed         bool
	checkErr, markErr error
	markCalls         int
}

func (s *processedStore) IsProcessed(context.Context, string, uuid.UUID) (bool, error) {
	return s.processed, s.checkErr
}
func (s *processedStore) MarkProcessed(context.Context, string, uuid.UUID, map[string]any) error {
	s.markCalls++
	return s.markErr
}

type publisher struct {
	calls   int
	subject string
	err     error
}

func (p *publisher) Publish(_ context.Context, subject string, _ []byte, _ map[string]string, _ string) error {
	p.calls++
	p.subject = subject
	return p.err
}

type testMessage struct {
	data                                           []byte
	headers                                        nats.Header
	subject                                        string
	metadata                                       *natsjs.MsgMetadata
	metadataErr                                    error
	ackCalls, nakCalls, delayedNakCalls, termCalls int
	delay                                          time.Duration
}

func (m *testMessage) Metadata() (*natsjs.MsgMetadata, error) { return m.metadata, m.metadataErr }
func (m *testMessage) Data() []byte                           { return m.data }
func (m *testMessage) Headers() nats.Header                   { return m.headers }
func (m *testMessage) Subject() string                        { return m.subject }
func (m *testMessage) Reply() string                          { return "" }
func (m *testMessage) Ack() error                             { m.ackCalls++; return nil }
func (m *testMessage) DoubleAck(context.Context) error        { return m.Ack() }
func (m *testMessage) Nak() error                             { m.nakCalls++; return nil }
func (m *testMessage) NakWithDelay(delay time.Duration) error {
	m.delayedNakCalls++
	m.delay = delay
	return nil
}
func (m *testMessage) InProgress() error           { return nil }
func (m *testMessage) Term() error                 { m.termCalls++; return nil }
func (m *testMessage) TermWithReason(string) error { m.termCalls++; return nil }

func testCommand() emailtenant.ProvisionCommand {
	return emailtenant.ProvisionCommand{EventID: uuid.New(), TenantID: uuid.New(), TeamID: uuid.New(), Provider: emailtenant.ProviderAWSSES, Region: "eu-north-1", ExternalName: "dugble-team", SchemaVersion: 1}
}
func messageFor(t *testing.T, command emailtenant.ProvisionCommand, deliveries uint64) *testMessage {
	t.Helper()
	data, err := json.Marshal(command)
	if err != nil {
		t.Fatal(err)
	}
	return &testMessage{data: data, headers: nats.Header{"Dugble-Event-Id": []string{command.EventID.String()}}, subject: emailtenant.ProvisionSubject, metadata: &natsjs.MsgMetadata{NumDelivered: deliveries, Sequence: natsjs.SequencePair{Stream: 7}}}
}
func testConsumer(handler commandHandler, store processedEventStore, pub messagePublisher) *Consumer {
	return &Consumer{handler: handler, processed: store, publisher: pub, config: Config{HandlerTimeout: time.Second, MaxDeliver: 6, RetryBackOff: DefaultRetryBackOff()}}
}

func TestNewConsumerDerivesConsistentRetryPolicy(t *testing.T) {
	consumer := NewConsumer(nil, nil, nil, Config{RetryBackOff: []time.Duration{time.Second, 3 * time.Second, 9 * time.Second}})
	if consumer.config.MaxDeliver != 3 || len(consumer.config.RetryBackOff) != 3 {
		t.Fatalf("policy = max %d, backoff %v", consumer.config.MaxDeliver, consumer.config.RetryBackOff)
	}

	consumer = NewConsumer(nil, nil, nil, Config{MaxDeliver: 5, RetryBackOff: []time.Duration{time.Second, 2 * time.Second}})
	want := []time.Duration{time.Second, 2 * time.Second, 2 * time.Second, 2 * time.Second, 2 * time.Second}
	if !slices.Equal(consumer.config.RetryBackOff, want) {
		t.Fatalf("backoff = %v, want %v", consumer.config.RetryBackOff, want)
	}
}

func TestDefaultRetryBackOffReturnsCopy(t *testing.T) {
	policy := DefaultRetryBackOff()
	policy[0] = time.Hour
	if DefaultRetryBackOff()[0] == time.Hour {
		t.Fatal("caller mutated default retry policy")
	}
}

func TestConsumerProcessesAndAcknowledgesCommand(t *testing.T) {
	message, handler, store := messageFor(t, testCommand(), 1), &consumerHandler{}, &processedStore{}
	testConsumer(handler, store, &publisher{}).process(context.Background(), message)
	if handler.calls != 1 || store.markCalls != 1 || message.ackCalls != 1 {
		t.Fatalf("calls: handler=%d mark=%d ack=%d", handler.calls, store.markCalls, message.ackCalls)
	}
}

func TestConsumerAcknowledgesPreviouslyProcessedCommand(t *testing.T) {
	message, handler := messageFor(t, testCommand(), 2), &consumerHandler{}
	testConsumer(handler, &processedStore{processed: true}, &publisher{}).process(context.Background(), message)
	if handler.calls != 0 || message.ackCalls != 1 {
		t.Fatalf("handler=%d ack=%d", handler.calls, message.ackCalls)
	}
}

func TestConsumerRetriesTransientFailure(t *testing.T) {
	message := messageFor(t, testCommand(), 2)
	testConsumer(&consumerHandler{err: errors.New("unavailable")}, &processedStore{}, &publisher{}).process(context.Background(), message)
	if message.delayedNakCalls != 1 || message.delay != 5*time.Second {
		t.Fatalf("delayed NAKs=%d delay=%s", message.delayedNakCalls, message.delay)
	}
}

func TestConsumerDeadLettersMalformedCommand(t *testing.T) {
	message := &testMessage{data: []byte(`{"bad":true}`), headers: nats.Header{}, subject: emailtenant.ProvisionSubject, metadata: &natsjs.MsgMetadata{NumDelivered: 1, Sequence: natsjs.SequencePair{Stream: 42}}}
	pub := &publisher{}
	testConsumer(&consumerHandler{}, &processedStore{}, pub).process(context.Background(), message)
	if pub.calls != 1 || pub.subject != DLQSubject || message.termCalls != 1 {
		t.Fatalf("publish=%d subject=%q term=%d", pub.calls, pub.subject, message.termCalls)
	}
}

func TestConsumerFinalizesAndDeadLettersExhaustedCommand(t *testing.T) {
	message, handler, pub := messageFor(t, testCommand(), 6), &consumerHandler{err: errors.New("failed")}, &publisher{}
	testConsumer(handler, &processedStore{}, pub).process(context.Background(), message)
	if handler.exhaustedCalls != 1 || pub.calls != 1 || message.termCalls != 1 {
		t.Fatalf("exhausted=%d publish=%d term=%d", handler.exhaustedCalls, pub.calls, message.termCalls)
	}
}

func TestConsumerRetriesWhenExhaustedCommandCannotBeFinalized(t *testing.T) {
	message := messageFor(t, testCommand(), 6)
	handler, pub := &consumerHandler{err: errors.New("failed"), exhaustedErr: errors.New("database unavailable")}, &publisher{}
	testConsumer(handler, &processedStore{}, pub).process(context.Background(), message)
	if message.delayedNakCalls != 1 || pub.calls != 0 || message.termCalls != 0 {
		t.Fatalf("nak=%d publish=%d term=%d", message.delayedNakCalls, pub.calls, message.termCalls)
	}
}

func TestConsumerRetriesWhenProcessedStateCannotBePersisted(t *testing.T) {
	message := messageFor(t, testCommand(), 1)
	testConsumer(&consumerHandler{}, &processedStore{markErr: errors.New("database unavailable")}, &publisher{}).process(context.Background(), message)
	if message.nakCalls != 1 || message.ackCalls != 0 {
		t.Fatalf("nak=%d ack=%d", message.nakCalls, message.ackCalls)
	}
}

func TestConsumerRetriesWhenDLQPublicationFails(t *testing.T) {
	message := &testMessage{data: []byte(`{}`), headers: nats.Header{}, subject: emailtenant.ProvisionSubject, metadata: &natsjs.MsgMetadata{NumDelivered: 1, Sequence: natsjs.SequencePair{Stream: 42}}}
	testConsumer(&consumerHandler{}, &processedStore{}, &publisher{err: errors.New("NATS unavailable")}).process(context.Background(), message)
	if message.delayedNakCalls != 1 || message.termCalls != 0 {
		t.Fatalf("nak=%d term=%d", message.delayedNakCalls, message.termCalls)
	}
}
