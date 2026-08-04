package ses

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"

	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/email"
)

type recordingSESV2Client struct {
	calls int
	input *sesv2.SendEmailInput
}

func (c *recordingSESV2Client) SendEmail(_ context.Context, input *sesv2.SendEmailInput, _ ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
	c.calls++
	c.input = input
	return &sesv2.SendEmailOutput{MessageId: aws.String("provider-message-id")}, nil
}

func TestSendUsesMessageRegion(t *testing.T) {
	defaultClient, regionalClient := &recordingSESV2Client{}, &recordingSESV2Client{}
	client, err := NewClient("us-east-1", "default@example.com", "access-key", "secret-key")
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	client.v2SendingClients["us-east-1"] = defaultClient
	client.v2SendingClients["eu-north-1"] = regionalClient
	route, err := platformemail.CustomerDeliveryRoute("transactional", "dugble-t-customer")
	if err != nil {
		t.Fatalf("create customer route: %v", err)
	}
	_, err = client.Send(context.Background(), platformemail.Message{
		Region:           "eu-north-1",
		Stream:           route.Stream,
		ConfigurationSet: route.ConfigurationSet,
		SESTenantName:    route.SESTenantName,
		From:             platformemail.Address{Email: "sender@example.com"},
		To:               []platformemail.Address{{Email: "recipient@example.com"}},
		Subject:          "Regional delivery",
		Text:             "Hello",
	})
	if err != nil {
		t.Fatalf("send email: %v", err)
	}
	if regionalClient.calls != 1 || defaultClient.calls != 0 {
		t.Fatalf("regional calls = %d, default calls = %d", regionalClient.calls, defaultClient.calls)
	}
}

func TestSendUsesPersistedConfigurationSetAndTenant(t *testing.T) {
	recordingClient := &recordingSESV2Client{}
	client, err := NewClient("us-east-1", "default@example.com", "access-key", "secret-key")
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	client.v2SendingClients["us-east-1"] = recordingClient
	route, err := platformemail.CustomerDeliveryRoute("marketing", "dugble-t-customer")
	if err != nil {
		t.Fatalf("create customer route: %v", err)
	}
	_, err = client.Send(context.Background(), platformemail.Message{
		Region:           "us-east-1",
		Stream:           route.Stream,
		ConfigurationSet: route.ConfigurationSet,
		SESTenantName:    route.SESTenantName,
		From:             platformemail.Address{Email: "sender@example.com"},
		To:               []platformemail.Address{{Email: "recipient@example.com"}},
		Subject:          "Configuration set",
		Text:             "Hello",
	})
	if err != nil {
		t.Fatalf("send email: %v", err)
	}
	if got := aws.ToString(recordingClient.input.ConfigurationSetName); got != "dugble-marketing" {
		t.Fatalf("ConfigurationSetName = %q, want %q", got, "dugble-marketing")
	}
	if got := aws.ToString(recordingClient.input.TenantName); got != "dugble-t-customer" {
		t.Fatalf("TenantName = %q, want %q", got, "dugble-t-customer")
	}
}

func TestSendRejectsMissingOrUnsupportedRegion(t *testing.T) {
	t.Parallel()

	client, err := NewClient("us-east-1", "default@example.com", "access-key", "secret-key")
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	for _, region := range []string{"", "eu-west-1"} {
		region := region
		t.Run(region, func(t *testing.T) {
			_, sendErr := client.Send(context.Background(), platformemail.Message{Region: region})
			if sendErr == nil || !strings.Contains(sendErr.Error(), "region") {
				t.Fatalf("Send() error = %v, want region validation error", sendErr)
			}
			if platformemail.IsRetryable(sendErr) || platformemail.FailureCode(sendErr) != "invalid_region" {
				t.Fatalf("Send() error = %v, want non-retryable invalid_region", sendErr)
			}
		})
	}
}

func TestGenerateBYODKIMMaterial(t *testing.T) {
	selector, privateKey, publicKey, err := generateBYODKIMMaterial()
	if err != nil {
		t.Fatalf("generateBYODKIMMaterial returned error: %v", err)
	}
	if selector == "" || privateKey == "" || publicKey == "" {
		t.Fatal("generateBYODKIMMaterial returned an empty value")
	}
	if len(selector) > 63 || !strings.HasPrefix(selector, "dugble") {
		t.Fatalf("selector = %q", selector)
	}
}

func TestMapVerificationRecords(t *testing.T) {
	records := mapVerificationRecords(platformemail.DomainProvisionRequest{Domain: "example.com", Region: "us-east-1", CustomReturnPath: "send", SESTenantName: "dugble-t-customer"}, "dugble123", "public-key")
	if len(records) != 3 {
		t.Fatalf("records length = %d, want 3", len(records))
	}
	if records[0].Record != platformemail.RecordDKIM || records[1].Type != platformemail.RecordTypeMX || records[2].Type != platformemail.RecordTypeTXT {
		t.Fatalf("unexpected verification records: %#v", records)
	}
	if got, want := records[0].Value, "p=public-key"; got != want {
		t.Fatalf("DKIM value = %q, want %q", got, want)
	}
}
