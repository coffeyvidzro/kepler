package ses

import (
	"context"
	"net/mail"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"

	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/email"
)

func TestSendPreservesFriendlyFromName(t *testing.T) {
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
		From:             platformemail.Address{Email: "onboarding@runnage.dev", Name: "Dugble"},
		To:               []platformemail.Address{{Email: "recipient@example.com"}},
		Subject:          "Welcome",
		Text:             "Hello",
	})
	if err != nil {
		t.Fatalf("send email: %v", err)
	}

	assertFriendlyFrom(t, recordingClient, "Dugble", "onboarding@runnage.dev")
}

func TestSendEncodesUnicodeFriendlyFromName(t *testing.T) {
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
		From:             platformemail.Address{Email: "onboarding@runnage.dev", Name: "Dugblé"},
		To:               []platformemail.Address{{Email: "recipient@example.com"}},
		Subject:          "Welcome",
		Text:             "Hello",
	})
	if err != nil {
		t.Fatalf("send email: %v", err)
	}

	assertFriendlyFrom(t, recordingClient, "Dugblé", "onboarding@runnage.dev")
}

func assertFriendlyFrom(t *testing.T, recordingClient *recordingSESV2Client, wantName, wantEmail string) {
	t.Helper()

	got := aws.ToString(recordingClient.input.FromEmailAddress)
	parsed, err := mail.ParseAddress(got)
	if err != nil {
		t.Fatalf("parse FromEmailAddress %q: %v", got, err)
	}
	if parsed.Name != wantName || parsed.Address != wantEmail {
		t.Fatalf(
			"FromEmailAddress parsed as name=%q address=%q, want name=%q address=%q",
			parsed.Name,
			parsed.Address,
			wantName,
			wantEmail,
		)
	}

	raw := string(recordingClient.input.Content.Raw.Data)
	if !strings.Contains(raw, "From: "+got+"\r\n") {
		t.Fatalf("raw MIME From header does not match SES FromEmailAddress:\n%s", raw)
	}
}
