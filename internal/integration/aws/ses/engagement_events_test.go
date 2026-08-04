package ses

import "testing"

func TestParseFeedbackEventNormalizesOpen(t *testing.T) {
	event, err := ParseFeedbackEvent(`{
		"eventType":"Open",
		"mail":{"timestamp":"2026-08-01T08:00:00Z","messageId":"open-id","destination":["USER@example.com"]},
		"open":{"timestamp":"2026-08-01T08:01:00Z","ipAddress":"192.0.2.40","userAgent":"Example Mail Client"}
	}`)
	if err != nil {
		t.Fatalf("ParseFeedbackEvent() error = %v", err)
	}
	if event.EventType != "open" || event.Diagnostics.IPAddress != "192.0.2.40" || event.Diagnostics.UserAgent != "Example Mail Client" {
		t.Fatalf("open event = %#v", event)
	}
	if len(event.Recipients) != 1 || event.Recipients[0] != "user@example.com" {
		t.Fatalf("recipients = %#v", event.Recipients)
	}
}

func TestParseFeedbackEventNormalizesClick(t *testing.T) {
	event, err := ParseFeedbackEvent(`{
		"eventType":"Click",
		"mail":{"timestamp":"2026-08-01T08:00:00Z","messageId":"click-id","destination":["user@example.com"]},
		"click":{"timestamp":"2026-08-01T08:02:00Z","ipAddress":"192.0.2.41","userAgent":"Example Browser","link":"https://example.com/path","linkTags":{"campaign":["launch","launch"]}}
	}`)
	if err != nil {
		t.Fatalf("ParseFeedbackEvent() error = %v", err)
	}
	if event.EventType != "click" || event.Diagnostics.Link != "https://example.com/path" {
		t.Fatalf("click event = %#v", event)
	}
	if values := event.Diagnostics.LinkTags["campaign"]; len(values) != 1 || values[0] != "launch" {
		t.Fatalf("link tags = %#v", event.Diagnostics.LinkTags)
	}
}

func TestParseFeedbackEventNormalizesSubscription(t *testing.T) {
	event, err := ParseFeedbackEvent(`{
		"eventType":"Subscription",
		"mail":{"timestamp":"2026-08-01T08:00:00Z","messageId":"subscription-id","destination":["user@example.com"]},
		"subscription":{"timestamp":"2026-08-01T08:03:00Z","contactList":"customers","source":"UnsubscribePage","newTopicPreferences":[{"topicName":"product","subscriptionStatus":"OptOut"}],"oldTopicPreferences":[{"topicName":"product","subscriptionStatus":"OptIn"}]}
	}`)
	if err != nil {
		t.Fatalf("ParseFeedbackEvent() error = %v", err)
	}
	if event.EventType != "subscription" || event.Diagnostics.ContactList != "customers" || event.Diagnostics.SubscriptionSource != "UnsubscribePage" {
		t.Fatalf("subscription event = %#v", event)
	}
	if len(event.Diagnostics.NewTopicPreferences) != 1 || event.Diagnostics.NewTopicPreferences[0].SubscriptionStatus != "OptOut" {
		t.Fatalf("new topic preferences = %#v", event.Diagnostics.NewTopicPreferences)
	}
}
