package emaildelivery

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/email"
)

type recordingDeliveryRepository struct {
	message          DeliveryMessage
	claimErr         error
	requestStarted   uuid.UUID
	submitted        platformemail.Result
	markSubmittedErr error
	retryableErr     error
	unknownCode      string
	unknownErr       error
	failedCode       string
	failedErr        error
	exhaustedErr     error
	markUnknownErr   error
	markFailedErr    error
}

func (r *recordingDeliveryRepository) Claim(context.Context, uuid.UUID, uuid.UUID) (DeliveryMessage, error) {
	return r.message, r.claimErr
}

func (r *recordingDeliveryRepository) MarkRequestStarted(_ context.Context, _ uuid.UUID, _ uuid.UUID, attemptID uuid.UUID) error {
	r.requestStarted = attemptID
	return nil
}

func (r *recordingDeliveryRepository) MarkSubmitted(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ uuid.UUID, result platformemail.Result) error {
	r.submitted = result
	return r.markSubmittedErr
}

func (r *recordingDeliveryRepository) MarkRetryable(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ uuid.UUID, err error) error {
	r.retryableErr = err
	return nil
}

func (r *recordingDeliveryRepository) MarkSubmissionUnknown(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ uuid.UUID, code string, err error) error {
	r.unknownCode = code
	r.unknownErr = err
	return r.markUnknownErr
}

func (r *recordingDeliveryRepository) MarkFailed(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ uuid.UUID, code string, err error) error {
	r.failedCode = code
	r.failedErr = err
	return r.markFailedErr
}

func (r *recordingDeliveryRepository) MarkExhausted(_ context.Context, _ uuid.UUID, _ uuid.UUID, err error) error {
	r.exhaustedErr = err
	return nil
}

type stubSender struct {
	result platformemail.Result
	err    error
	sent   platformemail.Message
	calls  int
}

func (s *stubSender) Send(_ context.Context, message platformemail.Message) (platformemail.Result, error) {
	s.calls++
	s.sent = message
	return s.result, s.err
}

func deliveryTestMessage() DeliveryMessage {
	return DeliveryMessage{
		ID:        uuid.New(),
		AttemptID: uuid.New(),
		Provider:  "aws_ses",
		Region:    "eu-west-1",
		FromEmail: "sender@example.com",
		FromName:  "Sender",
		To:        []platformemail.Address{{Email: "recipient@example.com"}},
		Subject:   "Hello",
		Text:      "Body",
		Headers:   map[string]string{"X-Test": "true"},
	}
}

func TestHandlerSubmitsAcceptedMessage(t *testing.T) {
	repository := &recordingDeliveryRepository{message: deliveryTestMessage()}
	sender := &stubSender{result: platformemail.Result{Provider: "test", MessageID: "provider-1"}}

	err := NewHandler(repository, sender).Handle(context.Background(), DeliverCommand{MessageID: uuid.New(), TeamID: uuid.New()})
	if err != nil {
		t.Fatalf("handle email delivery: %v", err)
	}
	if repository.requestStarted != repository.message.AttemptID {
		t.Fatalf("request attempt = %s, want %s", repository.requestStarted, repository.message.AttemptID)
	}
	if repository.submitted.MessageID != "provider-1" {
		t.Fatalf("submitted result = %+v", repository.submitted)
	}
	if sender.sent.MessageID != repository.message.ID.String() || sender.sent.AttemptID != repository.message.AttemptID.String() {
		t.Fatalf("missing provider correlation identifiers: %+v", sender.sent)
	}
}

func TestHandlerStopsUnavailableSenderDomainBeforeProvider(t *testing.T) {
	repository := &recordingDeliveryRepository{claimErr: ErrSenderDomainUnavailable}
	sender := &stubSender{}

	err := NewHandler(repository, sender).Handle(context.Background(), DeliverCommand{MessageID: uuid.New(), TeamID: uuid.New()})
	if err != nil {
		t.Fatalf("unavailable sender domain should be handled: %v", err)
	}
	if sender.calls != 0 {
		t.Fatalf("sender calls = %d, want 0", sender.calls)
	}
}

func TestHandlerRecordsRetryableProviderFailure(t *testing.T) {
	providerErr := platformemail.NewSendError("throttling", true, errors.New("slow down"))
	repository := &recordingDeliveryRepository{message: deliveryTestMessage()}
	sender := &stubSender{err: providerErr}

	err := NewHandler(repository, sender).Handle(context.Background(), DeliverCommand{MessageID: uuid.New(), TeamID: uuid.New()})
	if err == nil {
		t.Fatal("expected retryable provider error")
	}
	if !errors.Is(err, providerErr) || repository.retryableErr == nil {
		t.Fatalf("retryable failure was not recorded: err=%v recorded=%v", err, repository.retryableErr)
	}
	if repository.unknownCode != "" || repository.failedCode != "" {
		t.Fatalf("retryable failure reached terminal state: unknown=%q failed=%q", repository.unknownCode, repository.failedCode)
	}
}

func TestHandlerQuarantinesAmbiguousProviderFailure(t *testing.T) {
	providerErr := platformemail.NewSubmissionUnknownError("ses_submission_unknown", errors.New("connection reset"))
	repository := &recordingDeliveryRepository{message: deliveryTestMessage()}
	sender := &stubSender{err: providerErr}

	err := NewHandler(repository, sender).Handle(context.Background(), DeliverCommand{MessageID: uuid.New(), TeamID: uuid.New()})
	if err != nil {
		t.Fatalf("ambiguous provider result should be quarantined and acknowledged: %v", err)
	}
	if repository.unknownCode != "ses_submission_unknown" || !errors.Is(repository.unknownErr, providerErr) {
		t.Fatalf("ambiguous result was not recorded: code=%q err=%v", repository.unknownCode, repository.unknownErr)
	}
	if repository.retryableErr != nil {
		t.Fatalf("ambiguous result must not be retried: %v", repository.retryableErr)
	}
}

func TestHandlerQuarantinesAcceptedMessageWhenPersistenceFails(t *testing.T) {
	persistErr := errors.New("database unavailable")
	repository := &recordingDeliveryRepository{message: deliveryTestMessage(), markSubmittedErr: persistErr}
	sender := &stubSender{result: platformemail.Result{Provider: "aws_ses", MessageID: "provider-1"}}

	err := NewHandler(repository, sender).Handle(context.Background(), DeliverCommand{MessageID: uuid.New(), TeamID: uuid.New()})
	if err != nil {
		t.Fatalf("accepted message should be quarantined when persistence recovers: %v", err)
	}
	if repository.unknownCode != "submission_persistence_failed" || !errors.Is(repository.unknownErr, persistErr) {
		t.Fatalf("persistence ambiguity was not recorded: code=%q err=%v", repository.unknownCode, repository.unknownErr)
	}
}

func TestHandlerRecordsPermanentProviderFailure(t *testing.T) {
	providerErr := platformemail.NewSendError("message rejected", false, errors.New("bad recipient"))
	repository := &recordingDeliveryRepository{message: deliveryTestMessage()}
	sender := &stubSender{err: providerErr}

	err := NewHandler(repository, sender).Handle(context.Background(), DeliverCommand{MessageID: uuid.New(), TeamID: uuid.New()})
	if err != nil {
		t.Fatalf("permanent provider rejection should be handled: %v", err)
	}
	if repository.failedCode != "message_rejected" || !errors.Is(repository.failedErr, providerErr) {
		t.Fatalf("unexpected permanent failure: code=%q err=%v", repository.failedCode, repository.failedErr)
	}
}

func TestHandlerExhaustedMarksFailed(t *testing.T) {
	repository := &recordingDeliveryRepository{}
	cause := errors.New("still failing")

	err := NewHandler(repository, nil).HandleExhausted(context.Background(), DeliverCommand{MessageID: uuid.New(), TeamID: uuid.New()}, cause)
	if err != nil {
		t.Fatalf("handle exhausted: %v", err)
	}
	if !errors.Is(repository.exhaustedErr, cause) {
		t.Fatalf("unexpected exhausted failure: %v", repository.exhaustedErr)
	}
}
