package feedback

import (
	"encoding/json"

	"github.com/google/uuid"
)

const (
	ProviderSES        = "ses"
	TransportSNS       = "sns"
	ProviderEventTopic = "dugble.event.provider.email.ses.v1"

	ConsumerName = "dugble-email-feedback-v1"
	DLQSubject   = "dugble.dlq.email.feedback.v1"
)

var eventNamespace = uuid.MustParse("37340be8-2f13-5e18-b044-6f42dc65a6ad")

type ProviderEventReference struct {
	EventID  uuid.UUID `json:"event_id"`
	Provider string    `json:"provider"`
}

func encodeProviderEventReference(eventID uuid.UUID) (json.RawMessage, error) {
	return json.Marshal(ProviderEventReference{
		EventID:  eventID,
		Provider: ProviderSES,
	})
}
