package feedback

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/coffeyvidzro/dugble/server/internal/platform/messaging"
	"github.com/coffeyvidzro/dugble/server/internal/platform/messaging/delivery"
)

type repositoryStub struct {
	attempt delivery.Attempt
	result  ApplyResult
	update  AttemptUpdate
	err     error
}

func (repository *repositoryStub) FindAttempt(context.Context, Lookup) (delivery.Attempt, error) {
	if repository.err != nil {
		return delivery.Attempt{}, repository.err
	}
	return repository.attempt, nil
}

func (repository *repositoryStub) ApplyEvent(_ context.Context, _ Event, update AttemptUpdate) (ApplyResult, error) {
	repository.update = update
	if repository.err != nil {
		return ApplyResult{}, repository.err
	}
	return repository.result, nil
}

func TestProcessorAppliesMonotonicFeedback(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	repository := &repositoryStub{
		attempt: delivery.Attempt{
			ID:                uuid.New(),
			Channel:           messaging.ChannelSMS,
			Provider:          "mnotify",
			ProviderMessageID: "provider-message",
			Status:            delivery.StatusRequestStarted,
		},
		result: ApplyResult{Applied: true, Transitioned: true},
	}
	processor, err := NewProcessor(repository)
	if err != nil {
		t.Fatalf("NewProcessor() error = %v", err)
	}

	result, err := processor.Process(context.Background(), Event{
		Provider:          "mnotify",
		ProviderEventID:   "event-1",
		ProviderMessageID: "provider-message",
		EventType:         "delivery_status",
		Channel:           messaging.ChannelSMS,
		Status:            delivery.StatusDelivered,
		OccurredAt:        now,
		ReceivedAt:        now,
	})
	if err != nil {
		t.Fatalf("Processor.Process() error = %v", err)
	}
	if !result.Applied || !result.Transitioned || result.Status != delivery.StatusDelivered {
		t.Fatalf("Processor.Process() result = %+v", result)
	}
	if repository.update.Status == nil || *repository.update.Status != delivery.StatusDelivered {
		t.Fatalf("ApplyEvent() status = %v", repository.update.Status)
	}
	if repository.update.ExpectedStatus != delivery.StatusRequestStarted || repository.update.TerminalAt == nil {
		t.Fatalf("ApplyEvent() update = %+v", repository.update)
	}
}

func TestProcessorRecordsBackwardFeedbackWithoutRegressing(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	repository := &repositoryStub{
		attempt: delivery.Attempt{
			ID:                uuid.New(),
			Channel:           messaging.ChannelEmail,
			Provider:          "ses",
			ProviderMessageID: "provider-message",
			Status:            delivery.StatusDelivered,
		},
		result: ApplyResult{Applied: true},
	}
	processor, err := NewProcessor(repository)
	if err != nil {
		t.Fatalf("NewProcessor() error = %v", err)
	}

	result, err := processor.Process(context.Background(), Event{
		Provider:          "ses",
		ProviderEventID:   "event-2",
		ProviderMessageID: "provider-message",
		EventType:         "delivery_delay",
		Channel:           messaging.ChannelEmail,
		Status:            delivery.StatusSent,
		OccurredAt:        now,
		ReceivedAt:        now,
	})
	if err != nil {
		t.Fatalf("Processor.Process() error = %v", err)
	}
	if !result.Ignored || result.Status != delivery.StatusDelivered {
		t.Fatalf("Processor.Process() result = %+v", result)
	}
	if repository.update.Status != nil {
		t.Fatalf("ApplyEvent() status = %v, want nil", repository.update.Status)
	}
}

func TestEventDedupeKeyIncludesChannel(t *testing.T) {
	t.Parallel()

	email := Event{Provider: "shared", ProviderEventID: "event-1", Channel: messaging.ChannelEmail}
	sms := email
	sms.Channel = messaging.ChannelSMS
	if email.DedupeKey() == sms.DedupeKey() {
		t.Fatalf("DedupeKey() collided across channels: %q", email.DedupeKey())
	}
}
