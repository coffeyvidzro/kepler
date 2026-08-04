package webhook

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	platformevent "github.com/coffeyvidzro/dugble/server/internal/platform/event"
)

func SubscribableEventTypes() []string {
	return platformevent.SubscribableTypes()
}

func IsSubscribableEventType(eventType string) bool {
	return platformevent.IsSubscribable(platformevent.Type(eventType))
}

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

type Event struct {
	ID         uuid.UUID
	TeamID     uuid.UUID
	Type       string
	ObjectType string
	ObjectID   *uuid.UUID
	Payload    json.RawMessage
	OccurredAt time.Time
}
