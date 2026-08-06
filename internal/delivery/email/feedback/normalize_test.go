package feedback

import (
	"encoding/json"
	"testing"
	"time"

	awsses "github.com/coffeyvidzro/dugble/server/internal/adapters/amazon/ses"
	platformdelivery "github.com/coffeyvidzro/dugble/server/internal/platform/messaging/delivery"
)

func TestNormalizeSESFeedbackEventUsesAggregateState(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	tests := []struct {
		eventType     string
		messageStatus string
		want          platformdelivery.AttemptStatus
	}{
		{eventType: "send", messageStatus: "submitted", want: platformdelivery.StatusSubmitted},
		{eventType: "delivery_delay", messageStatus: "delayed", want: platformdelivery.StatusSent},
		{eventType: "delivery", messageStatus: "partially_delivered", want: platformdelivery.StatusSent},
		{eventType: "delivery", messageStatus: "delivered", want: platformdelivery.StatusDelivered},
		{eventType: "bounce", messageStatus: "partially_failed", want: platformdelivery.StatusSent},
		{eventType: "bounce", messageStatus: "bounced", want: platformdelivery.StatusPermanentFailure},
		{eventType: "complaint", messageStatus: "complained", want: platformdelivery.StatusDelivered},
		{eventType: "reject", messageStatus: "rejected", want: platformdelivery.StatusRejected},
		{eventType: "rendering_failure", messageStatus: "failed", want: platformdelivery.StatusPermanentFailure},
	}
	for _, test := range tests {
		test := test
		t.Run(test.eventType+"_"+test.messageStatus, func(t *testing.T) {
			t.Parallel()
			event, err := normalizeSESFeedbackEvent(
				"notification-1",
				awsses.FeedbackEvent{
					EventType:         test.eventType,
					ProviderMessageID: "provider-message",
					OccurredAt:        now,
				},
				now,
				json.RawMessage(`{}`),
				test.messageStatus,
			)
			if err != nil {
				t.Fatalf("normalizeSESFeedbackEvent() error = %v", err)
			}
			if event.Status != test.want {
				t.Fatalf("normalizeSESFeedbackEvent() status = %s, want %s", event.Status, test.want)
			}
		})
	}
}
