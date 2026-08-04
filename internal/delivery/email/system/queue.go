package systememail

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"

	"github.com/coffeyvidzro/dugble/server/internal/messaging/outbox"
	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/email"
)

const (
	DeliverSubject    = "dugble.job.email.system.v1"
	deliveryNamespace = "https://dugble.com/events/email/system/"
)

type eventStore interface {
	Enqueue(context.Context, outbox.Event) (uuid.UUID, error)
}

type DeliverCommand struct {
	EventID       uuid.UUID             `json:"event_id"`
	Message       platformemail.Message `json:"message"`
	SchemaVersion int                   `json:"schema_version"`
}

type Queue struct {
	store    eventStore
	defaults platformemail.DeliveryRoute
	region   string
	provider string
}

func NewQueue(store eventStore, defaults ...platformemail.Message) *Queue {
	queue := &Queue{store: store}
	if len(defaults) > 0 {
		queue.provider = strings.TrimSpace(defaults[0].Provider)
		queue.region = strings.TrimSpace(defaults[0].Region)
		queue.defaults = platformemail.DeliveryRoute{
			Stream:           strings.TrimSpace(defaults[0].Stream),
			ConfigurationSet: strings.TrimSpace(defaults[0].ConfigurationSet),
			SESTenantName:    strings.TrimSpace(defaults[0].SESTenantName),
		}
	}
	return queue
}

func (q *Queue) Send(ctx context.Context, message platformemail.Message) (platformemail.Result, error) {
	if q == nil || q.store == nil {
		return platformemail.Result{}, errors.New("system email outbox is not configured")
	}
	if strings.TrimSpace(message.Provider) == "" {
		message.Provider = q.provider
	}
	if strings.TrimSpace(message.Region) == "" {
		message.Region = q.region
	}
	if strings.TrimSpace(message.Stream) == "" {
		message.Stream = q.defaults.Stream
	}
	if strings.TrimSpace(message.ConfigurationSet) == "" {
		message.ConfigurationSet = q.defaults.ConfigurationSet
	}
	if strings.TrimSpace(message.SESTenantName) == "" {
		message.SESTenantName = q.defaults.SESTenantName
	}

	eventID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(deliveryNamespace+uuid.NewString()))
	payload, err := json.Marshal(DeliverCommand{EventID: eventID, Message: message, SchemaVersion: 1})
	if err != nil {
		return platformemail.Result{}, err
	}
	_, err = q.store.Enqueue(ctx, outbox.Event{
		ID:            eventID,
		Subject:       DeliverSubject,
		AggregateType: "system_email",
		AggregateID:   eventID,
		Payload:       payload,
		Headers: map[string]string{
			"Dugble-Event-Type": "email.system.send.requested.v1",
		},
	})
	if err != nil {
		return platformemail.Result{}, err
	}
	return platformemail.Result{Provider: "outbox", MessageID: eventID.String()}, nil
}
