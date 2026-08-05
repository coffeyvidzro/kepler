package systememail

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"

	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/awsses"
	"github.com/coffeyvidzro/dugble/server/internal/platform/outbox"
)

type eventStore interface {
	Enqueue(context.Context, outbox.Event) (uuid.UUID, error)
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

func (queue *Queue) Send(ctx context.Context, message platformemail.Message) (platformemail.Result, error) {
	if queue == nil || queue.store == nil {
		return platformemail.Result{}, ErrQueueNotConfigured
	}
	if strings.TrimSpace(message.Provider) == "" {
		message.Provider = queue.provider
	}
	if strings.TrimSpace(message.Region) == "" {
		message.Region = queue.region
	}
	if strings.TrimSpace(message.Stream) == "" {
		message.Stream = queue.defaults.Stream
	}
	if strings.TrimSpace(message.ConfigurationSet) == "" {
		message.ConfigurationSet = queue.defaults.ConfigurationSet
	}
	if strings.TrimSpace(message.SESTenantName) == "" {
		message.SESTenantName = queue.defaults.SESTenantName
	}

	eventID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(deliveryNamespace+uuid.NewString()))
	payload, err := json.Marshal(DeliverCommand{EventID: eventID, Message: message, SchemaVersion: 1})
	if err != nil {
		return platformemail.Result{}, err
	}
	_, err = queue.store.Enqueue(ctx, outbox.Event{
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
