package feedback

import (
	"context"
	"errors"
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
			Status:            delivery.StatusSent,
		},
		result: ApplyResult{Applied: true},
	}
	processor, err := NewProcessor(repository)
	if err != nil {
		t.Fatalf("NewProcessor() error = %v", err)
	}

	result, err := processor.Process(context.Background(), Event{
		Provider:          "mnotify",
		ProviderEventID:   "event-1",
		ProviderMessageID: "provider-message",
		Channel:           messaging.ChannelSMS,
		Status:            delivery.StatusDelivered,
		OccurredAt:        now,
		ReceivedAt:        now,
	})
	if err != nil {
		t.Fatalf("Processor.Process() error = %v", err)
	}
	if !result.Applied || result.Status != delivery.StatusDelivered {
		t.Fatalf("Processor.Process() result = %+v", result)
	}
	if repository.update.ExpectedStatus != delivery.StatusSent || repository.update.TerminalAt == nil {
		t.Fatalf("ApplyEvent() update = %+v", repository.update)
	}
}

func TestProcessorRejectsBackwardFeedback(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	repository := &repositoryStub{attempt: delivery.Attempt{
		ID:       uuid.New(),
		Channel:  messaging.ChannelEmail,
		Provider: "ses",
		Status:   delivery.StatusDelivered,
	}}
	processor, err := NewProcessor(repository)
	if err != nil {
		t.Fatalf("NewProcessor() error = %v", err)
	}

	_, err = processor.Process(context.Background(), Event{
		Provider:          "ses",
		ProviderEventID:   "event-2",
		ProviderMessageID: "provider-message",
		Channel:           messaging.ChannelEmail,
		Status:            delivery.StatusSent,
		OccurredAt:        now,
		ReceivedAt:        now,
	})
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("Processor.Process() error = %v, want ErrInvalidTransition", err)
	}
}
