package smsdelivery

import (
	"context"
	"testing"

	"github.com/google/uuid"

	smsmodule "github.com/coffeyvidzro/dugble/server/internal/modules/sms"
	smsapi "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
)

func TestProviderAvailable(t *testing.T) {
	t.Parallel()

	providers := []string{"mnotify", " moolre "}
	for _, provider := range []string{"MNOTIFY", "moolre"} {
		if !providerAvailable(provider, providers) {
			t.Fatalf("providerAvailable(%q) = false, want true", provider)
		}
	}
	for _, provider := range []string{"", "unknown"} {
		if providerAvailable(provider, providers) {
			t.Fatalf("providerAvailable(%q) = true, want false", provider)
		}
	}
}

type processorRepositoryStub struct {
	message          smsmodule.Message
	route            DeliveryRoute
	routeErr         error
	failedReason     string
	attemptCreated   bool
	attemptStarted   bool
	attemptSubmitted bool
}

func (stub *processorRepositoryStub) MarkProcessing(context.Context, uuid.UUID, uuid.UUID) (smsmodule.Message, error) {
	return stub.message, nil
}
func (stub *processorRepositoryStub) Get(context.Context, uuid.UUID, uuid.UUID) (smsmodule.Message, error) {
	return stub.message, nil
}
func (stub *processorRepositoryStub) MarkDeliveryUnknown(context.Context, uuid.UUID, uuid.UUID, string) (smsmodule.Message, error) {
	return stub.message, nil
}
func (stub *processorRepositoryStub) MarkFailed(_ context.Context, _, _ uuid.UUID, reason string) (smsmodule.Message, error) {
	stub.failedReason = reason
	return stub.message, nil
}
func (stub *processorRepositoryStub) ResolveDeliveryRoute(context.Context, uuid.UUID, uuid.UUID) (DeliveryRoute, error) {
	return stub.route, stub.routeErr
}
func (stub *processorRepositoryStub) CreateDeliveryAttempt(context.Context, uuid.UUID, uuid.UUID, DeliveryRoute) (uuid.UUID, error) {
	stub.attemptCreated = true
	return uuid.New(), nil
}
func (stub *processorRepositoryStub) MarkDeliveryAttemptStarted(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	stub.attemptStarted = true
	return nil
}
func (*processorRepositoryStub) MarkDeliveryAttemptRetryable(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, error) error {
	return nil
}
func (*processorRepositoryStub) MarkDeliveryAttemptUnknown(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, error) error {
	return nil
}
func (*processorRepositoryStub) MarkDeliveryAttemptFailed(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, error) error {
	return nil
}
func (stub *processorRepositoryStub) MarkDeliveryAttemptSubmitted(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, *smsapi.SendResponse) error {
	stub.attemptSubmitted = true
	return nil
}
func (*processorRepositoryStub) FinalizeInFlightDelivery(context.Context, uuid.UUID, uuid.UUID, error) error {
	return nil
}

type processorSenderStub struct {
	providers []string
	sent      bool
}

func (stub *processorSenderStub) ProviderIDs() []string { return stub.providers }
func (stub *processorSenderStub) SendWithProvider(context.Context, string, smsapi.SendRequest) (*smsapi.SendResponse, error) {
	stub.sent = true
	return &smsapi.SendResponse{ProviderID: "moolre", ProviderMsgID: "provider-message", Status: "submitted"}, nil
}

func TestProcessorHandleSubmitsThroughCanonicalRoute(t *testing.T) {
	t.Parallel()

	repository := &processorRepositoryStub{
		message: smsmodule.Message{ID: uuid.NewString(), Status: smsmodule.StatusProcessing},
		route:   DeliveryRoute{SenderID: uuid.New(), Provider: "moolre"},
	}
	sender := &processorSenderStub{providers: []string{"moolre"}}
	processor := NewProcessor(repository, sender)
	command := DeliverCommand{MessageID: uuid.New(), TeamID: uuid.New()}

	if err := processor.Handle(context.Background(), command); err != nil {
		t.Fatalf("Processor.Handle() error = %v", err)
	}
	if !repository.attemptCreated || !repository.attemptStarted || !repository.attemptSubmitted || !sender.sent {
		t.Fatalf("delivery operations = created:%t started:%t submitted:%t sent:%t",
			repository.attemptCreated, repository.attemptStarted, repository.attemptSubmitted, sender.sent)
	}
}

func TestProcessorHandleFailsBeforeAttemptWhenProviderUnavailable(t *testing.T) {
	t.Parallel()

	repository := &processorRepositoryStub{
		message: smsmodule.Message{ID: uuid.NewString(), Status: smsmodule.StatusProcessing},
		route:   DeliveryRoute{SenderID: uuid.New(), Provider: "moolre"},
	}
	sender := &processorSenderStub{providers: []string{"mnotify"}}
	processor := NewProcessor(repository, sender)
	command := DeliverCommand{MessageID: uuid.New(), TeamID: uuid.New()}

	if err := processor.Handle(context.Background(), command); err != nil {
		t.Fatalf("Processor.Handle() error = %v", err)
	}
	if repository.failedReason == "" {
		t.Fatal("Processor.Handle() did not fail message with unavailable provider")
	}
	if repository.attemptCreated || sender.sent {
		t.Fatalf("provider unavailable created attempt = %t, sent = %t", repository.attemptCreated, sender.sent)
	}
}
