package smsdelivery

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	smsapi "github.com/coffeyvidzro/dugble/server/internal/integration/sms"
	smsmodule "github.com/coffeyvidzro/dugble/server/internal/modules/sms"
)

func TestHandlerSafeProviderRejectionMarksFailed(t *testing.T) {
	messageID := uuid.New()
	teamID := uuid.New()
	repo := newFakeRepository(messageID, teamID, smsmodule.StatusQueued)
	senderErr := &smsapi.SendError{Attempts: []smsapi.ProviderAttempt{{ProviderID: "test", Err: safeSendError{}}}}
	handler := newTestHandler(repo, &fakeSender{sendErr: senderErr})

	if err := handler.Handle(context.Background(), testCommand(messageID, teamID)); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if repo.message.Status != smsmodule.StatusFailed {
		t.Fatalf("status = %q, want %q", repo.message.Status, smsmodule.StatusFailed)
	}
}

func TestHandlerAmbiguousProviderErrorStaysProcessingAndRetries(t *testing.T) {
	messageID := uuid.New()
	teamID := uuid.New()
	repo := newFakeRepository(messageID, teamID, smsmodule.StatusQueued)
	handler := newTestHandler(repo, &fakeSender{sendErr: errors.New("connection reset")})

	if err := handler.Handle(context.Background(), testCommand(messageID, teamID)); err == nil {
		t.Fatal("Handle returned nil error for ambiguous provider failure")
	}
	if repo.message.Status != smsmodule.StatusProcessing {
		t.Fatalf("status = %q, want %q", repo.message.Status, smsmodule.StatusProcessing)
	}
}

func TestHandlerProcessingRetryDoesNotResendBeforeStale(t *testing.T) {
	messageID := uuid.New()
	teamID := uuid.New()
	repo := newFakeRepository(messageID, teamID, smsmodule.StatusProcessing)
	repo.message.UpdatedAt = time.Now()
	sender := &fakeSender{}
	handler := newTestHandler(repo, sender)
	handler.staleProcessingAfter = time.Hour

	if err := handler.Handle(context.Background(), testCommand(messageID, teamID)); err == nil {
		t.Fatal("Handle returned nil error for active processing message")
	}
	if sender.sendCalls != 0 {
		t.Fatalf("sendCalls = %d, want 0", sender.sendCalls)
	}
	if repo.message.Status != smsmodule.StatusProcessing {
		t.Fatalf("status = %q, want %q", repo.message.Status, smsmodule.StatusProcessing)
	}
}

func TestHandlerStaleProcessingFailsWithoutResend(t *testing.T) {
	messageID := uuid.New()
	teamID := uuid.New()
	repo := newFakeRepository(messageID, teamID, smsmodule.StatusProcessing)
	repo.message.UpdatedAt = time.Now().Add(-2 * time.Hour)
	sender := &fakeSender{}
	handler := newTestHandler(repo, sender)
	handler.staleProcessingAfter = time.Hour

	if err := handler.Handle(context.Background(), testCommand(messageID, teamID)); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if sender.sendCalls != 0 {
		t.Fatalf("sendCalls = %d, want 0", sender.sendCalls)
	}
	if repo.message.Status != smsmodule.StatusFailed {
		t.Fatalf("status = %q, want %q", repo.message.Status, smsmodule.StatusFailed)
	}
}

func TestHandlerExhaustedProcessingBecomesUnknown(t *testing.T) {
	messageID := uuid.New()
	teamID := uuid.New()
	repo := newFakeRepository(messageID, teamID, smsmodule.StatusProcessing)
	handler := newTestHandler(repo, &fakeSender{})

	if err := handler.HandleExhausted(context.Background(), testCommand(messageID, teamID), errors.New("connection reset")); err != nil {
		t.Fatalf("HandleExhausted returned error: %v", err)
	}
	if repo.message.Status != smsmodule.StatusUnknown {
		t.Fatalf("status = %q, want %q", repo.message.Status, smsmodule.StatusUnknown)
	}
}

func TestHandlerExhaustedCompletedMessageIsUnchanged(t *testing.T) {
	messageID := uuid.New()
	teamID := uuid.New()
	repo := newFakeRepository(messageID, teamID, smsmodule.StatusSubmitted)
	handler := newTestHandler(repo, &fakeSender{})

	if err := handler.HandleExhausted(context.Background(), testCommand(messageID, teamID), errors.New("late handler error")); err != nil {
		t.Fatalf("HandleExhausted returned error: %v", err)
	}
	if repo.message.Status != smsmodule.StatusSubmitted {
		t.Fatalf("status = %q, want %q", repo.message.Status, smsmodule.StatusSubmitted)
	}
}

func TestHandlerExhaustedQueuedMessageRequestsAnotherRetry(t *testing.T) {
	messageID := uuid.New()
	teamID := uuid.New()
	repo := newFakeRepository(messageID, teamID, smsmodule.StatusQueued)
	handler := newTestHandler(repo, &fakeSender{})

	if err := handler.HandleExhausted(context.Background(), testCommand(messageID, teamID), errors.New("database unavailable")); err == nil {
		t.Fatal("HandleExhausted returned nil error for an unclaimed queued message")
	}
	if repo.message.Status != smsmodule.StatusQueued {
		t.Fatalf("status = %q, want %q", repo.message.Status, smsmodule.StatusQueued)
	}
}

func newTestHandler(repo *fakeRepository, sender *fakeSender) *Handler {
	return &Handler{repository: repo, sender: sender, staleProcessingAfter: defaultStaleProcessingAfter}
}

func testCommand(messageID uuid.UUID, teamID uuid.UUID) DeliverCommand {
	return DeliverCommand{EventID: uuid.New(), MessageID: messageID, TeamID: teamID}
}

type fakeRepository struct {
	message smsmodule.Message
}

func newFakeRepository(messageID uuid.UUID, teamID uuid.UUID, status string) *fakeRepository {
	return &fakeRepository{message: smsmodule.Message{
		ID:        messageID.String(),
		TeamID:    teamID.String(),
		To:        "+233241234567",
		From:      "DUGBLE",
		Body:      "hello",
		Status:    status,
		UpdatedAt: time.Now(),
	}}
}

func (r *fakeRepository) MarkProcessing(_ context.Context, id uuid.UUID, teamID uuid.UUID) (smsmodule.Message, error) {
	if r.message.ID != id.String() || r.message.TeamID != teamID.String() || r.message.Status != smsmodule.StatusQueued {
		return smsmodule.Message{}, smsmodule.ErrMessageNotFound
	}
	r.message.Status = smsmodule.StatusProcessing
	r.message.UpdatedAt = time.Now()
	return r.message, nil
}

func (r *fakeRepository) Get(_ context.Context, id uuid.UUID, teamID uuid.UUID) (smsmodule.Message, error) {
	if r.message.ID != id.String() || r.message.TeamID != teamID.String() {
		return smsmodule.Message{}, smsmodule.ErrMessageNotFound
	}
	return r.message, nil
}

func (r *fakeRepository) MarkDeliveryUnknown(_ context.Context, id uuid.UUID, teamID uuid.UUID, message string) (smsmodule.Message, error) {
	if r.message.ID != id.String() || r.message.TeamID != teamID.String() || r.message.Status != smsmodule.StatusProcessing || r.message.ProviderMessageID != nil {
		return smsmodule.Message{}, smsmodule.ErrMessageNotFound
	}
	r.message.Status = smsmodule.StatusUnknown
	r.message.ErrorMessage = &message
	r.message.UpdatedAt = time.Now()
	return r.message, nil
}

func (r *fakeRepository) MarkFailed(_ context.Context, id uuid.UUID, teamID uuid.UUID, message string) (smsmodule.Message, error) {
	if r.message.ID != id.String() || r.message.TeamID != teamID.String() {
		return smsmodule.Message{}, smsmodule.ErrMessageNotFound
	}
	r.message.Status = smsmodule.StatusFailed
	r.message.ErrorMessage = &message
	r.message.UpdatedAt = time.Now()
	return r.message, nil
}

func (r *fakeRepository) MarkSubmitted(_ context.Context, id uuid.UUID, teamID uuid.UUID, providerID string, providerMessageID string, status string) (smsmodule.Message, error) {
	if r.message.ID != id.String() || r.message.TeamID != teamID.String() {
		return smsmodule.Message{}, smsmodule.ErrMessageNotFound
	}
	r.message.Status = status
	r.message.ProviderID = &providerID
	r.message.ProviderMessageID = &providerMessageID
	r.message.UpdatedAt = time.Now()
	return r.message, nil
}

type fakeSender struct {
	sendCalls int
	sendErr   error
}

func (s *fakeSender) Send(context.Context, smsapi.SendRequest) (*smsapi.SendResponse, error) {
	s.sendCalls++
	if s.sendErr != nil {
		return nil, s.sendErr
	}
	return &smsapi.SendResponse{ProviderID: "test", ProviderMsgID: "provider-123", Status: smsapi.StatusSubmitted}, nil
}

func (s *fakeSender) CheckStatus(context.Context, string, string) (*smsapi.StatusResponse, error) {
	return nil, errors.New("not implemented")
}

type safeSendError struct{}

func (safeSendError) Error() string        { return "safe provider rejection" }
func (safeSendError) SafeToFallback() bool { return true }
