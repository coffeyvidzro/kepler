package attempt

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAttemptValidate(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	messageID := uuid.New()
	attempt := Attempt{
		ID:              uuid.New(),
		TeamID:          uuid.New(),
		Channel:         ChannelSMS,
		SMSMessageID:    &messageID,
		AttemptNumber:   1,
		Status:          StatusClaimed,
		ProviderAccount: "default",
		ClaimedAt:       now,
		Metadata:        json.RawMessage(`{}`),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := attempt.Validate(); err != nil {
		t.Fatalf("Attempt.Validate() error = %v", err)
	}

	attempt.Channel = ChannelEmail
	if err := attempt.Validate(); err == nil {
		t.Fatal("Attempt.Validate() error = nil for mismatched channel and message")
	}
}

func TestAttemptTerminalRequiresTimestamp(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	messageID := uuid.New()
	attempt := Attempt{
		ID:              uuid.New(),
		TeamID:          uuid.New(),
		Channel:         ChannelEmail,
		EmailMessageID:  &messageID,
		AttemptNumber:   1,
		Status:          StatusDelivered,
		Provider:        "ses",
		ProviderAccount: "default",
		ClaimedAt:       now,
		SubmittedAt:     &now,
		Metadata:        json.RawMessage(`{}`),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := attempt.Validate(); err == nil {
		t.Fatal("Attempt.Validate() error = nil for terminal status without terminal time")
	}

	attempt.TerminalAt = &now
	if err := attempt.Validate(); err != nil {
		t.Fatalf("Attempt.Validate() error = %v", err)
	}
}

func TestAttemptValidateRejectsWrongChannelSender(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	messageID := uuid.New()
	senderID := uuid.New()
	value := Attempt{
		ID: uuid.New(), TeamID: uuid.New(), Channel: ChannelEmail,
		EmailMessageID: &messageID, SenderID: &senderID, AttemptNumber: 1,
		Status: StatusClaimed, ProviderAccount: "default", ClaimedAt: now,
		Metadata: json.RawMessage(`{}`), CreatedAt: now, UpdatedAt: now,
	}
	if err := value.Validate(); err == nil {
		t.Fatal("Attempt.Validate() error = nil for email attempt with Sender ID")
	}

	value.Channel = ChannelSMS
	value.EmailMessageID = nil
	value.SMSMessageID = &messageID
	value.SenderID = nil
	value.SenderDomainID = &senderID
	if err := value.Validate(); err == nil {
		t.Fatal("Attempt.Validate() error = nil for SMS attempt with sender domain")
	}
}

func TestAttemptStatusTransitions(t *testing.T) {
	t.Parallel()

	if !StatusRequestStarted.CanTransitionTo(StatusSubmissionUnknown) {
		t.Fatal("request_started should transition to submission_unknown")
	}
	if !StatusSent.CanTransitionTo(StatusDelivered) {
		t.Fatal("sent should transition to delivered")
	}
	if StatusDelivered.CanTransitionTo(StatusSent) {
		t.Fatal("terminal delivered status must not move backward")
	}
	if !StatusSubmitted.NeedsReconciliation() {
		t.Fatal("submitted status should require reconciliation")
	}
}
