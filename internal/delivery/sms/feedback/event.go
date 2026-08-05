package feedback

import (
	"encoding/json"
	"strings"
	"time"

	smsapi "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
)

type Event struct {
	ProviderID        string          `json:"provider_id"`
	ProviderMessageID string          `json:"provider_message_id"`
	Status            string          `json:"status"`
	OccurredAt        time.Time       `json:"occurred_at"`
	ErrorMessage      string          `json:"error_message,omitempty"`
	Payload           json.RawMessage `json:"payload,omitempty"`
}

func (event Event) Normalize() Event {
	event.ProviderID = strings.ToLower(strings.TrimSpace(event.ProviderID))
	event.ProviderMessageID = strings.TrimSpace(event.ProviderMessageID)
	event.Status = strings.ToLower(strings.TrimSpace(event.Status))
	event.ErrorMessage = strings.TrimSpace(event.ErrorMessage)
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	} else {
		event.OccurredAt = event.OccurredAt.UTC()
	}
	return event
}

func (event Event) Validate() error {
	event = event.Normalize()
	if event.ProviderID == "" {
		return ErrProviderRequired
	}
	if event.ProviderMessageID == "" {
		return ErrProviderMessageRequired
	}
	if !smsapi.IsKnownStatus(event.Status) {
		return ErrUnsupportedStatus
	}
	return nil
}
