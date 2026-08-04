package ses

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/smithy-go"

	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/email"
)

func TestSendUsesEnvelopeDestinationsForBCC(t *testing.T) {
	recordingClient := &recordingSESV2Client{}
	client, err := NewClient("us-east-1", "default@example.com", "access-key", "secret-key")
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	client.v2SendingClients["us-east-1"] = recordingClient
	route, err := platformemail.CustomerDeliveryRoute("transactional", "dugble-t-customer")
	if err != nil {
		t.Fatalf("create customer route: %v", err)
	}

	_, err = client.Send(context.Background(), platformemail.Message{
		Region:           "us-east-1",
		Stream:           route.Stream,
		ConfigurationSet: route.ConfigurationSet,
		SESTenantName:    route.SESTenantName,
		From:             platformemail.Address{Email: "sender@example.com"},
		To:               []platformemail.Address{{Email: "to@example.com"}},
		CC:               []platformemail.Address{{Email: "cc@example.com"}},
		BCC:              []platformemail.Address{{Email: "hidden@example.com"}, {Email: "TO@example.com"}},
		Subject:          "BCC delivery",
		Text:             "Hello",
	})
	if err != nil {
		t.Fatalf("send email: %v", err)
	}

	if !reflect.DeepEqual(recordingClient.input.Destination.ToAddresses, []string{"to@example.com"}) {
		t.Fatalf("ToAddresses = %#v", recordingClient.input.Destination.ToAddresses)
	}
	if !reflect.DeepEqual(recordingClient.input.Destination.CcAddresses, []string{"cc@example.com"}) {
		t.Fatalf("CcAddresses = %#v", recordingClient.input.Destination.CcAddresses)
	}
	if !reflect.DeepEqual(recordingClient.input.Destination.BccAddresses, []string{"hidden@example.com", "TO@example.com"}) {
		t.Fatalf("BccAddresses = %#v", recordingClient.input.Destination.BccAddresses)
	}
	raw := string(recordingClient.input.Content.Raw.Data)
	if strings.Contains(strings.ToLower(raw), "\r\nbcc:") || strings.HasPrefix(strings.ToLower(raw), "bcc:") {
		t.Fatalf("raw MIME exposed a Bcc header:\n%s", raw)
	}
}

func TestSendAddsDeliveryCorrelationTags(t *testing.T) {
	recordingClient := &recordingSESV2Client{}
	client, err := NewClient("us-east-1", "default@example.com", "access-key", "secret-key")
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	client.v2SendingClients["us-east-1"] = recordingClient
	route, err := platformemail.CustomerDeliveryRoute("transactional", "dugble-t-customer")
	if err != nil {
		t.Fatalf("create customer route: %v", err)
	}

	_, err = client.Send(context.Background(), platformemail.Message{
		Region:           "us-east-1",
		MessageID:        "message-123",
		AttemptID:        "attempt-456",
		Stream:           route.Stream,
		ConfigurationSet: route.ConfigurationSet,
		SESTenantName:    route.SESTenantName,
		From:             platformemail.Address{Email: "sender@example.com"},
		To:               []platformemail.Address{{Email: "recipient@example.com"}},
		Subject:          "Correlation",
		Text:             "Hello",
	})
	if err != nil {
		t.Fatalf("send email: %v", err)
	}
	if len(recordingClient.input.EmailTags) != 3 {
		t.Fatalf("EmailTags = %#v, want two correlation tags and one stream tag", recordingClient.input.EmailTags)
	}
	got := map[string]string{}
	for _, tag := range recordingClient.input.EmailTags {
		got[aws.ToString(tag.Name)] = aws.ToString(tag.Value)
	}
	if got[messageIDTagName] != "message-123" || got[attemptIDTagName] != "attempt-456" || got[streamTagName] != "transactional" {
		t.Fatalf("delivery tags = %#v", got)
	}
}

func TestBuildMIMERejectsOversizedEncodedMessage(t *testing.T) {
	_, err := buildMIME(platformemail.Message{
		From:    platformemail.Address{Email: "sender@example.com"},
		To:      []platformemail.Address{{Email: "recipient@example.com"}},
		Subject: "Oversized",
		Text:    strings.Repeat("x", platformemail.MaxRawMessageBytes),
	})
	if !errors.Is(err, ErrMessageTooLarge) {
		t.Fatalf("buildMIME() error = %v, want ErrMessageTooLarge", err)
	}
}

func TestBuildMIMERejectsReservedHeaders(t *testing.T) {
	tests := []string{
		"X-SES-SOURCE-ARN",
		"x-amazon-trace-id",
		"Date",
		"Message-ID",
		"Return-Path",
	}
	for _, header := range tests {
		t.Run(header, func(t *testing.T) {
			_, err := buildMIME(platformemail.Message{
				From:    platformemail.Address{Email: "sender@example.com"},
				To:      []platformemail.Address{{Email: "recipient@example.com"}},
				Subject: "Reserved header",
				Text:    "Hello",
				Headers: map[string]string{header: "value"},
			})
			if !errors.Is(err, ErrReservedHeader) {
				t.Fatalf("buildMIME() error = %v, want ErrReservedHeader", err)
			}
		})
	}
}

func TestBuildMIMEAllowsApplicationHeaders(t *testing.T) {
	raw, err := buildMIME(platformemail.Message{
		From:    platformemail.Address{Email: "sender@example.com"},
		To:      []platformemail.Address{{Email: "recipient@example.com"}},
		Subject: "Custom header",
		Text:    "Hello",
		Headers: map[string]string{"X-Dugble-Trace": "trace-123"},
	})
	if err != nil {
		t.Fatalf("buildMIME() error = %v", err)
	}
	if !strings.Contains(string(raw), "X-Dugble-Trace: trace-123\r\n") {
		t.Fatalf("raw MIME does not contain the allowed custom header:\n%s", raw)
	}
}

type fakeSESAPIError struct {
	code    string
	message string
}

func (e fakeSESAPIError) Error() string                 { return e.code }
func (e fakeSESAPIError) ErrorCode() string             { return e.code }
func (e fakeSESAPIError) ErrorMessage() string          { return e.message }
func (e fakeSESAPIError) ErrorFault() smithy.ErrorFault { return smithy.FaultServer }

func TestClassifySESFailureTreatsTransportErrorsAsUnknown(t *testing.T) {
	err := classifySESFailure(errors.New("connection reset"))
	if !platformemail.IsSubmissionUnknown(err) || platformemail.IsRetryable(err) {
		t.Fatalf("transport failure classification = %v", err)
	}
}

func TestClassifySESFailureKeepsExplicitThrottlingRetryable(t *testing.T) {
	err := classifySESFailure(fakeSESAPIError{code: "Throttling"})
	if platformemail.IsSubmissionUnknown(err) || !platformemail.IsRetryable(err) {
		t.Fatalf("throttling classification = %v", err)
	}
}

func TestClassifySESFailureTreatsRequestTimeoutAsUnknown(t *testing.T) {
	err := classifySESFailure(fakeSESAPIError{code: "RequestTimeout"})
	if !platformemail.IsSubmissionUnknown(err) || platformemail.IsRetryable(err) {
		t.Fatalf("request timeout classification = %v", err)
	}
}
