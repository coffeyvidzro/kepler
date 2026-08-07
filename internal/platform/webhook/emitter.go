package webhook

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	platformevent "github.com/coffeyvidzro/dugble/server/internal/platform/event"
)

type Store interface {
	CreateEventTx(context.Context, pgx.Tx, Event) (uuid.UUID, error)
	CreateDeliveriesForEventTx(context.Context, pgx.Tx, uuid.UUID, time.Time) (int64, error)
	CreateDeliveryTx(context.Context, pgx.Tx, uuid.UUID, uuid.UUID, time.Time) (uuid.UUID, error)
}

type Emitter struct {
	store Store
	now   func() time.Time
}

func NewEmitter(store Store) *Emitter {
	emitter := &Emitter{store: store, now: time.Now}
	platformevent.SetDefaultEmitter(platformevent.NewEmitter(NewEventSink(emitter)))
	return emitter
}

// EmitTx is the compatibility entry point for callers that still construct a
// webhook Event. The event is converted to the canonical platform event
// envelope before it is validated and persisted.
func (e *Emitter) EmitTx(ctx context.Context, tx pgx.Tx, event Event) (uuid.UUID, int64, error) {
	result, err := e.emitEnvelopeTx(ctx, tx, event.envelope())
	if err != nil {
		return uuid.Nil, 0, err
	}
	return result.EventID, result.DeliveryCount, nil
}

func (e *Emitter) EmitToEndpointTx(
	ctx context.Context,
	tx pgx.Tx,
	event Event,
	endpointID uuid.UUID,
) (uuid.UUID, uuid.UUID, error) {
	if tx == nil {
		return uuid.Nil, uuid.Nil, errors.New("webhook transaction is required")
	}
	if endpointID == uuid.Nil {
		return uuid.Nil, uuid.Nil, errors.New("webhook endpoint id is required")
	}

	envelope, err := e.normalizeEnvelope(event.envelope())
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	storedEvent := eventFromEnvelope(envelope)
	eventID, err := e.store.CreateEventTx(ctx, tx, storedEvent)
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("create webhook event: %w", err)
	}
	if eventID == uuid.Nil {
		return uuid.Nil, uuid.Nil, errors.New("webhook store returned an empty event id")
	}
	if eventID != envelope.ID {
		return uuid.Nil, uuid.Nil, errors.New("webhook store returned a different event id")
	}
	deliveryID, err := e.store.CreateDeliveryTx(ctx, tx, eventID, endpointID, e.now().UTC())
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("create webhook delivery: %w", err)
	}
	if deliveryID == uuid.Nil {
		return uuid.Nil, uuid.Nil, errors.New("webhook store returned an empty delivery id")
	}
	return eventID, deliveryID, nil
}

func (e *Emitter) emitEnvelopeTx(
	ctx context.Context,
	tx pgx.Tx,
	envelope platformevent.Envelope,
) (platformevent.Result, error) {
	if tx == nil {
		return platformevent.Result{}, errors.New("webhook transaction is required")
	}

	normalized, err := e.normalizeEnvelope(envelope)
	if err != nil {
		return platformevent.Result{}, err
	}
	eventID, err := e.store.CreateEventTx(ctx, tx, eventFromEnvelope(normalized))
	if err != nil {
		return platformevent.Result{}, fmt.Errorf("create webhook event: %w", err)
	}
	if eventID == uuid.Nil {
		return platformevent.Result{}, errors.New("webhook store returned an empty event id")
	}
	if eventID != normalized.ID {
		return platformevent.Result{}, errors.New("webhook store returned a different event id")
	}
	count, err := e.store.CreateDeliveriesForEventTx(ctx, tx, eventID, e.now().UTC())
	if err != nil {
		return platformevent.Result{}, fmt.Errorf("create webhook deliveries: %w", err)
	}
	if count < 0 {
		return platformevent.Result{}, errors.New("webhook store returned a negative delivery count")
	}
	return platformevent.Result{EventID: eventID, DeliveryCount: count}, nil
}

func (e *Emitter) normalizeEnvelope(envelope platformevent.Envelope) (platformevent.Envelope, error) {
	if e == nil || e.store == nil {
		return platformevent.Envelope{}, errors.New("webhook emitter is not configured")
	}
	normalized, err := envelope.Normalize(e.now)
	if err != nil {
		return platformevent.Envelope{}, fmt.Errorf("normalize webhook event: %w", err)
	}
	return normalized, nil
}

func (event Event) envelope() platformevent.Envelope {
	return platformevent.Envelope{
		ID:         event.ID,
		Type:       platformevent.Type(event.Type),
		Version:    platformevent.CurrentVersion,
		TeamID:     event.TeamID,
		ObjectType: event.ObjectType,
		ObjectID:   event.ObjectID,
		Data:       event.Payload,
		OccurredAt: event.OccurredAt,
	}
}

func eventFromEnvelope(envelope platformevent.Envelope) Event {
	return Event{
		ID:         envelope.ID,
		TeamID:     envelope.TeamID,
		Type:       string(envelope.Type),
		ObjectType: envelope.ObjectType,
		ObjectID:   envelope.ObjectID,
		Payload:    envelope.Data,
		OccurredAt: envelope.OccurredAt,
	}
}
