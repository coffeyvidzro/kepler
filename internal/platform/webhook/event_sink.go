package webhook

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	platformevent "github.com/coffeyvidzro/dugble/server/internal/platform/event"
)

type EventSink struct {
	emitter *Emitter
}

func NewEventSink(emitter *Emitter) *EventSink {
	return &EventSink{emitter: emitter}
}

func (sink *EventSink) EmitTx(ctx context.Context, tx pgx.Tx, envelope platformevent.Envelope) (platformevent.Result, error) {
	if sink == nil || sink.emitter == nil {
		return platformevent.Result{}, errors.New("webhook event sink is not configured")
	}
	eventID, deliveryCount, err := sink.emitter.EmitTx(ctx, tx, Event{
		ID: envelope.ID, TeamID: envelope.TeamID, Type: string(envelope.Type),
		ObjectType: envelope.ObjectType, ObjectID: envelope.ObjectID,
		Payload: envelope.Data, OccurredAt: envelope.OccurredAt,
	})
	if err != nil {
		return platformevent.Result{}, err
	}
	return platformevent.Result{EventID: eventID, DeliveryCount: deliveryCount}, nil
}
