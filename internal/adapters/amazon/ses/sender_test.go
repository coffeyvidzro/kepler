package ses

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/smithy-go"

	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/awsses"
)

type sendClientStub struct {
	retryer aws.Retryer
}

func (stub *sendClientStub) SendEmail(
	_ context.Context,
	_ *sesv2.SendEmailInput,
	options ...func(*sesv2.Options),
) (*sesv2.SendEmailOutput, error) {
	configured := sesv2.Options{}
	for _, configure := range options {
		configure(&configured)
	}
	stub.retryer = configured.Retryer
	return &sesv2.SendEmailOutput{MessageId: aws.String("provider-message")}, nil
}

func TestSendDisablesSDKRetries(t *testing.T) {
	stub := &sendClientStub{}
	client := &Client{
		defaultRegion: "eu-north-1",
		defaultFrom:   "sender@example.com",
		v2SendingClients: map[string]sesV2SendAPI{
			"eu-north-1": stub,
		},
	}

	_, err := client.Send(context.Background(), platformemail.Message{
		Region:           "eu-north-1",
		Stream:           "transactional",
		ConfigurationSet: "dugble-transactional",
		SESTenantName:    "dugble-system",
		From:             platformemail.Address{Email: "sender@example.com"},
		To:               []platformemail.Address{{Email: "recipient@example.com"}},
		Subject:          "Retry safety",
		Text:             "Test message",
	})
	if err != nil {
		t.Fatalf("send email: %v", err)
	}
	if stub.retryer == nil {
		t.Fatal("expected an explicit SES retryer")
	}
	if attempts := stub.retryer.MaxAttempts(); attempts != 1 {
		t.Fatalf("expected one SDK attempt, got %d", attempts)
	}
}

func TestClassifySESFailureTreatsTooManyRequestsAsRetryable(t *testing.T) {
	err := classifySESFailure(&smithy.GenericAPIError{
		Code:    "TooManyRequestsException",
		Message: "rate limited",
		Fault:   smithy.FaultServer,
	})
	if !platformemail.IsRetryable(err) {
		t.Fatalf("expected retryable SES error, got %v", err)
	}
}

func TestClassifySESFailureTreatsRequestTimeoutAsSubmissionUnknown(t *testing.T) {
	err := classifySESFailure(&smithy.GenericAPIError{
		Code:    "RequestTimeoutException",
		Message: "response outcome unknown",
		Fault:   smithy.FaultServer,
	})
	if !platformemail.IsSubmissionUnknown(err) {
		t.Fatalf("expected submission-unknown SES error, got %v", err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("expected classified provider error, not a raw context deadline")
	}
}
