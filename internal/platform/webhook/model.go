package webhook

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	platformevent "github.com/coffeyvidzro/dugble/server/internal/platform/event"
)

// SubscribableEventTypes delegates to the canonical platform event catalog.
// Deprecated: use event.SubscribableTypes directly in new code.
func SubscribableEventTypes() []string {
	return platformevent.SubscribableTypes()
}

// IsSubscribableEventType delegates to the canonical platform event catalog.
// Deprecated: use event.IsSubscribable directly in new code.
func IsSubscribableEventType(eventType string) bool {
	return platformevent.IsSubscribable(platformevent.Type(eventType))
}

// These string aliases remain temporarily for existing webhook producers. New
// code should use the typed constants from internal/platform/event.
const (
	EventSMSSubmitted   = string(platformevent.TypeSMSSubmitted)
	EventSMSSent        = string(platformevent.TypeSMSSent)
	EventSMSDelivered   = string(platformevent.TypeSMSDelivered)
	EventSMSUndelivered = string(platformevent.TypeSMSUndelivered)
	EventSMSFailed      = string(platformevent.TypeSMSFailed)

	EventEmailSubmitted           = string(platformevent.TypeEmailSubmitted)
	EventEmailDelivered           = string(platformevent.TypeEmailDelivered)
	EventEmailDelayed             = string(platformevent.TypeEmailDelayed)
	EventEmailBounced             = string(platformevent.TypeEmailBounced)
	EventEmailComplained          = string(platformevent.TypeEmailComplained)
	EventEmailRejected            = string(platformevent.TypeEmailRejected)
	EventEmailFailed              = string(platformevent.TypeEmailFailed)
	EventEmailOpened              = string(platformevent.TypeEmailOpened)
	EventEmailClicked             = string(platformevent.TypeEmailClicked)
	EventEmailSubscriptionChanged = string(platformevent.TypeEmailSubscriptionChanged)

	EventTest = string(platformevent.TypeWebhookTest)
)

// Event is the compatibility input used by existing webhook producers. The
// emitter converts it into event.Envelope before validation or persistence.
// Deprecated: new producers should emit event.Envelope through event.Emitter.
type Event struct {
	ID         uuid.UUID
	TeamID     uuid.UUID
	Type       string
	ObjectType string
	ObjectID   *uuid.UUID
	Payload    json.RawMessage
	OccurredAt time.Time
}
