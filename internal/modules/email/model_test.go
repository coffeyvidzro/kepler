package email

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

func TestMessageSummaryOmitsContentBodies(t *testing.T) {
	htmlBody := "<p>sensitive body</p>"
	textBody := "sensitive body"
	message := Message{
		ID:       "message-id",
		ToEmail:  "recipient@example.com",
		Subject:  "Subject",
		HTMLBody: &htmlBody,
		TextBody: &textBody,
		Status:   StatusQueued,
	}

	encoded, err := json.Marshal(message.Summary())
	if err != nil {
		t.Fatalf("marshal message summary: %v", err)
	}
	for _, forbidden := range [][]byte{[]byte("html_body"), []byte("text_body"), []byte("sensitive body")} {
		if bytes.Contains(encoded, forbidden) {
			t.Fatalf("summary contains %q: %s", forbidden, encoded)
		}
	}
}

func TestBatchSendRequestAcceptsTopLevelArray(t *testing.T) {
	var request BatchSendRequest
	if err := json.Unmarshal([]byte(`[
		{"to":"first@example.com","subject":"First","text":"one"},
		{"to":["second@example.com"],"subject":"Second","html":"<p>two</p>"}
	]`), &request); err != nil {
		t.Fatalf("unmarshal batch: %v", err)
	}
	if len(request.Messages) != 2 || request.Messages[1].To[0].Email != "second@example.com" {
		t.Fatalf("unexpected batch: %#v", request)
	}
}

func TestSendResponsesContainOnlyIDsInRequestOrder(t *testing.T) {
	encoded, err := json.Marshal(SendResponses([]Message{{ID: "first", Subject: "secret"}, {ID: "second"}}))
	if err != nil {
		t.Fatalf("marshal responses: %v", err)
	}
	if string(encoded) != `[{"id":"first"},{"id":"second"}]` {
		t.Fatalf("unexpected response: %s", encoded)
	}
}

func TestRetrieveResponseMatchesSentEmailShape(t *testing.T) {
	html := "<p>Hello</p>"
	fromName := "Acme"
	providerMessageID := "<provider-message@example.com>"
	createdAt := time.Date(2026, time.April, 3, 22, 13, 42, 0, time.UTC)
	message := Message{
		ID: "email-id", MessageType: MessageTypeTransactional,
		FromEmail: "sender@example.com", FromName: &fromName,
		To: []EmailAddress{{Email: "ada@example.com", Name: "Ada"}}, CC: []EmailAddress{}, BCC: []EmailAddress{},
		Subject: "Hello", HTMLBody: &html, Status: StatusDelivered, ProviderMessageID: &providerMessageID,
		Tags: []Tag{{Name: "category", Value: "confirm_email"}}, CreatedAt: createdAt,
	}

	encoded, err := json.Marshal(message.RetrieveResponse())
	if err != nil {
		t.Fatalf("marshal retrieve response: %v", err)
	}
	response := message.RetrieveResponse()
	if response.Object != "email" || response.Stream != MessageTypeTransactional ||
		response.From != "Acme <sender@example.com>" || len(response.To) != 1 ||
		response.To[0] != "Ada <ada@example.com>" || response.LastEvent != StatusDelivered {
		t.Fatalf("unexpected retrieve response: %#v", response)
	}
	if response.CC == nil || response.BCC == nil || response.ReplyTo == nil {
		t.Fatalf("recipient arrays must not be null: %#v", response)
	}
	for _, forbidden := range [][]byte{[]byte(`"team_id"`), []byte(`"metadata"`), []byte(`"error_message"`)} {
		if bytes.Contains(encoded, forbidden) {
			t.Fatalf("retrieve response contains internal field %s: %s", forbidden, encoded)
		}
	}
}

func TestRetrieveResponseUsesPersistedMarketingStream(t *testing.T) {
	response := (Message{MessageType: MessageTypeMarketing}).RetrieveResponse()
	if response.Stream != MessageTypeMarketing {
		t.Fatalf("stream = %q, want %q", response.Stream, MessageTypeMarketing)
	}
}

func TestFormatEmailAddressWithoutNameReturnsBareAddress(t *testing.T) {
	if got := formatEmailAddress(EmailAddress{Email: " recipient@example.com "}); got != "recipient@example.com" {
		t.Fatalf("formatEmailAddress() = %q, want bare address", got)
	}
}

func TestFormatEmailAddressQuotesComplexDisplayName(t *testing.T) {
	got := formatEmailAddress(EmailAddress{Email: "recipient@example.com", Name: "Doe, Ada"})
	if got != `"Doe, Ada" <recipient@example.com>` {
		t.Fatalf("formatEmailAddress() = %q", got)
	}
}
