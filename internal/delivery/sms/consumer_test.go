package smsdelivery

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	natsjs "github.com/nats-io/nats.go/jetstream"
)

func TestConsumerProcessesAndAcknowledgesCommand(t *testing.T) {
	command := DeliverCommand{EventID: uuid.New(), MessageID: uuid.New(), TeamID: uuid.New()}
	message := newTestMessage(t, command, 1)
	handler := &fakeDeliveryHandler{}
	processed := &fakeProcessedStore{}
	consumer := newTestConsumer(handler, processed, &fakePublisher{})

	consumer.processMessage(context.Background(), message)

	if handler.calls != 1 {
		t.Fatalf("handler calls = %d, want 1", handler.calls)
	}
	if processed.markCalls != 1 {
		t.Fatalf("mark calls = %d, want 1", processed.markCalls)
	}
	if message.doubleAckCalls != 1 {
		t.Fatalf("double ack calls = %d, want 1", message.doubleAckCalls)
	}
}

func TestConsumerAcknowledgesPreviouslyProcessedCommandWithoutResend(t *testing.T) {
	command := DeliverCommand{EventID: uuid.New(), MessageID: uuid.New(), TeamID: uuid.New()}
	message := newTestMessage(t, command, 2)
	handler := &fakeDeliveryHandler{}
	processed := &fakeProcessedStore{processed: true}
	consumer := newTestConsumer(handler, processed, &fakePublisher{})

	consumer.processMessage(context.Background(), message)

	if handler.calls != 0 {
		t.Fatalf("handler calls = %d, want 0", handler.calls)
	}
	if message.doubleAckCalls != 1 {
		t.Fatalf("double ack calls = %d, want 1", message.doubleAckCalls)
	}
}

func TestConsumerRetriesTransientDeliveryFailure(t *testing.T) {
	command := DeliverCommand{EventID: uuid.New(), MessageID: uuid.New(), TeamID: uuid.New()}
	message := newTestMessage(t, command, 2)
	handler := &fakeDeliveryHandler{err: errors.New("provider unavailable")}
	consumer := newTestConsumer(handler, &fakeProcessedStore{}, &fakePublisher{})

	consumer.processMessage(context.Background(), message)

	if message.nakDelayCalls != 1 {
		t.Fatalf("nak delay calls = %d, want 1", message.nakDelayCalls)
	}
	if message.nakDelay != 30*time.Second {
		t.Fatalf("nak delay = %s, want 30s", message.nakDelay)
	}
	if message.termCalls != 0 {
		t.Fatalf("term calls = %d, want 0", message.termCalls)
	}
}

func TestConsumerMovesMalformedCommandToDLQ(t *testing.T) {
	message := &fakeJetStreamMessage{
		data:    []byte(`{"message_id":"not-a-uuid"}`),
		headers: nats.Header{"Dugble-Event-Id": []string{uuid.NewString()}},
		subject: DeliverSubject,
		metadata: &natsjs.MsgMetadata{
			NumDelivered: 1,
			Sequence:     natsjs.SequencePair{Stream: 42},
		},
	}
	publisher := &fakePublisher{}
	consumer := newTestConsumer(&fakeDeliveryHandler{}, &fakeProcessedStore{}, publisher)

	consumer.processMessage(context.Background(), message)

	if publisher.calls != 1 {
		t.Fatalf("publisher calls = %d, want 1", publisher.calls)
	}
	if publisher.subject != DeliverDLQSubject {
		t.Fatalf("DLQ subject = %q, want %q", publisher.subject, DeliverDLQSubject)
	}
	if message.termCalls != 1 {
		t.Fatalf("term calls = %d, want 1", message.termCalls)
	}
}

func TestConsumerMovesExhaustedCommandToDLQ(t *testing.T) {
	command := DeliverCommand{EventID: uuid.New(), MessageID: uuid.New(), TeamID: uuid.New()}
	message := newTestMessage(t, command, 6)
	publisher := &fakePublisher{}
	consumer := newTestConsumer(
		&fakeDeliveryHandler{err: errors.New("permanent failure")},
		&fakeProcessedStore{},
		publisher,
	)

	consumer.processMessage(context.Background(), message)

	if publisher.calls != 1 {
		t.Fatalf("publisher calls = %d, want 1", publisher.calls)
	}
	if message.termCalls != 1 {
		t.Fatalf("term calls = %d, want 1", message.termCalls)
	}
	if message.nakDelayCalls != 0 {
		t.Fatalf("nak delay calls = %d, want 0", message.nakDelayCalls)
	}
	if handler := consumer.handler.(*fakeDeliveryHandler); handler.exhaustedCalls != 1 {
		t.Fatalf("exhausted calls = %d, want 1", handler.exhaustedCalls)
	}
}

func TestConsumerRetriesWhenExhaustedCommandCannotBeFinalized(t *testing.T) {
	command := DeliverCommand{EventID: uuid.New(), MessageID: uuid.New(), TeamID: uuid.New()}
	message := newTestMessage(t, command, 6)
	handler := &fakeDeliveryHandler{err: errors.New("provider unavailable"), exhaustedErr: errors.New("database unavailable")}
	publisher := &fakePublisher{}
	consumer := newTestConsumer(handler, &fakeProcessedStore{}, publisher)

	consumer.processMessage(context.Background(), message)

	if handler.exhaustedCalls != 1 {
		t.Fatalf("exhausted calls = %d, want 1", handler.exhaustedCalls)
	}
	if message.nakDelayCalls != 1 {
		t.Fatalf("nak delay calls = %d, want 1", message.nakDelayCalls)
	}
	if publisher.calls != 0 || message.termCalls != 0 {
		t.Fatalf("publisher calls = %d, term calls = %d; want both 0", publisher.calls, message.termCalls)
	}
}

func newTestConsumer(handler deliveryHandler, processed processedEventStore, publisher messagePublisher) *Consumer {
	return &Consumer{
		publisher: publisher,
		processed: processed,
		handler:   handler,
		config:    DefaultConsumerConfig(),
	}
}

func newTestMessage(t *testing.T, command DeliverCommand, delivered uint64) *fakeJetStreamMessage {
	t.Helper()
	payload, err := json.Marshal(command)
	if err != nil {
		t.Fatalf("marshal command: %v", err)
	}
	return &fakeJetStreamMessage{
		data:    payload,
		headers: nats.Header{"Dugble-Event-Id": []string{command.EventID.String()}},
		subject: DeliverSubject,
		metadata: &natsjs.MsgMetadata{
			NumDelivered: delivered,
			Sequence:     natsjs.SequencePair{Stream: 7},
		},
	}
}

type fakeDeliveryHandler struct {
	calls          int
	err            error
	exhaustedCalls int
	exhaustedErr   error
}

func (h *fakeDeliveryHandler) HandleExhausted(context.Context, DeliverCommand, error) error {
	h.exhaustedCalls++
	return h.exhaustedErr
}

func (h *fakeDeliveryHandler) Handle(context.Context, DeliverCommand) error {
	h.calls++
	return h.err
}

type fakeProcessedStore struct {
	processed bool
	checkErr  error
	markErr   error
	markCalls int
}

func (s *fakeProcessedStore) IsProcessed(context.Context, string, uuid.UUID) (bool, error) {
	return s.processed, s.checkErr
}

func (s *fakeProcessedStore) MarkProcessed(context.Context, string, uuid.UUID, map[string]any) error {
	s.markCalls++
	return s.markErr
}

type fakePublisher struct {
	calls   int
	subject string
	err     error
}

func (p *fakePublisher) Publish(_ context.Context, subject string, _ []byte, _ map[string]string, _ string) error {
	p.calls++
	p.subject = subject
	return p.err
}

type fakeJetStreamMessage struct {
	data           []byte
	headers        nats.Header
	subject        string
	metadata       *natsjs.MsgMetadata
	metadataErr    error
	doubleAckCalls int
	nakDelayCalls  int
	nakDelay       time.Duration
	termCalls      int
}

func (m *fakeJetStreamMessage) Metadata() (*natsjs.MsgMetadata, error) {
	return m.metadata, m.metadataErr
}
func (m *fakeJetStreamMessage) Data() []byte         { return m.data }
func (m *fakeJetStreamMessage) Headers() nats.Header { return m.headers }
func (m *fakeJetStreamMessage) Subject() string      { return m.subject }
func (m *fakeJetStreamMessage) Reply() string        { return "" }
func (m *fakeJetStreamMessage) Ack() error           { return nil }
func (m *fakeJetStreamMessage) DoubleAck(context.Context) error {
	m.doubleAckCalls++
	return nil
}
func (m *fakeJetStreamMessage) Nak() error { return nil }
func (m *fakeJetStreamMessage) NakWithDelay(delay time.Duration) error {
	m.nakDelayCalls++
	m.nakDelay = delay
	return nil
}
func (m *fakeJetStreamMessage) InProgress() error { return nil }
func (m *fakeJetStreamMessage) Term() error {
	m.termCalls++
	return nil
}
func (m *fakeJetStreamMessage) TermWithReason(string) error {
	m.termCalls++
	return nil
}
