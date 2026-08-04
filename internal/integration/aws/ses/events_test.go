package ses

import (
	"encoding/json"
	"testing"
)

func TestParseFeedbackEventNormalizesSESDetails(t *testing.T) {
	event, err := ParseFeedbackEvent(`{
		"eventType":"Bounce",
		"mail":{
			"timestamp":"2026-07-31T08:00:00Z",
			"messageId":" ses-message-id ",
			"destination":["Fallback@example.com"],
			"tags":{
				"dugble_message_id":[" message-123 "],
				"dugble_attempt_id":["attempt-456"],
				"campaign":["summer","summer"]
			}
		},
		"bounce":{
			"timestamp":"2026-07-31T08:01:00Z",
			"bounceType":"Permanent",
			"bounceSubType":"General",
			"feedbackId":"feedback-123",
			"remoteMtaIp":"192.0.2.10",
			"reportingMTA":"dsn; mx.example.com",
			"bouncedRecipients":[
				{"emailAddress":"USER@example.com","action":"failed","status":"5.1.1","diagnosticCode":"smtp; 550 user unknown"},
				{"emailAddress":"user@example.com","action":"failed","status":"5.1.1","diagnosticCode":"smtp; 550 duplicate"}
			]
		}
	}`)
	if err != nil {
		t.Fatalf("ParseFeedbackEvent() error = %v", err)
	}
	if event.EventType != "bounce" || event.ProviderMessageID != "ses-message-id" {
		t.Fatalf("unexpected event identity: %#v", event)
	}
	if event.InternalMessageID != "message-123" || event.InternalAttemptID != "attempt-456" {
		t.Fatalf("correlation tags were not normalized: %#v", event)
	}
	if values := event.Tags["campaign"]; len(values) != 1 || values[0] != "summer" {
		t.Fatalf("tags = %#v", event.Tags)
	}
	if event.Diagnostics.BounceType != "Permanent" || event.Diagnostics.BounceSubType != "General" {
		t.Fatalf("bounce diagnostics were not normalized: %#v", event.Diagnostics)
	}
	if event.Diagnostics.ReportingMTA != "dsn; mx.example.com" || event.Diagnostics.RemoteMTAIPAddress != "192.0.2.10" {
		t.Fatalf("MTA diagnostics were not normalized: %#v", event.Diagnostics)
	}
	if len(event.RecipientDiagnostics) != 1 {
		t.Fatalf("recipient diagnostics = %#v", event.RecipientDiagnostics)
	}
	recipient := event.RecipientDiagnostics[0]
	if recipient.Email != "user@example.com" || recipient.Action != "failed" || recipient.StatusCode != "5.1.1" {
		t.Fatalf("unexpected recipient diagnostics: %#v", recipient)
	}
	if recipient.DiagnosticCode != "smtp; 550 user unknown" {
		t.Fatalf("diagnostic code = %q", recipient.DiagnosticCode)
	}
	if len(event.Recipients) != 1 || event.Recipients[0] != "user@example.com" {
		t.Fatalf("recipients = %#v", event.Recipients)
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal normalized event: %v", err)
	}
	if !json.Valid(encoded) {
		t.Fatalf("normalized event is invalid JSON: %s", encoded)
	}
}

func TestParseFeedbackEventNormalizesDeliveryDiagnostics(t *testing.T) {
	event, err := ParseFeedbackEvent(`{
		"eventType":"Delivery",
		"mail":{"timestamp":"2026-07-31T08:00:00Z","messageId":"delivery-id","destination":["user@example.com"]},
		"delivery":{
			"timestamp":"2026-07-31T08:00:05Z",
			"processingTimeMillis":1234,
			"recipients":["user@example.com"],
			"smtpResponse":"250 2.0.0 accepted",
			"reportingMTA":"dsn; a1-2.smtp-out.amazonses.com",
			"remoteMtaIp":"192.0.2.20"
		}
	}`)
	if err != nil {
		t.Fatalf("ParseFeedbackEvent() error = %v", err)
	}
	if event.Diagnostics.ProcessingTimeMillis != 1234 || event.Diagnostics.SMTPResponse != "250 2.0.0 accepted" {
		t.Fatalf("delivery diagnostics = %#v", event.Diagnostics)
	}
}

func TestParseFeedbackEventNormalizesDelayAndComplaintDiagnostics(t *testing.T) {
	delay, err := ParseFeedbackEvent(`{
		"eventType":"DeliveryDelay",
		"mail":{"timestamp":"2026-07-31T08:00:00Z","messageId":"delay-id","destination":["user@example.com"]},
		"deliveryDelay":{
			"timestamp":"2026-07-31T08:05:00Z",
			"delayType":"TransientCommunicationFailure",
			"expirationTime":"2026-08-01T08:00:00Z",
			"delayedRecipients":[{"emailAddress":"user@example.com","status":"4.4.1","diagnosticCode":"smtp; connection timed out"}]
		}
	}`)
	if err != nil {
		t.Fatalf("parse delay event: %v", err)
	}
	if delay.Diagnostics.DelayType != "TransientCommunicationFailure" || delay.Diagnostics.ExpirationTime == "" {
		t.Fatalf("delay diagnostics = %#v", delay.Diagnostics)
	}
	if len(delay.RecipientDiagnostics) != 1 || delay.RecipientDiagnostics[0].StatusCode != "4.4.1" {
		t.Fatalf("delay recipient diagnostics = %#v", delay.RecipientDiagnostics)
	}

	complaint, err := ParseFeedbackEvent(`{
		"eventType":"Complaint",
		"mail":{"timestamp":"2026-07-31T08:00:00Z","messageId":"complaint-id","destination":["user@example.com"]},
		"complaint":{
			"timestamp":"2026-07-31T09:00:00Z",
			"complaintFeedbackType":"abuse",
			"feedbackId":"complaint-feedback-id",
			"userAgent":"Example FBL",
			"arrivalDate":"2026-07-31T08:59:00Z",
			"complainedRecipients":[{"emailAddress":"user@example.com"}]
		}
	}`)
	if err != nil {
		t.Fatalf("parse complaint event: %v", err)
	}
	if complaint.Diagnostics.ComplaintFeedbackType != "abuse" || complaint.Diagnostics.ComplaintFeedbackID != "complaint-feedback-id" {
		t.Fatalf("complaint diagnostics = %#v", complaint.Diagnostics)
	}
}
