package emaildelivery

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/awsses"
)

type processorClaimResult struct {
	message DeliveryMessage
	err     error
}

type processorRepositoryStub struct {
	claims           []processorClaimResult
	claimCalls       int
	recoveryDecision RecoveryDecision
	recoveryErr      error
	recoveryCutoff   time.Time
	requestStarted   bool
	submitted        bool
}

func (stub *processorRepositoryStub) Claim(context.Context, uuid.UUID, uuid.UUID) (DeliveryMessage, error) {
	index := stub.claimCalls
	stub.claimCalls++
	if index >= len(stub.claims) {
		return DeliveryMessage{}, errors.New("unexpected claim")
	}
	return stub.claims[index].message, stub.claims[index].err
}

func (stub *processorRepositoryStub) RecoverStale(
	_ context.Context,
	_, _ uuid.UUID,
	staleBefore time.Time,
) (RecoveryDecision, error) {
	stub.recoveryCutoff = staleBefore
	return stub.recoveryDecision, stub.recoveryErr
}

func (stub *processorRepositoryStub) MarkRequestStarted(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	stub.requestStarted = true
	return nil
}

func (stub *processorRepositoryStub) MarkSubmitted(
	context.Context,
	uuid.UUID,
	uuid.UUID,
	uuid.UUID,
	platformemail.Result,
) error {
	stub.submitted = true
	return nil
}

func (*processorRepositoryStub) MarkRetryable(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, error) error {
	return nil
}

func (*processorRepositoryStub) MarkSubmissionUnknown(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string, error) error {
	return nil
}

func (*processorRepositoryStub) MarkFailed(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string, error) error {
	return nil
}

func (*processorRepositoryStub) MarkExhausted(context.Context, uuid.UUID, uuid.UUID, error) error {
	return nil
}

type processorSenderStub struct {
	calls int
}

func (stub *processorSenderStub) Send(context.Context, platformemail.Message) (platformemail.Result, error) {
	stub.calls++
	return platformemail.Result{Provider: "ses", MessageID: "provider-message"}, nil
}

func TestProcessorKeepsActiveProcessingCommandUnacknowledged(t *testing.T) {
	now := time.Date(2026, time.August, 5, 12, 0, 0, 0, time.UTC)
	repository := &processorRepositoryStub{
		claims:           []processorClaimResult{{err: ErrMessageNotDeliverable}},
		recoveryDecision: RecoveryPending,
	}
	sender := &processorSenderStub{}
	processor := NewProcessor(repository, sender)
	processor.currentTime = func() time.Time { return now }

	err := processor.Handle(context.Background(), DeliverCommand{
		MessageID: uuid.New(),
		TeamID:    uuid.New(),
	})
	if !errors.Is(err, ErrMessageRecoveryPending) {
		t.Fatalf("expected recovery-pending error, got %v", err)
	}
	if sender.calls != 0 {
		t.Fatalf("expected no SES call, got %d", sender.calls)
	}
	wantCutoff := now.Add(-defaultStaleProcessingAfter)
	if !repository.recoveryCutoff.Equal(wantCutoff) {
		t.Fatalf("expected recovery cutoff %s, got %s", wantCutoff, repository.recoveryCutoff)
	}
}

func TestProcessorRetriesRecoveredUnstartedAttempt(t *testing.T) {
	messageID := uuid.New()
	teamID := uuid.New()
	attemptID := uuid.New()
	repository := &processorRepositoryStub{
		claims: []processorClaimResult{
			{err: ErrMessageNotDeliverable},
			{message: DeliveryMessage{
				ID:        messageID,
				TeamID:    teamID,
				AttemptID: attemptID,
				Provider:  "ses",
				Region:    "eu-north-1",
				FromEmail: "sender@example.com",
				To:        []platformemail.Address{{Email: "recipient@example.com"}},
				Subject:   "Recovered",
				Text:      "Recovered delivery",
			}},
		},
		recoveryDecision: RecoveryRetry,
	}
	sender := &processorSenderStub{}
	processor := NewProcessor(repository, sender)

	if err := processor.Handle(context.Background(), DeliverCommand{MessageID: messageID, TeamID: teamID}); err != nil {
		t.Fatalf("handle recovered delivery: %v", err)
	}
	if repository.claimCalls != 2 {
		t.Fatalf("expected two claim attempts, got %d", repository.claimCalls)
	}
	if !repository.requestStarted || !repository.submitted {
		t.Fatalf("expected recovered delivery to be started and submitted")
	}
	if sender.calls != 1 {
		t.Fatalf("expected one SES call, got %d", sender.calls)
	}
}

func TestProcessorDoesNotResendRecoveredUnknownSubmission(t *testing.T) {
	repository := &processorRepositoryStub{
		claims:           []processorClaimResult{{err: ErrMessageNotDeliverable}},
		recoveryDecision: RecoverySubmissionUnknown,
	}
	sender := &processorSenderStub{}
	processor := NewProcessor(repository, sender)

	if err := processor.Handle(context.Background(), DeliverCommand{
		MessageID: uuid.New(),
		TeamID:    uuid.New(),
	}); err != nil {
		t.Fatalf("handle unknown submission recovery: %v", err)
	}
	if sender.calls != 0 {
		t.Fatalf("expected no SES resend, got %d calls", sender.calls)
	}
}
