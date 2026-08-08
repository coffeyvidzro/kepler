package feedback

import (
	"encoding/json"
	attempt "github.com/coffeyvidzro/dugble/server/internal/delivery/attempt"
	"testing"
	"time"

	awsses "github.com/coffeyvidzro/dugble/server/internal/adapters/amazon/ses"
)

func TestNormalizeSESFeedbackEventUsesAggregateState(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	tests := []struct {
		eventType     string
		messageStatus string
		want          attempt.AttemptStatus
	}{
		{eventType: "send", messageStatus: "submitted", want: attempt.StatusSubmitted},
		{eventType: "delivery_delay", messageStatus: "delayed", want: attempt.StatusSent},
		{eventType: "delivery", messageStatus: "partially_delivered", want: attempt.StatusSent},
		{eventType: "delivery", messageStatus: "delivered", want: attempt.StatusDelivered},
		{eventType: "bounce", messageStatus: "partially_failed", want: attempt.StatusSent},
		{eventType: "bounce", messageStatus: "bounced", want: attempt.StatusPermanentFailure},
		{eventType: "complaint", messageStatus: "complained", want: attempt.StatusDelivered},
		{eventType: "reject", messageStatus: "rejected", want: attempt.StatusRejected},
		{eventType: "rendering_failure", messageStatus: "failed", want: attempt.StatusPermanentFailure},
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
