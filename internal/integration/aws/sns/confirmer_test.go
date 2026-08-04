package sns

import (
	"context"
	"errors"
	"testing"
)

type confirmSubscriptionClientStub struct {
	called bool
	input  ConfirmSubscriptionInput
	err    error
}

func (s *confirmSubscriptionClientStub) ConfirmSubscription(
	_ context.Context,
	input ConfirmSubscriptionInput,
) error {
	s.called = true
	s.input = input
	return s.err
}

func TestConfirmerConfirmsSubscription(t *testing.T) {
	envelope := confirmationEnvelope()
	client := &confirmSubscriptionClientStub{}

	err := NewConfirmer(client).Confirm(context.Background(), envelope)
	if err != nil {
		t.Fatalf("Confirm() error = %v", err)
	}

	if !client.called {
		t.Fatal("confirmation client was not called")
	}

	if client.input.TopicARN != envelope.TopicARN {
		t.Fatalf(
			"TopicARN = %q, want %q",
			client.input.TopicARN,
			envelope.TopicARN,
		)
	}

	if envelope.Token == nil {
		t.Fatal("confirmation envelope token is nil")
	}

	if client.input.Token != *envelope.Token {
		t.Fatalf(
			"Token = %q, want %q",
			client.input.Token,
			*envelope.Token,
		)
	}
}

func TestConfirmerRejectsNonConfirmationEnvelope(t *testing.T) {
	envelope := confirmationEnvelope()
	envelope.Type = TypeNotification

	client := &confirmSubscriptionClientStub{}

	err := NewConfirmer(client).Confirm(context.Background(), envelope)
	if !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("Confirm() error = %v, want ErrInvalidEnvelope", err)
	}

	if client.called {
		t.Fatal("confirmation client should not be called")
	}
}

func TestConfirmerRequiresClient(t *testing.T) {
	err := NewConfirmer(nil).Confirm(
		context.Background(),
		confirmationEnvelope(),
	)

	if !errors.Is(err, ErrConfirmationUnavailable) {
		t.Fatalf(
			"Confirm() error = %v, want ErrConfirmationUnavailable",
			err,
		)
	}
}

func TestConfirmerWrapsClientFailure(t *testing.T) {
	client := &confirmSubscriptionClientStub{
		err: errors.New("AWS SNS unavailable"),
	}

	err := NewConfirmer(client).Confirm(
		context.Background(),
		confirmationEnvelope(),
	)

	if !errors.Is(err, ErrConfirmationUnavailable) {
		t.Fatalf(
			"Confirm() error = %v, want ErrConfirmationUnavailable",
			err,
		)
	}

	if !client.called {
		t.Fatal("confirmation client was not called")
	}
}

func confirmationEnvelope() Envelope {
	topicARN := "arn:aws:sns:us-east-1:123456789012:ses-events"
	token := "confirmation-token"
	subscribeURL := "https://sns.us-east-1.amazonaws.com/" +
		"?Action=ConfirmSubscription" +
		"&TopicArn=" + topicARN +
		"&Token=" + token +
		"&Version=2010-03-31"

	return Envelope{
		Type:             TypeSubscriptionConfirmation,
		MessageID:        "message-id",
		TopicARN:         topicARN,
		Message:          "confirm subscription",
		Timestamp:        "2026-07-31T08:00:00Z",
		SignatureVersion: "2",
		Signature:        "signature",
		SigningCertURL:   testCertificateURL,
		SubscribeURL:     &subscribeURL,
		Token:            &token,
	}
}
